package retention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

const protectionUnknown = "unknown"

// SnapshotProtection is persisted with the snapshot so ordinary cleanup history
// is excluded from resource-protection reads.
func SnapshotProtection(payload string) string {
	var saved snapshot
	if err := json.Unmarshal([]byte(payload), &saved); err != nil {
		return protectionUnknown
	}
	return saved.protection()
}

func (s snapshot) protection() string {
	switch s.WorkspaceCleanup.Normalized() {
	case models.WorkspaceCleanupRetain:
		if s.Version > 1 || !s.InventoryCaptured {
			return protectionUnknown
		}
		return "retain"
	case models.WorkspaceCleanupDelete:
		if !s.AllowUnverifiedWorkspaceChanges {
			return ""
		}
		if s.CleanupVersion != 1 || !s.CleanupInventoryCaptured || !s.DiscardWorktreeChanges {
			return protectionUnknown
		}
		return "cleanup"
	default:
		return protectionUnknown
	}
}

// cleanupOwnerTask accepts only the newest outstanding cleanup's current claim.
// Explicit retention remains authoritative even for this worker.
func cleanupOwnerTask(ctx context.Context, db *sqlx.DB) (string, error) {
	claim, ok := recoveryclaim.TaskCleanupJobFromContext(ctx)
	if !ok {
		return "", nil
	}
	var id, state string
	var attempts int
	err := db.QueryRowxContext(ctx, db.Rebind(`SELECT id,state,attempts FROM task_resource_cleanup_jobs
 WHERE task_id=? AND workspace_protection='cleanup' AND state NOT IN ('succeeded','cancelled')
 ORDER BY created_at DESC,id DESC LIMIT 1`), claim.TaskID).Scan(&id, &state, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if id == claim.ID && state == string(models.TaskResourceCleanupStateRunning) && attempts == claim.Attempts {
		return claim.TaskID, nil
	}
	return "", nil
}
