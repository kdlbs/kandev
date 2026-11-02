package sandbox

import (
	"context"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

const (
	auditVerificationRunning     = "running"
	auditVerificationCompleted   = "completed"
	auditVerificationFailed      = "failed"
	auditVerificationInterrupted = reportFinalizationInterrupted
	auditVerificationCancelled   = "cancelled"
	auditVerificationRestarted   = "restarted"
)

func (s *Service) ReadAuditVerificationKey(ctx context.Context, org, id, epoch string, expected int64) ([]byte, error) {
	record, err := s.ownedAuditVerification(ctx, org, id, epoch, expected)
	if err != nil {
		return nil, err
	}
	if record.Verification.State != auditVerificationRunning {
		return nil, ErrConflict
	}
	return []byte(record.ExpectedPublicKeyPEM), nil
}

// ClaimAuditVerification is an internal worker operation fenced by its runtime epoch.
func (s *Service) ClaimAuditVerification(ctx context.Context, org, id, epoch string, expected int64) (*AuditVerification, error) {
	record, err := s.ownedAuditVerification(ctx, org, id, epoch, expected)
	if err != nil {
		return nil, err
	}
	job := record.Verification
	if job.State != auditVerificationPending {
		return nil, ErrConflict
	}
	now := time.Now().UTC()
	job.State, job.StartedAt = auditVerificationRunning, &now
	return s.updateAuditVerification(ctx, org, record, expected, auditVerificationPending)
}

// FinishAuditVerification preserves the captured input and trust provenance.
func (s *Service) FinishAuditVerification(ctx context.Context, org, id, epoch string, expected int64, result *AuditVerificationResult, failure string) (*AuditVerification, error) {
	record, err := s.ownedAuditVerification(ctx, org, id, epoch, expected)
	if err != nil {
		return nil, err
	}
	job := record.Verification
	previous := job.State
	if previous != auditVerificationPending && previous != auditVerificationRunning {
		return nil, ErrConflict
	}
	if result != nil {
		if previous != auditVerificationRunning || failure != "" || !validAuditVerificationResult(job, result) {
			return nil, contract.ErrInvalidPolicy
		}
		copy := *result
		job.Result, job.State = &copy, auditVerificationCompleted
	} else {
		if !validAuditVerificationFailure(failure) {
			return nil, contract.ErrInvalidPolicy
		}
		job.State, job.FailureCode = auditVerificationFailed, failure
		if failure == auditVerificationCancelled || failure == auditVerificationRestarted {
			job.State = auditVerificationInterrupted
		}
	}
	now := time.Now().UTC()
	job.CompletedAt = &now
	return s.updateAuditVerification(ctx, org, record, expected, previous)
}

func (s *Service) ownedAuditVerification(ctx context.Context, org, id, epoch string, expected int64) (*auditVerificationRecord, error) {
	record, err := s.readAuditVerification(ctx, org, id)
	if err != nil {
		return nil, err
	}
	if record.OwnerEpoch != epoch || record.Verification.Version != expected {
		return nil, ErrConflict
	}
	return record, nil
}

func (s *Service) updateAuditVerification(ctx context.Context, org string, record *auditVerificationRecord, expected int64, previous string) (*AuditVerification, error) {
	record.Verification.Version++
	var err error
	record.Digest, err = auditVerificationRecordDigest(*record)
	if err != nil {
		return nil, err
	}
	if !validAuditVerificationRecord(record) {
		return nil, contract.ErrInvalidPolicy
	}
	if err = s.writeAuditVerification(ctx, org, record, expected, previous); err != nil {
		return nil, err
	}
	return record.Verification, nil
}
