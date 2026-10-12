package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestHandleGitPushBindsRemoteAndExpectedBranch proves the HTTP surface carries
// both optional inputs into the operator rather than dropping them.
func TestHandleGitPushBindsRemoteAndExpectedBranch(t *testing.T) {
	fixture := newGitAPIFixture(t)
	backup := filepath.Join(t.TempDir(), "backup.git")
	runGitAPI(t, fixture.repo, "init", "--bare", "--initial-branch=main", backup)
	runGitAPI(t, fixture.repo, "remote", "add", "backup", backup)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/push", GitPushRequest{
		Remote:         "backup",
		ExpectedBranch: "feature/work",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if !result.Success {
		t.Fatalf("push failed: %+v", result)
	}
	if result.PushedRemote != "backup" || result.PushedBranch != "feature/work" {
		t.Errorf("destination = (%q, %q), want (backup, feature/work)", result.PushedRemote, result.PushedBranch)
	}
	head := fixture.head(t)
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "--git-dir="+backup, "rev-parse", "refs/heads/feature/work")); got != head {
		t.Errorf("backup feature/work = %q, want %q", got, head)
	}
	// origin must be untouched by a push that named another destination.
	runGitAPIExpectFailure(t, fixture.repo, "--git-dir="+fixture.bare, "rev-parse", "--verify", "refs/heads/feature/work")
}

// TestHandleGitPushRejectsMismatchedExpectedBranch proves the expected branch is
// enforced through the HTTP surface, not merely accepted.
func TestHandleGitPushRejectsMismatchedExpectedBranch(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/push", GitPushRequest{ExpectedBranch: "feature/other"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if result.Success {
		t.Fatalf("push succeeded on a mismatch: %+v", result)
	}
	if result.ErrorCode != "push_branch_mismatch" {
		t.Errorf("error_code = %q, want push_branch_mismatch", result.ErrorCode)
	}
	if result.ExpectedBranch != "feature/other" || result.CurrentBranch != "feature/work" {
		t.Errorf("branches = (%q, %q), want (feature/other, feature/work)", result.ExpectedBranch, result.CurrentBranch)
	}
}

// TestHandleGitPushPreflightBindsRemoteAndExpectedBranch covers the preflight
// endpoint's binding and its reporting of the destination it validated.
func TestHandleGitPushPreflightBindsRemoteAndExpectedBranch(t *testing.T) {
	fixture := newGitAPIFixture(t)
	backup := filepath.Join(t.TempDir(), "backup.git")
	runGitAPI(t, fixture.repo, "init", "--bare", "--initial-branch=main", backup)
	runGitAPI(t, fixture.repo, "remote", "add", "backup", backup)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/push-preflight", GitPushPreflightRequest{
		Remote:         "backup",
		ExpectedBranch: "feature/work",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if !result.Success {
		t.Fatalf("preflight failed: %+v", result)
	}
	if result.PushedRemote != "backup" || result.PushedBranch != "feature/work" {
		t.Errorf("validated (%q, %q), want (backup, feature/work)", result.PushedRemote, result.PushedBranch)
	}
	runGitAPIExpectFailure(t, fixture.repo, "--git-dir="+backup, "rev-parse", "--verify", "refs/heads/feature/work")
}

// TestGitPushRequestScopesTargetToSelectedRepo proves both optional inputs act
// on the repository the request selects and leave every sibling untouched.
func TestGitPushRequestScopesTargetToSelectedRepo(t *testing.T) {
	root := t.TempDir()
	backups := map[string]string{}
	for _, name := range []string{"alpha", "beta"} {
		repo := filepath.Join(root, name)
		if err := os.Mkdir(repo, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		backup := filepath.Join(root, name+"-backup.git")
		runGitAPI(t, root, "init", "--bare", "--initial-branch=main", backup)
		backups[name] = backup

		runGitAPI(t, repo, "init", "--initial-branch=main")
		runGitAPI(t, repo, "config", "user.email", "test@test.com")
		runGitAPI(t, repo, "config", "user.name", "Test User")
		writeFileAPI(t, repo, "README.md", name+"\n")
		runGitAPI(t, repo, "add", ".")
		runGitAPI(t, repo, "commit", "-m", "initial")
		runGitAPI(t, repo, "remote", "add", "backup", backup)
		runGitAPI(t, repo, "checkout", "-b", "feature/work")
		writeFileAPI(t, repo, "one.txt", "one\n")
		runGitAPI(t, repo, "add", ".")
		runGitAPI(t, repo, "commit", "-m", "work")
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: root}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	rec := postGitAPI(t, server, "/api/v1/git/push", GitPushRequest{
		Repo:           "alpha",
		Remote:         "backup",
		ExpectedBranch: "feature/work",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("push failed: %+v", result)
	}

	alphaHead := strings.TrimSpace(runGitAPI(t, filepath.Join(root, "alpha"), "rev-parse", "HEAD"))
	if got := strings.TrimSpace(runGitAPI(t, root, "--git-dir="+backups["alpha"], "rev-parse", "refs/heads/feature/work")); got != alphaHead {
		t.Errorf("alpha backup = %q, want %q", got, alphaHead)
	}
	runGitAPIExpectFailure(t, root, "--git-dir="+backups["beta"], "rev-parse", "--verify", "refs/heads/feature/work")
}
