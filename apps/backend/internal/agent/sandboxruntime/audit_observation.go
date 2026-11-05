package sandboxruntime

import (
	"context"
	"errors"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (m *Manager) observeAudit(ctx context.Context, attempt *policy.Attempt, observation *Observation) error {
	view, err := m.policies.ReadCapturedAuditVerification(ctx, attempt.Snapshot.OrgID, attempt.ID)
	if errors.Is(err, policy.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	job, result := view.Verification, view.Verification.Result
	if result == nil || job.CompletedAt == nil {
		return policy.ErrConflict
	}
	observation.Facts.Audit = contract.AuditFacts{Integrity: result.ChainState, Finalization: result.Finalization, Signer: auditSignerFact(result.SignatureState, view.SignerTrustedNow), Verification: &contract.AuditVerificationFact{ID: job.ID, CompletedAt: *job.CompletedAt, ArtifactSetDigest: result.ArtifactSetDigest, VerifierVersion: result.VerifierVersion, SignatureState: result.SignatureState, LedgerState: result.LedgerState, TrustRevision: result.TrustRevision}}
	return nil
}

func auditSignerFact(signature string, trusted bool) string {
	switch signature {
	case contract.AuditVerified:
		if trusted {
			return contract.AuditTrusted
		}
		return contract.AuditUntrusted
	case contract.AuditUnsigned, contract.AuditUntrusted, contract.AuditInvalid:
		return signature
	default:
		return contract.AuditUnchecked
	}
}
