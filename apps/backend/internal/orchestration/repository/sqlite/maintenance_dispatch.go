package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) ReserveMaintenanceRepair(ctx context.Context, b *models.AssistantBinding, g models.MaintenanceGrant, candidateRevision, intent int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockAssistantBinding(ctx, tx, b); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_improvements SET state='investigating',revision=revision+1,updated_at=?
 WHERE id=? AND binding_id=? AND workspace_id=? AND revision=? AND state='proposed' AND repair_task_id='' AND objective_id<>''
 AND EXISTS(SELECT 1 FROM orchestration_maintenance_grants g WHERE g.candidate_id=orchestration_improvements.id AND g.revision=? AND g.binding_version=? AND g.revoked_at IS NULL AND g.expires_at>?)
 AND COALESCE((SELECT revision FROM orchestration_conversation_intents WHERE task_id=?),0)=?`), time.Now().UTC(), g.CandidateID, b.ID, b.WorkspaceID, candidateRevision, g.Revision, b.Version, time.Now().UTC(), b.ConversationID, intent)
	if err = maintenanceRowChanged(result, err); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_objectives SET intent_revision=?,updated_at=? WHERE binding_id=? AND id=(SELECT objective_id FROM orchestration_improvements WHERE id=? AND binding_id=?)`), intent, time.Now().UTC(), b.ID, g.CandidateID, b.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) RecordMaintenanceTask(ctx context.Context, binding, candidate, task, contextRef, operation string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_improvements SET repair_task_id=?,revision=revision+1,updated_at=?
 WHERE id=? AND binding_id=? AND state='investigating' AND (repair_task_id='' OR repair_task_id=?)`), task, time.Now().UTC(), candidate, binding, task)
	if err = maintenanceRowChanged(result, err); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_objective_tasks(objective_id,task_id,session_id,role,context_ref,operation_id)
 SELECT objective_id,?,'','workflow_maintenance',?,? FROM orchestration_improvements WHERE id=? AND binding_id=? ON CONFLICT DO NOTHING`), task, contextRef, operation, candidate, binding)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) UnknownMaintenanceRepair(ctx context.Context, binding, candidate string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_improvements SET state='unknown',revision=revision+1,updated_at=? WHERE binding_id=? AND id=? AND state='investigating' AND repair_task_id=''`), time.Now().UTC(), binding, candidate)
	return err
}

func (r *Repository) RecoverMaintenance(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE orchestration_improvements SET state='unknown',revision=revision+1,updated_at=CURRENT_TIMESTAMP
 WHERE state='investigating' AND (repair_task_id='' OR EXISTS(SELECT 1 FROM orchestration_operations o
 WHERE o.binding_id=orchestration_improvements.binding_id AND o.state='unknown'
 AND o.target IN ('/api/v1/orchestration/assistant/improvements/' || orchestration_improvements.id || '/maintenance',
 '/api/v1/orchestration/runtime/improvements/' || orchestration_improvements.id || '/maintenance')))`)
	return err
}
