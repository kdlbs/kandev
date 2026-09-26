package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveRemoteDefaultBranchReadsOriginHeadBeforeRunningGit(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"), []byte("ref: refs/remotes/origin/main\n"), 0o644); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}

	scriptDir := writeFakeGitScript(t, `echo "unexpected git invocation" >&2; exit 99`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newLiveDefaultTestManager(t)

	branch, err := mgr.ResolveRemoteDefaultBranch(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("ResolveRemoteDefaultBranch() error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
}

func TestResolveRemoteDefaultBranchRefreshesOriginHeadNonInteractively(t *testing.T) {
	repoPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoPath, ".git", "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "git.log")
	scriptDir := writeFakeGitScript(t, `
	case "${1:-}" in
	  remote)
	    printf '%s %s %s|%s|%s|%s\n' "${1:-}" "${2:-}" "${3:-}" "${GIT_TERMINAL_PROMPT:-}" "${GCM_INTERACTIVE:-}" "${GIT_SSH_COMMAND:-}" >> "${KD_GIT_LOG:?}"
    printf 'ref: refs/remotes/origin/main\n' > "$PWD/.git/refs/remotes/origin/HEAD"
    exit 0
    ;;
  *)
    echo "unexpected command" >&2
    exit 99
    ;;
esac
`)
	t.Setenv("KD_GIT_LOG", logPath)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newLiveDefaultTestManager(t)

	branch, err := mgr.ResolveRemoteDefaultBranch(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("ResolveRemoteDefaultBranch() error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	if !strings.Contains(string(log), "remote set-head origin") {
		t.Fatalf("command log %q does not contain remote set-head origin", string(log))
	}
	if !strings.Contains(string(log), "|0|Never|") {
		t.Fatalf("command log %q does not show the noninteractive git environment", string(log))
	}
}

func TestResolveRemoteDefaultBranchClassifiesAuthFailure(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `echo "fatal: Authentication failed" >&2; exit 1`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newLiveDefaultTestManager(t)

	_, err := mgr.ResolveRemoteDefaultBranch(context.Background(), t.TempDir())
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("error = %v, want ErrAuthFailed", err)
	}
}

func TestResolveRemoteDefaultBranchClassifiesNetworkFailure(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `echo "fatal: Could not resolve host: github.com" >&2; exit 1`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newLiveDefaultTestManager(t)

	_, err := mgr.ResolveRemoteDefaultBranch(context.Background(), t.TempDir())
	if !errors.Is(err, ErrRemoteDefaultNetwork) {
		t.Fatalf("error = %v, want ErrRemoteDefaultNetwork", err)
	}
}

func TestResolveRemoteDefaultBranchClassifiesMissingRemoteHead(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `echo "error: Cannot determine remote HEAD" >&2; exit 1`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newLiveDefaultTestManager(t)

	_, err := mgr.ResolveRemoteDefaultBranch(context.Background(), t.TempDir())
	if !errors.Is(err, ErrRemoteDefaultUnresolved) {
		t.Fatalf("error = %v, want ErrRemoteDefaultUnresolved", err)
	}
}

func TestResolveRemoteDefaultBranchPreservesTimeout(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `sleep 30`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newLiveDefaultTestManager(t)
	mgr.inspectTimeout = 100 * time.Millisecond

	_, err := mgr.ResolveRemoteDefaultBranch(context.Background(), t.TempDir())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline", err)
	}
}

func TestResolveRemoteDefaultBranchReportsUnresolvedResult(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `exit 0`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := newLiveDefaultTestManager(t)

	_, err := mgr.ResolveRemoteDefaultBranch(context.Background(), t.TempDir())
	if !errors.Is(err, ErrRemoteDefaultUnresolved) {
		t.Fatalf("error = %v, want ErrRemoteDefaultUnresolved", err)
	}
}

func TestResolveBaseRefWithFallbackUsesLiveRemoteDefault(t *testing.T) {
	mgr := newLiveDefaultTestManager(t)
	repoPath := initGitRepoForWorktreeTest(t)
	originHead := filepath.Join(repoPath, ".git", "refs", "remotes", "origin")
	if err := os.MkdirAll(originHead, 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originHead, "HEAD"), []byte("ref: refs/remotes/origin/main\n"), 0o644); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}

	req := CreateRequest{RepositoryPath: repoPath, BaseBranch: "stale-base"}
	base, warning, detail, err := mgr.resolveBaseRefWithFallback(context.Background(), &req)
	if err != nil {
		t.Fatalf("resolveBaseRefWithFallback() error: %v", err)
	}
	if base != "main" || req.BaseBranch != "main" {
		t.Fatalf("resolved base = %q, request base = %q, want main", base, req.BaseBranch)
	}
	if warning == "" || detail == "" {
		t.Fatalf("expected fallback warning and detail, got %q / %q", warning, detail)
	}
}

func newLiveDefaultTestManager(t *testing.T) *Manager {
	t.Helper()
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	return mgr
}
