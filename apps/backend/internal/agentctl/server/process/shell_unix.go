//go:build !windows

package process

import (
	"os"
	"os/exec"
	"strings"
)

// defaultShellCommand returns the command and args for starting an interactive login shell.
// On Unix, uses $SHELL (or /bin/sh) with the -l (login) flag.
func defaultShellCommand(preferredShell string) []string {
	shell := resolveShellPath(preferredShell)
	if shell == "" {
		shell = resolveShellPath(os.Getenv("SHELL"))
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{shell, "-l"}
}

func resolveShellPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	path, err := exec.LookPath(value)
	if err != nil {
		return ""
	}
	return path
}

// shellExecArgs returns the program and arguments needed to execute a command
// string through the system shell.
// On Unix: sh -lc "command"
func shellExecArgs(command string) (prog string, args []string) {
	return "sh", []string{"-lc", command}
}
