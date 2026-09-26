package github

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ghResponse is one dispatch arm of the fake `gh` binary. Prefix is a literal
// string matched against the start of the joined argv ("$*"); the first arm
// that matches wins, so order the arms most-specific first.
type ghResponse struct {
	Prefix string
	Stdout string
	Stderr string
	Exit   int
}

// ghInvocations returns the exact argv of every fake-gh call, in order.
type ghInvocations func(t *testing.T) [][]string

// newFakeGH installs a fake `gh` on PATH that logs each invocation's argv
// verbatim (one argument per line, so quoting and spacing survive) and replies
// from the given dispatch arms. An invocation matching no arm exits non-zero
// with a loud stderr, so a test that quietly changes the command it issues
// fails rather than silently reading a default payload.
//
// t.Setenv is used for PATH, so tests calling this must not be parallel.
func newFakeGH(t *testing.T, responses ...ghResponse) ghInvocations {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh-argv.log")

	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	script.WriteString(`{ for a in "$@"; do printf '%s\n' "$a"; done; printf '%s\n' '--END--'; } >> "$GH_ARGS_LOG"` + "\n")
	// gh reads its request body from stdin for `--input -`. Commands launched
	// without a stdin pipe see an immediate EOF here, so this is safe for all
	// invocations.
	script.WriteString(`cat >> "$GH_STDIN_LOG"` + "\n")
	script.WriteString(`case "$*" in` + "\n")
	for _, r := range responses {
		// The literal half is single-quoted so a pattern containing spaces or
		// slashes parses (dash rejects an unquoted multi-word case pattern);
		// the trailing * stays outside the quotes so it still globs.
		script.WriteString("  " + shellQuote(r.Prefix) + "*)\n")
		if r.Stdout != "" {
			script.WriteString("    printf '%s' " + shellQuote(r.Stdout) + "\n")
		}
		if r.Stderr != "" {
			script.WriteString("    printf '%s' " + shellQuote(r.Stderr) + " >&2\n")
		}
		script.WriteString("    exit " + strconv.Itoa(r.Exit) + " ;;\n")
	}
	script.WriteString("  *)\n")
	script.WriteString(`    printf 'fake gh: no dispatch arm for: %s\n' "$*" >&2` + "\n")
	script.WriteString("    exit 97 ;;\n")
	script.WriteString("esac\n")

	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script.String()), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_ARGS_LOG", logPath)
	t.Setenv("GH_STDIN_LOG", filepath.Join(filepath.Dir(logPath), "gh-stdin.log"))

	return func(t *testing.T) [][]string {
		t.Helper()
		return readGHInvocations(t, logPath)
	}
}

// ghStdin returns everything the fake gh read on stdin across all invocations.
func ghStdin(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(os.Getenv("GH_STDIN_LOG"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read gh stdin log: %v", err)
	}
	return string(raw)
}

// readGHInvocations parses the argv log written by the fake gh script.
func readGHInvocations(t *testing.T, logPath string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read gh argv log: %v", err)
	}
	var calls [][]string
	current := []string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if line == "--END--" {
			calls = append(calls, current)
			current = []string{}
			continue
		}
		current = append(current, line)
	}
	return calls
}

// shellQuote wraps s in single quotes for POSIX sh, escaping any embedded
// single quote via the close-quote / literal-quote / reopen-quote idiom.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// assertGHArgv fails unless the recorded invocation matches want exactly.
func assertGHArgv(t *testing.T, calls [][]string, index int, want []string) {
	t.Helper()
	if index >= len(calls) {
		t.Fatalf("gh invocation %d missing; recorded %d call(s): %v", index, len(calls), calls)
	}
	got := calls[index]
	if len(got) != len(want) {
		t.Fatalf("gh argv[%d] = %q, want %q", index, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("gh argv[%d][%d] = %q, want %q\nfull argv = %q", index, i, got[i], want[i], got)
		}
	}
}
