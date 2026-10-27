package subproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// countingGitOnPath puts a git wrapper first on PATH that appends one line per
// invocation to the returned log file and then runs the real Git.
func countingGitOnPath(t *testing.T, extra string) string {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	log := filepath.Join(t.TempDir(), "invocations")
	writeExecutable(t, filepath.Join(dir, "git"),
		"#!/bin/sh\necho \"$*\" >> '"+log+"'\n"+extra+"exec '"+realGit+"' \"$@\"\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func invocationCount(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Split(strings.TrimSpace(string(data)), "\n"))
}

func TestManagedGitSpawnsAtMostOneEnumerationProcess(t *testing.T) {
	repo := newPolicyRepo(t)
	log := countingGitOnPath(t, "")
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"rev-parse", "--git-dir"}, 0},
		{[]string{"status"}, 1},
		{[]string{"commit", "-m", "x"}, 1},
	} {
		before := invocationCount(t, log)
		if _, err := buildManagedGitVector(context.Background(), repo.dir, os.Environ(), tc.args); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if got := invocationCount(t, log) - before; got != tc.want {
			t.Errorf("%v spawned %d Git processes, want %d", tc.args, got, tc.want)
		}
	}
}

func TestManagedGitRefusesWhenDriverListingExceedsLimit(t *testing.T) {
	repo := newPolicyRepo(t)
	name := strings.Repeat("n", 200)
	var config strings.Builder
	for i := 0; i < enumerationMaxOutput/200; i++ {
		fmt.Fprintf(&config, "[hook \"%s%d\"]\n\tcommand = true\n", name, i)
	}
	f, err := os.OpenFile(filepath.Join(repo.dir, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(config.String()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = buildManagedGitVector(context.Background(), repo.dir, os.Environ(), []string{"status"})
	if err == nil || !strings.Contains(err.Error(), "driver listing exceeds") {
		t.Fatalf("want a refusal naming the listing limit, got %v", err)
	}
}

func TestManagedGitRefusesWhenEnumerationTimesOut(t *testing.T) {
	repo := newPolicyRepo(t)
	countingGitOnPath(t, "case \"$*\" in *--get-regexp*) exec sleep 30 ;; esac\n")
	previous := enumerationTimeout
	enumerationTimeout = 300 * time.Millisecond
	t.Cleanup(func() { enumerationTimeout = previous })

	started := time.Now()
	_, err := buildManagedGitVector(context.Background(), repo.dir, os.Environ(), []string{"status"})
	if err == nil || !strings.Contains(err.Error(), "could not determine") {
		t.Fatalf("want a refusal naming the policy determination, got %v", err)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatalf("enumeration did not stop at its bound")
	}
}
