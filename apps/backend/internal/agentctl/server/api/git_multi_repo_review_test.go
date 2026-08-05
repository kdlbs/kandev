package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestMultiRepoReviewEndpointsUseStoredBaseBranches(t *testing.T) {
	taskRoot := t.TempDir()
	bases := map[string]string{"frontend": "develop", "backend": "release"}
	baseCommits := make(map[string]string, len(bases))
	baseCommit := ""
	for repo, baseBranch := range bases {
		repoDir := filepath.Join(taskRoot, repo)
		if err := os.Mkdir(repoDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		runGitAPI(t, repoDir, "init", "--initial-branch="+baseBranch)
		runGitAPI(t, repoDir, "config", "user.email", "test@test.com")
		runGitAPI(t, repoDir, "config", "user.name", "Test User")
		writeFileAPI(t, repoDir, "README.md", "base\n")
		runGitAPI(t, repoDir, "add", ".")
		runGitAPI(t, repoDir, "commit", "-m", "initial")
		repoBaseCommit := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))
		baseCommits[repo] = repoBaseCommit
		if baseCommit == "" {
			baseCommit = repoBaseCommit
		}
		runGitAPI(t, repoDir, "checkout", "-b", "feature/review")
		writeFileAPI(t, repoDir, "changed.txt", repo+" change\n")
		runGitAPI(t, repoDir, "add", ".")
		runGitAPI(t, repoDir, "commit", "-m", "feature change")
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: taskRoot, BaseBranches: bases}
	mgr := process.NewManager(cfg, log)
	srv := NewServer(cfg, mgr, nil, nil, log)

	logResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		logResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/git/log?limit=100", nil),
	)
	if logResponse.Code != http.StatusOK {
		t.Fatalf("git log status = %d: %s", logResponse.Code, logResponse.Body.String())
	}
	var commits process.GitLogResult
	if err := json.Unmarshal(logResponse.Body.Bytes(), &commits); err != nil {
		t.Fatalf("decode git log: %v", err)
	}
	commitsByRepo := make(map[string]int)
	for _, commit := range commits.Commits {
		commitsByRepo[commit.RepositoryName]++
	}
	for repo := range bases {
		if commitsByRepo[repo] != 1 {
			t.Fatalf("commits for %s = %d, want 1: %s", repo, commitsByRepo[repo], logResponse.Body.String())
		}
	}

	diffResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		diffResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/git/cumulative-diff?base="+baseCommit, nil),
	)
	if diffResponse.Code != http.StatusOK {
		t.Fatalf("cumulative diff status = %d: %s", diffResponse.Code, diffResponse.Body.String())
	}
	var diff process.CumulativeDiffResult
	if err := json.Unmarshal(diffResponse.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	if len(diff.Files) != len(bases) {
		t.Fatalf("cumulative diff files = %d, want %d: %s", len(diff.Files), len(bases), diffResponse.Body.String())
	}
	for repo := range bases {
		payload, ok := diff.Files[repo+"\x00changed.txt"]
		if !ok {
			t.Errorf("cumulative diff missing %s/changed.txt: %s", repo, diffResponse.Body.String())
			continue
		}
		file, ok := payload.(map[string]interface{})
		if !ok {
			t.Fatalf("cumulative diff payload for %s has type %T", repo, payload)
		}
		if got := file["base_ref"]; got != baseCommits[repo] {
			t.Errorf("cumulative diff base_ref for %s = %v, want %s", repo, got, baseCommits[repo])
		}
	}

	missingRepoResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		missingRepoResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/workspace/file/content-at-ref?path=README.md&ref=HEAD",
			nil,
		),
	)
	if !strings.Contains(missingRepoResponse.Body.String(), "repo is required for multi-repo workspace") {
		t.Fatalf(
			"repo-less file content response = %d: %s",
			missingRepoResponse.Code,
			missingRepoResponse.Body.String(),
		)
	}

	for repo, baseBranch := range bases {
		contentResponse := httptest.NewRecorder()
		path := "/api/v1/workspace/file/content-at-ref?repo=" + repo +
			"&path=README.md&ref=" + baseBranch
		srv.Router().ServeHTTP(
			contentResponse,
			httptest.NewRequest(http.MethodGet, path, nil),
		)
		if contentResponse.Code != http.StatusOK {
			t.Fatalf(
				"file content at ref for %s status = %d: %s",
				repo,
				contentResponse.Code,
				contentResponse.Body.String(),
			)
		}
		var content struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(contentResponse.Body.Bytes(), &content); err != nil {
			t.Fatalf("decode file content at ref for %s: %v", repo, err)
		}
		if content.Content != "base\n" {
			t.Errorf("file content at ref for %s = %q, want %q", repo, content.Content, "base\n")
		}
	}
}

func TestNestedSubmoduleReviewEndpointsIncludeRootAndStableChildBase(t *testing.T) {
	parent, parentCleanup := setupAPITestRepo(t)
	t.Cleanup(parentCleanup)
	child, childCleanup := setupAPITestRepo(t)
	t.Cleanup(childCleanup)

	runGitAPI(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "vendor/lib")
	runGitAPI(t, parent, "add", ".")
	runGitAPI(t, parent, "commit", "-m", "add submodule")
	runGitAPI(t, parent, "push", "origin", "main")
	runGitAPI(t, parent, "checkout", "-b", "feature/review")

	childBase := strings.TrimSpace(runGitAPI(t, parent, "rev-parse", "origin/main:vendor/lib"))
	writeFileAPI(t, parent, "README.md", "root change\n")
	childDir := filepath.Join(parent, "vendor/lib")
	writeFileAPI(t, childDir, "child-change.txt", "child change\n")
	runGitAPI(t, childDir, "add", ".")
	runGitAPI(t, childDir, "commit", "-m", "child change")
	writeFileAPI(t, childDir, "child-uncommitted.txt", "uncommitted child change\n")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{
		WorkDir:      parent,
		BaseBranches: map[string]string{"": "main"},
	}
	mgr := process.NewManager(cfg, log)
	srv := NewServer(cfg, mgr, nil, nil, log)

	statusResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(statusResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/git/status/multi?fresh=true", nil,
	))
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("git status response = %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status MultiRepoGitStatusResult
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode git status: %v", err)
	}
	statusByRepo := make(map[string]GitStatusResult, len(status.Repos))
	for _, repo := range status.Repos {
		statusByRepo[repo.RepositoryName] = repo.Status
	}
	if _, ok := statusByRepo[""]; !ok {
		t.Fatalf("root status missing: %s", statusResponse.Body.String())
	}
	if childStatus, ok := statusByRepo["vendor/lib"]; !ok || !containsString(childStatus.Modified, "child-uncommitted.txt") && !containsString(childStatus.Untracked, "child-uncommitted.txt") {
		t.Fatalf("child status missing child-uncommitted.txt: %s", statusResponse.Body.String())
	}

	diffResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(diffResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/git/cumulative-diff?base="+childBase, nil,
	))
	if diffResponse.Code != http.StatusOK {
		t.Fatalf("cumulative diff status = %d: %s", diffResponse.Code, diffResponse.Body.String())
	}
	var diff process.CumulativeDiffResult
	if err := json.Unmarshal(diffResponse.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	childPayload, ok := diff.Files["vendor/lib\x00child-change.txt"]
	if !ok {
		t.Fatalf("child cumulative diff missing: %s", diffResponse.Body.String())
	}
	childFile, ok := childPayload.(map[string]interface{})
	if !ok || childFile["repository_name"] != "vendor/lib" || childFile["path"] != "child-change.txt" || childFile["base_ref"] != childBase {
		t.Fatalf("child cumulative payload = %#v, want scoped path and base", childPayload)
	}
	if _, ok := diff.Files["\x00README.md"]; !ok {
		t.Fatalf("root cumulative diff missing: %s", diffResponse.Body.String())
	}

	contentResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(contentResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/workspace/file/content-at-ref?path=README.md&ref=HEAD", nil,
	))
	if contentResponse.Code != http.StatusOK {
		t.Fatalf("root file content status = %d: %s", contentResponse.Code, contentResponse.Body.String())
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestMultiRepoReviewEndpointsCorrectStaleBases(t *testing.T) {
	taskRoot := t.TempDir()
	bases := map[string]string{
		"frontend": "feature/parent",
		"backend":  "feature/parent",
	}
	integrationBases := make(map[string]string, len(bases))
	for repo := range bases {
		repoDir := filepath.Join(taskRoot, repo)
		if err := os.Mkdir(repoDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		integrationBases[repo] = setupStaleComparisonRepoAt(t, repoDir)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: taskRoot, BaseBranches: bases}
	srv := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	logResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		logResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/git/log?limit=100", nil),
	)
	if logResponse.Code != http.StatusOK {
		t.Fatalf("git log status = %d: %s", logResponse.Code, logResponse.Body.String())
	}
	var commits process.GitLogResult
	if err := json.Unmarshal(logResponse.Body.Bytes(), &commits); err != nil {
		t.Fatalf("decode git log: %v", err)
	}
	commitsByRepo := make(map[string]int)
	for _, commit := range commits.Commits {
		commitsByRepo[commit.RepositoryName]++
		if commit.CommitMessage != "feat: child work" {
			t.Errorf("unexpected commit for %s: %s", commit.RepositoryName, commit.CommitMessage)
		}
	}
	for repo := range bases {
		if commitsByRepo[repo] != 1 {
			t.Fatalf("commits for %s = %d, want 1: %s", repo, commitsByRepo[repo], logResponse.Body.String())
		}
	}

	diffResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		diffResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/git/cumulative-diff?base="+integrationBases["frontend"],
			nil,
		),
	)
	if diffResponse.Code != http.StatusOK {
		t.Fatalf("cumulative diff status = %d: %s", diffResponse.Code, diffResponse.Body.String())
	}
	var diff process.CumulativeDiffResult
	if err := json.Unmarshal(diffResponse.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	for repo, integrationBase := range integrationBases {
		payload, ok := diff.Files[repo+"\x00child.txt"]
		if !ok {
			t.Fatalf("cumulative diff missing %s/child.txt: %s", repo, diffResponse.Body.String())
		}
		file, ok := payload.(map[string]interface{})
		if !ok {
			t.Fatalf("cumulative diff payload for %s has type %T", repo, payload)
		}
		if got := file["base_ref"]; got != integrationBase {
			t.Errorf("cumulative diff base_ref for %s = %v, want %s", repo, got, integrationBase)
		}
		if _, ok := diff.Files[repo+"\x00parent.txt"]; ok {
			t.Errorf("cumulative diff includes stale parent range for %s", repo)
		}
	}
}
