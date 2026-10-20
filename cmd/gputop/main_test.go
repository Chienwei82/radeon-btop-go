package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gputop/gputop-go/internal/cli"
)

func TestVersionFlagPrintsVersion(t *testing.T) {
	if version == "" {
		t.Fatal("version must not be empty")
	}
	var out bytes.Buffer
	status, err := runWith([]string{"--version"}, &out)
	if err != nil {
		t.Fatalf("runWith(--version) = %v, want nil", err)
	}
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	want := "gputop " + version + "\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestShortConfigAndIntervalAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	if err := os.WriteFile(path, []byte("interval_ms: 400\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := runWith([]string{
		"--check", "--drm-root", "/nonexistent/drm", "--proc-root", "/proc",
		"-c", path, "-i", "0.2",
	}, io.Discard)
	if err != nil {
		t.Fatalf("runWith(-c/-i) = %v, want nil", err)
	}
	if status != cli.ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, cli.ExitNoDevice)
	}
}

func TestThemeFlagValidatesNames(t *testing.T) {
	if _, err := runWith([]string{"--theme", "solarized", "--version"}, io.Discard); err == nil {
		t.Fatal("an unknown theme was accepted")
	}
	for _, name := range []string{"default", "dracula", "gruvbox", "monochrome", "radeon"} {
		if _, err := runWith([]string{"--theme", name, "--version"}, io.Discard); err != nil {
			t.Fatalf("--theme %s = %v, want nil", name, err)
		}
	}
}

func TestKindFlagValidatesNames(t *testing.T) {
	if _, err := runWith([]string{"--kind", "apu", "--version"}, io.Discard); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
	for _, kind := range []string{"auto", "igpu", "dgpu"} {
		if _, err := runWith([]string{"--kind", kind, "--version"}, io.Discard); err != nil {
			t.Fatalf("--kind %s = %v, want nil", kind, err)
		}
	}
}

func TestCheckWithoutHardwareIsNotAnError(t *testing.T) {
	// --check must answer "what can this machine show" even when the answer is
	// "no GPUs", and exit with the no-device status rather than an error.
	status, err := runWith([]string{
		"--check", "--drm-root", "/nonexistent/drm", "--proc-root", "/proc",
	}, io.Discard)
	if err != nil {
		t.Fatalf("runWith(--check) = %v, want nil", err)
	}
	if status != cli.ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, cli.ExitNoDevice)
	}
}

func TestCheckJSONFlagIsAccepted(t *testing.T) {
	status, err := runWith([]string{
		"--check", "--json", "--drm-root", "/nonexistent/drm", "--proc-root", "/proc",
	}, io.Discard)
	if err != nil {
		t.Fatalf("runWith(--check --json) = %v", err)
	}
	if status != cli.ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, cli.ExitNoDevice)
	}
}

func TestLogIsRejectedWithHeadlessModes(t *testing.T) {
	for _, mode := range []string{"--check", "--dump", "--devices"} {
		if _, err := runWith([]string{mode, "--log", "s.csv"}, io.Discard); err == nil {
			t.Errorf("%s --log was accepted; recording is an interface feature", mode)
		}
	}
}

func TestDumpWithoutHardwareIsNotAnError(t *testing.T) {
	// --dump is the headless parity path. On a machine with no AMD card it
	// must report the reason and exit with the "no device" status, never an
	// error: "no hardware" is not "gputop is broken".
	status, err := runWith([]string{"--dump", "--drm-root", "/nonexistent/drm"}, io.Discard)
	if err != nil {
		t.Fatalf("runWith(--dump) = %v, want nil", err)
	}
	if status != cli.ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, cli.ExitNoDevice)
	}
}

func TestDevicesWithoutHardwareIsNotAnError(t *testing.T) {
	status, err := runWith([]string{"--devices", "--drm-root", "/nonexistent/drm"}, io.Discard)
	if err != nil {
		t.Fatalf("runWith(--devices) = %v, want nil", err)
	}
	if status != cli.ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, cli.ExitNoDevice)
	}
}

func TestUnknownFlagIsAnError(t *testing.T) {
	if _, err := runWith([]string{"--nope"}, io.Discard); err == nil {
		t.Fatal("runWith(--nope) = nil, want an error")
	}
}
