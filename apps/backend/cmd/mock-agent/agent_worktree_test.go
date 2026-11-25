package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAgentWorktreeArgsDefaultToFixBranch(t *testing.T) {
	args, err := parseAgentWorktreeArgs("")
	if err != nil {
		t.Fatalf("parseAgentWorktreeArgs: %v", err)
	}
	if args.Branch != "fix-b" {
		t.Fatalf("branch = %q, want fix-b", args.Branch)
	}
	args, err = parseAgentWorktreeArgs(`{"branch":"feature/second"}`)
	if err != nil || args.Branch != "feature/second" {
		t.Fatalf("branch = %q, err=%v", args.Branch, err)
	}
}

func TestAgentWorktreeRootsFindSingleCheckoutParent(t *testing.T) {
	taskRoot := t.TempDir()
	repo := filepath.Join(taskRoot, "primary")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	root, primary, err := agentWorktreeRoots(repo)
	if err != nil {
		t.Fatalf("agentWorktreeRoots: %v", err)
	}
	if root != taskRoot || primary != repo {
		t.Fatalf("roots = %q / %q", root, primary)
	}
	if root, primary, err := agentWorktreeRoots(taskRoot); err != nil || root != taskRoot || primary != repo {
		t.Fatalf("task-root roots = %q / %q, err=%v", root, primary, err)
	}
}
