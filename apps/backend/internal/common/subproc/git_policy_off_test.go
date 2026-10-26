package subproc

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestManagedGitOffLeavesCommandUntouched(t *testing.T) {
	setIsolationForTest(t, false)
	vectors := [][]string{
		{"rev-parse", "HEAD"},
		{"status", "--porcelain"},
		{"diff", "--stat"},
		{"push", "origin", "main"},
		{"commit", "-m", "x"},
		{"blame", "a.txt"},
		{"diff", "--textconv"},
		{"-c", "core.hooksPath=/tmp/x", "log", "-1"},
	}
	for _, args := range vectors {
		cmd := NewGitCommand(context.Background(), args...)
		// A directory that does not exist makes any driver enumeration fail,
		// so success also proves no extra Git process was started.
		cmd.Dir = filepath.Join(t.TempDir(), "missing")
		cmd.Env = []string{"GIT_EXTERNAL_DIFF=/bin/true", "GIT_ALLOW_PROTOCOL=ext", "PATH=" + os.Getenv("PATH")}
		wantArgs := append([]string(nil), cmd.Args...)
		wantEnv := append([]string(nil), cmd.Env...)
		if err := applyManagedGitPolicy(context.Background(), cmd); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		if !reflect.DeepEqual(cmd.Args, wantArgs) || !reflect.DeepEqual(cmd.Env, wantEnv) {
			t.Fatalf("git %v: command changed with the mode off\nargs %v\nenv %v", args, cmd.Args, cmd.Env)
		}
	}
}

func TestManagedGitOffHelperArgsAndEnvPassThrough(t *testing.T) {
	setIsolationForTest(t, false)
	args := []string{"commit", "-m", "x"}
	got, err := ManagedGitArgs(context.Background(), nil, args)
	if err != nil || !reflect.DeepEqual(got, args) {
		t.Fatalf("ManagedGitArgs = %v, %v; want the caller's arguments", got, err)
	}
	env := []string{"GIT_EXTERNAL_DIFF=/bin/true", "A=b"}
	if got := ManagedGitEnv(env); !reflect.DeepEqual(got, env) {
		t.Fatalf("ManagedGitEnv = %v; want the caller's environment", got)
	}
}

func TestManagedGitOffRunsRepositorySuppliedCode(t *testing.T) {
	setIsolationForTest(t, false)
	repo := newPolicyRepo(t)
	mustGit(context.Background(), t, repo.dir, "commit", "--allow-empty", "-m", "plain")
	if !repo.marked() {
		t.Fatal("with the mode off the repository's own hook must run, as it did before isolation existed")
	}
}
