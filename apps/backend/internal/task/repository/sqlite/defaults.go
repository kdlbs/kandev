package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	commonsqlite "github.com/kandev/kandev/internal/common/sqlite"
	"github.com/kandev/kandev/internal/task/models"
)

// ensureDefaultWorkspace creates a default workspace if none exists
func (r *Repository) ensureDefaultWorkspace() error {
	ctx := context.Background()

	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM workspaces").Scan(&count); err != nil {
		return err
	}

	if count == 0 {
		var boardCount int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM boards").Scan(&boardCount); err != nil {
			return err
		}
		var taskCount int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM tasks").Scan(&taskCount); err != nil {
			return err
		}
		defaultID := uuid.New().String()
		now := time.Now().UTC()
		workspaceName := "Default Workspace"
		workspaceDescription := "Default workspace"
		if boardCount > 0 || taskCount > 0 {
			workspaceName = "Migrated Workspace"
			workspaceDescription = ""
		}
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO workspaces (
				id,
				name,
				description,
				owner_id,
				default_executor_id,
				default_environment_id,
				default_agent_profile_id,
				created_at,
				updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, defaultID, workspaceName, workspaceDescription, "", nil, nil, nil, now, now); err != nil {
			return err
		}

		if boardCount == 0 && taskCount == 0 {
			boardID := uuid.New().String()
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO boards (id, workspace_id, name, description, workflow_template_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, boardID, defaultID, "Development", "Default development board", "simple", now, now); err != nil {
				return err
			}
			// Note: Workflow steps will be created by the workflow repository after it initializes
		}
	}

	var defaultWorkspaceID string
	if err := r.db.QueryRowContext(ctx, "SELECT id FROM workspaces ORDER BY created_at LIMIT 1").Scan(&defaultWorkspaceID); err != nil {
		return err
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE boards SET workspace_id = ? WHERE workspace_id = '' OR workspace_id IS NULL
	`, defaultWorkspaceID); err != nil {
		return err
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET workspace_id = (
			SELECT workspace_id FROM boards WHERE boards.id = tasks.board_id
		)
		WHERE workspace_id = '' OR workspace_id IS NULL
	`); err != nil {
		return err
	}

	return nil
}

// ensureDefaultExecutorsAndEnvironments creates default executors and environments if none exist
func (r *Repository) ensureDefaultExecutorsAndEnvironments() error {
	ctx := context.Background()

	var executorCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM executors").Scan(&executorCount); err != nil {
		return err
	}

	if executorCount == 0 {
		now := time.Now().UTC()
		executors := []struct {
			id        string
			name      string
			execType  models.ExecutorType
			status    models.ExecutorStatus
			isSystem  bool
			resumable bool
			config    map[string]string
		}{
			{
				id:        models.ExecutorIDLocalPC,
				name:      "Local PC",
				execType:  models.ExecutorTypeLocalPC,
				status:    models.ExecutorStatusActive,
				isSystem:  true,
				resumable: true,
				config:    map[string]string{},
			},
			{
				id:        models.ExecutorIDLocalDocker,
				name:      "Local Docker",
				execType:  models.ExecutorTypeLocalDocker,
				status:    models.ExecutorStatusActive,
				isSystem:  false,
				resumable: true,
				config: map[string]string{
					"docker_host": "unix:///var/run/docker.sock",
				},
			},
		}

		for _, executor := range executors {
			configJSON, err := json.Marshal(executor.config)
			if err != nil {
				return fmt.Errorf("failed to serialize executor config: %w", err)
			}
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO executors (id, name, type, status, is_system, resumable, config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, executor.id, executor.name, executor.execType, executor.status, commonsqlite.BoolToInt(executor.isSystem), commonsqlite.BoolToInt(executor.resumable), string(configJSON), now, now); err != nil {
				return err
			}
		}
	} else {
		if _, err := r.db.ExecContext(ctx, `
			UPDATE executors SET is_system = 1 WHERE id = ?
		`, models.ExecutorIDLocalPC); err != nil {
			return err
		}
	}

	var envCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM environments").Scan(&envCount); err != nil {
		return err
	}
	if envCount == 0 {
		now := time.Now().UTC()
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, models.EnvironmentIDLocal, "Local", models.EnvironmentKindLocalPC, commonsqlite.BoolToInt(true), "~/kandev", "", "", "{}", now, now); err != nil {
			return err
		}
	} else {
		var localCount int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM environments WHERE id = ?", models.EnvironmentIDLocal).Scan(&localCount); err != nil {
			return err
		}
		if localCount == 0 {
			now := time.Now().UTC()
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, models.EnvironmentIDLocal, "Local", models.EnvironmentKindLocalPC, commonsqlite.BoolToInt(true), "~/kandev", "", "", "{}", now, now); err != nil {
				return err
			}
		}
		if _, err := r.db.ExecContext(ctx, `
			UPDATE environments
			SET is_system = 1,
				image_tag = '',
				dockerfile = '',
				build_config = '{}'
			WHERE id = ?
		`, models.EnvironmentIDLocal); err != nil {
			return err
		}
		if _, err := r.db.ExecContext(ctx, `
			UPDATE environments
			SET worktree_root = ?
			WHERE id = ? AND (worktree_root IS NULL OR worktree_root = '')
		`, "~/kandev", models.EnvironmentIDLocal); err != nil {
			return err
		}
	}

	return nil
}

