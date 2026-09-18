package sqlite

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
	"time"
)

func (r *Repository) AttentionTargets(ctx context.Context, taskID, after string, limit int) ([]models.AttentionTarget, error) {
	rows := []models.AttentionTarget{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT DISTINCT b.id AS binding_id,l.task_id,b.id || ':' || l.task_id AS cursor
 FROM orchestration_assistant_bindings b JOIN orchestration_objectives o ON o.binding_id=b.id
 JOIN orchestration_objective_tasks l ON l.objective_id=o.id JOIN tasks t ON t.id=l.task_id
 WHERE t.workspace_id=o.workspace_id AND (t.workspace_id=b.workspace_id OR EXISTS (
 SELECT 1 FROM orchestration_workspace_grants g WHERE g.binding_id=b.id AND g.workspace_id=t.workspace_id
 AND g.owner_user_id=b.owner_user_id AND g.binding_version=b.version AND g.revoked_at IS NULL))
 AND t.id<>b.conversation_id AND (?='' OR t.id=?)
 AND (b.id || ':' || l.task_id)>? ORDER BY cursor LIMIT ?`), taskID, taskID, after, min(max(limit, 1), 100))
	return rows, err
}
func (r *Repository) AssistantBindingByID(ctx context.Context, id string) (*models.AssistantBinding, error) {
	var b models.AssistantBinding
	err := r.ro.GetContext(ctx, &b, r.ro.Rebind(`SELECT * FROM orchestration_assistant_bindings WHERE id=?`), id)
	return &b, err
}
func (r *Repository) AttentionPage(ctx context.Context, binding, after string, limit int) ([]models.Attention, error) {
	rows := []models.Attention{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT * FROM orchestration_attention WHERE binding_id=? AND id>? ORDER BY id LIMIT ?`), binding, after, min(max(limit, 1), 101))
	return rows, err
}
func (r *Repository) AttentionForTask(ctx context.Context, binding, task string) ([]models.Attention, error) {
	rows := []models.Attention{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT * FROM orchestration_attention WHERE binding_id=? AND task_id=? ORDER BY id LIMIT 1001`), binding, task)
	if len(rows) > 1000 {
		return nil, fmt.Errorf("attention source budget exceeded")
	}
	return rows, err
}
func (r *Repository) AttentionByID(ctx context.Context, binding, id string) (*models.Attention, error) {
	var row models.Attention
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`SELECT * FROM orchestration_attention WHERE binding_id=? AND id=?`), binding, id)
	return &row, err
}

// Projection and wake insertion share a transaction. Native occurrence identity
// survives unknown reads, enqueue acknowledgement loss and process restart.
func (r *Repository) ProjectAttention(ctx context.Context, b *models.AssistantBinding, task string, sources []models.AttentionSource, now time.Time) (bool, error) {
	return r.ProjectWorkspaceAttention(ctx, b, nil, task, sources, now)
}

func (r *Repository) ProjectWorkspaceAttention(ctx context.Context, b *models.AssistantBinding, grant *models.WorkspaceGrant, task string, sources []models.AttentionSource, now time.Time) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockAssistantBinding(ctx, tx, b); err != nil {
		return false, err
	}
	scoped := *b
	if grant != nil {
		var current int
		err = tx.GetContext(ctx, &current, tx.Rebind(`SELECT count(*) FROM orchestration_workspace_grants WHERE id=? AND binding_id=? AND owner_user_id=? AND binding_version=? AND revision=? AND revoked_at IS NULL`), grant.ID, b.ID, b.OwnerUserID, b.Version, grant.Revision)
		if err != nil {
			return false, err
		}
		if current != 1 {
			return false, models.ErrConflict
		}
		scoped.WorkspaceID = grant.WorkspaceID
	}
	changed := false
	for _, source := range sources {
		updated, err := projectAttentionSource(ctx, tx, &scoped, task, source, now)
		if err != nil {
			return false, err
		}
		changed = changed || updated
	}
	return changed, tx.Commit()
}
func projectAttentionSource(ctx context.Context, tx *sqlx.Tx, b *models.AssistantBinding, task string, s models.AttentionSource, now time.Time) (bool, error) {
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(b.ID+":"+task+":"+s.SessionID+":"+s.Kind+":"+s.SourceID)).String()
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_attention
 (id,binding_id,workspace_id,task_id,session_id,source_id,kind,state,source_revision,summary,revision,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,1,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,source_revision=excluded.source_revision,
 summary=excluded.summary,revision=orchestration_attention.revision+1,updated_at=excluded.updated_at
 WHERE orchestration_attention.source_revision<>excluded.source_revision OR orchestration_attention.state<>excluded.state OR orchestration_attention.summary<>excluded.summary`),
		id, b.ID, b.WorkspaceID, task, s.SessionID, s.SourceID, s.Kind, s.State, s.SourceRevision, s.Summary, now)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	wake := uuid.NewSHA1(uuid.NameSpaceOID, []byte(id+":"+s.SourceRevision)).String()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_attention_wakes(id,attention_id,source_revision,revision,state)
 SELECT ?,id,source_revision,revision,'pending' FROM orchestration_attention WHERE id=? AND state='pending'
 ON CONFLICT(attention_id,source_revision) DO NOTHING`), wake, id)
	return true, err
}
func (r *Repository) PendingAttentionWakes(ctx context.Context, binding, task string) ([]models.AttentionWake, error) {
	rows := []models.AttentionWake{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT w.* FROM orchestration_attention_wakes w JOIN orchestration_attention a ON a.id=w.attention_id
 WHERE a.binding_id=? AND a.task_id=? AND a.state='pending' AND a.source_revision=w.source_revision AND w.state='pending' ORDER BY w.id LIMIT 100`), binding, task)
	return rows, err
}
func (r *Repository) AcknowledgeAttentionWake(ctx context.Context, w models.AttentionWake) error {
	return r.AcknowledgeAttentionWakes(ctx, []models.AttentionWake{w})
}
func (r *Repository) AcknowledgeAttentionWakes(ctx context.Context, wakes []models.AttentionWake) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, w := range wakes {
		if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_attention_wakes SET state='acknowledged' WHERE id=? AND state='pending'`), w.ID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_attention SET last_notified_revision=revision WHERE id=? AND source_revision=?`), w.AttentionID, w.SourceRevision); err != nil {
			return err
		}
	}
	return tx.Commit()
}
