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
