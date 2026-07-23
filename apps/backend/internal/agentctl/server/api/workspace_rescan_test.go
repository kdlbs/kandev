package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// TestHandleRescanWorkspace_AcceptsEmptyBody covers the materializer's
// no-work_dir form: a rescan that doesn't touch cfg.WorkDir and just
// reconciles trackers against current on-disk state.
func TestHandleRescanWorkspace_AcceptsEmptyBody(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/rescan", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestHandleRescanWorkspace_AcceptsWorkDir covers the materializer's
// transition form: a rescan that promotes WorkDir to the task root before
// re-discovering repo subdirs.
func TestHandleRescanWorkspace_RejectsInvalidWorkDir(t *testing.T) {
	s := newTestServer(t)

	body, _ := json.Marshal(RescanWorkspaceRequest{WorkDir: "/tmp/task-root"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/rescan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// A rejected rescan must leave the established linked-source policy intact.
// In particular, the proposed roots belong to the failed transition and must
// not become usable by the current workspace tracker.
func TestHandleRescanWorkspace_FailedRescanRetainsExistingSourceRoots(t *testing.T) {
	workspace := t.TempDir()
	allowedSource := t.TempDir()
	proposedSource := t.TempDir()
	if err := os.Symlink(allowedSource, filepath.Join(workspace, "allowed")); err != nil {
		t.Skip("symlinks not supported")
	}
	if err := os.Symlink(proposedSource, filepath.Join(workspace, "proposed")); err != nil {
		t.Skip("symlinks not supported")
	}

	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: workspace, WorkspaceSourceRoots: []string{allowedSource}}
	procMgr := process.NewManager(cfg, log)
	s := NewServer(cfg, procMgr, nil, nil, log)

	body, err := json.Marshal(RescanWorkspaceRequest{
		WorkDir:              filepath.Join(workspace, "missing"),
		WorkspaceSourceRoots: []string{proposedSource},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/rescan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d (body: %s)", w.Code, w.Body.String())
	}
	tracker := procMgr.GetWorkspaceTracker()
	if err := tracker.CreateFile(filepath.Join("allowed", "still-allowed.txt")); err != nil {
		t.Fatalf("existing source was lost after failed rescan: %v", err)
	}
	if err := tracker.CreateFile(filepath.Join("proposed", "must-stay-blocked.txt")); err == nil {
		t.Fatal("failed rescan installed its proposed source roots")
	}
}

// A rollback must prune trackers for checkouts that have already been removed
// from disk. The ordinary empty-workdir rescan deliberately only appends, so
// rollback uses its own exact reconciliation endpoint.
func TestHandleReconcileWorkspace_PrunesRemovedRepositoryTracker(t *testing.T) {
	workspace := t.TempDir()
	source := t.TempDir()
	initWorkspaceGitRepo(t, filepath.Join(workspace, "original"))
	stale := filepath.Join(workspace, "rolled-back")
	initWorkspaceGitRepo(t, stale)

	log := newTestLogger()
	if err := os.Symlink(source, filepath.Join(workspace, "linked")); err != nil {
		t.Skip("symlinks not supported")
	}
	cfg := &config.InstanceConfig{WorkDir: workspace, WorkspaceSourceRoots: []string{source}}
	procMgr := process.NewManager(cfg, log)
	defer func() { _ = procMgr.Stop(context.Background()) }()
	s := NewServer(cfg, procMgr, nil, nil, log)

	if err := os.RemoveAll(stale); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/reconcile", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if got := procMgr.RepoSubpaths(); len(got) != 1 || got[0] != "original" {
		t.Fatalf("RepoSubpaths = %v, want [original] after rollback", got)
	}
	if err := procMgr.GetWorkspaceTracker().CreateFile(filepath.Join("linked", "preserved.txt")); err != nil {
		t.Fatalf("reconcile dropped the existing source allowlist: %v", err)
	}
}

func initWorkspaceGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		command := exec.Command("git", args...)
		command.Dir = path
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v: %s", args, path, err, output)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	run("commit", "--allow-empty", "-m", "initial")
}
