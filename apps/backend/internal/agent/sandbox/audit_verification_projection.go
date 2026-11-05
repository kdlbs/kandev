package sandbox

import (
	"context"
	"database/sql"
	"errors"
)

// ReadCapturedAuditVerification reads historical evidence for the trusted observer.
func (s *Service) ReadCapturedAuditVerification(ctx context.Context, org, attemptID string) (*AuditVerificationView, error) {
	attempt, err := s.ReadAttempt(ctx, org, attemptID)
	if err != nil {
		return nil, err
	}
	var id string
	err = s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT id FROM sandbox_audit_verifications WHERE org_id=? AND attempt_id=? AND state='completed' ORDER BY requested_at_ms DESC,id DESC LIMIT 1`), org, attemptID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	record, err := s.readAuditVerification(ctx, org, id)
	if err != nil {
		return nil, err
	}
	report, err := s.ReadCapturedReport(ctx, org, attemptID)
	if err != nil {
		return nil, err
	}
	if !auditVerificationMatchesReport(record.Verification, report) {
		return nil, ErrConflict
	}
	snapshot := attempt.Snapshot
	trust, err := s.readAuditTrust(ctx, auditTrustScope{OrgID: org, WorkspaceID: snapshot.WorkspaceID, InstallationID: snapshot.Provider.InstallationID, ProviderKey: snapshot.Provider.Key})
	if err != nil {
		return nil, err
	}
	return &AuditVerificationView{Verification: record.Verification, SignerTrustedNow: auditSignerTrustedNow(record.Verification, trust)}, nil
}
