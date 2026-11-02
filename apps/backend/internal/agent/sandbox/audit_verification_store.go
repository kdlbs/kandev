package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) initAuditVerifications() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sandbox_audit_verifications (
	org_id TEXT NOT NULL,id TEXT NOT NULL,attempt_id TEXT NOT NULL,request_id TEXT NOT NULL,
	operator_id TEXT NOT NULL,version BIGINT NOT NULL,state TEXT NOT NULL,owner_epoch TEXT NOT NULL,
	requested_at_ms BIGINT NOT NULL,content_json TEXT NOT NULL,PRIMARY KEY(org_id,id),
	UNIQUE(org_id,attempt_id,operator_id,request_id),
	FOREIGN KEY(org_id,attempt_id) REFERENCES sandbox_attempts(org_id,attempt_id) ON DELETE CASCADE);
	CREATE UNIQUE INDEX IF NOT EXISTS sandbox_audit_verifications_active ON sandbox_audit_verifications(org_id,attempt_id) WHERE state IN ('pending','running');
	CREATE INDEX IF NOT EXISTS sandbox_audit_verifications_history ON sandbox_audit_verifications(org_id,attempt_id,requested_at_ms,id);`)
	return err
}

func (s *Service) insertAuditVerification(ctx context.Context, org string, record *auditVerificationRecord) error {
	var err error
	record.Digest, err = auditVerificationRecordDigest(*record)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	job := record.Verification
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO sandbox_audit_verifications (org_id,id,attempt_id,request_id,operator_id,version,state,owner_epoch,requested_at_ms,content_json) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`), org, job.ID, job.AttemptID, record.RequestID, job.RequestedBy, job.Version, job.State, record.OwnerEpoch, job.RequestedAt.UnixMilli(), string(raw))
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
	return nil
}

func (s *Service) readAuditVerificationNonce(ctx context.Context, org, attempt, operator, nonce string) (*auditVerificationRecord, error) {
	var id string
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT id FROM sandbox_audit_verifications WHERE org_id=? AND attempt_id=? AND operator_id=? AND request_id=?`), org, attempt, operator, nonce).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.readAuditVerification(ctx, org, id)
}

func (s *Service) readAuditVerification(ctx context.Context, org, id string) (*auditVerificationRecord, error) {
	var raw, attempt, nonce, operator, state, epoch string
	var version, requested int64
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT attempt_id,request_id,operator_id,version,state,owner_epoch,requested_at_ms,content_json FROM sandbox_audit_verifications WHERE org_id=? AND id=?`), org, id).Scan(&attempt, &nonce, &operator, &version, &state, &epoch, &requested, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var record auditVerificationRecord
	if len(raw) > 64<<10 || json.Unmarshal([]byte(raw), &record) != nil || !validAuditVerificationRecord(&record) {
		return nil, ErrConflict
	}
	job := record.Verification
	if job.ID != id || job.AttemptID != attempt || job.RequestedBy != operator || job.Version != version || job.State != state || job.RequestedAt.UnixMilli() != requested || record.RequestID != nonce || record.OwnerEpoch != epoch {
		return nil, ErrConflict
	}
	return &record, nil
}

func validAuditVerificationRecord(record *auditVerificationRecord) bool {
	job := record.Verification
	if !validAuditVerificationHeader(job) {
		return false
	}
	if !validAuditUUID(record.RequestID) || !validAuditUUID(record.OwnerEpoch) || !validApprovalDigest(record.RequestDigest) {
		return false
	}
	digest, err := auditVerificationRecordDigest(*record)
	if err != nil || digest != record.Digest {
		return false
	}
	if job.Trust.Revoked {
		return record.ExpectedPublicKeyPEM == "" && job.Trust.Fingerprint == ""
	}
	key, fingerprint, err := canonicalAuditPublicKey(record.ExpectedPublicKeyPEM)
	return err == nil && key == record.ExpectedPublicKeyPEM && fingerprint == job.Trust.Fingerprint
}

func (s *Service) writeAuditVerification(ctx context.Context, org string, record *auditVerificationRecord, expected int64, previous string) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	job := record.Verification
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE sandbox_audit_verifications SET version=?,state=?,content_json=? WHERE org_id=? AND id=? AND owner_epoch=? AND version=? AND state=?`), job.Version, job.State, string(raw), org, job.ID, record.OwnerEpoch, expected, previous)
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
	return nil
}
