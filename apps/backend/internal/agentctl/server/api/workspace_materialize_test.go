package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestMaterializeRepository_ClonesIntoWorkspaceDestination(t *testing.T) {
	origin := createMaterializeOrigin(t)
	workDir := t.TempDir()
	destination := filepath.Join(workDir, "second-repo")
	if _, err := materializeRepository(context.Background(), origin, destination, "main", ""); err != nil {
		t.Fatalf("materialize repository: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "second-repo", ".git")); err != nil {
		t.Fatalf("cloned repository missing: %v", err)
	}
}

func TestMaterializeRepository_ChecksOutBaseAsNamedTrackingBranch(t *testing.T) {
	origin := createMaterializeOrigin(t)
	workDir := t.TempDir()
	destination := filepath.Join(workDir, "second-repo")
	if _, err := materializeRepository(context.Background(), origin, destination, "main", ""); err != nil {
		t.Fatalf("materialize repository: %v", err)
	}
	if branch := strings.TrimSpace(materializeGitOutputForTest(t, destination, "branch", "--show-current")); branch != "main" {
		t.Fatalf("current branch = %q, want main", branch)
	}
	if upstream := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "--abbrev-ref", "@{upstream}")); upstream != "origin/main" {
		t.Fatalf("upstream = %q, want origin/main", upstream)
	}
}

func TestMaterializeRepository_CreatesNewCheckoutBranchFromBase(t *testing.T) {
	origin := createMaterializeOrigin(t)
	destination := filepath.Join(t.TempDir(), "second-repo")
	if _, err := materializeRepository(context.Background(), origin, destination, "main", "feature/new-work"); err != nil {
		t.Fatalf("materialize repository: %v", err)
	}
	if branch := strings.TrimSpace(materializeGitOutputForTest(t, destination, "branch", "--show-current")); branch != "feature/new-work" {
		t.Fatalf("current branch = %q, want feature/new-work", branch)
	}
	if got, want := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "HEAD")), strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "origin/main")); got != want {
		t.Fatalf("HEAD = %q, want base commit %q", got, want)
	}
}

func TestMaterializeRepository_ChecksOutExistingRemoteBranchWithTracking(t *testing.T) {
	origin := createMaterializeOriginWithBranch(t, "feature/existing")
	destination := filepath.Join(t.TempDir(), "second-repo")
	if _, err := materializeRepository(context.Background(), origin, destination, "main", "feature/existing"); err != nil {
		t.Fatalf("materialize repository: %v", err)
	}
	if upstream := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "--abbrev-ref", "@{upstream}")); upstream != "origin/feature/existing" {
		t.Fatalf("upstream = %q, want origin/feature/existing", upstream)
	}
}

func TestWorkspaceMaterializeRepository_RejectsEscapingDestination(t *testing.T) {
	origin := createMaterializeOrigin(t)
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	s := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	body, err := json.Marshal(MaterializeRepositoryRequest{
		RepositoryURL: origin,
		Destination:   "../escape",
		BaseBranch:    "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/materialize-repository", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body: %s)", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(workDir), "escape")); !os.IsNotExist(err) {
		t.Fatalf("escape destination was created: %v", err)
	}
}

func TestWorkspaceMaterializeRepository_RejectsCredentialedLocator(t *testing.T) {
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	s := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	body, err := json.Marshal(MaterializeRepositoryRequest{
		RepositoryURL: "https://secret-token@github.com/kdlbs/kandev.git",
		Destination:   "kandev",
		BaseBranch:    "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/materialize-repository", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body: %s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret-token") {
		t.Fatalf("credential leaked in response: %s", w.Body.String())
	}
}

func TestWorkspaceMaterializeRepository_RejectsLocalLocators(t *testing.T) {
	s := newMaterializeTestServer(t, t.TempDir())
	for _, locator := range []string{"/srv/repos/private.git", "file:///srv/repos/private.git"} {
		body, err := json.Marshal(MaterializeRepositoryRequest{RepositoryURL: locator, Destination: "repo", BaseBranch: "main"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/materialize-repository", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", locator, w.Code)
		}
	}
}

func TestMaterializeRepository_CancelledCloneLeavesNoDestination(t *testing.T) {
	origin := createMaterializeOrigin(t)
	workDir := t.TempDir()
	destination := filepath.Join(workDir, "second-repo")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := materializeRepository(ctx, origin, destination, "main", "")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial destination remains after cancellation: %v", err)
	}
}

func TestMaterializeRepository_RejectsExistingCheckoutAtDifferentCommit(t *testing.T) {
	origin := createMaterializeOrigin(t)
	workDir := t.TempDir()
	destination := filepath.Join(workDir, "second-repo")
	if reused, err := materializeRepository(context.Background(), origin, destination, "main", ""); err != nil || reused {
		t.Fatalf("initial materialization = reused:%t, err:%v", reused, err)
	}
	runMaterializeTestGit(t, destination, "checkout", "-b", "different-head")
	runMaterializeTestGit(t, destination, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--allow-empty", "-m", "different")

	reused, err := materializeRepository(context.Background(), origin, destination, "main", "")

	if reused || !errors.Is(err, errMaterializeCollision) {
		t.Fatalf("mismatched checkout = reused:%t, err:%v; want collision", reused, err)
	}
}

func TestMaterializeRepository_RejectsExistingCheckoutOnWrongNamedBranch(t *testing.T) {
	origin := createMaterializeOrigin(t)
	destination := filepath.Join(t.TempDir(), "second-repo")
	if _, err := materializeRepository(context.Background(), origin, destination, "main", "feature/work"); err != nil {
		t.Fatal(err)
	}
	runMaterializeTestGit(t, destination, "checkout", "main")
	reused, err := materializeRepository(context.Background(), origin, destination, "main", "feature/work")
	if reused || !errors.Is(err, errMaterializeCollision) {
		t.Fatalf("wrong branch reuse = reused:%t err:%v; want collision", reused, err)
	}
}

func TestWorkspaceRemoveMaterializedRepository_RemovesOwnedCheckout(t *testing.T) {
	origin := createMaterializeOrigin(t)
	workDir := t.TempDir()
	destination := filepath.Join(workDir, "second-repo")
	if _, err := materializeRepository(context.Background(), origin, destination, "main", ""); err != nil {
		t.Fatal(err)
	}
	s := newMaterializeTestServer(t, workDir)
	w := removeMaterializedRepositoryRequest(t, s, RemoveMaterializedRepositoryRequest{RepositoryURL: origin, Destination: "second-repo"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("owned checkout remains: %v", err)
	}
}

func TestWorkspaceRemoveMaterializedRepository_NonexistentIsIdempotent(t *testing.T) {
	s := newMaterializeTestServer(t, t.TempDir())
	w := removeMaterializedRepositoryRequest(t, s, RemoveMaterializedRepositoryRequest{RepositoryURL: "https://github.com/kdlbs/kandev.git", Destination: "missing"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestWorkspaceRemoveMaterializedRepository_RejectsUnownedDestination(t *testing.T) {
	origin := createMaterializeOrigin(t)
	otherOrigin := createMaterializeOrigin(t)
	workDir := t.TempDir()
	s := newMaterializeTestServer(t, workDir)
	cases := []struct {
		name        string
		destination string
		prepare     func(t *testing.T, path string)
		requestURL  string
	}{
		{name: "non git", destination: "plain", prepare: func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}, requestURL: origin},
		{name: "wrong origin", destination: "other", prepare: func(t *testing.T, path string) {
			if _, err := materializeRepository(context.Background(), otherOrigin, path, "main", ""); err != nil {
				t.Fatal(err)
			}
		}, requestURL: origin},
		{name: "symlink", destination: "linked", prepare: func(t *testing.T, path string) {
			target := filepath.Join(workDir, "target")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}, requestURL: origin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(workDir, tc.destination)
			tc.prepare(t, path)
			w := removeMaterializedRepositoryRequest(t, s, RemoveMaterializedRepositoryRequest{RepositoryURL: tc.requestURL, Destination: tc.destination})
			if w.Code != http.StatusConflict {
				t.Fatalf("expected 409, got %d (body: %s)", w.Code, w.Body.String())
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("unowned destination was deleted: %v", err)
			}
		})
	}
}

func TestWorkspaceRemoveMaterializedRepository_RejectsTraversal(t *testing.T) {
	workDir := t.TempDir()
	s := newMaterializeTestServer(t, workDir)
	w := removeMaterializedRepositoryRequest(t, s, RemoveMaterializedRepositoryRequest{RepositoryURL: "https://github.com/kdlbs/kandev.git", Destination: "../escape"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func newMaterializeTestServer(t *testing.T, workDir string) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
}

func removeMaterializedRepositoryRequest(t *testing.T, s *Server, removal RemoveMaterializedRepositoryRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(removal)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/materialize-repository/remove", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func createMaterializeOrigin(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	runMaterializeTestGit(t, root, "init", "--bare", "--initial-branch=main", origin)
	runMaterializeTestGit(t, root, "init", "--initial-branch=main", seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	runMaterializeTestGit(t, seed, "add", "README.md")
	runMaterializeTestGit(t, seed, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "initial")
	runMaterializeTestGit(t, seed, "remote", "add", "origin", origin)
	runMaterializeTestGit(t, seed, "push", "origin", "main")
	return origin
}

func createMaterializeOriginWithBranch(t *testing.T, branch string) string {
	t.Helper()
	origin := createMaterializeOrigin(t)
	clone := filepath.Join(t.TempDir(), "clone")
	runMaterializeTestGit(t, filepath.Dir(clone), "clone", origin, clone)
	runMaterializeTestGit(t, clone, "checkout", "-b", branch)
	runMaterializeTestGit(t, clone, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--allow-empty", "-m", "branch")
	runMaterializeTestGit(t, clone, "push", "-u", "origin", branch)
	return origin
}

func runMaterializeTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func materializeGitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
