// Package shellexec runs scripts through a host-appropriate shell.
//
// On Unix it uses the requested shell directly (/bin/sh or /bin/bash). On
// Windows it prefers bash.exe from Git for Windows when available — that's
// what the user already has installed if they cloned the repo via git — and
// falls back to cmd.exe /c when no bash is found. The Git-Bash preference
// keeps Bourne-style scripts portable (pipelines, redirects, expansions);
// the cmd.exe fallback handles the simple-command cases that don't need a
// real POSIX shell (the agent installer's `npm install -g ...` for example).
package shellexec

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

// Shell tells CommandContext what dialect the script is written in. The hint
// is only consulted on Unix; on Windows the same resolution (Git Bash, then
// cmd.exe) is used regardless of hint, because the practical alternatives
// don't differ in a way that affects script compatibility.
type Shell string

const (
	// PosixSh — Bourne-compatible script. Picks /bin/sh on Unix.
	PosixSh Shell = "sh"
	// Bash — explicitly bash. Picks /bin/bash on Unix.
	Bash Shell = "bash"
)

// CommandContext returns an exec.Cmd that runs `script` under the host shell.
// The returned Cmd is not started — the caller wires stdout/stderr/env then
// calls Start/Run/Output as usual.
func CommandContext(ctx context.Context, shell Shell, script string) *exec.Cmd {
	prog, args := resolve(shell, script)
	return exec.CommandContext(ctx, prog, args...)
}

// resolve picks the shell program and argv-prefix appropriate for the host.
// Exposed for tests.
func resolve(shell Shell, script string) (string, []string) {
	if runtime.GOOS == "windows" {
		if bash := findWindowsBash(); bash != "" {
			return bash, []string{"-c", script}
		}
		return "cmd", []string{"/c", script}
	}
	if shell == Bash {
		return "bash", []string{"-c", script}
	}
	return "sh", []string{"-c", script}
}

// findWindowsBash looks for bash.exe, preferring Git-for-Windows install
// locations over whatever PATH happens to resolve. Returns "" if none found.
// Exposed for tests via package-level overrides; the production code reads
// from disk.
//
// Order matters: on a Windows 10/11 box with WSL installed,
// C:\Windows\System32\bash.exe (the WSL launcher) is in PATH ahead of any
// Git addition. Calling that would run our scripts inside a Linux
// subsystem where Windows-native paths, npm.cmd, node.exe, etc. are
// invisible — agent install scripts would silently fail. Probe the known
// Git paths first, then fall back to PATH for non-standard Git installs.
var findWindowsBash = func() string {
	for _, candidate := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\Git\usr\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	return ""
}
