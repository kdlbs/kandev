package sandbox

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) initSchema() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS sandbox_policy_heads (
		org_id TEXT NOT NULL, policy_id TEXT NOT NULL, revision BIGINT NOT NULL,
		workspace_id TEXT NOT NULL, PRIMARY KEY (org_id,policy_id)
	);
	CREATE TABLE IF NOT EXISTS sandbox_policy_revisions (
		id TEXT PRIMARY KEY, org_id TEXT NOT NULL, policy_id TEXT NOT NULL,
		revision BIGINT NOT NULL, content_json TEXT NOT NULL, revoked INTEGER NOT NULL DEFAULT 0,
		UNIQUE (org_id,policy_id,revision)
	);
	CREATE TABLE IF NOT EXISTS sandbox_policy_bindings (
		org_id TEXT NOT NULL, scope TEXT NOT NULL, target_id TEXT NOT NULL,
		version BIGINT NOT NULL, content_json TEXT NOT NULL,
		PRIMARY KEY (org_id,scope,target_id)
	);
	CREATE TABLE IF NOT EXISTS sandbox_task_guards (
		org_id TEXT NOT NULL, task_id TEXT NOT NULL, version BIGINT NOT NULL,
		content_json TEXT NOT NULL, PRIMARY KEY (org_id,task_id)
	);
 CREATE TABLE IF NOT EXISTS sandbox_attempts (
 org_id TEXT NOT NULL, attempt_id TEXT NOT NULL, execution_id TEXT NOT NULL,
 generation BIGINT NOT NULL, version BIGINT NOT NULL, state TEXT NOT NULL,
 snapshot_json TEXT NOT NULL, owner_json TEXT NOT NULL, updated_at TEXT NOT NULL,
 PRIMARY KEY (org_id,attempt_id), UNIQUE (org_id,execution_id,generation)
 );
 CREATE UNIQUE INDEX IF NOT EXISTS sandbox_attempts_active_execution ON sandbox_attempts(org_id,execution_id)
 WHERE state IN ('prepared','starting','running','recovery_required');
 CREATE TABLE IF NOT EXISTS sandbox_approval_requests (
 org_id TEXT NOT NULL, attempt_id TEXT NOT NULL, request_id TEXT NOT NULL,
 version BIGINT NOT NULL, state TEXT NOT NULL, operator_id TEXT NOT NULL,
 expires_at_ms BIGINT NOT NULL, session_id TEXT NOT NULL, snapshot_json TEXT NOT NULL,
 PRIMARY KEY (org_id,attempt_id,request_id)
 );`)
	if err != nil {
		return err
	}
	if err = s.initAttemptSessions(); err != nil {
		return err
	}
	if err = s.initReports(); err != nil {
		return err
	}
	return s.initLearning()
}

func (s *Service) Publish(ctx context.Context, policy Policy, expected int64) (*Revision, error) {
	id, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.workspaceAccess(ctx, policy.WorkspaceID); err != nil {
		return nil, err
	}
	if expected < 0 || !validPolicyIdentity(policy) {
		return nil, contract.ErrInvalidPolicy
	}
	policy.Grants, err = contract.Canonicalize(policy.Grants, s.protectedRoots)
	if err != nil {
		return nil, err
	}
	if err = s.validateSources(ctx, policy); err != nil {
		return nil, err
	}
	content, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	if len(content) > 256<<10 {
		return nil, contract.ErrInvalidPolicy
	}
	digest := sha256.Sum256(content)
	revision := &Revision{ID: uuid.NewString(), Number: expected + 1, SchemaVersion: 1, Digest: hex.EncodeToString(digest[:]), Policy: policy, OperatorID: id.UserID, OrgID: id.OrgID, CreatedAt: time.Now().UTC()}
	raw, err := json.Marshal(revision)
	if err != nil {
		return nil, err
	}
	if err = s.insertRevision(ctx, revision, raw, expected); err != nil {
		return nil, err
	}
	return revision, nil
}

func (s *Service) insertRevision(ctx context.Context, revision *Revision, raw []byte, expected int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = insertRevisionTx(ctx, tx, revision, raw, expected); err != nil {
		return err
	}
	return tx.Commit()
}

func insertRevisionTx(ctx context.Context, tx *sqlx.Tx, revision *Revision, raw []byte, expected int64) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO sandbox_policy_heads (org_id,policy_id,revision,workspace_id) VALUES (?,?,0,?) ON CONFLICT (org_id,policy_id) DO NOTHING`), revision.OrgID, revision.Policy.ID, revision.Policy.WorkspaceID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE sandbox_policy_heads SET revision=? WHERE org_id=? AND policy_id=? AND revision=? AND workspace_id=?`), revision.Number, revision.OrgID, revision.Policy.ID, expected, revision.Policy.WorkspaceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO sandbox_policy_revisions (id,org_id,policy_id,revision,content_json) VALUES (?,?,?,?,?)`), revision.ID, revision.OrgID, revision.Policy.ID, revision.Number, string(raw))
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) GetRevision(ctx context.Context, id string) (*Revision, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.Instance || identity.UserID == "" {
		return nil, ErrNotFound
	}
	revision, err := s.readRevision(ctx, identity.OrgID, id)
	if err != nil {
		return nil, err
	}
	if err = s.workspaceAccess(ctx, revision.Policy.WorkspaceID); err != nil {
		return nil, ErrNotFound
	}
	return revision, nil
}

func (s *Service) GetLatest(ctx context.Context, policyID string) (*Revision, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.Instance || identity.UserID == "" {
		return nil, ErrNotFound
	}
	var id string
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT r.id FROM sandbox_policy_revisions r JOIN sandbox_policy_heads h ON r.org_id=h.org_id AND r.policy_id=h.policy_id AND r.revision=h.revision WHERE h.org_id=? AND h.policy_id=?`), identity.OrgID, policyID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetRevision(ctx, id)
}

// Revoke blocks future admission without changing immutable revision content.
func (s *Service) Revoke(ctx context.Context, id, digest string) error {
	identity, err := s.operator(ctx)
	if err != nil {
		return err
	}
	revision, err := s.GetRevision(ctx, id)
	if err != nil {
		return err
	}
	if digest == "" || revision.Digest != digest {
		return ErrConflict
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`UPDATE sandbox_policy_revisions SET revoked=1 WHERE org_id=? AND id=?`), identity.OrgID, id)
	return err
}

func (s *Service) readRevision(ctx context.Context, orgID, id string) (*Revision, error) {
	var raw string
	var revoked int
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT content_json,revoked FROM sandbox_policy_revisions WHERE org_id=? AND id=?`), orgID, id).Scan(&raw, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var revision Revision
	if err = json.Unmarshal([]byte(raw), &revision); err != nil {
		return nil, fmt.Errorf("decode sandbox policy: %w", err)
	}
	revision.Revoked = revoked != 0
	return &revision, nil
}

func validPolicyIdentity(policy Policy) bool {
	for _, value := range []string{policy.ID, policy.Name, policy.InstallationID, policy.ProviderKey} {
		if value == "" || len(value) > 128 || strings.ContainsFunc(value, unicode.IsControl) {
			return false
		}
	}
	return len(policy.Sources) <= 32
}

func (s *Service) validateSources(ctx context.Context, policy Policy) error {
	for _, source := range policy.Sources {
		if source.Kind != "policy_revision" {
			return contract.ErrInvalidPolicy
		}
		revision, err := s.GetRevision(ctx, source.ID)
		if err != nil || revision.Revoked || revision.Digest != source.Digest || revision.Policy.InstallationID != policy.InstallationID || revision.Policy.ProviderKey != policy.ProviderKey {
			return contract.ErrInvalidPolicy
		}
		if err = contract.WithinCeiling(revision.Policy.Grants, policy.Grants); err != nil {
			return contract.ErrInvalidPolicy
		}
	}
	return nil
}
