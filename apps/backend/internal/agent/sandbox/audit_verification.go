package sandbox

import (
	"context"
	"time"

	"github.com/google/uuid"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

type AuditVerificationRequest struct {
	RequestID              string `json:"request_id"`
	ExpectedReportDigest   string `json:"expected_report_digest"`
	ExpectedArtifactDigest string `json:"expected_artifact_digest"`
	ExpectedTrustVersion   int64  `json:"expected_trust_version"`
}

type AuditTrustProvenance struct {
	RevisionID   string    `json:"revision_id"`
	Version      int64     `json:"version"`
	Fingerprint  string    `json:"fingerprint"`
	Revoked      bool      `json:"revoked"`
	Source       string    `json:"source"`
	OperatorID   string    `json:"operator_id"`
	ConfiguredAt time.Time `json:"configured_at"`
}

type AuditVerification struct {
	SchemaVersion     int                      `json:"schema_version"`
	ID                string                   `json:"id"`
	Version           int64                    `json:"version"`
	State             string                   `json:"state"`
	AttemptID         string                   `json:"attempt_id"`
	SessionID         string                   `json:"session_id"`
	ProviderSessionID string                   `json:"provider_session_id"`
	ReportDigest      string                   `json:"report_digest"`
	ArtifactSetDigest string                   `json:"artifact_set_digest"`
	BindingDigest     string                   `json:"binding_digest"`
	Provider          AttemptProvider          `json:"provider"`
	Trust             AuditTrustProvenance     `json:"trust"`
	RequestedBy       string                   `json:"requested_by"`
	RequestedAt       time.Time                `json:"requested_at"`
	StartedAt         *time.Time               `json:"started_at,omitempty"`
	CompletedAt       *time.Time               `json:"completed_at,omitempty"`
	FailureCode       string                   `json:"failure_code,omitempty"`
	Result            *AuditVerificationResult `json:"result,omitempty"`
}

type AuditVerificationResult struct {
	ArtifactSetDigest      string `json:"artifact_set_digest"`
	VerifierVersion        string `json:"verifier_version"`
	SessionID              string `json:"session_id"`
	EventRoot              string `json:"event_root"`
	EventCount             uint64 `json:"event_count"`
	ChainState             string `json:"chain_state"`
	Finalization           string `json:"finalization"`
	SignatureState         string `json:"signature_state"`
	ExpectedKeyFingerprint string `json:"expected_key_fingerprint"`
	SignerKeyFingerprint   string `json:"signer_key_fingerprint"`
	LedgerState            string `json:"ledger_state"`
	TrustRevision          string `json:"trust_revision"`
}

type AuditVerificationView struct {
	Verification     *AuditVerification `json:"verification"`
	SignerTrustedNow bool               `json:"signer_trusted_now"`
}

func (s *Service) BeginAuditVerification(ctx context.Context, sessionID, attemptID string, request AuditVerificationRequest, ownerEpoch string) (*AuditVerification, error) {
	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	if !validAuditVerificationRequest(request, ownerEpoch) {
		return nil, contract.ErrInvalidPolicy
	}
	report, err := s.ReadReport(ctx, sessionID, attemptID)
	if err != nil {
		return nil, err
	}
	if !report.Data.Finalized || report.Data.FinalAudit == nil {
		return nil, ErrIneligible
	}
	trust, err := s.GetAuditTrust(ctx, sessionID, attemptID)
	if err != nil {
		return nil, err
	}
	if !auditVerificationRequestMatches(request, report, trust) {
		return nil, ErrConflict
	}
	attempt, err := s.ReadAttempt(ctx, operator.OrgID, attemptID)
	if err != nil {
		return nil, err
	}
	requestDigest, err := auditVerificationRequestDigest(request)
	if err != nil {
		return nil, err
	}
	job := &AuditVerification{SchemaVersion: 1, ID: uuid.NewString(), Version: 1, State: auditVerificationPending, AttemptID: attemptID, SessionID: sessionID, ProviderSessionID: attempt.Owner.ProviderSessionID, ReportDigest: report.Digest, ArtifactSetDigest: request.ExpectedArtifactDigest, BindingDigest: report.Data.FinalAudit.BindingDigest, Provider: attempt.Snapshot.Provider, RequestedBy: operator.UserID, RequestedAt: time.Now().UTC(), Trust: AuditTrustProvenance{RevisionID: trust.ID, Version: trust.Version, Fingerprint: trust.Fingerprint, Revoked: trust.Revoked, Source: trust.Source, OperatorID: trust.OperatorID, ConfiguredAt: trust.CreatedAt}}
	record := &auditVerificationRecord{Verification: job, RequestID: request.RequestID, RequestDigest: requestDigest, OwnerEpoch: ownerEpoch, ExpectedPublicKeyPEM: trust.PublicKeyPEM}
	return s.admitAuditVerification(ctx, operator.OrgID, record)
}

func (s *Service) admitAuditVerification(ctx context.Context, org string, record *auditVerificationRecord) (*AuditVerification, error) {
	prior, err := s.readAuditVerificationNonce(ctx, org, record.Verification.AttemptID, record.Verification.RequestedBy, record.RequestID)
	if err == nil {
		return auditVerificationRetry(prior, record.RequestDigest)
	}
	if err != ErrNotFound {
		return nil, err
	}
	if err = s.insertAuditVerification(ctx, org, record); err != nil {
		if err != ErrConflict {
			return nil, err
		}
		retry, retryErr := s.readAuditVerificationNonce(ctx, org, record.Verification.AttemptID, record.Verification.RequestedBy, record.RequestID)
		if retryErr == nil {
			return auditVerificationRetry(retry, record.RequestDigest)
		}
		return nil, ErrConflict
	}
	return record.Verification, nil
}

func (s *Service) ReadAuditVerification(ctx context.Context, sessionID, attemptID, id string) (*AuditVerificationView, error) {
	org, err := s.authorizeReport(ctx, sessionID, attemptID)
	if err != nil {
		return nil, err
	}
	record, err := s.readAuditVerification(ctx, org, id)
	if err != nil {
		return nil, err
	}
	job := record.Verification
	if job.AttemptID != attemptID || job.SessionID != sessionID {
		return nil, ErrNotFound
	}
	report, err := s.ReadCapturedReport(ctx, org, attemptID)
	if err != nil {
		return nil, err
	}
	if !auditVerificationMatchesReport(job, report) {
		return nil, ErrConflict
	}
	trust, err := s.GetAuditTrust(ctx, sessionID, attemptID)
	if err != nil {
		return nil, err
	}
	return &AuditVerificationView{Verification: job, SignerTrustedNow: auditSignerTrustedNow(job, trust)}, nil
}

func validAuditVerificationRequest(request AuditVerificationRequest, epoch string) bool {
	return validAuditUUID(request.RequestID) && validAuditUUID(epoch) && validApprovalDigest(request.ExpectedReportDigest) && validApprovalDigest(request.ExpectedArtifactDigest) && request.ExpectedTrustVersion >= 0
}

func auditVerificationRequestMatches(request AuditVerificationRequest, report *Report, trust *AuditTrustRevision) bool {
	return report.Digest == request.ExpectedReportDigest && report.Data.FinalAudit.ArtifactSetDigest == request.ExpectedArtifactDigest && trust.Version == request.ExpectedTrustVersion
}
