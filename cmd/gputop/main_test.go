package main

import (
	"io"
	"testing"

	"github.com/gputop/gputop-go/internal/cli"
)

func TestVersionFlagPrintsVersion(t *testing.T) {
	if version == "" {
		t.Fatal("version must not be empty")
	}
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("run(--version) = %v, want nil", err)
	}
}

func TestRunWithoutTUIErrorsCleanly(t *testing.T) {
	// Phase 0: the TUI does not exist yet (Phase 2). Starting it must fail
	// with a helpful error, never by panicking or hanging.
	if err := run([]string{}); err == nil {
		t.Fatal("run() = nil, want an error until the TUI lands")
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
