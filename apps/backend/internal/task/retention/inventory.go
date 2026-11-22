// Package retention projects durable keep-files decisions for cleanup consumers.
package retention

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// Inventory includes completed and cancelled jobs: job state is not permission
// to discard a user's retained files.
type Inventory struct {
	TaskIDs     map[string]struct{}
	WorktreeIDs map[string]struct{}
	Paths       []string
	ResourceIDs map[string]struct{}
	invalidPath bool
}

type snapshot struct {
	PriorCleanupInventory           []json.RawMessage           `json:"prior_cleanup_inventory"`
	AllowUnverifiedWorkspaceChanges bool                        `json:"allow_unverified_workspace_changes"`
	DiscardWorktreeChanges          bool                        `json:"discard_worktree_changes"`
	CleanupInventoryCaptured        bool                        `json:"cleanup_inventory_captured"`
	CleanupVersion                  int                         `json:"cleanup_version"`
	Version                         int                         `json:"retention_version"`
	InventoryCaptured               bool                        `json:"retention_inventory_captured"`
	Executors                       []*models.ExecutorRunning   `json:"retained_executors"`
	WorkspaceCleanup                models.WorkspaceCleanupMode `json:"workspace_cleanup"`
	TaskEnvironment                 *models.TaskEnvironment     `json:"task_environment"`
	Worktrees                       []struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	} `json:"worktrees"`
}

func Load(ctx context.Context, db *sqlx.DB) (Inventory, error) {
	return LoadForTask(ctx, db, "")
}

// LoadForTask scopes unknown inventory to a trusted task identity. Empty scope
// requires complete global inventory for path-only and orphan sweeps.
func LoadForTask(ctx context.Context, db *sqlx.DB, scope string) (Inventory, error) {
	inventory := Inventory{TaskIDs: map[string]struct{}{}, WorktreeIDs: map[string]struct{}{}, ResourceIDs: map[string]struct{}{}}
	owner, err := cleanupOwnerTask(ctx, db)
	if err != nil {
		return Inventory{}, err
	}
	rows, err := db.QueryxContext(ctx, `SELECT task_id,state,resource_snapshot FROM task_resource_cleanup_jobs
 WHERE task_id IN (SELECT task_id FROM task_resource_cleanup_jobs
 WHERE workspace_protection IN ('retain','unknown','unclassified')
 OR (workspace_protection='cleanup' AND state NOT IN ('succeeded','cancelled')))`)
	if err != nil {
		return Inventory{}, err
	}
	defer func() { _ = rows.Close() }()
	snapshots := make(map[string][]snapshot)
	unknown := make(map[string]bool)
	for rows.Next() {
		var taskID, state, payload string
		if err := rows.Scan(&taskID, &state, &payload); err != nil {
			return Inventory{}, err
		}
		var saved snapshot
		if err := json.Unmarshal([]byte(payload), &saved); err != nil {
			unknown[taskID] = true
			continue
		}
		snapshots[taskID] = append(snapshots[taskID], saved)
		switch saved.protection() {
		case protectionUnknown:
			unknown[taskID] = true
		case "retain":
			inventory.TaskIDs[taskID] = struct{}{}
		case "cleanup":
			if state != "succeeded" && state != "cancelled" && taskID != owner {
				inventory.TaskIDs[taskID] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Inventory{}, err
	}
	return inventory.finishProjection(snapshots, unknown, scope)
}

func (inventory Inventory) finishProjection(snapshots map[string][]snapshot, unknown map[string]bool, scope string) (Inventory, error) {
	for taskID := range inventory.TaskIDs {
		inventory.invalidPath = false
		for _, saved := range snapshots[taskID] {
			inventory.addSnapshot(saved)
		}
		if inventory.invalidPath {
			unknown[taskID] = true
		}
	}
	for taskID := range unknown {
		inventory.TaskIDs[taskID] = struct{}{}
		if scope == "" || taskID == "" || scope == taskID {
			return Inventory{}, fmt.Errorf("workspace protection inventory is incomplete")
		}
	}
	return inventory, nil
}

func (i *Inventory) addPath(path string) {
	if path == "" {
		return
	}
	if !filepath.IsAbs(path) {
		i.invalidPath = true
		return
	}
	i.Paths = append(i.Paths, filepath.Clean(path))
}

func (i Inventory) Protects(taskID, worktreeID, path string) bool {
	if _, ok := i.TaskIDs[taskID]; taskID != "" && ok {
		return true
	}
	if _, ok := i.WorktreeIDs[worktreeID]; worktreeID != "" && ok {
		return true
	}
	if path != "" {
		for _, retained := range i.Paths {
			if filepath.Clean(path) == retained {
				return true
			}
		}
	}
	return false
}

func (inventory *Inventory) addSnapshot(saved snapshot) {
	for _, payload := range saved.PriorCleanupInventory {
		var prior snapshot
		if err := json.Unmarshal(payload, &prior); err != nil {
			inventory.invalidPath = true
			continue
		}
		inventory.addSnapshot(prior)
	}

	for _, runtime := range saved.Executors {
		if runtime == nil {
			continue
		}
		addRetentionID(inventory.ResourceIDs, runtime.AgentExecutionID)
		addRetentionID(inventory.ResourceIDs, runtime.ContainerID)
		inventory.addPath(runtime.WorktreePath)
		addRetentionID(inventory.WorktreeIDs, runtime.WorktreeID)
	}
	for _, wt := range saved.Worktrees {
		addRetentionID(inventory.WorktreeIDs, wt.ID)
		inventory.addPath(wt.Path)
	}
	if env := saved.TaskEnvironment; env != nil {
		addRetentionID(inventory.ResourceIDs, env.ID)
		addRetentionID(inventory.ResourceIDs, env.ContainerID)
		addRetentionID(inventory.ResourceIDs, env.SandboxID)
		inventory.addPath(env.WorkspacePath)
		for _, repo := range env.Repos {
			if repo != nil {
				inventory.addPath(repo.WorktreePath)
				addRetentionID(inventory.WorktreeIDs, repo.WorktreeID)
			}
		}
	}
}

func addRetentionID(ids map[string]struct{}, id string) {
	if id != "" {
		ids[id] = struct{}{}
	}
}
