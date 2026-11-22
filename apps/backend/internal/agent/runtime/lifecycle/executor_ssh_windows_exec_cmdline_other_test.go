//go:build !windows

package lifecycle

import "os/exec"

// setRawCommandLine is unused off Windows; the Windows execution tests skip.
func setRawCommandLine(*exec.Cmd, string) {}
