package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	cleanupPhasePending                 = "pending"
	agentDeliveryRetirementDefaultLimit = 200
	agentDeliveryRetirementTimeout      = 250 * time.Millisecond
)

var agentDeliveryRetirementPhases = []string{"inbox", "effects", "submissions", "cursors"}

type agentDeliveryRetirementJob struct {
	sessionID     string `db:"session_id"`
	incarnationID string `db:"incarnation_id"`
	phase         string `db:"phase"`
	lastStreamID  string `db:"last_stream_id"`
}

// RunAgentDeliveryRetirementBatch removes bounded chunks from one retired
// identity. Each deletion and its phase checkpoint commit atomically.
func (r *Repository) RunAgentDeliveryRetirementBatch(
	ctx context.Context,
	options models.AgentDeliveryRetirementBatchOptions,
) (models.AgentDeliveryRetirementBatchResult, error) {
	options = normalizeAgentDeliveryRetirementOptions(options.MaxCandidates, options.WorkTimeout)
	workCtx, cancel := context.WithTimeout(ctx, options.WorkTimeout)
	defer cancel()
	var result models.AgentDeliveryRetirementBatchResult
	if err := workCtx.Err(); err != nil {
		return result, err
	}
	tx, err := r.db.BeginTxx(workCtx, nil)
	if err != nil {
		return result, fmt.Errorf("begin agent delivery retirement batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	job, err := r.nextAgentDeliveryRetirementJobTx(workCtx, tx)
	if err != nil {
		return result, err
	}
	if job == nil {
		return result, tx.Commit()
	}
	if err := lockAgentDeliveryWrites(workCtx, tx, r.db.DriverName(), []string{job.sessionID}, nil); err != nil {
		return result, err
	}
	live, err := r.agentDeliverySessionExistsTx(workCtx, tx, job.sessionID)
	if err != nil {
		return result, err
	}
	if live {
		if err := r.suspendAgentDeliveryRetirementJobTx(workCtx, tx, job.sessionID, job.incarnationID); err != nil {
			return result, err
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		result.Suspended = 1
		result.Pending = true
		return result, nil
	}
	if err := r.purgeAgentDeliveryRetirementJobTx(workCtx, tx, job, options.MaxCandidates, &result); err != nil {
		return models.AgentDeliveryRetirementBatchResult{}, err
	}
	if err := tx.QueryRowxContext(workCtx, `SELECT EXISTS (SELECT 1 FROM agent_delivery_cleanup_jobs LIMIT 1)`).Scan(&result.Pending); err != nil {
		return models.AgentDeliveryRetirementBatchResult{}, fmt.Errorf("check remaining agent delivery retirement jobs: %w", err)
	}
	if err := workCtx.Err(); err != nil {
		return models.AgentDeliveryRetirementBatchResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.AgentDeliveryRetirementBatchResult{}, fmt.Errorf("commit agent delivery retirement batch: %w", err)
	}
	return result, nil
}

func (r *Repository) nextAgentDeliveryRetirementJobTx(
	ctx context.Context,
	tx *sqlx.Tx,
) (*agentDeliveryRetirementJob, error) {
	query := `SELECT session_id, incarnation_id, phase, last_stream_id FROM agent_delivery_cleanup_jobs ORDER BY updated_at, session_id, incarnation_id LIMIT 1`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE SKIP LOCKED`
	}
	var job agentDeliveryRetirementJob
	err := tx.QueryRowxContext(ctx, query).Scan(&job.sessionID, &job.incarnationID, &job.phase, &job.lastStreamID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select agent delivery retirement job: %w", err)
	}
	return &job, nil
}

func (r *Repository) purgeAgentDeliveryRetirementJobTx(
	ctx context.Context,
	tx *sqlx.Tx,
	job *agentDeliveryRetirementJob,
	limit int,
	result *models.AgentDeliveryRetirementBatchResult,
) error {
	phaseIndex := retirementPhaseIndex(job.phase)
	if phaseIndex < 0 {
		return fmt.Errorf("unsupported agent delivery retirement phase %q", job.phase)
	}
	remaining := limit
	for phaseIndex < len(agentDeliveryRetirementPhases) && remaining > 0 {
		phase := agentDeliveryRetirementPhases[phaseIndex]
		phaseLimit := remaining
		batch, err := r.deleteAgentDeliveryRetirementPhaseTx(ctx, tx, phase, job, phaseLimit)
		if err != nil {
			return err
		}
		result.RowsPurged += batch.deleted
		remaining -= max(batch.deleted, batch.scanned)
		if err := r.failAgentDeliveryRetention("retirement_after_" + phase + "_delete"); err != nil {
			return err
		}
		if !batch.complete {
			return r.updateAgentDeliveryRetirementPhaseTx(ctx, tx, job, phase)
		}
		phaseIndex++
		if phaseIndex == len(agentDeliveryRetirementPhases) {
			return r.completeAgentDeliveryRetirementJobTx(ctx, tx, job, result)
		}
		job.phase = agentDeliveryRetirementPhases[phaseIndex]
		if err := r.persistAgentDeliveryRetirementPhaseTx(ctx, tx, job); err != nil {
			return err
		}
	}
	return nil
}

func retirementPhaseIndex(phase string) int {
	if phase == cleanupPhasePending {
		return 0
	}
	for index, candidate := range agentDeliveryRetirementPhases {
		if phase == candidate {
			return index
		}
	}
	return -1
}

func (r *Repository) deleteAgentDeliveryRetirementPhaseTx(
	ctx context.Context,
	tx *sqlx.Tx,
	phase string,
	job *agentDeliveryRetirementJob,
	limit int,
) (agentDeliveryRetirementPhaseResult, error) {
	var query string
	switch phase {
	case "inbox":
		query = `DELETE FROM agent_delivery_inbox WHERE (stream_id, sequence) IN (
			SELECT stream_id, sequence FROM agent_delivery_inbox WHERE session_id = ?
			ORDER BY incarnation_id, stream_id, sequence LIMIT ?)`
	case "effects":
		return r.deleteAgentDeliveryRetirementEffectsTx(ctx, tx, job, limit)
	case "submissions":
		query = `DELETE FROM agent_delivery_submissions WHERE id IN (
			SELECT id FROM agent_delivery_submissions WHERE session_id = ? ORDER BY incarnation_id, id LIMIT ?)`
	case "cursors":
		query = `DELETE FROM agent_delivery_cursors WHERE stream_id IN (
			SELECT stream_id FROM agent_delivery_cursors WHERE session_id = ? ORDER BY stream_id LIMIT ?)`
	default:
		return agentDeliveryRetirementPhaseResult{}, fmt.Errorf("unknown agent delivery retirement phase %q", phase)
	}
	args := []any{job.sessionID, limit}
	res, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return agentDeliveryRetirementPhaseResult{}, fmt.Errorf("purge agent delivery %s rows: %w", phase, err)
	}
	rows, err := res.RowsAffected()
	return agentDeliveryRetirementPhaseResult{deleted: int(rows), complete: rows < int64(limit)}, err
}

func (r *Repository) updateAgentDeliveryRetirementPhaseTx(
	ctx context.Context,
	tx *sqlx.Tx,
	job *agentDeliveryRetirementJob,
	phase string,
) error {
	job.phase = phase
	return r.persistAgentDeliveryRetirementPhaseTx(ctx, tx, job)
}

func (r *Repository) persistAgentDeliveryRetirementPhaseTx(
	ctx context.Context,
	tx *sqlx.Tx,
	job *agentDeliveryRetirementJob,
) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_cleanup_jobs SET phase = ?, last_stream_id = ?, error_code = '', updated_at = ?
		WHERE session_id = ? AND incarnation_id = ?`), job.phase, job.lastStreamID, r.nowUTC(), job.sessionID, job.incarnationID)
	if err != nil {
		return fmt.Errorf("persist agent delivery retirement phase: %w", err)
	}
	return r.failAgentDeliveryRetention("retirement_after_phase_progress")
}

func (r *Repository) completeAgentDeliveryRetirementJobTx(
	ctx context.Context,
	tx *sqlx.Tx,
	job *agentDeliveryRetirementJob,
	result *models.AgentDeliveryRetirementBatchResult,
) error {
	remaining, err := r.agentDeliveryRetirementEvidenceExistsTx(ctx, tx, job.sessionID)
	if err != nil {
		return err
	}
	if remaining {
		return fmt.Errorf("agent delivery retirement still has attributable rows for session %s", job.sessionID)
	}
	if err := r.failAgentDeliveryRetention("retirement_before_job_delete"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM agent_delivery_cleanup_jobs WHERE session_id = ? AND incarnation_id = ?`),
		job.sessionID, job.incarnationID); err != nil {
		return fmt.Errorf("complete agent delivery retirement job: %w", err)
	}
	if err := r.failAgentDeliveryRetention("retirement_after_job_delete"); err != nil {
		return err
	}
	result.Completed++
	return nil
}

func (r *Repository) agentDeliveryRetirementEvidenceExistsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) (bool, error) {
	var exists bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM agent_delivery_inbox WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_effects WHERE session_id = ? AND session_id <> ''
			UNION ALL SELECT 1 FROM agent_delivery_submissions WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_cursors WHERE session_id = ?
		)`), sessionID, sessionID, sessionID, sessionID).Scan(&exists)
	return exists, err
}

func (r *Repository) suspendAgentDeliveryRetirementJobTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID, incarnationID string,
) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_cleanup_jobs SET error_code = 'live_session_conflict', updated_at = ?
		WHERE session_id = ? AND incarnation_id = ?`), r.nowUTC(), sessionID, incarnationID)
	return err
}

func (r *Repository) agentDeliverySessionExistsTx(ctx context.Context, tx *sqlx.Tx, sessionID string) (bool, error) {
	var exists bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (SELECT 1 FROM task_sessions WHERE id = ?)`), sessionID).Scan(&exists)
	return exists, err
}

func normalizeAgentDeliveryRetirementOptions(limit int, timeout time.Duration) models.AgentDeliveryRetirementBatchOptions {
	if limit <= 0 || limit > agentDeliveryRetirementDefaultLimit {
		limit = agentDeliveryRetirementDefaultLimit
	}
	if timeout <= 0 || timeout > agentDeliveryRetirementTimeout {
		timeout = agentDeliveryRetirementTimeout
	}
	return models.AgentDeliveryRetirementBatchOptions{MaxCandidates: limit, WorkTimeout: timeout}
}

// RunAgentDeliveryOrphanBatch schedules bounded cleanup for rows whose
// session is authoritatively absent. Unknown identities remain untouched.
func (r *Repository) RunAgentDeliveryOrphanBatch(
	ctx context.Context,
	options models.AgentDeliveryOrphanBatchOptions,
) (models.AgentDeliveryOrphanBatchResult, error) {
	options = normalizeAgentDeliveryOrphanOptions(options.MaxCandidates, options.WorkTimeout)
	workCtx, cancel := context.WithTimeout(ctx, options.WorkTimeout)
	defer cancel()
	var result models.AgentDeliveryOrphanBatchResult
	if err := workCtx.Err(); err != nil {
		return result, err
	}
	tx, err := r.db.BeginTxx(workCtx, nil)
	if err != nil {
		return result, fmt.Errorf("begin agent delivery orphan scan: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(workCtx, `UPDATE agent_delivery_retention_state SET updated_at = updated_at WHERE id = 1`); err != nil {
		return result, fmt.Errorf("lock orphan scan progress: %w", err)
	}
	progress, err := loadAgentDeliveryRetentionProgress(workCtx, tx, r.db.Rebind)
	if err != nil {
		return result, err
	}
	candidates, scanned, lastSessionID, err := r.scanAgentDeliveryOrphanCandidatesTx(workCtx, tx, progress.lastSessionID, options.MaxCandidates)
	if err != nil {
		return result, err
	}
	result.Candidates = scanned
	if err := r.scheduleAgentDeliveryOrphansTx(workCtx, tx, candidates, &result); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(workCtx, r.db.Rebind(`UPDATE agent_delivery_retention_state SET last_session_id = ?, updated_at = ? WHERE id = 1`), lastSessionID, r.nowUTC()); err != nil {
		return result, fmt.Errorf("save orphan scan progress: %w", err)
	}
	result.UnboundWorkflowRows, err = r.countUnboundWorkflowRowsTx(workCtx, tx, options.MaxCandidates)
	if err != nil {
		return result, err
	}
	if err := workCtx.Err(); err != nil {
		return models.AgentDeliveryOrphanBatchResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.AgentDeliveryOrphanBatchResult{}, fmt.Errorf("commit agent delivery orphan scan: %w", err)
	}
	return result, nil
}

func (r *Repository) scheduleAgentDeliveryOrphansTx(ctx context.Context, tx *sqlx.Tx, candidates []agentDeliverySessionIdentity, result *models.AgentDeliveryOrphanBatchResult) error {

	if err := r.lockAgentDeliverySessionIdentitiesTx(ctx, tx, candidates); err != nil {
		return err
	}
	for _, candidate := range candidates {
		live, err := r.agentDeliverySessionExistsTx(ctx, tx, candidate.sessionID)
		if err != nil {
			return fmt.Errorf("recheck orphan session ownership: %w", err)
		}
		if live {
			continue
		}
		var retiring bool
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
			SELECT EXISTS (SELECT 1 FROM agent_delivery_cleanup_jobs WHERE session_id = ?)`), candidate.sessionID).Scan(&retiring); err != nil {
			return fmt.Errorf("recheck orphan retirement job: %w", err)
		}
		if retiring {
			continue
		}
		if err := r.insertAgentDeliveryRetirementJobsTx(ctx, tx, []agentDeliverySessionIdentity{candidate}); err != nil {
			return err
		}
		result.JobsScheduled++
	}
	return nil
}

func normalizeAgentDeliveryOrphanOptions(limit int, timeout time.Duration) models.AgentDeliveryOrphanBatchOptions {
	if limit <= 0 || limit > agentDeliveryRetirementDefaultLimit {
		limit = agentDeliveryRetirementDefaultLimit
	}
	if timeout <= 0 || timeout > agentDeliveryRetirementTimeout {
		timeout = agentDeliveryRetirementTimeout
	}
	return models.AgentDeliveryOrphanBatchOptions{MaxCandidates: limit, WorkTimeout: timeout}
}

func (r *Repository) countUnboundWorkflowRowsTx(ctx context.Context, tx *sqlx.Tx, limit int) (int, error) {
	var count int
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT COUNT(*) FROM (
			SELECT effect_key FROM agent_delivery_effects
			WHERE effect_type = ? AND (session_id = '' OR incarnation_id = '')
			ORDER BY effect_key LIMIT ?
		)`), "workflow.on_turn_complete", limit).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count bounded unbound workflow receipts: %w", err)
	}
	return count, nil
}
