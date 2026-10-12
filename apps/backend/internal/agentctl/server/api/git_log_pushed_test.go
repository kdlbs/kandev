package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

type pushedAPIFixture struct {
	root    string
	alpha   string
	beta    string
	bareA   string
	bareB   string
	base    string
	feature string
	merge   string
}

func runPushedAPIGit(t *testing.T, dir, date string, args ...string) string {
	t.Helper()
	argv := append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", argv...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func commitPushedAPIFile(t *testing.T, dir, name, date string) string {
	t.Helper()
	writeFileAPI(t, dir, name, name+"\n")
	runGitAPI(t, dir, "add", name)
	runPushedAPIGit(t, dir, date, "commit", "-m", name)
	return strings.TrimSpace(runGitAPI(t, dir, "rev-parse", "HEAD"))
}

func newPushedAPIFixture(t *testing.T) *pushedAPIFixture {
	t.Helper()
	temp := t.TempDir()
	f := &pushedAPIFixture{
		root:  filepath.Join(temp, "workspace"),
		bareA: filepath.Join(temp, "alpha.git"),
		bareB: filepath.Join(temp, "beta.git"),
	}
	f.alpha, f.beta = filepath.Join(f.root, "alpha"), filepath.Join(f.root, "beta")
	if err := os.MkdirAll(f.alpha, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, bare := range []string{f.bareA, f.bareB} {
		runGitAPI(t, temp, "init", "--bare", "--initial-branch=main", bare)
	}
	runGitAPI(t, f.alpha, "init", "--initial-branch=main")
	runGitAPI(t, f.alpha, "config", "user.email", "test@test.com")
	runGitAPI(t, f.alpha, "config", "user.name", "Test User")
	runGitAPI(t, f.alpha, "config", "core.hooksPath", filepath.Join(temp, "no-hooks"))
	runGitAPI(t, f.alpha, "config", "core.autocrlf", "false")
	f.base = commitPushedAPIFile(t, f.alpha, "base.txt", "2025-12-01T00:00:00Z")
	runGitAPI(t, f.alpha, "remote", "add", "origin", filepath.ToSlash(f.bareA))
	runGitAPI(t, f.alpha, "push", "-u", "origin", "main")
	runGitAPI(t, f.alpha, "checkout", "-b", "feature")
	runGitAPI(t, f.alpha, "push", "-u", "origin", "feature")
	f.feature = commitPushedAPIFile(t, f.alpha, "feature.txt", "2026-01-01T00:00:00Z")
	runGitAPI(t, f.alpha, "checkout", "-b", "side", f.base)
	commitPushedAPIFile(t, f.alpha, "side1.txt", "2026-02-01T00:00:00Z")
	commitPushedAPIFile(t, f.alpha, "side2.txt", "2026-03-01T00:00:00Z")
	runGitAPI(t, f.alpha, "checkout", "feature")
	runPushedAPIGit(t, f.alpha, "2026-04-01T00:00:00Z", "merge", "--no-ff", "-m", "Merge side", "side")
	f.merge = strings.TrimSpace(runGitAPI(t, f.alpha, "rev-parse", "HEAD"))
	runGitAPI(t, f.root, "clone", "--no-local", filepath.ToSlash(f.alpha), f.beta)
	runGitAPI(t, f.beta, "checkout", "-b", "main", "origin/main")
	runGitAPI(t, f.beta, "checkout", "feature")
	runGitAPI(t, f.beta, "remote", "set-url", "origin", filepath.ToSlash(f.bareB))
	runGitAPI(t, f.beta, "push", "-u", "origin", "main", "feature")
	return f
}

type pushedAPIReadSnapshot struct {
	head, refs, status, index, remoteRefs string
}

func snapshotPushedAPIRepo(t *testing.T, dir, bare string) pushedAPIReadSnapshot {
	t.Helper()
	snapshot := pushedAPIReadSnapshot{
		head:       runGitAPI(t, dir, "rev-parse", "HEAD"),
		refs:       runGitAPI(t, dir, "show-ref"),
		status:     runGitAPI(t, dir, "--no-optional-locks", "status", "--porcelain"),
		remoteRefs: runGitAPI(t, bare, "show-ref"),
	}
	index, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.index = string(index)
	return snapshot
}

func readPushedAPILog(t *testing.T, srv *Server, query string) *process.GitLogResult {
	t.Helper()
	response := getGitAPI(t, srv, "/api/v1/git/log"+query)
	var result process.GitLogResult
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP status %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.Error != "" || len(result.PerRepoErrors) != 0 {
		t.Fatalf("log result: %+v", result)
	}
	return &result
}

func assertPushedAPIRows(t *testing.T, rows []*process.GitCommitInfo, f *pushedAPIFixture, pushed bool) {
	t.Helper()
	if len(rows) != 2 || rows[0].CommitSHA != f.merge || rows[1].CommitSHA != f.feature {
		t.Fatalf("unexpected first-parent rows: %+v", rows)
	}
	for i, c := range rows {
		parent, files, message := f.feature, 2, "Merge side"
		date := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
		if i == 1 {
			parent, files, message = f.base, 1, "feature.txt"
			date = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		}
		committedAt, dateErr := time.Parse(time.RFC3339, c.CommittedAt)
		if c.Pushed != pushed || c.ParentSHA != parent || c.FilesChanged != files ||
			c.Insertions != files || c.Deletions != 0 || c.CommitMessage != message ||
			dateErr != nil || !committedAt.Equal(date) || c.AuthorName != "Test User" || c.AuthorEmail != "test@test.com" {
			t.Errorf("unexpected published commit fields: %+v, want pushed=%v", c, pushed)
		}
	}
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1, .4, .5, .6
func TestHandleGitLog_PushedRepositoryEvidence(t *testing.T) {
	f := newPushedAPIFixture(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: f.root, BaseBranches: map[string]string{"alpha": "main", "beta": "main"}}
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	srv := NewServer(cfg, mgr, nil, nil, log)
	beforeA, beforeB := snapshotPushedAPIRepo(t, f.alpha, f.bareA), snapshotPushedAPIRepo(t, f.beta, f.bareB)
	for _, repo := range []string{"alpha", "beta"} {
		result := readPushedAPILog(t, srv, "?repo="+repo+"&since="+f.base)
		assertPushedAPIRows(t, result.Commits, f, repo == "beta")
		for _, c := range result.Commits {
			if c.RepositoryName != "" {
				t.Errorf("selected log repository_name = %q, want existing empty value", c.RepositoryName)
			}
		}
	}
	result := readPushedAPILog(t, srv, "?limit=100")
	if len(result.Commits) != 4 {
		t.Fatalf("aggregate rows = %d, want 4", len(result.Commits))
	}
	byRepo := make(map[string][]*process.GitCommitInfo)
	for _, c := range result.Commits {
		byRepo[c.RepositoryName] = append(byRepo[c.RepositoryName], c)
	}
	if len(byRepo) != 2 {
		t.Fatalf("aggregate repositories: %+v", byRepo)
	}
	assertPushedAPIRows(t, byRepo["alpha"], f, false)
	assertPushedAPIRows(t, byRepo["beta"], f, true)
	if after := snapshotPushedAPIRepo(t, f.alpha, f.bareA); after != beforeA {
		t.Error("alpha history reads changed HEAD, refs, status, index, or remote refs")
	}
	if after := snapshotPushedAPIRepo(t, f.beta, f.bareB); after != beforeB {
		t.Error("beta history reads changed HEAD, refs, status, index, or remote refs")
	}
}
