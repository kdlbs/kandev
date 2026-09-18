package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const (
	reviewStateRejected = "rejected"
	reviewStateResolved = "resolved"
)

func (r *Repository) ReviewImprovement(ctx context.Context, b *models.AssistantBinding, id string, expected int64, state string, evidence models.Evidence, now time.Time) error {
	if !slices.Contains([]string{reviewStateRejected, reviewStateResolved}, state) || now.IsZero() {
		return models.ErrConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockAssistantBinding(ctx, tx, b); err != nil {
		return err
	}
	var candidate models.ImprovementCandidate
	if err = tx.GetContext(ctx, &candidate, tx.Rebind(`SELECT * FROM orchestration_improvements WHERE id=? AND binding_id=? AND workspace_id=?`), id, b.ID, b.WorkspaceID); err != nil {
		return err
	}
	if err = validateImprovementReviewCandidate(candidate, expected, state, evidence); err != nil {
		return err
	}
	var resolved *time.Time
	if state == reviewStateResolved {
		resolved = &now
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_improvements SET state=?,revision=revision+1,updated_at=?,resolved_at=? WHERE id=?`), state, now, resolved, id)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_improvement_reviews(candidate_id,owner_user_id,state,evidence_json,created_at) VALUES(?,?,?,?,?)`), id, b.OwnerUserID, state, string(raw), now)
	if err != nil {
		return err
	}
	if err = finishMaintenanceObjective(ctx, tx, &candidate, state, evidence, now); err != nil {
		return err
	}
	body := fmt.Sprintf("Workflow improvement %s reviewed as %s. Native evidence task: %s; session: %s; source: %s.", id, state, evidence.TaskID, evidence.SessionID, evidence.SourceID)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_comments(id,task_id,author_type,author_id,body,source,created_at) VALUES(?,?,'user',?,?,'maintenance_review',?)`), uuid.NewString(), b.ConversationID, b.OwnerUserID, body, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func validateImprovementReviewCandidate(candidate models.ImprovementCandidate, expected int64, state string, evidence models.Evidence) error {
	if candidate.Revision != expected || slices.Contains([]string{reviewStateResolved, reviewStateRejected}, candidate.State) {
		return models.ErrConflict
	}
	if state == reviewStateResolved && (candidate.State != "prepared" || candidate.CommitOID == "" || candidate.PreparedAt == nil || evidence.SourceKind != "task_message" || evidence.SourceID == "" || evidence.TaskID == "" || evidence.SessionID == "") {
		return fmt.Errorf("resolution requires a prepared repair and observed native success")
	}
	return nil
}

func (r *Repository) ImprovementReview(ctx context.Context, binding, id string) (*models.ImprovementReview, error) {
	var row models.ImprovementReview
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`SELECT v.* FROM orchestration_improvement_reviews v JOIN orchestration_improvements c ON c.id=v.candidate_id WHERE c.binding_id=? AND c.id=?`), binding, id)
	if err != nil {
		return nil, err
	}
	return &row, json.Unmarshal([]byte(row.EvidenceJSON), &row.Evidence)
}

func (r *Repository) ImprovementAffectedTask(ctx context.Context, binding, candidate, task string) (bool, error) {
	row, err := r.ImprovementCandidate(ctx, binding, candidate)
	if err != nil {
		return false, err
	}
	var count int
	err = r.ro.GetContext(ctx, &count, r.ro.Rebind(`SELECT count(*) `+candidateFriction+` AND f.task_id=? AND f.outcome='blocked'`), binding, candidate, row.CreatedAt.Add(-7*24*time.Hour), time.Unix(0, 0).UTC(), task)
	return count > 0, err
}

func (r *Repository) ImprovementAffectedTasks(ctx context.Context, binding, candidate, after string, limit int) ([]string, error) {
	rows := []string{}
	row, err := r.ImprovementCandidate(ctx, binding, candidate)
	if err != nil {
		return rows, err
	}
	err = r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT DISTINCT f.task_id `+candidateFriction+` AND f.outcome='blocked' AND f.task_id>? ORDER BY f.task_id LIMIT ?`), binding, candidate, row.CreatedAt.Add(-7*24*time.Hour), time.Unix(0, 0).UTC(), after, min(max(limit, 1), 101))
	return rows, err
}

func finishMaintenanceObjective(ctx context.Context, tx *sqlx.Tx, candidate *models.ImprovementCandidate, state string, evidence models.Evidence, now time.Time) error {
	if candidate.ObjectiveID == "" {
		return nil
	}
	if state == reviewStateRejected {
		_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_objectives SET status='cancelled',revision=revision+1,updated_at=? WHERE id=? AND binding_id=?`), now, candidate.ObjectiveID, candidate.BindingID)
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_objective_tasks(objective_id,task_id,session_id,role,context_ref,operation_id) VALUES(?,?,?,'workflow_recovery','',?) ON CONFLICT DO NOTHING`), candidate.ObjectiveID, evidence.TaskID, evidence.SessionID, "maintenance-review:"+candidate.ID); err != nil {
		return err
	}
	var objective models.Objective
	if err := tx.GetContext(ctx, &objective, tx.Rebind(`SELECT * FROM orchestration_objectives WHERE id=? AND binding_id=?`), candidate.ObjectiveID, candidate.BindingID); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(objective.AcceptanceJSON), &objective.Acceptance); err != nil {
		return err
	}
	proof := []models.Evidence{}
	for _, criterion := range objective.Acceptance {
		entry := evidence
		entry.CriterionID, entry.AcceptanceRevision = criterion.ID, objective.AcceptanceRevision
		proof = append(proof, entry)
	}
	raw, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_objectives SET status='complete',evidence_json=?,revision=revision+1,updated_at=? WHERE id=? AND binding_id=?`), string(raw), now, candidate.ObjectiveID, candidate.BindingID)
	return err
}
