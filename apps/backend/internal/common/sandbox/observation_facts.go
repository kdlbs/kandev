package sandbox

import "time"

// ExecutionFacts inherits the enclosing observation's attempt and generation.
// Process ownership, preparation, runtime reports and audit trust are separate claims.
type ExecutionFacts struct {
	SchemaVersion int              `json:"schema_version"`
	Process       ProcessFact      `json:"process"`
	LaunchChecks  LaunchChecksFact `json:"launch_checks"`
	Runtime       RuntimeFacts     `json:"runtime"`
	Audit         AuditFacts       `json:"audit"`
}

type ProcessFact struct {
	State      string     `json:"state"`
	CheckedAt  *time.Time `json:"checked_at,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

type LaunchChecksFact struct {
	State   string              `json:"state"`
	Receipt *LaunchCheckReceipt `json:"receipt,omitempty"`
}

// LaunchCheckReceipt records preparation only. It does not attest kernel confinement.
type LaunchCheckReceipt struct {
	CheckedAt     time.Time `json:"checked_at"`
	PolicyDigest  string    `json:"policy_digest"`
	PackageDigest string    `json:"package_digest"`
	RuntimeDigest string    `json:"runtime_digest"`
	Checks        []string  `json:"checks"`
}

type RuntimeFacts struct {
	State        string               `json:"state"`
	Observations []RuntimeObservation `json:"observations,omitempty"`
}

type RuntimeObservation struct {
	SourceID               string    `json:"source_id"`
	EvidenceDigest         string    `json:"evidence_digest"`
	ObservedAt             time.Time `json:"observed_at"`
	Platform               string    `json:"platform"`
	LandlockABI            *uint32   `json:"landlock_abi,omitempty"`
	ExecuteEnforced        *bool     `json:"execute_enforced,omitempty"`
	CommandMediationActive *bool     `json:"command_mediation_active,omitempty"`
	Limitations            []string  `json:"limitations,omitempty"`
}

type AuditFacts struct {
	Integrity    string                 `json:"integrity"`
	Finalization string                 `json:"finalization"`
	Signer       string                 `json:"signer"`
	Verification *AuditVerificationFact `json:"verification,omitempty"`
}

type AuditVerificationFact struct {
	ID                string    `json:"id"`
	CompletedAt       time.Time `json:"completed_at"`
	ArtifactSetDigest string    `json:"artifact_set_digest"`
	VerifierVersion   string    `json:"verifier_version"`
	SignatureState    string    `json:"signature_state"`
	LedgerState       string    `json:"ledger_state"`
	TrustRevision     string    `json:"trust_revision,omitempty"`
}

// InvalidateLiveFacts preserves immutable receipts and historical runtime observations.
func (o *Observation) InvalidateLiveFacts(now time.Time, ownerChanged bool) {
	if o.Facts == nil {
		return
	}
	process := &o.Facts.Process
	if process.State == ProcessRunning && (ownerChanged || process.ValidUntil == nil || !now.Before(*process.ValidUntil)) {
		process.State, process.ValidUntil = StatusUnknown, nil
	}
	if ownerChanged && o.Facts.Runtime.State == RuntimeReported {
		o.Facts.Runtime.State = FactUnavailable
	}
	if o.Facts.Audit.Signer == AuditTrusted && ownerChanged {
		o.Facts.Audit.Signer = AuditUntrusted
	}
}
