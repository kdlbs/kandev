//go:build linux || darwin

package gitinit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/subproc"
)

// With the flag off the helper hands Git exactly the arguments the caller asked
// for, one process, and an environment that still carries the variables the
// managed policy would otherwise remove.
func TestCommandContextWithIsolationOffHandsGitTheBaselineVector(t *testing.T) {
	subproc.SetManagedGitIsolation(false)
	t.Cleanup(func() { subproc.SetManagedGitIsolation(true) })

	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "calls.log")
	script := "#!/bin/sh\n{ echo ARGS; for a in \"$@\"; do echo \"$a\"; done; echo ENV; env; echo END; } >> \"$KANDEV_GOLDEN_LOG\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatalf("write recording git: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KANDEV_GOLDEN_LOG", logPath)
	t.Setenv("GIT_EXTERNAL_DIFF", "/bin/true")
	t.Setenv("GIT_ALLOW_PROTOCOL", "ext")

	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open directory: %v", err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	command, err := CommandContext(context.Background(), directory.Name(), directory)
	if err != nil {
		t.Fatalf("CommandContext: %v", err)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v: %s", err, output)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	log := string(raw)
	if got := strings.Count(log, "ARGS\n"); got != 1 {
		t.Fatalf("Git ran %d times, want 1: %s", got, log)
	}
	argv, env, _ := strings.Cut(strings.TrimPrefix(log, "ARGS\n"), "ENV\n")
	if argv != "init\n--initial-branch=main\n" {
		t.Fatalf("argv = %q, want the unmodified init vector", argv)
	}
	for _, want := range []string{"GIT_EXTERNAL_DIFF=/bin/true", "GIT_ALLOW_PROTOCOL=ext"} {
		if !strings.Contains(env, want+"\n") {
			t.Fatalf("environment lost %s with the mode off", want)
		}
	}
	if strings.Contains(env, helperEnvironmentVariable+"=") {
		t.Fatalf("helper environment leaked into Git: %s", env)
	}
}
