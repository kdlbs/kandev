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
	Version           int                         `json:"retention_version"`
	InventoryCaptured bool                        `json:"retention_inventory_captured"`
	Executors         []*models.ExecutorRunning   `json:"retained_executors"`
	WorkspaceCleanup  models.WorkspaceCleanupMode `json:"workspace_cleanup"`
	TaskEnvironment   *models.TaskEnvironment     `json:"task_environment"`
	Worktrees         []struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	} `json:"worktrees"`
}

func Load(ctx context.Context, db *sqlx.DB) (Inventory, error) {
	inventory := Inventory{TaskIDs: map[string]struct{}{}, WorktreeIDs: map[string]struct{}{}, ResourceIDs: map[string]struct{}{}}
	rows, err := db.QueryxContext(ctx, `SELECT task_id, resource_snapshot FROM task_resource_cleanup_jobs`)
	if err != nil {
		return Inventory{}, err
	}
	defer func() { _ = rows.Close() }()
	snapshots := make(map[string][]snapshot)
	for rows.Next() {
		var taskID, payload string
		if err := rows.Scan(&taskID, &payload); err != nil {
			return Inventory{}, err
		}
		var saved snapshot
		if err := json.Unmarshal([]byte(payload), &saved); err != nil {
			return Inventory{}, fmt.Errorf("read workspace retention: %w", err)
		}
		snapshots[taskID] = append(snapshots[taskID], saved)
		switch saved.WorkspaceCleanup.Normalized() {
		case models.WorkspaceCleanupDelete:
			continue
		case models.WorkspaceCleanupRetain:
		default:
			return Inventory{}, fmt.Errorf("unsupported workspace cleanup mode")
		}
		if saved.Version > 1 {
			return Inventory{}, fmt.Errorf("unsupported workspace retention version")
		}
		if !saved.InventoryCaptured {
			return Inventory{}, fmt.Errorf("retained workspace inventory is incomplete")
		}
		inventory.TaskIDs[taskID] = struct{}{}

	}
	if err := rows.Err(); err != nil {
		return Inventory{}, err
	}
	for taskID := range inventory.TaskIDs {
		for _, saved := range snapshots[taskID] {
			inventory.addSnapshot(saved)
		}
	}
	if inventory.invalidPath {
		return Inventory{}, fmt.Errorf("retained workspace path is not absolute")
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
	for _, runtime := range saved.Executors {
		if runtime == nil {
			continue
		}
		inventory.ResourceIDs[runtime.AgentExecutionID] = struct{}{}
		inventory.ResourceIDs[runtime.ContainerID] = struct{}{}
		inventory.addPath(runtime.WorktreePath)
		inventory.WorktreeIDs[runtime.WorktreeID] = struct{}{}
	}
	for _, wt := range saved.Worktrees {
		inventory.WorktreeIDs[wt.ID] = struct{}{}
		inventory.addPath(wt.Path)
	}
	if env := saved.TaskEnvironment; env != nil {
		inventory.ResourceIDs[env.ID] = struct{}{}
		inventory.ResourceIDs[env.ContainerID] = struct{}{}
		inventory.ResourceIDs[env.SandboxID] = struct{}{}
		inventory.addPath(env.WorkspacePath)
		for _, repo := range env.Repos {
			if repo != nil {
				inventory.addPath(repo.WorktreePath)
				inventory.WorktreeIDs[repo.WorktreeID] = struct{}{}
			}
		}
	}
}
