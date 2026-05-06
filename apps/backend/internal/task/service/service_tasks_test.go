package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestPerformTaskCleanup_QuickChatDir verifies that performTaskCleanup removes
// quick-chat workspace directories for both ephemeral and non-ephemeral tasks,
// and that tasks with no directory on disk produce no error.
func TestPerformTaskCleanup_QuickChatDir(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	quickChatDir := t.TempDir()
	svc.SetQuickChatDir(quickChatDir)

	makeSession := func(id string) *models.TaskSession {
		return &models.TaskSession{ID: id}
	}

	t.Run("ephemeral task with dir — dir removed", func(t *testing.T) {
		sessID := "sess-ephemeral"
		dir := filepath.Join(quickChatDir, sessID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		errs := svc.performTaskCleanup(ctx, "task-eph", []*models.TaskSession{makeSession(sessID)}, nil, taskEnvironmentCleanup{}, true)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("expected dir %s to be removed", dir)
		}
	})

	t.Run("non-ephemeral task with dir — dir removed", func(t *testing.T) {
		sessID := "sess-nonephemeral"
		dir := filepath.Join(quickChatDir, sessID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		errs := svc.performTaskCleanup(ctx, "task-noeph", []*models.TaskSession{makeSession(sessID)}, nil, taskEnvironmentCleanup{}, false)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("expected dir %s to be removed", dir)
		}
	})

	t.Run("task with no dir on disk — no error", func(t *testing.T) {
		sessID := "sess-nodir"
		// Do not create the directory.
		errs := svc.performTaskCleanup(ctx, "task-nodir", []*models.TaskSession{makeSession(sessID)}, nil, taskEnvironmentCleanup{}, true)
		if len(errs) != 0 {
			t.Fatalf("expected no errors when dir absent, got: %v", errs)
		}
	})
}
