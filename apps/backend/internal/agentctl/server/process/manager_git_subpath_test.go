package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestManager(t *testing.T, workDir string) *Manager {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return NewManager(&config.InstanceConfig{WorkDir: workDir}, log)
}

func TestManager_GitOperatorFor_EmptySubpathReturnsRoot(t *testing.T) {
	tmp := t.TempDir()
	mgr := newTestManager(t, tmp)

	op, err := mgr.GitOperatorFor("")
	if err != nil {
		t.Fatalf("empty subpath: %v", err)
	}
	if op == nil || op.workDir != tmp {
		t.Errorf("expected root operator, got %+v", op)
	}
}

func TestManager_GitOperatorFor_ValidSubpathReturnsScopedOperator(t *testing.T) {
	tmp := t.TempDir()
	subDir := filepath.Join(tmp, "frontend")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mgr := newTestManager(t, tmp)

	op, err := mgr.GitOperatorFor("frontend")
	if err != nil {
		t.Fatalf("subpath: %v", err)
	}
	if op.workDir != subDir {
		t.Errorf("expected workDir %q, got %q", subDir, op.workDir)
	}

	// Cached on second call.
	op2, _ := mgr.GitOperatorFor("frontend")
	if op2 != op {
		t.Error("expected cached operator on second call")
	}
}

func TestManager_GitOperatorFor_RejectsPathTraversal(t *testing.T) {
	tmp := t.TempDir()
	mgr := newTestManager(t, tmp)

	cases := []string{"..", "../escape", "frontend/..", "/abs/path"}
	for _, c := range cases {
		if _, err := mgr.GitOperatorFor(c); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestManager_GitOperatorFor_RejectsMissingDir(t *testing.T) {
	tmp := t.TempDir()
	mgr := newTestManager(t, tmp)

	if _, err := mgr.GitOperatorFor("does-not-exist"); err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestManager_GetWorkspaceTrackerFor_EmptySubpathReturnsRoot(t *testing.T) {
	tmp := t.TempDir()
	mgr := newTestManager(t, tmp)

	wt, err := mgr.GetWorkspaceTrackerFor("")
	if err != nil {
		t.Fatalf("empty subpath: %v", err)
	}
	if wt != mgr.GetWorkspaceTracker() {
		t.Errorf("expected root tracker, got distinct instance")
	}
}

func TestManager_GetWorkspaceTrackerFor_ValidSubpathCachesPerSubpath(t *testing.T) {
	tmp := t.TempDir()
	subDir := filepath.Join(tmp, "frontend")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mgr := newTestManager(t, tmp)

	wt1, err := mgr.GetWorkspaceTrackerFor("frontend")
	if err != nil {
		t.Fatalf("subpath: %v", err)
	}
	if wt1 == nil || wt1 == mgr.GetWorkspaceTracker() {
		t.Errorf("expected dedicated subpath tracker, got root")
	}
	wt2, _ := mgr.GetWorkspaceTrackerFor("frontend")
	if wt1 != wt2 {
		t.Error("expected per-subpath tracker to be cached")
	}
}

func TestManager_GetWorkspaceTrackerFor_RejectsTraversal(t *testing.T) {
	tmp := t.TempDir()
	mgr := newTestManager(t, tmp)
	for _, c := range []string{"..", "../escape", "/abs/path"} {
		if _, err := mgr.GetWorkspaceTrackerFor(c); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}
