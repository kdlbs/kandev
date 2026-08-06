package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCreateWorktree_RemoteContributionUsesSourceRemoteAndExactHead(t *testing.T) {
	contributionURL := "https://github.com/contributor/widget.git"
	sourceBare, sourceSHA := initContributionSource(t)
	repoPath := initContributionTarget(t)
	runGit(t, repoPath, "config", "url.file://"+sourceBare+".insteadOf", contributionURL)

	binding := testRemoteContribution(sourceSHA, contributionURL)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-contribution",
		SessionID:          "session-contribution",
		RepositoryID:       "repo-target",
		RepositoryPath:     repoPath,
		BaseBranch:         binding.BaseBranch,
		CheckoutBranch:     binding.HeadBranch,
		RemoteContribution: &binding,
		TaskDirName:        "task-contribution",
		RepoName:           "widget",
	})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != sourceSHA {
		t.Fatalf("worktree HEAD = %q, want source SHA %q", got, sourceSHA)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin")); !strings.HasSuffix(got, "target.git") {
		t.Fatalf("target origin changed to %q", got)
	}
	remoteName := binding.ContributionRemoteName()
	if got := strings.TrimSpace(runGit(t, repoPath, "config", "--get", "remote."+remoteName+".url")); got != contributionURL {
		t.Fatalf("contribution remote = %q, want %q", got, contributionURL)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")); got != remoteName+"/"+binding.HeadBranch {
		t.Fatalf("upstream = %q, want %q", got, remoteName+"/"+binding.HeadBranch)
	}

	second, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-contribution-2",
		SessionID:          "session-contribution-2",
		RepositoryID:       "repo-target",
		RepositoryPath:     repoPath,
		BaseBranch:         binding.BaseBranch,
		CheckoutBranch:     binding.HeadBranch,
		RemoteContribution: &binding,
		TaskDirName:        "task-contribution-2",
		RepoName:           "widget",
	})
	if err != nil {
		t.Fatalf("Create() collision worktree failed: %v", err)
	}
	if second.Branch == binding.HeadBranch || !strings.HasPrefix(second.Branch, binding.HeadBranch+"-") {
		t.Fatalf("collision branch = %q, want suffixed contribution branch", second.Branch)
	}
	if got := strings.TrimSpace(runGit(t, second.Path, "rev-parse", "HEAD")); got != sourceSHA {
		t.Fatalf("collision worktree HEAD = %q, want source SHA %q", got, sourceSHA)
	}
}

func TestCreateWorktree_RemoteContributionRejectsStaleHead(t *testing.T) {
	contributionURL := "https://github.com/contributor/widget.git"
	sourceBare, _ := initContributionSource(t)
	repoPath := initContributionTarget(t)
	runGit(t, repoPath, "config", "url.file://"+sourceBare+".insteadOf", contributionURL)
	binding := testRemoteContribution(strings.Repeat("0", 40), contributionURL)

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-stale",
		SessionID:          "session-stale",
		RepositoryID:       "repo-target",
		RepositoryPath:     repoPath,
		BaseBranch:         binding.BaseBranch,
		CheckoutBranch:     binding.HeadBranch,
		RemoteContribution: &binding,
		TaskDirName:        "task-stale",
		RepoName:           "widget",
	})
	if err == nil || !strings.Contains(err.Error(), "head changed") {
		t.Fatalf("Create() error = %v, want stale-head rejection", err)
	}
}

func TestCreateWorktree_RemoteContributionReuseAllowsLocalCommits(t *testing.T) {
	contributionURL := "https://github.com/contributor/widget.git"
	sourceBare, sourceSHA := initContributionSource(t)
	repoPath := initContributionTarget(t)
	runGit(t, repoPath, "config", "url.file://"+sourceBare+".insteadOf", contributionURL)
	binding := testRemoteContribution(sourceSHA, contributionURL)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	first, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-reuse",
		SessionID:          "session-reuse",
		RepositoryID:       "repo-target",
		RepositoryPath:     repoPath,
		BaseBranch:         binding.BaseBranch,
		CheckoutBranch:     binding.HeadBranch,
		RemoteContribution: &binding,
		TaskDirName:        "task-reuse",
		RepoName:           "widget",
	})
	if err != nil {
		t.Fatalf("initial Create() failed: %v", err)
	}

	runGit(t, first.Path, "config", "user.email", "test@example.com")
	runGit(t, first.Path, "config", "user.name", "Test User")
	writeTestFile(t, filepath.Join(first.Path, "agent-change.txt"), "agent change\n")
	runGit(t, first.Path, "add", "agent-change.txt")
	runGit(t, first.Path, "commit", "-m", "agent contribution")
	localSHA := strings.TrimSpace(runGit(t, first.Path, "rev-parse", "HEAD"))

	reused, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-reuse",
		SessionID:          "session-reuse",
		RepositoryID:       "repo-target",
		RepositoryPath:     repoPath,
		BaseBranch:         binding.BaseBranch,
		CheckoutBranch:     binding.HeadBranch,
		RemoteContribution: &binding,
		TaskDirName:        "task-reuse",
		RepoName:           "widget",
	})
	if err != nil {
		t.Fatalf("reuse Create() failed after local commit: %v", err)
	}
	if reused.ID != first.ID {
		t.Fatalf("reused worktree ID = %q, want %q", reused.ID, first.ID)
	}
	if got := strings.TrimSpace(runGit(t, reused.Path, "rev-parse", "HEAD")); got != localSHA {
		t.Fatalf("reused worktree HEAD = %q, want local commit %q", got, localSHA)
	}
}

func testRemoteContribution(headSHA, sourceURL string) models.RemoteContribution {
	return models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/target/widget/pull/7",
		Number:       7,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "contributor/feature",
		HeadSHA:      headSHA,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "contributor/widget", RemoteURL: sourceURL,
		},
		CollaborationAllowed: true,
	}
}

func initContributionSource(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "source.git")
	work := filepath.Join(root, "source-work")
	runGit(t, root, "init", "--bare", bare)
	runGit(t, root, "init", "-b", "main", work)
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test User")
	writeTestFile(t, filepath.Join(work, "README.md"), "source\n")
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "source base")
	runGit(t, work, "checkout", "-b", "contributor/feature")
	writeTestFile(t, filepath.Join(work, "change.txt"), "contribution\n")
	runGit(t, work, "add", "change.txt")
	runGit(t, work, "commit", "-m", "contribution")
	sha := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	runGit(t, work, "remote", "add", "origin", bare)
	runGit(t, work, "push", "origin", "main", "contributor/feature")
	return bare, sha
}

func initContributionTarget(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "target.git")
	work := filepath.Join(root, "target-work")
	runGit(t, root, "init", "--bare", bare)
	runGit(t, root, "init", "-b", "main", work)
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test User")
	writeTestFile(t, filepath.Join(work, "README.md"), "target\n")
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "target base")
	runGit(t, work, "remote", "add", "origin", bare)
	runGit(t, work, "push", "origin", "main")
	return work
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
