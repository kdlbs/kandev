package sqlite

import (
	"context"
	"database/sql"
	"time"
)

// OnboardingState holds the persisted onboarding state for a workspace.
type OnboardingState struct {
	WorkspaceID string     `db:"workspace_id"`
	Completed   bool       `db:"completed"`
	CEOAgentID  string     `db:"ceo_agent_id"`
	FirstTaskID string     `db:"first_task_id"`
	CompletedAt *time.Time `db:"completed_at"`
	CreatedAt   time.Time  `db:"created_at"`
}

// GetOnboardingState returns the onboarding state for a workspace.
// Returns nil, nil if no row exists.
func (r *Repository) GetOnboardingState(ctx context.Context, workspaceID string) (*OnboardingState, error) {
	var state OnboardingState
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT workspace_id, completed, ceo_agent_id, first_task_id, completed_at, created_at
		 FROM office_onboarding WHERE workspace_id = ?`), workspaceID).StructScan(&state)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// HasAnyCompletedOnboarding returns true if any workspace has completed onboarding.
func (r *Repository) HasAnyCompletedOnboarding(ctx context.Context) (bool, error) {
	var count int
	err := r.ro.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM office_onboarding WHERE completed = 1`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetFirstCompletedOnboarding returns the first completed onboarding row, or nil.
func (r *Repository) GetFirstCompletedOnboarding(ctx context.Context) (*OnboardingState, error) {
	var state OnboardingState
	err := r.ro.QueryRowxContext(ctx,
		`SELECT workspace_id, completed, ceo_agent_id, first_task_id, completed_at, created_at
		 FROM office_onboarding WHERE completed = 1 LIMIT 1`).StructScan(&state)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// MarkOnboardingComplete records that onboarding is finished for a workspace.
func (r *Repository) MarkOnboardingComplete(ctx context.Context, workspaceID, ceoAgentID, firstTaskID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_onboarding (workspace_id, completed, ceo_agent_id, first_task_id, completed_at, created_at)
		VALUES (?, 1, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			completed = 1,
			ceo_agent_id = excluded.ceo_agent_id,
			first_task_id = excluded.first_task_id,
			completed_at = excluded.completed_at
	`), workspaceID, ceoAgentID, firstTaskID, now, now)
	return err
}
