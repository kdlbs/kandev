package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// FinalAuditReceipt identifies retained private bytes independently of report interpretation.
type FinalAuditReceipt struct {
	BindingDigest     string    `json:"binding_digest"`
	ArtifactSetDigest string    `json:"artifact_set_digest"`
	CapturedAt        time.Time `json:"captured_at"`
}

// AuditBindingDigest binds historical evidence to the immutable launch snapshot
// and captured runtime owner. Later policy changes cannot change this identity.
func AuditBindingDigest(attempt *Attempt) (string, error) {
	if attempt == nil || attempt.ID == "" || attempt.Snapshot.Provider.ProtocolVersion() != 2 || attempt.Owner == nil || attempt.Owner.ProviderSessionID == "" {
		return "", ErrConflict
	}
	raw, err := json.Marshal(struct {
		ID       string
		Snapshot AttemptSnapshot
		Owner    *AttemptOwner
	}{attempt.ID, attempt.Snapshot, attempt.Owner})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func validFinalAuditReceipt(receipt *FinalAuditReceipt, finalized bool, captured time.Time) bool {
	return receipt == nil || finalized && validApprovalDigest(receipt.BindingDigest) && validApprovalDigest(receipt.ArtifactSetDigest) && !receipt.CapturedAt.IsZero() && !receipt.CapturedAt.After(captured)
}

func auditReceiptMatchesAttempt(receipt *FinalAuditReceipt, attempt *Attempt) bool {
	if receipt == nil {
		return true
	}
	binding, err := AuditBindingDigest(attempt)
	return err == nil && receipt.BindingDigest == binding
}
