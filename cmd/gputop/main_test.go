package main

import "testing"

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
