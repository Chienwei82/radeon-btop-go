// Package control sends a signal to a process, and holds the rules about when
// not to.
//
// This is the only part of gputop that can end something other than itself, so
// the restrictions live in one place, with no terminal code in it, and are
// pure: deciding whether a signal is allowed is a function of the target and
// can be tested exhaustively without ever sending anything.
//
// The default is to refuse. A monitor that can kill a process is one keystroke
// away from terminating the wrong one, and the cost of that mistake is
// measured in someone's unsaved work, not in a rendering glitch.
// process.allow_kill turns the capability on at all, and even then every
// target is checked.
package control

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/gputop/gputop-go/internal/model"
)

// The signal names the confirmation offers.
const (
	SignalTerm = "SIGTERM"
	SignalKill = "SIGKILL"
)

// Signal is one offered signal: its name and the number it delivers.
type Signal struct {
	Name   string
	Number syscall.Signal
}

// Signals are the offered signals in the order the confirmation lists them.
// SIGTERM first: a process asked politely gets to clean up, and the dialog
// says so.
var Signals = []Signal{
	{Name: SignalTerm, Number: syscall.SIGTERM},
	{Name: SignalKill, Number: syscall.SIGKILL},
}

// SignalHelp says what each signal means. A user who does not know what
// SIGKILL is cannot meaningfully consent to it.
var SignalHelp = map[string]string{
	SignalTerm: "ask the process to exit",
	SignalKill: "terminate immediately; no cleanup, no chance to save",
}

// SignalNumber looks a signal number up by name, defaulting to SIGTERM.
//
// An unknown name falls back to the reversible signal rather than the other
// way round: if anything about the request is not understood, the answer
// should be the option that can still be undone.
func SignalNumber(name string) syscall.Signal {
	for _, signal := range Signals {
		if signal.Name == name {
			return signal.Number
		}
	}
	return syscall.SIGTERM
}

// HelpText is the help-overlay line for the k binding, which depends on
// whether the configuration allows signalling at all.
func HelpText(allowKill bool) string {
	if allowKill {
		return "send SIGTERM or SIGKILL to the selected process"
	}
	return "disabled: set process.allow_kill = true"
}

// Refusal says why this process must not be signalled, or "" when it may be.
//
// The checks run cheapest first and, more importantly, from "can this even be
// signalled" to "should it". A kernel thread has no userspace task to signal;
// PID 1 is the machine's init, and killing it is not a thing a monitor should
// be able to do even by accident; and gputop signalling itself would exit
// without saving the state the user just configured.
//
// ownPID of 0 means "not known", and skips only the self-check.
func Refusal(process *model.GpuProcess, ownPID int) string {
	if process == nil {
		return "no process is selected"
	}
	if process.PID <= 0 {
		return fmt.Sprintf("pid %d is not a signalable target", process.PID)
	}
	if process.PID == 1 {
		return "refusing to signal init (pid 1)"
	}
	if process.IsKernelThread() {
		return fmt.Sprintf("%d is a kernel thread and has no signalable task", process.PID)
	}
	if ownPID != 0 && process.PID == ownPID {
		return "refusing to signal gputop itself"
	}
	return ""
}

// deliver sends the signal. It is a variable so the test suite can record
// calls instead of sending real signals to real pids, the way the reference
// patches os.kill.
var deliver = func(pid int, signal syscall.Signal) error {
	return syscall.Kill(pid, signal)
}

// Terminate sends signalName to process.
//
// It returns "" on success, otherwise a message explaining the refusal or the
// failure. The one syscall cannot block -- the kernel delivers a signal and
// returns -- so it runs on the UI goroutine rather than in a worker; handing
// it to a goroutine would add a round trip and a failure mode to avoid
// waiting for nothing.
func Terminate(process *model.GpuProcess, signalName string, ownPID int) string {
	if reason := Refusal(process, ownPID); reason != "" {
		return reason
	}
	err := deliver(process.PID, SignalNumber(signalName))
	switch {
	case err == nil:
		return ""
	case errors.Is(err, syscall.ESRCH):
		return fmt.Sprintf("%d (%s) exited before the signal was sent", process.PID, process.Name)
	case errors.Is(err, syscall.EPERM):
		return fmt.Sprintf("not permitted to signal %d (%s); try again as root", process.PID, process.Name)
	default:
		return fmt.Sprintf("%d (%s): %v", process.PID, process.Name, err)
	}
}
