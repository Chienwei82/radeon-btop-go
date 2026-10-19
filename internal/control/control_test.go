package control

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/gputop/gputop-go/internal/model"
)

// testProcess builds a client; cmdline nil makes it look like a kernel thread.
func testProcess(pid int, cmdline []string) *model.GpuProcess {
	return &model.GpuProcess{
		PID: pid, Name: "proc", User: "david", BDF: "0000:0c:00.0", ClientID: 1,
		Cmdline: cmdline,
	}
}

// withRecorder swaps the delivery function for one that records calls, and
// restores it when the test ends.
func withRecorder(t *testing.T) *[]struct {
	pid    int
	signal syscall.Signal
} {
	t.Helper()
	var calls []struct {
		pid    int
		signal syscall.Signal
	}
	original := deliver
	deliver = func(pid int, signal syscall.Signal) error {
		calls = append(calls, struct {
			pid    int
			signal syscall.Signal
		}{pid, signal})
		return nil
	}
	t.Cleanup(func() { deliver = original })
	return &calls
}

func TestSignalsAreOfferedInOrder(t *testing.T) {
	if len(Signals) != 2 || Signals[0].Name != SignalTerm || Signals[1].Name != SignalKill {
		t.Fatalf("Signals = %+v, want SIGTERM then SIGKILL", Signals)
	}
	if Signals[0].Number != syscall.SIGTERM || Signals[1].Number != syscall.SIGKILL {
		t.Fatalf("signal numbers = %v, want the real ones", Signals)
	}
}

func TestSignalNumberDefaultsToTheReversibleSignal(t *testing.T) {
	if got := SignalNumber(SignalKill); got != syscall.SIGKILL {
		t.Errorf("SignalNumber(SIGKILL) = %v", got)
	}
	for _, name := range []string{"", "SIGSTOP", "nonsense"} {
		if got := SignalNumber(name); got != syscall.SIGTERM {
			t.Errorf("SignalNumber(%q) = %v, want SIGTERM", name, got)
		}
	}
}

func TestSignalHelpExplainsBoth(t *testing.T) {
	for _, name := range []string{SignalTerm, SignalKill} {
		if SignalHelp[name] == "" {
			t.Errorf("SignalHelp[%q] is empty", name)
		}
	}
}

func TestHelpTextNamesTheConfigKeyWhenDisabled(t *testing.T) {
	if got := HelpText(false); got != "disabled: set process.allow_kill = true" {
		t.Errorf("HelpText(false) = %q", got)
	}
	if got := HelpText(true); got == "" || got == HelpText(false) {
		t.Errorf("HelpText(true) = %q, want the enabled wording", got)
	}
}

func TestRefusalTable(t *testing.T) {
	cases := []struct {
		name    string
		process *model.GpuProcess
		ownPID  int
		want    string
	}{
		{"nothing selected", nil, 0, "no process is selected"},
		{"pid zero", testProcess(0, []string{"x"}), 0, "pid 0 is not a signalable target"},
		{"negative pid", testProcess(-5, []string{"x"}), 0, "pid -5 is not a signalable target"},
		{"init", testProcess(1, []string{"/sbin/init"}), 0, "refusing to signal init (pid 1)"},
		{"kernel thread", testProcess(7, nil), 0, "7 is a kernel thread and has no signalable task"},
		{"self", testProcess(4242, []string{"gputop"}), 4242, "refusing to signal gputop itself"},
		{"ordinary client", testProcess(4242, []string{"renderfarm"}), 999, ""},
		{"self unknown", testProcess(4242, []string{"gputop"}), 0, ""},
	}
	for _, c := range cases {
		if got := Refusal(c.process, c.ownPID); got != c.want {
			t.Errorf("%s: Refusal = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTerminateRefusesWithoutSendingAnything(t *testing.T) {
	calls := withRecorder(t)
	for _, c := range []struct {
		process *model.GpuProcess
		ownPID  int
	}{
		{nil, 0},
		{testProcess(1, []string{"/sbin/init"}), 0},
		{testProcess(7, nil), 0},
		{testProcess(4242, []string{"gputop"}), 4242},
	} {
		if msg := Terminate(c.process, SignalTerm, c.ownPID); msg == "" {
			t.Errorf("Terminate(%+v) reported success", c.process)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("a refused target was signalled %d times", len(*calls))
	}
}

func TestTerminateSendsTheNamedSignal(t *testing.T) {
	calls := withRecorder(t)
	if msg := Terminate(testProcess(4242, []string{"renderfarm"}), SignalKill, 999); msg != "" {
		t.Fatalf("Terminate = %q, want success", msg)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(*calls))
	}
	if (*calls)[0].pid != 4242 || (*calls)[0].signal != syscall.SIGKILL {
		t.Fatalf("delivered %+v, want pid 4242 SIGKILL", (*calls)[0])
	}
}

func TestTerminateReportsAFailedDelivery(t *testing.T) {
	original := deliver
	t.Cleanup(func() { deliver = original })
	cases := []struct {
		err  error
		want string
	}{
		{syscall.ESRCH, "4242 (proc) exited before the signal was sent"},
		{syscall.EPERM, "not permitted to signal 4242 (proc); try again as root"},
		{errors.New("boom"), "4242 (proc): boom"},
	}
	for _, c := range cases {
		deliver = func(int, syscall.Signal) error { return c.err }
		if got := Terminate(testProcess(4242, []string{"proc"}), SignalTerm, 0); got != c.want {
			t.Errorf("err %v: got %q, want %q", c.err, got, c.want)
		}
	}
}

func TestTerminateSendsARealSignalToAnOwnChild(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("no sleep binary in this image")
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Skipf("cannot start a child: %v", err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })

	process := testProcess(child.Process.Pid, []string{"sleep", "30"})
	if msg := Terminate(process, SignalTerm, 0); msg != "" {
		t.Fatalf("Terminate = %q, want success", msg)
	}

	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if err != nil && !errors.As(err, &exitErr) {
			t.Fatalf("wait: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the child did not exit after SIGTERM")
	}
}
