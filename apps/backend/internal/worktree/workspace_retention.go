package worktree

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/task/retention"
)

type workspaceRetentionStore interface {
	IsWorkspaceRetained(context.Context, string, string, string) (bool, error)
}

func (s *SQLiteStore) IsWorkspaceRetained(ctx context.Context, taskID, worktreeID, path string) (bool, error) {
	inventory, err := retention.LoadForTask(ctx, s.ro, taskID)
	if err != nil {
		return false, err
	}
	return inventory.Protects(taskID, worktreeID, path), nil
}

func (m *Manager) checkWorkspaceRetention(ctx context.Context, taskID, worktreeID, path string) error {
	if store, ok := m.store.(workspaceRetentionStore); ok {
		retained, err := store.IsWorkspaceRetained(ctx, taskID, worktreeID, path)
		if err != nil {
			return fmt.Errorf("check workspace retention: %w", err)
		}
		if retained {
			return fmt.Errorf("workspace files are retained")
		}
	}
	return nil
}
