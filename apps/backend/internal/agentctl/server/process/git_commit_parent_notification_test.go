package process

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestGitCommitParentNotification exercises the values sent to a workspace
// subscriber by commits created through the operator.
func TestGitCommitParentNotification(t *testing.T) {
	dir := initUnstageOperatorRepo(t)
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	subscriber := tracker.SubscribeWorkspaceStream()
	t.Cleanup(func() {
		tracker.UnsubscribeWorkspaceStream(subscriber)
		tracker.Stop()
	})
	operator := NewGitOperator(dir, newTestLogger(t), tracker)

	commitAndRead := func(message string, stageAll bool) (string, string) {
		t.Helper()
		result, err := operator.Commit(context.Background(), message, stageAll, false)
		if err != nil || result == nil || !result.Success {
			t.Fatalf("Commit(%q): result=%+v error=%v", message, result, err)
		}
		sha := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
		deadline := time.After(5 * time.Second)
		for {
			select {
			case event := <-subscriber:
				if event.GitCommit != nil && event.GitCommit.CommitSHA == sha {
					return sha, event.GitCommit.ParentSHA
				}
			case <-deadline:
				t.Fatalf("missing commit notification for %s", sha)
			}
		}
	}

	writeFile(t, dir, "base.txt", "base\n")
	root, rootParent := commitAndRead("initial", true)
	if parents := strings.Fields(runGit(t, dir, "rev-list", "--parents", "-n", "1", root)); len(parents) != 1 {
		t.Fatalf("initial commit is not a root: %v", parents)
	}
	writeFile(t, dir, "main.txt", "main\n")
	mainHead, ordinaryParent := commitAndRead("main work", true)
	runGit(t, dir, "branch", "side", root)
	runGit(t, dir, "checkout", "side")
	writeFile(t, dir, "side.txt", "side\n")
	runGit(t, dir, "add", "side.txt")
	runGit(t, dir, "commit", "-m", "side work")
	sideHead := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	runGit(t, dir, "checkout", "main")
	runGit(t, dir, "merge", "--no-commit", "--no-ff", "side")
	mergeHead, mergeParent := commitAndRead("merge side", false)
	parents := strings.Fields(runGit(t, dir, "rev-list", "--parents", "-n", "1", mergeHead))
	if len(parents) != 3 || parents[1] != mainHead || parents[2] != sideHead {
		t.Fatalf("unexpected merge graph: %v", parents)
	}

	for _, tc := range []struct{ name, got, want string }{
		{"root", rootParent, ""},
		{"ordinary", ordinaryParent, root},
		{"merge", mergeParent, mainHead},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("notification parent = %q, want %q", tc.got, tc.want)
			}
		})
	}
}
