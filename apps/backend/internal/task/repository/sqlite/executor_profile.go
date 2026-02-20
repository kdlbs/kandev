package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

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

	isDefault := 0
	if profile.IsDefault {
		isDefault = 1
	}

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO executor_profiles (id, executor_id, name, is_default, config, setup_script, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), profile.ID, profile.ExecutorID, profile.Name, isDefault, string(configJSON), profile.SetupScript, profile.CreatedAt, profile.UpdatedAt)
	return err
}

func (r *Repository) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	profile := &models.ExecutorProfile{}
	var configJSON string
	var isDefault int

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, executor_id, name, is_default, config, setup_script, created_at, updated_at
		FROM executor_profiles WHERE id = ?
	`), id).Scan(
		&profile.ID, &profile.ExecutorID, &profile.Name, &isDefault,
		&configJSON, &profile.SetupScript, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("executor profile not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	profile.IsDefault = isDefault == 1
	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &profile.Config); err != nil {
			return nil, fmt.Errorf("failed to deserialize profile config: %w", err)
		}
	}
	return profile, nil
}

func (r *Repository) UpdateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error {
	profile.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(profile.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize profile config: %w", err)
	}

	isDefault := 0
	if profile.IsDefault {
		isDefault = 1
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE executor_profiles SET name = ?, is_default = ?, config = ?, setup_script = ?, updated_at = ?
		WHERE id = ?
	`), profile.Name, isDefault, string(configJSON), profile.SetupScript, profile.UpdatedAt, profile.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor profile not found: %s", profile.ID)
	}
	return nil
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
		SELECT id, executor_id, name, is_default, config, setup_script, created_at, updated_at
		FROM executor_profiles WHERE executor_id = ? ORDER BY is_default DESC, name ASC
	`), executorID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.ExecutorProfile
	for rows.Next() {
		profile := &models.ExecutorProfile{}
		var configJSON string
		var isDefault int
		if err := rows.Scan(
			&profile.ID, &profile.ExecutorID, &profile.Name, &isDefault,
			&configJSON, &profile.SetupScript, &profile.CreatedAt, &profile.UpdatedAt,
		); err != nil {
			return nil, err
		}
		profile.IsDefault = isDefault == 1
		if configJSON != "" && configJSON != "{}" {
			if err := json.Unmarshal([]byte(configJSON), &profile.Config); err != nil {
				return nil, fmt.Errorf("failed to deserialize profile config: %w", err)
			}
		}
		result = append(result, profile)
	}
	return result, rows.Err()
}

func (r *Repository) GetDefaultExecutorProfile(ctx context.Context, executorID string) (*models.ExecutorProfile, error) {
	profile := &models.ExecutorProfile{}
	var configJSON string
	var isDefault int

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, executor_id, name, is_default, config, setup_script, created_at, updated_at
		FROM executor_profiles WHERE executor_id = ? AND is_default = 1 LIMIT 1
	`), executorID).Scan(
		&profile.ID, &profile.ExecutorID, &profile.Name, &isDefault,
		&configJSON, &profile.SetupScript, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No default profile is not an error
	}
	if err != nil {
		return nil, err
	}

	profile.IsDefault = isDefault == 1
	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &profile.Config); err != nil {
			return nil, fmt.Errorf("failed to deserialize profile config: %w", err)
		}
	}
	return profile, nil
}
