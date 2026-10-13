// Command gputop is a btop-style terminal monitor for AMD Radeon GPUs
// driven by the amdgpu kernel driver.
//
// With no arguments it starts the terminal interface; see --help for the
// headless diagnostic modes (--dump, --devices, --check).
package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gputop: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("gputop", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("gputop %s\n", version)
		return nil
	}
	return fmt.Errorf("TUI not implemented yet (Phase 2); see --version")
}
