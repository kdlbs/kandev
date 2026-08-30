package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitbootstrap"
)

func TestGitOperatorPushPublishesEmptyRemoteBaseBeforeTaskBranch(t *testing.T) {
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v", result)
	}
	baseSHA := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/main"))
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/main")); got != baseSHA {
		t.Fatalf("remote base = %q, want local baseline %q", got, baseSHA)
	}
	branchSHA := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/feature/empty")); got != branchSHA {
		t.Fatalf("remote task branch = %q, want %q", got, branchSHA)
	}
}

func TestGitOperatorPushStopsWhenEmptyRemoteGainsHistory(t *testing.T) {
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)
	seedDir := filepath.Join(filepath.Dir(originDir), "seed")
	runGit(t, filepath.Dir(seedDir), "init", "-b", "external", seedDir)
	runGit(t, seedDir, "config", "user.name", "External User")
	runGit(t, seedDir, "config", "user.email", "external@example.com")
	runGit(t, seedDir, "remote", "add", "origin", originDir)
	if err := os.WriteFile(filepath.Join(seedDir, "external.txt"), []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seedDir, "add", "external.txt")
	runGit(t, seedDir, "commit", "-m", "external history")
	runGit(t, seedDir, "push", "origin", "external")

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success || result.ErrorCode != emptyRemoteRemoteChangedErrorCode {
		t.Fatalf("Push() result = %+v, want a remote-changed failure", result)
	}
	if _, err := os.Stat(filepath.Join(originDir, "refs", "heads", "feature", "empty")); !os.IsNotExist(err) {
		t.Fatalf("task branch was pushed after remote race, stat error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/main")); got == "" {
		t.Fatal("local baseline was removed after remote race")
	}
}

func TestGitOperatorPushReportsEmptyRemoteBasePublicationFailure(t *testing.T) {
	_, originDir, operator := setupEmptyRemoteTaskRepo(t)
	writeExecutable(t, filepath.Join(originDir, "hooks", "pre-receive"), "#!/bin/sh\necho reject >&2\nexit 1\n")

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success || result.ErrorCode != emptyRemoteBasePublishFailedErrorCode {
		t.Fatalf("Push() result = %+v, want base publication failure", result)
	}
	if _, err := os.Stat(filepath.Join(originDir, "refs", "heads", "main")); !os.IsNotExist(err) {
		t.Fatalf("base branch was published after rejection, stat error = %v", err)
	}
}

func TestGitOperatorPushReportsTaskBranchFailureAfterBasePublication(t *testing.T) {
	_, originDir, operator := setupEmptyRemoteTaskRepo(t)
	hook := "#!/bin/sh\nwhile read old new ref; do\n  case \"$ref\" in\n    refs/heads/main) exit 0 ;;\n    refs/heads/feature/empty) echo reject >&2; exit 1 ;;\n  esac\ndone\nexit 0\n"
	writeExecutable(t, filepath.Join(originDir, "hooks", "pre-receive"), hook)

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success || result.ErrorCode != emptyRemoteBranchPublishFailedErrorCode {
		t.Fatalf("Push() result = %+v, want task branch failure", result)
	}
	if _, err := os.Stat(filepath.Join(originDir, "refs", "heads", "main")); err != nil {
		t.Fatalf("base branch was not retained after task push failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(originDir, "refs", "heads", "feature", "empty")); !os.IsNotExist(err) {
		t.Fatalf("task branch unexpectedly exists after rejection, stat error = %v", err)
	}
}

func TestGitOperatorCreatePRPublishesEmptyRemoteBaseBeforeProviderRequest(t *testing.T) {
	_, originDir, operator := setupEmptyRemoteTaskRepo(t)
	const remoteURL = "https://github.com/acme/empty.git"

	scriptDir := t.TempDir()
	writeExecutable(t, filepath.Join(scriptDir, "gh"), "#!/bin/sh\nprintf 'https://github.com/acme/empty/pull/1\\n'\n")
	writeGitRemoteWrapper(t, scriptDir, remoteURL)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := operator.CreatePR(context.Background(), "Empty remote", "Publish the task", "main", false)
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}
	if !result.Success || !result.BranchPushed || result.PRURL == "" {
		t.Fatalf("CreatePR() result = %+v", result)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/main")); got == "" {
		t.Fatal("CreatePR() did not publish the base branch")
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/feature/empty")); got == "" {
		t.Fatal("CreatePR() did not publish the task branch")
	}
}

func setupEmptyRemoteTaskRepo(t *testing.T) (string, string, *GitOperator) {
	t.Helper()
	root := t.TempDir()
	originDir := filepath.Join(root, "origin.git")
	repoDir := filepath.Join(root, "repo")
	runGit(t, root, "init", "--bare", "--initial-branch=main", originDir)
	runGit(t, root, "init", "-b", "main", repoDir)
	runGit(t, repoDir, "config", "user.name", "Task User")
	runGit(t, repoDir, "config", "user.email", "task@example.com")
	runGit(t, repoDir, "remote", "add", "origin", originDir)
	if _, err := gitbootstrap.Ensure(context.Background(), repoDir, "main"); err != nil {
		t.Fatalf("gitbootstrap.Ensure() error = %v", err)
	}
	runGit(t, repoDir, "checkout", "-b", "feature/empty")
	if err := os.WriteFile(filepath.Join(repoDir, "change.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "change.txt")
	runGit(t, repoDir, "commit", "-m", "task change")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetBaseBranch("main")
	return repoDir, originDir, NewGitOperator(repoDir, newTestLogger(t), tracker)
}
