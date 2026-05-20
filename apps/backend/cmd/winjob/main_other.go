//go:build !windows

// winjob is a Win32 Job Object wrapper for the CLI launcher's backend spawn.
// On Unix it's a transparent passthrough: process groups already give us
// reliable cascading termination when the parent dies (Pdeathsig on Linux,
// process-group SIGINT on signal delivery to the foreground job).
package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: winjob <program> [args...]")
		os.Exit(2)
	}
	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "winjob: %v\n", err)
		os.Exit(1)
	}
}
