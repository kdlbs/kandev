//go:build unix

package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLocalPreparer_CheckoutRunsRepositoryHooksOnlyWhenOptedIn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		optIn    bool
		wantHook bool
	}{
		{"opted out", false, false},
		{"opted in", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateGitEnv(t)
			preparer := NewLocalPreparer(newTestLocalLogger())
			repoDir := initGitRepo(t)
			env := newIsolatedGitEnv()
			for _, args := range [][]string{
				{"config", "--unset", "core.hooksPath"},
				{"checkout", "-b", "feature/hooked"},
				{"commit", "--allow-empty", "-m", "feature commit"},
				{"checkout", "main"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = repoDir
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v failed: %s", args, out)
				}
			}
			marker := filepath.Join(t.TempDir(), "ran")
			hook := "#!/bin/sh\n: > '" + marker + "'\n"
			hookPath := filepath.Join(repoDir, ".git", "hooks", "post-checkout")
			if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
				t.Fatal(err)
			}

			result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
				TaskID: "task-1", RepositoryPath: repoDir, BaseBranch: "feature/hooked", RunGitHooks: tc.optIn,
			}, nil)
			if err != nil || !result.Success {
				t.Fatalf("Prepare() = %v, %v", result, err)
			}
			_, statErr := os.Stat(marker)
			if ran := statErr == nil; ran != tc.wantHook {
				t.Fatalf("post-checkout hook ran = %v, want %v", ran, tc.wantHook)
			}
		})
	}
}
