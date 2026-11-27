package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// UpdateExecutorProfileWithScriptIntent writes explicit scripts and captures
// this commit's script pair without a later profile read.
func (r *Repository) UpdateExecutorProfileWithScriptIntent(ctx context.Context, profile *models.ExecutorProfile, intent models.ExecutorProfileScriptIntent) error {
	return r.updateExecutorProfile(ctx, profile, intent, nil)
}

func (r *Repository) updateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile, intent models.ExecutorProfileScriptIntent, expected *time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	configJSON, err := json.Marshal(profile.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize profile config: %w", err)
	}
	envJSON, err := json.Marshal(profile.EnvVars)
	if err != nil {
		return fmt.Errorf("failed to serialize profile env_vars: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := r.profileOwnershipForUpdateTx(ctx, tx, profile)
	if err != nil {
		return err
	}
	query, args := profileUpdateQuery(profile, intent, owner, string(configJSON), string(envJSON), expected)
	rows, err := tx.QueryContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return err
	}
	committed, err := scanCommittedProfileScripts(rows, profile.ID)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	profile.PrepareScript = committed.PrepareScript
	profile.CleanupScript = committed.CleanupScript
	profile.UpdatedAt = committed.UpdatedAt
	profile.OwnerUserID = committed.OwnerUserID
	profile.OwnershipIntent = nil
	return nil
}

func scanCommittedProfileScripts(rows *sql.Rows, id string) (*models.ExecutorProfile, error) {
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("executor profile not found: %s", id)
	}
	var committed models.ExecutorProfile
	if err := rows.Scan(&committed.PrepareScript, &committed.CleanupScript, &committed.UpdatedAt, &committed.OwnerUserID); err != nil {
		return nil, err
	}
	if rows.Next() {
		return nil, fmt.Errorf("multiple executor profiles returned for %s", id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return &committed, nil
}

func profileUpdateQuery(profile *models.ExecutorProfile, intent models.ExecutorProfileScriptIntent, owner, configJSON, envJSON string, expected *time.Time) (string, []any) {
	query := `UPDATE executor_profiles SET name = ?, mcp_policy = ?, config = ?, env_vars = ?, updated_at = ?, owner_user_id = NULLIF(?, '')`
	args := []any{profile.Name, profile.McpPolicy, configJSON, envJSON, time.Now().UTC(), owner}
	if intent.PrepareScript != nil {
		query += ", prepare_script = ?"
		args = append(args, *intent.PrepareScript)
	}
	if intent.CleanupScript != nil {
		query += ", cleanup_script = ?"
		args = append(args, *intent.CleanupScript)
	}
	query += " WHERE id = ?"
	args = append(args, profile.ID)
	if expected != nil {
		query += " AND updated_at = ?"
		args = append(args, *expected)
	}
	query += " RETURNING prepare_script, cleanup_script, updated_at, COALESCE(owner_user_id, '')"
	return query, args
}
