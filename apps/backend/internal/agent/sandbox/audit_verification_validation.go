package sandbox

import "slices"

func validAuditVerificationHeader(job *AuditVerification) bool {
	if !validAuditVerificationBase(job) {
		return false
	}
	if !validApprovalDigest(job.ReportDigest) || !validApprovalDigest(job.ArtifactSetDigest) || !validApprovalDigest(job.BindingDigest) || !reportText(job.ProviderSessionID, 256, false) {
		return false
	}
	return validAuditVerificationTimes(job) && validAuditVerificationState(job)
}

func validAuditVerificationBase(job *AuditVerification) bool {
	return job != nil && job.SchemaVersion == 1 && validAuditUUID(job.ID) && job.Version >= 1 && job.RequestedBy != "" && !job.RequestedAt.IsZero()
}

func validAuditVerificationTimes(job *AuditVerification) bool {
	if job.StartedAt != nil && job.StartedAt.Before(job.RequestedAt) {
		return false
	}
	return job.CompletedAt == nil || !job.CompletedAt.Before(job.RequestedAt) && (job.StartedAt == nil || !job.CompletedAt.Before(*job.StartedAt))
}

func validAuditVerificationState(job *AuditVerification) bool {
	switch job.State {
	case auditVerificationPending:
		return job.Version == 1 && job.StartedAt == nil && validActiveAuditVerification(job)
	case auditVerificationRunning:
		return job.Version >= 2 && job.StartedAt != nil && validActiveAuditVerification(job)
	case auditVerificationCompleted:
		return validCompletedAuditVerification(job)
	case auditVerificationFailed, auditVerificationInterrupted:
		return validFailedAuditVerification(job)
	default:
		return false
	}
}

func validActiveAuditVerification(job *AuditVerification) bool {
	return job.CompletedAt == nil && job.Result == nil && job.FailureCode == ""
}

func validCompletedAuditVerification(job *AuditVerification) bool {
	return job.Version >= 3 && job.StartedAt != nil && job.CompletedAt != nil && job.FailureCode == "" && validAuditVerificationResult(job, job.Result)
}

func validFailedAuditVerification(job *AuditVerification) bool {
	interrupted := job.FailureCode == auditVerificationCancelled || job.FailureCode == auditVerificationRestarted
	return job.Version >= 2 && job.CompletedAt != nil && job.Result == nil && validAuditVerificationFailure(job.FailureCode) && (job.State == auditVerificationInterrupted) == interrupted
}

func validAuditVerificationFailure(value string) bool {
	return slices.Contains([]string{reportStatusUnavailable, reportStatusUnsupported, "invalid_evidence", "timeout", "limit_exceeded", auditVerificationCancelled, auditVerificationRestarted}, value)
}

func validAuditVerificationResult(job *AuditVerification, result *AuditVerificationResult) bool {
	if !validAuditResultHeader(job, result) {
		return false
	}
	return validAuditResultStates(result) && validAuditResultSignature(job, result) && auditResultTrustMatches(job, result)
}

func validAuditResultHeader(job *AuditVerification, result *AuditVerificationResult) bool {
	return result != nil && result.ArtifactSetDigest == job.ArtifactSetDigest && result.SessionID == job.ProviderSessionID && validApprovalDigest(result.EventRoot) && reportText(result.VerifierVersion, 128, false)
}

func validAuditResultStates(result *AuditVerificationResult) bool {
	return slices.Contains([]string{auditStateUnchecked, auditStateValid, auditStateInvalid}, result.ChainState) && slices.Contains([]string{reportFinalizationFinal, reportFinalizationInterrupted}, result.Finalization) && slices.Contains([]string{auditStateUnchecked, auditStateUnsigned, auditStateUntrusted, auditStateVerified, auditStateInvalid, reportStatusUnsupported}, result.SignatureState) && slices.Contains([]string{reportStatusUnavailable, auditStateValid, auditStateInvalid}, result.LedgerState)
}

func validAuditResultSignature(job *AuditVerification, result *AuditVerificationResult) bool {
	if result.SignerKeyFingerprint != "" && !validApprovalDigest(result.SignerKeyFingerprint) {
		return false
	}
	return result.SignatureState != auditStateVerified || !job.Trust.Revoked && result.ChainState == auditStateValid && result.Finalization == reportFinalizationFinal && result.SignerKeyFingerprint == job.Trust.Fingerprint && validApprovalDigest(job.Trust.Fingerprint)
}

func auditResultTrustMatches(job *AuditVerification, result *AuditVerificationResult) bool {
	if job.Trust.Revoked {
		return result.ExpectedKeyFingerprint == "" && result.TrustRevision == ""
	}
	return result.ExpectedKeyFingerprint == job.Trust.Fingerprint && result.TrustRevision == job.Trust.RevisionID
}

func auditSignerTrustedNow(job *AuditVerification, trust *AuditTrustRevision) bool {
	return job.State == auditVerificationCompleted && job.Result != nil && job.Result.SignatureState == auditStateVerified && !trust.Revoked && trust.ID == job.Trust.RevisionID && trust.Version == job.Trust.Version && trust.Fingerprint == job.Trust.Fingerprint
}
