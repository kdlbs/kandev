package ptyexec

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Command-line quoting for Windows. ConPTY takes a single command-line string
// rather than an argv, so callers have to reassemble cmd.Args following the
// MSDN CommandLineToArgvW rules — the same algorithm as syscall.EscapeArg.
//
// These are pure string transformations, so they are intentionally not behind
// //go:build windows: that keeps their tests running on every platform instead
// of only on a Windows runner.

// cmdExe is the batch interpreter used to wrap .cmd / .bat scripts, which
// CreateProcessW cannot execute directly.
const cmdExe = "cmd.exe"

// scanArgChars inspects each byte of s and returns whether any character
// requires backslash-escaping (double-quote or backslash) and whether
// the argument contains whitespace (space or tab).
func scanArgChars(s string) (needsBackslash, hasSpace bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			needsBackslash = true
		case ' ', '\t':
			hasSpace = true
		}
	}
	return needsBackslash, hasSpace
}

// appendEscapedBytes appends the bytes of s to b applying MSDN
// CommandLineToArgvW backslash-doubling rules for double-quote characters.
// Returns the updated byte slice and the trailing slash count.
func appendEscapedBytes(b []byte, s string) ([]byte, int) {
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		default:
			slashes = 0
		case '\\':
			slashes++
		case '"':
			for ; slashes > 0; slashes-- {
				b = append(b, '\\')
			}
			b = append(b, '\\')
		}
		b = append(b, c)
	}
	return b, slashes
}

// escapeArg rewrites a command-line argument following the MSDN
// CommandLineToArgvW parsing rules (same algorithm as Go's
// syscall.EscapeArg on Windows):
//
//   - every backslash (\) is doubled, but only if immediately
//     followed by a double quote (");
//   - every double quote (") is escaped with a backslash (\);
//   - finally, the argument is wrapped in double quotes only if
//     it contains spaces or tabs.
//   - an empty string becomes "" (two double-quote characters).
func escapeArg(s string) string {
	if len(s) == 0 {
		return `""`
	}

	needsBackslash, hasSpace := scanArgChars(s)

	if !needsBackslash && !hasSpace {
		return s
	}
	if !needsBackslash {
		return `"` + s + `"`
	}

	var b []byte
	if hasSpace {
		b = append(b, '"')
	}
	b, slashes := appendEscapedBytes(b, s)
	if hasSpace {
		for ; slashes > 0; slashes-- {
			b = append(b, '\\')
		}
		b = append(b, '"')
	}
	return string(b)
}

// buildCmdLine joins arguments into a single command-line string
// with proper quoting for Windows CreateProcess / ConPTY.
func buildCmdLine(args []string) string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = escapeArg(arg)
	}
	return strings.Join(escaped, " ")
}

// resolveConPtyCmdLine produces the command line conpty.Start should run.
//
// Win32 CreateProcessW (which ConPTY uses internally) has two limitations that
// matter here:
//
//  1. It does NOT apply PATHEXT — a bare "opencode" won't resolve to
//     "opencode.cmd" the way Go's exec.LookPath does. exec.Command on Windows
//     already PATHEXT-resolved cmd.Path for us, but cmd.Args[0] is still the
//     unresolved name. We have to substitute cmd.Path so CreateProcess sees a
//     concrete file.
//
//  2. It cannot execute .cmd / .bat scripts directly — those are interpreted
//     by cmd.exe, not by the Win32 loader. We wrap such commands with
//     "cmd.exe /c <script> <args...>" so the batch interpreter handles them.
//
// Without this, npm-installed CLIs (which install as .cmd shims on Windows)
// fail to launch under ConPTY with "Failed to create console process: The
// system cannot find the file specified" — even though `where.exe` and
// `exec.LookPath` both find them. This is the same family of issue handled in
// apps/cli/src/web.ts for Node's spawn().
//
// KNOWN LIMITATION (pre-existing, tracked separately): when the cmd.exe /c
// wrapper is used, arguments get two parsing passes — cmd.exe's, then
// CommandLineToArgvW's — and escapeArg only satisfies the second. This is the
// BatBadBut class (CVE-2024-24576); Go's os/exec has its own mitigation, which
// does not apply here because ConPTY calls CreateProcessW directly.
//
// The trigger is specifically an embedded double quote, not metacharacters as
// such: cmd.exe honors quoting, so `&` / `|` / `(` / `)` inside a quoted
// argument are already inert (a task description like `fix the bug & add tests`
// is safe). But CommandLineToArgvW's `\"` escape means nothing to cmd.exe, so a
// quote inside an argument closes the quoted run early and anything after it is
// read as shell syntax. Fixing this needs cmd.exe-aware escaping (caret escapes
// plus `""` doubling) — do NOT "fix" it by rejecting metacharacters, which
// would break ordinary prose prompts while leaving the actual hole open.
func resolveConPtyCmdLine(cmd *exec.Cmd) string {
	args := cmd.Args
	switch {
	case len(args) == 0 && cmd.Path == "":
		return ""
	case len(args) == 0:
		args = []string{cmd.Path}
	case cmd.Path != "":
		args = append([]string{cmd.Path}, args[1:]...)
	}
	if isBatchScript(args[0]) {
		args = append([]string{cmdExe, "/c"}, args...)
	}
	return buildCmdLine(args)
}

// isBatchScript reports whether path ends in .cmd or .bat — the two
// CreateProcessW-incompatible Windows script extensions npm shims may use.
func isBatchScript(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd", ".bat":
		return true
	}
	return false
}
