package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/task/retention"
)

// HasRetainedTaskWorkspace remains valid after live task rows are deleted.
func (r *Repository) HasRetainedTaskWorkspace(ctx context.Context, taskID string) (bool, error) {
	inventory, err := retention.LoadForTask(ctx, r.ro, taskID)
	if err != nil {
		return false, err
	}
	return inventory.Protects(taskID, "", ""), nil
}

func (r *Repository) HasRetainedWorkspaceResource(ctx context.Context, taskID string, resourceIDs ...string) (bool, error) {
	inventory, err := retention.LoadForTask(ctx, r.ro, taskID)
	if err != nil {
		return false, err
	}
	if inventory.Protects(taskID, "", "") {
		return true, nil
	}
	known := taskID != ""
	for _, id := range resourceIDs {
		if id == "" {
			continue
		}
		known = true
		if _, ok := inventory.ResourceIDs[id]; ok {
			return true, nil
		}
	}
	// Without an identifiable scope, cleanup cannot exclude retained resources.
	return !known && len(inventory.TaskIDs) > 0, nil
}
