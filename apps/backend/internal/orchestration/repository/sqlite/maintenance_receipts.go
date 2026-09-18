package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) SaveMaintenanceValidation(ctx context.Context, binding, candidate string, v models.MaintenanceValidation) error {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) > 16000 {
		return fmt.Errorf("invalid maintenance validation receipt")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO orchestration_maintenance_validation(candidate_id,validation_json,updated_at)
 SELECT id,?,? FROM orchestration_improvements WHERE id=? AND binding_id=? AND repair_task_id<>''
 ON CONFLICT(candidate_id) DO UPDATE SET validation_json=excluded.validation_json,updated_at=excluded.updated_at`), string(raw), time.Now().UTC(), candidate, binding)
	return maintenanceRowChanged(result, err)
}

func (r *Repository) MaintenanceValidation(ctx context.Context, binding, candidate string) (*models.MaintenanceValidation, error) {
	var raw string
	err := r.db.GetContext(ctx, &raw, r.db.Rebind(`SELECT v.validation_json FROM orchestration_maintenance_validation v JOIN orchestration_improvements c ON c.id=v.candidate_id WHERE c.binding_id=? AND c.id=?`), binding, candidate)
	if err != nil {
		return nil, err
	}
	var result models.MaintenanceValidation
	return &result, json.Unmarshal([]byte(raw), &result)
}

func (r *Repository) RecordMaintenancePrepared(ctx context.Context, b *models.AssistantBinding, id string, artifact models.MaintenanceArtifact) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_improvements SET commit_oid=?,prepared_at=?,updated_at=?,revision=revision+1,
 state=CASE WHEN state='investigating' THEN 'prepared' ELSE state END WHERE id=? AND binding_id=? AND repair_task_id<>'' AND commit_oid=''`), artifact.CommitOID, now, now, id, b.ID)
	if err = maintenanceRowChanged(result, err); err != nil {
		return err
	}
	body := fmt.Sprintf("Local repair prepared for proposal %s. Commit %s; validated tree %s. Review the scoped changes and checks before applying them. Workflow recovery has not been observed.", id, artifact.CommitOID, artifact.TreeOID)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_comments(id,task_id,author_type,author_id,body,source,created_at) VALUES(?,?,'agent',?,?,'maintenance_receipt',?)`), uuid.NewString(), b.ConversationID, b.OrchestratorID, body, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_objectives SET status='ready_for_review',revision=revision+1,updated_at=?
 WHERE binding_id=? AND status='active' AND id=(SELECT objective_id FROM orchestration_improvements WHERE id=? AND binding_id=?)`), now, b.ID, id, b.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) IsMaintenanceTask(ctx context.Context, id string) (bool, error) {
	var count int
	err := r.db.GetContext(ctx, &count, r.db.Rebind(`SELECT count(*) FROM orchestration_improvements WHERE repair_task_id=? AND repair_task_id<>''`), id)
	return count > 0, err
}

func (r *Repository) IsMaintenanceObjective(ctx context.Context, binding, id string) (bool, error) {
	var count int
	err := r.db.GetContext(ctx, &count, r.db.Rebind(`SELECT count(*) FROM orchestration_improvements WHERE binding_id=? AND objective_id=? AND objective_id<>''`), binding, id)
	return count > 0, err
}
