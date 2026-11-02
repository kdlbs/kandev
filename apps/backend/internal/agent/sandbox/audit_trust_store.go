package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Service) initAuditTrust() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sandbox_audit_trust_heads (
	org_id TEXT NOT NULL,scope_digest TEXT NOT NULL,version BIGINT NOT NULL,revision_id TEXT NOT NULL,
	PRIMARY KEY(org_id,scope_digest));
	CREATE TABLE IF NOT EXISTS sandbox_audit_trust_revisions (
	org_id TEXT NOT NULL,scope_digest TEXT NOT NULL,id TEXT NOT NULL,version BIGINT NOT NULL,content_json TEXT NOT NULL,
	PRIMARY KEY(org_id,id),UNIQUE(org_id,scope_digest,version));`)
	return err
}

func (s *Service) insertAuditTrust(ctx context.Context, scope auditTrustScope, revision *AuditTrustRevision, expected int64) error {
	raw, err := json.Marshal(revision)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	key := scope.digest()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO sandbox_audit_trust_heads(org_id,scope_digest,version,revision_id) VALUES(?,?,0,'') ON CONFLICT(org_id,scope_digest) DO NOTHING`), scope.OrgID, key)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE sandbox_audit_trust_heads SET version=?,revision_id=? WHERE org_id=? AND scope_digest=? AND version=?`), revision.Version, revision.ID, scope.OrgID, key, expected)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO sandbox_audit_trust_revisions(org_id,scope_digest,id,version,content_json) VALUES(?,?,?,?,?)`), scope.OrgID, key, revision.ID, revision.Version, string(raw))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) readAuditTrust(ctx context.Context, scope auditTrustScope) (*AuditTrustRevision, error) {
	var version int64
	var id string
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT h.version,h.revision_id,r.content_json FROM sandbox_audit_trust_heads h LEFT JOIN sandbox_audit_trust_revisions r ON r.org_id=h.org_id AND r.scope_digest=h.scope_digest AND r.id=h.revision_id WHERE h.org_id=? AND h.scope_digest=?`), scope.OrgID, scope.digest()).Scan(&version, &id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return scope.revision(), nil
	}
	if err != nil {
		return nil, err
	}
	var value AuditTrustRevision
	if !raw.Valid || len(raw.String) > 32<<10 || json.Unmarshal([]byte(raw.String), &value) != nil || !validAuditTrustRevision(value, scope, id, version) {
		return nil, ErrConflict
	}
	return &value, nil
}

func validAuditTrustRevision(value AuditTrustRevision, scope auditTrustScope, id string, version int64) bool {
	if !validAuditTrustHeader(value, scope, id, version) {
		return false
	}
	digest, err := auditTrustDigest(value)
	if err != nil || digest != value.Digest {
		return false
	}
	if value.Revoked {
		return value.PublicKeyPEM == "" && value.Fingerprint == ""
	}
	key, fingerprint, err := canonicalAuditPublicKey(value.PublicKeyPEM)
	return err == nil && key == value.PublicKeyPEM && fingerprint == value.Fingerprint
}

func validAuditTrustHeader(value AuditTrustRevision, scope auditTrustScope, id string, version int64) bool {
	storedScope := auditTrustScope{value.OrgID, value.WorkspaceID, value.InstallationID, value.ProviderKey}
	return value.SchemaVersion == 1 && value.ID == id && id != "" && value.Version == version && version > 0 && storedScope == scope && value.OperatorID != "" && value.Source == auditTrustSourceOperator && !value.CreatedAt.IsZero() && !value.CreatedAt.After(time.Now().Add(5*time.Second))
}
