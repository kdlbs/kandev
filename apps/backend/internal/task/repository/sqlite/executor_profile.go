package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const jsonNull = "null"

// Executor profile operations

func (r *Repository) CreateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error {
	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	configJSON, err := json.Marshal(profile.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize profile config: %w", err)
	}

	envVarsJSON, err := json.Marshal(profile.EnvVars)
	if err != nil {
		return fmt.Errorf("failed to serialize profile env_vars: %w", err)
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockExecutorOwnershipTx(ctx, tx, profile.ExecutorID); err != nil {
		return err
	}
	if err := r.checkProfileParentOwnerTx(ctx, tx, profile.ExecutorID, profile.OwnerUserID, nil); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO executor_profiles (id, executor_id, name, mcp_policy, config, prepare_script, cleanup_script, env_vars, created_at, updated_at, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
	`), profile.ID, profile.ExecutorID, profile.Name, profile.McpPolicy, string(configJSON), profile.PrepareScript, profile.CleanupScript, string(envVarsJSON), profile.CreatedAt, profile.UpdatedAt, profile.OwnerUserID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	profile := &models.ExecutorProfile{}
	var configJSON string
	var envVarsJSON sql.NullString

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, executor_id, name, mcp_policy, config, prepare_script, cleanup_script, env_vars, created_at, updated_at, COALESCE(owner_user_id, '')
		FROM executor_profiles WHERE id = ?
	`), id).Scan(
		&profile.ID, &profile.ExecutorID, &profile.Name, &profile.McpPolicy,
		&configJSON, &profile.PrepareScript, &profile.CleanupScript, &envVarsJSON, &profile.CreatedAt, &profile.UpdatedAt, &profile.OwnerUserID,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", repoerrors.ErrExecutorProfileNotFound, id)
	}
	if err != nil {
		return nil, err
	}

	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &profile.Config); err != nil {
			return nil, fmt.Errorf("failed to deserialize profile config: %w", err)
		}
	}
	if envVarsJSON.Valid && envVarsJSON.String != "" && envVarsJSON.String != jsonNull {
		if err := json.Unmarshal([]byte(envVarsJSON.String), &profile.EnvVars); err != nil {
			return nil, fmt.Errorf("failed to deserialize profile env_vars: %w", err)
		}
	}
	return profile, nil
}

func (r *Repository) UpdateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error {
	return r.updateExecutorProfile(ctx, profile, models.ExecutorProfileScriptIntent{PrepareScript: &profile.PrepareScript, CleanupScript: &profile.CleanupScript}, nil)
}

func (r *Repository) UpdateExecutorProfileIfUnmodified(ctx context.Context, profile *models.ExecutorProfile, expectedUpdatedAt time.Time) error {
	return r.updateExecutorProfile(ctx, profile, models.ExecutorProfileScriptIntent{PrepareScript: &profile.PrepareScript, CleanupScript: &profile.CleanupScript}, &expectedUpdatedAt)
}

func (r *Repository) DeleteExecutorProfile(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM executor_profiles WHERE id = ?`), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor profile not found: %s", id)
	}
	return nil
}

func (r *Repository) ListExecutorProfiles(ctx context.Context, executorID string) ([]*models.ExecutorProfile, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, executor_id, name, mcp_policy, config, prepare_script, cleanup_script, env_vars, created_at, updated_at, COALESCE(owner_user_id, '')
		FROM executor_profiles WHERE executor_id = ? ORDER BY name ASC
	`), executorID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanExecutorProfiles(rows)
}

func (r *Repository) ListAllExecutorProfiles(ctx context.Context) ([]*models.ExecutorProfile, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, executor_id, name, mcp_policy, config, prepare_script, cleanup_script, env_vars, created_at, updated_at, COALESCE(owner_user_id, '')
		FROM executor_profiles ORDER BY executor_id ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanExecutorProfiles(rows)
}

func scanExecutorProfiles(rows *sql.Rows) ([]*models.ExecutorProfile, error) {
	var result []*models.ExecutorProfile
	for rows.Next() {
		profile := &models.ExecutorProfile{}
		var configJSON string
		var envVarsJSON sql.NullString
		if err := rows.Scan(
			&profile.ID, &profile.ExecutorID, &profile.Name, &profile.McpPolicy,
			&configJSON, &profile.PrepareScript, &profile.CleanupScript, &envVarsJSON, &profile.CreatedAt, &profile.UpdatedAt, &profile.OwnerUserID,
		); err != nil {
			return nil, err
		}
		if configJSON != "" && configJSON != "{}" {
			if err := json.Unmarshal([]byte(configJSON), &profile.Config); err != nil {
				return nil, fmt.Errorf("failed to deserialize profile config: %w", err)
			}
		}
		if envVarsJSON.Valid && envVarsJSON.String != "" && envVarsJSON.String != jsonNull {
			if err := json.Unmarshal([]byte(envVarsJSON.String), &profile.EnvVars); err != nil {
				return nil, fmt.Errorf("failed to deserialize profile env_vars: %w", err)
			}
		}
		result = append(result, profile)
	}
	return result, rows.Err()
}
