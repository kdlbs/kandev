package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// setupPushTargetRepo builds a checkout with one commit on "feature/work" and
// no remotes. Tests add exactly the remotes they need.
func setupPushTargetRepo(t *testing.T) (string, *GitOperator) {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGit(t, root, "init", "-b", "main", repoDir)
	runGit(t, repoDir, "config", "user.name", "Task User")
	runGit(t, repoDir, "config", "user.email", "task@example.com")
	if err := os.WriteFile(filepath.Join(repoDir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "seed.txt")
	runGit(t, repoDir, "commit", "-m", "seed")
	runGit(t, repoDir, "checkout", "-b", "feature/work")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetBaseBranch("main")
	return repoDir, NewGitOperator(repoDir, newTestLogger(t), tracker)
}

func TestResolvePushTargetTrimsAndTreatsEmptyAsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     PushOptions
		remote   string
		expected string
	}{
		{name: "surrounding whitespace", opts: PushOptions{Remote: "  backup  ", ExpectedBranch: " main "}, remote: "backup", expected: "main"},
		{name: "whitespace only is absent", opts: PushOptions{Remote: "   ", ExpectedBranch: "\t\n"}, remote: "", expected: ""},
		{name: "empty stays absent", opts: PushOptions{}, remote: "", expected: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.opts.normalized()
			if got.Remote != tc.remote {
				t.Errorf("Remote = %q, want %q", got.Remote, tc.remote)
			}
			if got.ExpectedBranch != tc.expected {
				t.Errorf("ExpectedBranch = %q, want %q", got.ExpectedBranch, tc.expected)
			}
		})
	}
}

func TestResolvePushTargetDiscriminatesNameFromURL(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)
	runGit(t, repoDir, "remote", "add", "backup", "https://example.test/backup.git")

	name, refusal := operator.resolvePushTarget(context.Background(), "backup")
	if refusal != nil {
		t.Fatalf("name form refused: %+v", refusal)
	}
	if name != "backup" {
		t.Errorf("name form resolved to %q, want %q", name, "backup")
	}

	name, refusal = operator.resolvePushTarget(context.Background(), "https://example.test/backup.git")
	if refusal != nil {
		t.Fatalf("URL form refused: %+v", refusal)
	}
	if name != "backup" {
		t.Errorf("URL form resolved to %q, want %q", name, "backup")
	}
}

func TestResolvePushTargetRejectsCaseAndPrefixVariants(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)
	runGit(t, repoDir, "remote", "add", "backup", "https://example.test/backup.git")

	for _, target := range []string{"Backup", "BACKUP", "back", "backup2"} {
		t.Run(target, func(t *testing.T) {
			_, refusal := operator.resolvePushTarget(context.Background(), target)
			if refusal == nil {
				t.Fatalf("resolvePushTarget(%q) matched; want refusal", target)
			}
			if refusal.code != pushRemoteNotFoundErrorCode {
				t.Errorf("code = %q, want %q", refusal.code, pushRemoteNotFoundErrorCode)
			}
		})
	}
}

func TestResolvePushTargetReportsUnknownRemoteName(t *testing.T) {
	_, operator := setupPushTargetRepo(t)

	_, refusal := operator.resolvePushTarget(context.Background(), "missing")
	if refusal == nil || refusal.code != pushRemoteNotFoundErrorCode {
		t.Fatalf("refusal = %+v, want %q", refusal, pushRemoteNotFoundErrorCode)
	}
}

func TestResolvePushTargetMatchesSingleEntryPushURL(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)
	// pushurl shadows url, so the fetch URL must not match once a push URL exists.
	runGit(t, repoDir, "remote", "add", "backup", "https://example.test/fetch.git")
	runGit(t, repoDir, "config", "remote.backup.pushurl", "https://example.test/push.git")

	name, refusal := operator.resolvePushTarget(context.Background(), "https://example.test/push.git")
	if refusal != nil {
		t.Fatalf("push URL refused: %+v", refusal)
	}
	if name != "backup" {
		t.Errorf("resolved %q, want %q", name, "backup")
	}

	_, refusal = operator.resolvePushTarget(context.Background(), "https://example.test/fetch.git")
	if refusal == nil || refusal.code != pushRemoteURLUnmatchedErrorCode {
		t.Fatalf("shadowed fetch URL refusal = %+v, want %q", refusal, pushRemoteURLUnmatchedErrorCode)
	}
}

func TestResolvePushTargetPicksFirstByByteOrder(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)
	shared := "https://example.test/shared.git"
	runGit(t, repoDir, "remote", "add", "zulu", shared)
	runGit(t, repoDir, "remote", "add", "alpha", shared)
	runGit(t, repoDir, "remote", "add", "mike", shared)

	name, refusal := operator.resolvePushTarget(context.Background(), shared)
	if refusal != nil {
		t.Fatalf("shared URL refused: %+v", refusal)
	}
	if name != "alpha" {
		t.Errorf("resolved %q, want %q", name, "alpha")
	}
}

func TestResolvePushTargetReportsUnmatchedURL(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)
	runGit(t, repoDir, "remote", "add", "backup", "https://example.test/backup.git")

	_, refusal := operator.resolvePushTarget(context.Background(), "https://example.test/other.git")
	if refusal == nil || refusal.code != pushRemoteURLUnmatchedErrorCode {
		t.Fatalf("refusal = %+v, want %q", refusal, pushRemoteURLUnmatchedErrorCode)
	}
}

func TestResolvePushTargetReportsFanoutRemote(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)
	runGit(t, repoDir, "remote", "add", "mirror", "https://example.test/a.git")
	runGit(t, repoDir, "config", "--add", "remote.mirror.pushurl", "https://example.test/a.git")
	runGit(t, repoDir, "config", "--add", "remote.mirror.pushurl", "https://example.test/b.git")

	_, refusal := operator.resolvePushTarget(context.Background(), "https://example.test/a.git")
	if refusal == nil || refusal.code != pushRemoteFanoutErrorCode {
		t.Fatalf("refusal = %+v, want %q", refusal, pushRemoteFanoutErrorCode)
	}

	// A matching single-entry remote takes precedence over the fan-out refusal.
	runGit(t, repoDir, "remote", "add", "direct", "https://example.test/a.git")
	name, refusal := operator.resolvePushTarget(context.Background(), "https://example.test/a.git")
	if refusal != nil {
		t.Fatalf("single-entry remote refused: %+v", refusal)
	}
	if name != "direct" {
		t.Errorf("resolved %q, want %q", name, "direct")
	}
}

func TestResolvePushTargetReportsUnreadableRemoteConfig(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)
	runGit(t, repoDir, "remote", "add", "backup", "https://example.test/backup.git")
	// A config file git cannot parse must not read as "no such remote".
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "config"), []byte("[remote \"backup\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, refusal := operator.resolvePushTarget(context.Background(), "backup")
	if refusal == nil || refusal.code != pushRemoteConfigUnreadableErrorCode {
		t.Fatalf("refusal = %+v, want %q", refusal, pushRemoteConfigUnreadableErrorCode)
	}
}

func TestGitOperatorCurrentBranchReportsDetachedHeadAsEmpty(t *testing.T) {
	repoDir, operator := setupPushTargetRepo(t)

	branch, err := operator.currentBranch(context.Background())
	if err != nil {
		t.Fatalf("currentBranch() error = %v", err)
	}
	if branch != "feature/work" {
		t.Errorf("branch = %q, want %q", branch, "feature/work")
	}

	runGit(t, repoDir, "checkout", "--detach", "HEAD")
	branch, err = operator.currentBranch(context.Background())
	if err != nil {
		t.Fatalf("currentBranch() on detached HEAD error = %v", err)
	}
	if branch != "" {
		t.Errorf("detached branch = %q, want empty", branch)
	}
}
