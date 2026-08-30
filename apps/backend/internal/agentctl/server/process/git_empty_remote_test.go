package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	markerRef, err := gitbootstrap.MarkerRef("main")
	if err != nil {
		t.Fatal(err)
	}
	if hasLocalRef(t, repoDir, markerRef) {
		t.Fatalf("bootstrap marker %q remains after successful publication", markerRef)
	}
}

func TestGitOperatorPushPublishesAnotherTaskBranchAfterBaseInitialization(t *testing.T) {
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)

	first, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("first Push() error = %v", err)
	}
	if !first.Success {
		t.Fatalf("first Push() result = %+v", first)
	}

	runGit(t, repoDir, "checkout", "-b", "feature/second", "main")
	if err := os.WriteFile(filepath.Join(repoDir, "second.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "second.txt")
	runGit(t, repoDir, "commit", "-m", "second task change")

	second, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("second Push() error = %v", err)
	}
	if !second.Success {
		t.Fatalf("second Push() result = %+v", second)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/feature/second")); got == "" {
		t.Fatal("second task branch was not published after base initialization")
	}
}

func TestGitOperatorPushAcceptsMarkedBaseWithUnrelatedRemoteRefs(t *testing.T) {
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)
	runGit(t, repoDir, "push", "origin", "main")

	runGit(t, repoDir, "checkout", "-b", "external", "main")
	if err := os.WriteFile(filepath.Join(repoDir, "external.txt"), []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "external.txt")
	runGit(t, repoDir, "commit", "-m", "external history")
	runGit(t, repoDir, "push", "origin", "external")
	runGit(t, repoDir, "checkout", "feature/empty")

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want marked base recovery to continue", result)
	}
	markerRef, err := gitbootstrap.MarkerRef("main")
	if err != nil {
		t.Fatal(err)
	}
	if hasLocalRef(t, repoDir, markerRef) {
		t.Fatalf("bootstrap marker %q remains after crash recovery", markerRef)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/external")); got == "" {
		t.Fatal("unrelated remote ref was lost during crash recovery")
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

func TestGitOperatorPushRetainsMarkerWhenUnrelatedRefAppearsDuringBasePublication(t *testing.T) {
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)
	baseline, present, err := gitbootstrap.Validate(context.Background(), repoDir, "main")
	if err != nil || !present {
		t.Fatalf("Validate() = (%+v, %v, %v), want a baseline", baseline, present, err)
	}
	hook := "#!/bin/sh\n" +
		"while read old new ref; do\n" +
		"  if [ \"$ref\" = \"refs/heads/main\" ] && [ \"$new\" != \"0000000000000000000000000000000000000000\" ]; then\n" +
		"    tree=$(printf '' | env -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_QUARANTINE_PATH git hash-object -t tree -w --stdin)\n" +
		"    commit=$(printf 'tree %s\\nauthor External User \u003cexternal@example.com\u003e 0 +0000\\ncommitter External User \u003cexternal@example.com\u003e 0 +0000\\n\\nexternal history\\n' \"$tree\" | env -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_QUARANTINE_PATH git hash-object -t commit -w --stdin)\n" +
		"    env -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_QUARANTINE_PATH git update-ref refs/heads/external \"$commit\"\n" +
		"  fi\n" +
		"done\n" +
		"exit 0\n"
	writeExecutable(t, filepath.Join(originDir, "hooks", "pre-receive"), hook)

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success || result.ErrorCode != emptyRemoteRemoteChangedErrorCode {
		t.Fatalf("Push() result = %+v, want remote-changed failure", result)
	}
	markerRef, err := gitbootstrap.MarkerRef("main")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLocalRef(t, repoDir, markerRef) {
		t.Fatalf("bootstrap marker %q was retired after publication race", markerRef)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/main")); got != baseline.Commit {
		t.Fatalf("remote base = %q, want the validated baseline %q", got, baseline.Commit)
	}
	if !hasLocalRef(t, originDir, "refs/heads/external") {
		t.Fatal("unrelated remote ref was not preserved")
	}
	if hasLocalRef(t, originDir, "refs/heads/feature/empty") {
		t.Fatal("task branch was published after publication race")
	}
}

func TestGitOperatorPushPublishesValidatedBaselineCommit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}

	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)
	baseline, present, err := gitbootstrap.Validate(context.Background(), repoDir, "main")
	if err != nil || !present {
		t.Fatalf("Validate() = (%+v, %v, %v), want a baseline", baseline, present, err)
	}
	scriptDir := t.TempDir()
	moveMarker := filepath.Join(scriptDir, "moved")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	shim := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"ls-remote\" ] && [ ! -f %q ]; then\n  touch %q\n  %q update-ref refs/heads/main refs/heads/feature/empty\nfi\nexec %q \"$@\"\n", moveMarker, moveMarker, realGit, realGit)
	writeExecutable(t, filepath.Join(scriptDir, "git"), shim)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success || result.ErrorCode != emptyRemoteRemoteChangedErrorCode {
		t.Fatalf("Push() result = %+v, want local-baseline conflict", result)
	}
	if hasLocalRef(t, originDir, "refs/heads/main") {
		if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/main")); got != baseline.Commit {
			t.Fatalf("remote base = %q, want the validated baseline %q", got, baseline.Commit)
		}
	}
	markerRef, err := gitbootstrap.MarkerRef("main")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLocalRef(t, repoDir, markerRef) {
		t.Fatalf("bootstrap marker %q was retired after local baseline changed", markerRef)
	}
	if hasLocalRef(t, originDir, "refs/heads/feature/empty") {
		t.Fatal("task branch was published after local baseline changed")
	}
}

func TestGitOperatorCreatePRPublishesEmptyRemoteBaseBeforeProviderRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}
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

func TestGitOperatorCreatePRRejectsDifferentBootstrapBaseBeforePush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)
	const remoteURL = "https://github.com/acme/empty.git"

	scriptDir := t.TempDir()
	writeExecutable(t, filepath.Join(scriptDir, "gh"), "#!/bin/sh\nprintf 'https://github.com/acme/empty/pull/1\\n'\n")
	writeGitRemoteWrapper(t, scriptDir, remoteURL)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := operator.CreatePR(context.Background(), "Empty remote", "Publish the task", "develop", false)
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}
	if result.Success || result.ErrorCode != emptyRemoteRemoteChangedErrorCode {
		t.Fatalf("CreatePR() result = %+v, want a base mismatch before push", result)
	}
	if refs := strings.TrimSpace(runGit(t, repoDir, "ls-remote", "--refs", "origin")); refs != "" {
		t.Fatalf("remote refs were created for a mismatched base: %s", refs)
	}
	if hasLocalRef(t, originDir, "refs/heads/feature/empty") {
		t.Fatal("task branch was published for a mismatched base")
	}
}

func TestGitOperatorPushRedactsEmptyRemoteProbeFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}
	_, _, operator := setupEmptyRemoteTaskRepo(t)
	const secret = "super-secret-token"
	scriptDir := t.TempDir()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	writeExecutable(t, filepath.Join(scriptDir, "git"), fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"ls-remote\" ]; then\n  printf 'fatal: unable to access https://user:%s@example.com/empty.git: denied\\n' >&2\n  exit 1\nfi\nexec %q \"$@\"\n", secret, realGit))
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := operator.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success || result.ErrorCode != emptyRemoteBasePublishFailedErrorCode {
		t.Fatalf("Push() result = %+v, want bounded probe failure", result)
	}
	if strings.Contains(result.Error, secret) || strings.Contains(result.Output, secret) {
		t.Fatalf("probe secret leaked in result: %+v", result)
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

func hasLocalRef(t *testing.T, repoDir, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return cmd.Run() == nil
}
