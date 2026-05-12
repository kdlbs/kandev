//go:build !windows

package process

import (
	"os"
	"os/exec"
	"strings"
)

// defaultShellCommand returns the command and args for starting an interactive shell.
// On Unix, uses $SHELL (or /bin/sh) with no extra flags — the shell is interactive
// via the attached PTY. We deliberately do NOT pass -l: Debian's /etc/profile
// (and /etc/profile.d/*) can overwrite PATH, dropping container-set entries like
// /data/.npm-global/bin where agent CLIs (claude, codex, ...) are installed.
func defaultShellCommand(preferredShell string) []string {
	shell := resolveShellPath(preferredShell)
	if shell == "" {
		shell = resolveShellPath(os.Getenv("SHELL"))
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{shell}
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
