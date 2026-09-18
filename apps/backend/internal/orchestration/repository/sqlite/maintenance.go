package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) SaveMaintenanceGrant(ctx context.Context, b *models.AssistantBinding, grant *models.MaintenanceGrant, expected, candidateRevision int64) error {
	if grant.BindingVersion != b.Version || expected < 0 {
		return models.ErrConflict
	}
	if err := grant.Scope.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if !grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(7*24*time.Hour)) {
		return fmt.Errorf("maintenance grant expires within seven days")
	}
	raw, err := json.Marshal(grant.Scope)
	if err != nil {
		return err
	}
	grant.ScopeJSON = string(raw)
	grant.BindingID, grant.OwnerUserID, grant.WorkspaceID = b.ID, b.OwnerUserID, b.WorkspaceID
	grant.Revision, grant.UpdatedAt, grant.CreatedAt, grant.RevokedAt = expected+1, now, now, nil
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockAssistantBinding(ctx, tx, b); err != nil {
		return err
	}
	candidate, err := maintenanceGrantCandidate(ctx, tx, b, grant.CandidateID, expected, candidateRevision)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_maintenance_grants
 (candidate_id,binding_id,owner_user_id,workspace_id,binding_version,authority_revision,profile_revision,revision,scope_json,base_oid,expires_at,created_at,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(candidate_id) DO UPDATE SET binding_version=excluded.binding_version,
 authority_revision=excluded.authority_revision,profile_revision=excluded.profile_revision,revision=excluded.revision,
 scope_json=excluded.scope_json,base_oid=excluded.base_oid,expires_at=excluded.expires_at,revoked_at=NULL,updated_at=excluded.updated_at
 WHERE orchestration_maintenance_grants.binding_id=excluded.binding_id AND orchestration_maintenance_grants.revision=?`),
		grant.CandidateID, b.ID, b.OwnerUserID, b.WorkspaceID, b.Version, grant.AuthorityRevision, grant.ProfileRevision,
		grant.Revision, grant.ScopeJSON, grant.BaseOID, grant.ExpiresAt, now, now, expected)
	if err = maintenanceRowChanged(result, err); err != nil {
		return err
	}
	if candidate.ObjectiveID == "" {
		if err = createMaintenanceObjective(ctx, tx, b, grant, candidate); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func maintenanceGrantCandidate(ctx context.Context, tx *sqlx.Tx, b *models.AssistantBinding, id string, expected, candidateRevision int64) (*models.ImprovementCandidate, error) {
	var candidate models.ImprovementCandidate
	if err := tx.GetContext(ctx, &candidate, tx.Rebind(`SELECT * FROM orchestration_improvements WHERE binding_id=? AND id=?`), b.ID, id); err != nil {
		return nil, err
	}
	if candidate.Revision != candidateRevision || candidate.State != "proposed" {
		return nil, models.ErrConflict
	}
	var revision int64
	if err := tx.GetContext(ctx, &revision, tx.Rebind(`SELECT COALESCE((SELECT revision FROM orchestration_maintenance_grants WHERE candidate_id=? AND binding_id=?),0)`), id, b.ID); err != nil {
		return nil, err
	}
	if revision != expected {
		return nil, models.ErrConflict
	}
	return &candidate, nil
}

func (r *Repository) MaintenanceGrant(ctx context.Context, binding, candidate string) (*models.MaintenanceGrant, error) {
	var row models.MaintenanceGrant
	if err := r.db.GetContext(ctx, &row, r.db.Rebind(`SELECT * FROM orchestration_maintenance_grants WHERE binding_id=? AND candidate_id=?`), binding, candidate); err != nil {
		return nil, err
	}
	return &row, json.Unmarshal([]byte(row.ScopeJSON), &row.Scope)
}

func (r *Repository) RevokeMaintenanceGrant(ctx context.Context, b *models.AssistantBinding, candidate string, expected int64) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_maintenance_grants SET revision=revision+1,revoked_at=?,updated_at=?
 WHERE candidate_id=? AND binding_id=? AND owner_user_id=? AND revision=?
 AND EXISTS(SELECT 1 FROM orchestration_assistant_bindings b WHERE b.id=? AND b.version=? AND b.owner_user_id=?)`),
		time.Now().UTC(), time.Now().UTC(), candidate, b.ID, b.OwnerUserID, expected, b.ID, b.Version, b.OwnerUserID)
	return maintenanceRowChanged(result, err)
}

func maintenanceRowChanged(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return models.ErrConflict
	}
	return err
}
