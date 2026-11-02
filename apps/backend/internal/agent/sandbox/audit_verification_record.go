package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
)

const auditVerificationPending = "pending"

type auditVerificationRecord struct {
	Verification         *AuditVerification `json:"verification"`
	RequestID            string             `json:"request_id"`
	RequestDigest        string             `json:"request_digest"`
	OwnerEpoch           string             `json:"owner_epoch"`
	ExpectedPublicKeyPEM string             `json:"expected_public_key_pem"`
	Digest               string             `json:"digest"`
}

func validAuditUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && value == parsed.String()
}

func auditVerificationRequestDigest(value AuditVerificationRequest) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func auditVerificationRecordDigest(value auditVerificationRecord) (string, error) {
	value.Digest = ""
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func auditVerificationRetry(record *auditVerificationRecord, digest string) (*AuditVerification, error) {
	if record.RequestDigest != digest {
		return nil, ErrConflict
	}
	return record.Verification, nil
}

func auditVerificationMatchesReport(job *AuditVerification, report *Report) bool {
	receipt := report.Data.FinalAudit
	return report.Data.Finalized && receipt != nil && job.AttemptID == report.AttemptID && job.SessionID == report.SessionID && job.ReportDigest == report.Digest && job.Provider == report.Provider && job.ArtifactSetDigest == receipt.ArtifactSetDigest && job.BindingDigest == receipt.BindingDigest
}
