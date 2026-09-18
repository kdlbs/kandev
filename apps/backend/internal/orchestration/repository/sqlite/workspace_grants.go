package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
	"strings"
	"time"
)

func (r *Repository) WorkspaceGrants(ctx context.Context, binding, after string, limit int) ([]models.WorkspaceGrant, error) {
	rows := []models.WorkspaceGrant{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT g.* FROM orchestration_workspace_grants g
 JOIN orchestration_assistant_bindings b ON b.id=g.binding_id AND b.owner_user_id=g.owner_user_id
 WHERE g.binding_id=? AND g.id>? ORDER BY g.id LIMIT ?`), binding, after, min(max(limit, 1), 101))
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if err = json.Unmarshal([]byte(rows[i].ScopeJSON), &rows[i].Scope); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (r *Repository) WorkspaceGrant(ctx context.Context, binding, workspace string) (*models.WorkspaceGrant, error) {
	var row models.WorkspaceGrant
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`SELECT g.* FROM orchestration_workspace_grants g
 JOIN orchestration_assistant_bindings b ON b.id=g.binding_id AND b.owner_user_id=g.owner_user_id
 WHERE g.binding_id=? AND g.workspace_id=?`), binding, workspace)
	if err != nil {
		return nil, err
	}
	return &row, json.Unmarshal([]byte(row.ScopeJSON), &row.Scope)
}

func (r *Repository) SaveWorkspaceGrant(ctx context.Context, b *models.AssistantBinding, input *models.WorkspaceGrant, expected int64) error {
	if err := validateWorkspaceGrant(b, input, expected); err != nil {
		return err
	}
	g := *input
	raw, err := json.Marshal(g.Scope)
	if err != nil {
		return err
	}
	g.ScopeJSON = string(raw)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockAssistantBinding(ctx, tx, b); err != nil {
		return err
	}
	if err = prepareWorkspaceGrantRow(ctx, tx, b, &g, expected); err != nil {
		return err
	}
	_, err = tx.NamedExecContext(ctx, `INSERT INTO orchestration_workspace_grants
 (id,binding_id,owner_user_id,workspace_id,binding_version,revision,receiver_profile_id,receiver_profile_revision,authority_revision,scope_json,created_at,updated_at,revoked_at)
 VALUES(:id,:binding_id,:owner_user_id,:workspace_id,:binding_version,:revision,:receiver_profile_id,:receiver_profile_revision,:authority_revision,:scope_json,:created_at,:updated_at,NULL)
 ON CONFLICT(binding_id,workspace_id) DO UPDATE SET binding_version=excluded.binding_version,revision=excluded.revision,
 receiver_profile_id=excluded.receiver_profile_id,receiver_profile_revision=excluded.receiver_profile_revision,
 authority_revision=excluded.authority_revision,scope_json=excluded.scope_json,updated_at=excluded.updated_at,revoked_at=NULL`, g)
	if err != nil {
		return err
	}
	if err = recordWorkspaceGrantEvent(ctx, tx, g, "granted"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*input = g
	return nil
}

func validateWorkspaceGrant(b *models.AssistantBinding, g *models.WorkspaceGrant, expected int64) error {
	if b == nil || g == nil || expected < 0 || g.BindingVersion != b.Version {
		return models.ErrConflict
	}
	if g.WorkspaceID == b.WorkspaceID {
		return fmt.Errorf("home workspace does not require a linked grant")
	}
	for _, id := range []string{g.WorkspaceID, g.ReceiverProfileID, g.ReceiverProfileRevision, g.AuthorityRevision} {
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "\x00\r\n") {
			return fmt.Errorf("workspace grant requires explicit bounded identities")
		}
	}
	return g.Scope.Validate()
}

func prepareWorkspaceGrantRow(ctx context.Context, tx *sqlx.Tx, b *models.AssistantBinding, g *models.WorkspaceGrant, expected int64) error {
	var previous models.WorkspaceGrant
	err := tx.GetContext(ctx, &previous, tx.Rebind(`SELECT * FROM orchestration_workspace_grants WHERE binding_id=? AND workspace_id=?`), b.ID, g.WorkspaceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if previous.Revision != expected || (previous.ID != "" && previous.OwnerUserID != b.OwnerUserID) {
		return models.ErrConflict
	}
	g.ID, g.CreatedAt = previous.ID, previous.CreatedAt
	g.BindingID, g.OwnerUserID, g.Revision, g.UpdatedAt, g.RevokedAt = b.ID, b.OwnerUserID, expected+1, time.Now().UTC(), nil
	if g.ID == "" {
		g.ID, g.CreatedAt = uuid.NewString(), g.UpdatedAt
	}
	return nil
}
