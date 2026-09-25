package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
)

// ReserveRunSession atomically inserts a fresh run-owned session attempt and
// binds the run's first session ID while the run is claimed. A duplicate
// (run_id, attempt) is a lost reservation and leaves the predecessor row
// untouched. A later attempt may coexist with the predecessor, but never
// overwrites its durable identity.
func (r *Repository) ReserveRunSession(ctx context.Context, session *models.RunSession) (bool, error) {
	if err := validateRunSessionReservation(session); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("reserve run session: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateRunSessionWorkspace(ctx, tx, session); err != nil {
		return false, err
	}

	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_run_sessions (
			id, workspace_id, agent_profile_id, run_id, attempt, state,
			execution_id, execution_profile_id, adapter, model, acp_session_id,
			created_at, started_at, finished_at, cancel_requested_at, error_message, version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`), session.ID, session.WorkspaceID, session.AgentProfileID, session.RunID,
		session.Attempt, session.State, session.ExecutionID, session.ExecutionProfileID,
		session.Adapter, session.Model, session.ACPSessionID, session.CreatedAt,
		session.StartedAt, session.FinishedAt, session.CancelRequestedAt,
		session.ErrorMessage, session.Version)
	if err != nil {
		return false, fmt.Errorf("reserve run session: insert: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reserve run session: rows affected: %w", err)
	}
	if inserted == 0 {
		return false, nil
	}

	bound, err := r.bindReservedRunSession(ctx, tx, session)
	if err != nil {
		return false, err
	}
	if !bound {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("reserve run session: commit: %w", err)
	}
	return true, nil
}

func validateRunSessionReservation(session *models.RunSession) error {
	if session == nil || session.ID == "" || session.RunID == "" || session.WorkspaceID == "" ||
		session.AgentProfileID == "" || session.Attempt <= 0 || session.State == "" {
		return errors.New("reserve run session: incomplete session")
	}
	return nil
}

func (r *Repository) validateRunSessionWorkspace(
	ctx context.Context,
	tx *sqlx.Tx,
	session *models.RunSession,
) error {
	var profileWorkspace string
	if err := tx.GetContext(ctx, &profileWorkspace, r.db.Rebind(`
		SELECT COALESCE(workspace_id, '') FROM agent_profiles WHERE id = ?
	`), session.AgentProfileID); err != nil {
		return fmt.Errorf("reserve run session: resolve agent workspace: %w", err)
	}
	if profileWorkspace != session.WorkspaceID {
		return fmt.Errorf("reserve run session: agent workspace %q does not match %q", profileWorkspace, session.WorkspaceID)
	}
	return nil
}

func (r *Repository) bindReservedRunSession(
	ctx context.Context,
	tx *sqlx.Tx,
	session *models.RunSession,
) (bool, error) {
	// Only the first attempt is projected onto runs.session_id. The dedicated
	// session table remains authoritative for retries and preserves the
	// predecessor identity.
	boundResult, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET session_id = ?
		WHERE id = ? AND status = 'claimed' AND COALESCE(session_id, '') = ''
	`), session.ID, session.RunID)
	if err != nil {
		return false, fmt.Errorf("reserve run session: bind run: %w", err)
	}
	bound, err := boundResult.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reserve run session: bind rows affected: %w", err)
	}
	if bound == 1 {
		return true, nil
	}
	var runStatus string
	if err := tx.GetContext(ctx, &runStatus, r.db.Rebind(`SELECT status FROM runs WHERE id = ?`), session.RunID); err != nil {
		return false, fmt.Errorf("reserve run session: verify run claim: %w", err)
	}
	return runStatus == "claimed", nil
}

// GetRunSession returns one run-owned session by durable identity.
func (r *Repository) GetRunSession(ctx context.Context, sessionID string) (*models.RunSession, error) {
	var session models.RunSession
	err := r.ro.GetContext(ctx, &session, r.ro.Rebind(`SELECT * FROM office_run_sessions WHERE id = ?`), sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get run session: %w", err)
	}
	return &session, nil
}

// ListRunSessions returns all attempts for a run in reservation order.
func (r *Repository) ListRunSessions(ctx context.Context, runID string) ([]models.RunSession, error) {
	var sessions []models.RunSession
	err := r.ro.SelectContext(ctx, &sessions, r.ro.Rebind(`
		SELECT * FROM office_run_sessions WHERE run_id = ? ORDER BY attempt ASC
	`), runID)
	if err != nil {
		return nil, fmt.Errorf("list run sessions: %w", err)
	}
	if sessions == nil {
		sessions = []models.RunSession{}
	}
	return sessions, nil
}

// ListLiveRunSessionsForWorkspace returns every run-owned attempt that still
// has a runtime execution. It deliberately does not join runs: a pause must
// also find executions whose run was cancelled before the process stopped.
func (r *Repository) ListLiveRunSessionsForWorkspace(ctx context.Context, workspaceID string) ([]models.RunSession, error) {
	var sessions []models.RunSession
	err := r.ro.SelectContext(ctx, &sessions, r.ro.Rebind(`
		SELECT * FROM office_run_sessions
		WHERE workspace_id = ? AND COALESCE(execution_id, '') <> '' AND state IN (?, ?)
		ORDER BY created_at ASC
	`), workspaceID, models.RunSessionStatePreparing, models.RunSessionStateRunning)
	if err != nil {
		return nil, fmt.Errorf("list live run sessions: %w", err)
	}
	if sessions == nil {
		sessions = []models.RunSession{}
	}
	return sessions, nil
}

// BindRunSessionExecution records the runtime identity while the attempt is
// still preparing. This makes a registered execution visible to pause and
// deletion sweeps before process startup can race those controls.
func (r *Repository) BindRunSessionExecution(
	ctx context.Context, sessionID, executionID, executionProfileID, adapter, model, acpSessionID string,
) (bool, error) {
	if executionID == "" {
		return false, errors.New("bind run session execution: execution ID is required")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_run_sessions
		SET execution_id = ?, execution_profile_id = ?, adapter = ?, model = ?,
		    acp_session_id = ?, version = version + 1
		WHERE id = ? AND state = ? AND COALESCE(execution_id, '') = ''
		  AND cancel_requested_at IS NULL
	`), executionID, executionProfileID, adapter, model, acpSessionID, sessionID,
		models.RunSessionStatePreparing)
	if err != nil {
		return false, fmt.Errorf("bind run session execution: %w", err)
	}
	wrote, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("bind run session execution: rows affected: %w", err)
	}
	return wrote == 1, nil
}

// ListUnfinishedRunSessions returns every non-terminal attempt for startup
// reconciliation, including reservations that never received an execution ID.
func (r *Repository) ListUnfinishedRunSessions(ctx context.Context) ([]models.RunSession, error) {
	var sessions []models.RunSession
	err := r.ro.SelectContext(ctx, &sessions, r.ro.Rebind(`
		SELECT * FROM office_run_sessions
		WHERE state IN (?, ?)
		ORDER BY created_at ASC
	`), models.RunSessionStatePreparing, models.RunSessionStateRunning)
	if err != nil {
		return nil, fmt.Errorf("list unfinished run sessions: %w", err)
	}
	if sessions == nil {
		sessions = []models.RunSession{}
	}
	return sessions, nil
}

// RequestRunSessionCancellation persists cancellation intent before a pause
// asks the runtime to stop the process. The registration admission gate
// rejects a preparing attempt after this CAS wins.
func (r *Repository) RequestRunSessionCancellation(ctx context.Context, sessionID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_run_sessions
		SET cancel_requested_at = ?, version = version + 1
		WHERE id = ? AND state IN (?, ?) AND cancel_requested_at IS NULL
	`), time.Now().UTC(), sessionID, models.RunSessionStatePreparing, models.RunSessionStateRunning)
	if err != nil {
		return false, fmt.Errorf("request run session cancellation: %w", err)
	}
	wrote, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("request run session cancellation: rows affected: %w", err)
	}
	return wrote == 1, nil
}

// MarkRunSessionStarted records the runtime execution identity and moves a
// preparing attempt to running. The compare-and-set prevents a late starter
// from resurrecting a cancelled or already-terminal attempt.
func (r *Repository) MarkRunSessionStarted(
	ctx context.Context, sessionID, executionID, executionProfileID, adapter, model, acpSessionID string,
) (bool, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_run_sessions
		SET state = ?, execution_id = ?, execution_profile_id = ?, adapter = ?, model = ?,
		    acp_session_id = ?, started_at = ?, version = version + 1
		WHERE id = ? AND state = ? AND execution_id = ? AND cancel_requested_at IS NULL
	`), models.RunSessionStateRunning, executionID, executionProfileID, adapter, model,
		acpSessionID, now, sessionID, models.RunSessionStatePreparing, executionID)
	if err != nil {
		return false, fmt.Errorf("mark run session started: %w", err)
	}
	wrote, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("mark run session started: rows affected: %w", err)
	}
	return wrote == 1, nil
}

// FinishRunSession applies a terminal state exactly once to an attempt.
func (r *Repository) FinishRunSession(
	ctx context.Context, sessionID string, state models.RunSessionState, errorMessage string,
) (bool, error) {
	if state != models.RunSessionStateFinished && state != models.RunSessionStateFailed &&
		state != models.RunSessionStateCancelled && state != models.RunSessionStateInterrupted {
		return false, errors.New("finish run session: invalid terminal state")
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_run_sessions
		SET state = ?, finished_at = ?, error_message = ?, version = version + 1
		WHERE id = ? AND state IN (?, ?)
		  AND (cancel_requested_at IS NULL OR ? <> ?)
	`), state, now, errorMessage, sessionID,
		models.RunSessionStatePreparing, models.RunSessionStateRunning,
		state, models.RunSessionStateFinished)
	if err != nil {
		return false, fmt.Errorf("finish run session: %w", err)
	}
	wrote, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("finish run session: rows affected: %w", err)
	}
	return wrote == 1, nil
}
