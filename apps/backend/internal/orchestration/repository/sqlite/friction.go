package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) RecordFriction(ctx context.Context, b *models.AssistantBinding, row models.Friction, now time.Time) error {
	if err := row.Validate(); err != nil {
		return err
	}
	if now.IsZero() || b == nil {
		return fmt.Errorf("friction requires current binding and observation time")
	}
	row.ID, row.BindingID, row.WorkspaceID = uuid.NewString(), b.ID, b.WorkspaceID
	if row.ObservedAt.IsZero() {
		row.ObservedAt = now.UTC()
	}
	if row.ObservedAt.After(now) {
		return fmt.Errorf("native observation is in the future")
	}
	if row.ObservedAt.Before(now.Add(-30 * 24 * time.Hour)) {
		return nil
	}
	row.Fingerprint = row.ScopeFingerprint()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockAssistantBinding(ctx, tx, b); err != nil {
		return err
	}
	result, err := tx.NamedExecContext(ctx, `INSERT INTO orchestration_friction(id,binding_id,workspace_id,task_id,session_id,occurrence_id,profile_id,account_revision,origin,operation,reason,cause,policy_version,fingerprint,outcome,observed_at)
 VALUES(:id,:binding_id,:workspace_id,:task_id,:session_id,:occurrence_id,:profile_id,:account_revision,:origin,:operation,:reason,:cause,:policy_version,:fingerprint,:outcome,:observed_at)
 ON CONFLICT(binding_id,occurrence_id,outcome) DO NOTHING`, row)
	if err != nil {
		return err
	}
	added, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if added > 0 && row.Outcome == "blocked" {
		if err = projectImprovement(ctx, tx, row, now.UTC()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Serializes aggregation with grant/owner revision changes on both engines.
func lockAssistantBinding(ctx context.Context, tx *sqlx.Tx, b *models.AssistantBinding) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_assistant_bindings SET updated_at=updated_at
 WHERE id=? AND version=? AND owner_user_id=? AND workspace_id=?`), b.ID, b.Version, b.OwnerUserID, b.WorkspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return models.ErrConflict
	}
	return err
}

func projectImprovement(ctx context.Context, tx *sqlx.Tx, row models.Friction, now time.Time) error {
	var count struct {
		Incidents int `db:"incidents"`
		Tasks     int `db:"tasks"`
	}
	err := tx.GetContext(ctx, &count, tx.Rebind(`SELECT count(*) AS incidents,count(DISTINCT task_id) AS tasks
 FROM orchestration_friction WHERE binding_id=? AND fingerprint=? AND outcome='blocked' AND observed_at>=? AND observed_at<=?
 AND observed_at>COALESCE((SELECT max(v.created_at) FROM orchestration_improvement_reviews v
 JOIN orchestration_improvements c ON c.id=v.candidate_id WHERE c.binding_id=? AND c.fingerprint=?),?)`),
		row.BindingID, row.Fingerprint, now.Add(-7*24*time.Hour), now, row.BindingID, row.Fingerprint, time.Unix(0, 0).UTC())
	if err != nil || count.Incidents < 3 || count.Tasks < 2 {
		return err
	}
	candidate := models.ImprovementCandidate{ID: uuid.NewString(), BindingID: row.BindingID, WorkspaceID: row.WorkspaceID,
		Fingerprint: row.Fingerprint, ProfileID: row.ProfileID, AccountRevision: row.AccountRevision,
		Origin: row.Origin, Operation: row.Operation, Reason: row.Reason, Cause: row.Cause, PolicyVersion: row.PolicyVersion,
		State: "proposed", Revision: 1, IncidentCount: count.Incidents, TaskCount: count.Tasks, CreatedAt: now, UpdatedAt: now}
	_, err = tx.NamedExecContext(ctx, `INSERT INTO orchestration_improvements(id,binding_id,workspace_id,fingerprint,profile_id,account_revision,origin,operation,reason,cause,policy_version,state,revision,incident_count,task_count,created_at,updated_at)
 VALUES(:id,:binding_id,:workspace_id,:fingerprint,:profile_id,:account_revision,:origin,:operation,:reason,:cause,:policy_version,:state,:revision,:incident_count,:task_count,:created_at,:updated_at)
 ON CONFLICT(binding_id,fingerprint) WHERE state NOT IN ('resolved','rejected')
 DO UPDATE SET incident_count=excluded.incident_count,task_count=excluded.task_count,updated_at=excluded.updated_at`, candidate)
	return err
}

func (r *Repository) ImprovementCandidates(ctx context.Context, binding, after string, limit int) ([]models.ImprovementCandidate, error) {
	rows := []models.ImprovementCandidate{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT i.* FROM orchestration_improvements i
 JOIN orchestration_assistant_bindings b ON b.id=i.binding_id AND b.workspace_id=i.workspace_id
 WHERE i.binding_id=? AND i.id>? ORDER BY i.id LIMIT ?`), binding, after, min(max(limit, 1), 101))
	return rows, err
}

func (r *Repository) ImprovementCandidate(ctx context.Context, binding, id string) (*models.ImprovementCandidate, error) {
	var row models.ImprovementCandidate
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`SELECT * FROM orchestration_improvements WHERE binding_id=? AND id=?`), binding, id)
	return &row, err
}

func (r *Repository) ImprovementEvidence(ctx context.Context, binding, fingerprint, after string, limit int) ([]models.Friction, error) {
	rows := []models.Friction{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT * FROM orchestration_friction WHERE binding_id=? AND fingerprint=? AND id>? ORDER BY id LIMIT ?`), binding, fingerprint, after, min(max(limit, 1), 101))
	return rows, err
}

func (r *Repository) CandidateEvidence(ctx context.Context, candidate *models.ImprovementCandidate, after string, limit int) ([]models.Friction, error) {
	rows := []models.Friction{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT f.* `+candidateFriction+` AND f.id>? ORDER BY f.id LIMIT ?`),
		candidate.BindingID, candidate.ID, candidate.CreatedAt.Add(-7*24*time.Hour), time.Unix(0, 0).UTC(), after, min(max(limit, 1), 101))
	return rows, err
}

// Each human closure starts a new evidence cohort. Late delivery retains its
// native event time instead of becoming evidence for an unrelated recurrence.
const candidateFriction = `FROM orchestration_friction f
 JOIN orchestration_improvements c ON c.binding_id=f.binding_id AND c.fingerprint=f.fingerprint
 LEFT JOIN orchestration_improvement_reviews v ON v.candidate_id=c.id
 WHERE f.binding_id=? AND c.id=? AND f.observed_at>=?
 AND (v.created_at IS NULL OR f.observed_at<=v.created_at)
 AND f.observed_at>COALESCE((SELECT max(previous.created_at) FROM orchestration_improvement_reviews previous
 JOIN orchestration_improvements old ON old.id=previous.candidate_id
 WHERE old.binding_id=c.binding_id AND old.fingerprint=c.fingerprint AND previous.created_at<c.created_at),?)`

func (r *Repository) PruneFriction(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM orchestration_friction WHERE observed_at<?`), now.UTC().Add(-30*24*time.Hour))
	return err
}
