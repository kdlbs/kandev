package sqlite

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// RecordWorkspaceExport serializes delivery receipts with grant revocation.
// Receipts conservatively record possible delivery, even if the transport fails.
func (r *Repository) RecordWorkspaceExport(ctx context.Context, b *models.AssistantBinding, grant *models.WorkspaceGrant, kind string) error {
	if b == nil || grant == nil || b.ConversationID == "" {
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
	var current models.WorkspaceGrant
	err = tx.GetContext(ctx, &current, tx.Rebind(`SELECT * FROM orchestration_workspace_grants WHERE id=? AND binding_id=? AND owner_user_id=?`), grant.ID, b.ID, b.OwnerUserID)
	if err != nil || current.Revision != grant.Revision || current.RevokedAt != nil || current.BindingVersion != b.Version || current.WorkspaceID != grant.WorkspaceID {
		return models.ErrConflict
	}
	if err = json.Unmarshal([]byte(current.ScopeJSON), &current.Scope); err != nil {
		return err
	}
	if !workspaceExportAllowed(current.Scope, kind) {
		return models.ErrConflict
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_workspace_exports
 (id,binding_id,owner_user_id,conversation_id,workspace_id,grant_id,grant_revision,receiver_profile_id,receiver_profile_revision,authority_revision,kind,created_at,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(binding_id,conversation_id,workspace_id,receiver_profile_id,receiver_profile_revision,authority_revision,kind)
 DO UPDATE SET grant_id=excluded.grant_id,grant_revision=excluded.grant_revision,updated_at=excluded.updated_at`),
		uuid.NewString(), b.ID, b.OwnerUserID, b.ConversationID, current.WorkspaceID, current.ID, current.Revision,
		current.ReceiverProfileID, current.ReceiverProfileRevision, current.AuthorityRevision, kind, now, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func workspaceExportAllowed(scope models.WorkspaceGrantScope, kind string) bool {
	return slices.Contains(scope.ContextExports, kind) || (kind == "workspace_link" && slices.Contains(scope.Operations, "observe"))
}

func (r *Repository) WorkspaceExports(ctx context.Context, binding, conversation, after string, limit int) ([]models.WorkspaceExport, error) {
	rows := []models.WorkspaceExport{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT e.* FROM orchestration_workspace_exports e
 JOIN orchestration_assistant_bindings b ON b.id=e.binding_id AND b.owner_user_id=e.owner_user_id
 WHERE e.binding_id=? AND e.conversation_id=? AND e.id>? ORDER BY e.id LIMIT ?`), binding, conversation, after, min(max(limit, 1), 101))
	return rows, err
}
