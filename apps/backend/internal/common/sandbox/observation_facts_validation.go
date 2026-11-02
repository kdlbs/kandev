package sandbox

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (f *ExecutionFacts) valid(observation Observation) bool {
	if f == nil {
		return true
	}
	if f.SchemaVersion != 1 || !f.Process.valid(observation) || !f.LaunchChecks.valid(observation.CheckedAt) || !f.Runtime.valid(observation.CheckedAt) {
		return false
	}
	return f.Audit.valid(observation.CheckedAt)
}

func (a AuditFacts) valid(now time.Time) bool {
	if !slices.Contains([]string{AuditUnchecked, AuditValid, AuditInvalid}, a.Integrity) ||
		!slices.Contains([]string{StatusUnknown, "live", AuditFinal, "interrupted"}, a.Finalization) ||
		!slices.Contains([]string{AuditUnchecked, AuditUnsigned, AuditUntrusted, AuditTrusted, AuditInvalid}, a.Signer) {
		return false
	}
	v := a.Verification
	if v == nil {
		return true
	}
	if !v.valid(now) {
		return false
	}
	switch v.SignatureState {
	case AuditVerified:
		return a.Integrity == AuditValid && a.Finalization == AuditFinal && v.TrustRevision != "" && (a.Signer == AuditTrusted || a.Signer == AuditUntrusted)
	case AuditUnsigned, AuditInvalid, AuditUntrusted:
		return a.Signer == v.SignatureState
	default:
		return a.Signer == AuditUnchecked
	}
}

func (v AuditVerificationFact) valid(now time.Time) bool {
	if !ValidMechanismKey(v.ID) || v.CompletedAt.IsZero() || v.CompletedAt.After(now) || !validLaunchDigest(v.ArtifactSetDigest) {
		return false
	}
	if v.VerifierVersion == "" || len(v.VerifierVersion) > 128 || !utf8.ValidString(v.VerifierVersion) || strings.ContainsFunc(v.VerifierVersion, unicode.IsControl) {
		return false
	}
	return (v.TrustRevision == "" || ValidMechanismKey(v.TrustRevision)) &&
		slices.Contains([]string{AuditUnchecked, AuditUnsigned, AuditUntrusted, AuditVerified, AuditInvalid, "unsupported"}, v.SignatureState) &&
		slices.Contains([]string{"unavailable", AuditValid, AuditInvalid}, v.LedgerState)
}

func (p ProcessFact) valid(observation Observation) bool {
	if !slices.Contains([]string{ProcessRunning, StatusStopped, StatusUnknown}, p.State) {
		return false
	}
	if p.CheckedAt != nil && (p.CheckedAt.IsZero() || p.CheckedAt.After(observation.CheckedAt)) {
		return false
	}
	if p.State != ProcessRunning {
		return p.ValidUntil == nil
	}
	return observation.Status != StatusStopped && p.CheckedAt != nil && p.ValidUntil != nil &&
		p.ValidUntil.Equal(p.CheckedAt.Add(30*time.Second)) && observation.CheckedAt.Before(*p.ValidUntil)
}

func (f LaunchChecksFact) valid(now time.Time) bool {
	if f.State == FactUnavailable {
		return f.Receipt == nil
	}
	return f.State == "passed" && f.Receipt.Valid(now)
}

func (r *LaunchCheckReceipt) Valid(now time.Time) bool {
	if r == nil || r.CheckedAt.IsZero() || r.CheckedAt.After(now) || !validLaunchDigest(r.PolicyDigest) || !validLaunchDigest(r.PackageDigest) || !validLaunchDigest(r.RuntimeDigest) {
		return false
	}
	expected := []string{"policy_ceiling", "runtime_checksum", "private_artifacts", "process_containment"}
	return slices.Equal(r.Checks, expected)
}

func (r RuntimeFacts) valid(now time.Time) bool {
	if !slices.Contains([]string{RuntimeReported, RuntimeMissing, "unsupported", FactUnavailable}, r.State) || len(r.Observations) > 32 {
		return false
	}
	if r.State == RuntimeReported && len(r.Observations) == 0 {
		return false
	}
	for _, fact := range r.Observations {
		if !ValidMechanismKey(fact.SourceID) || !validLaunchDigest(fact.EvidenceDigest) || fact.ObservedAt.IsZero() || fact.ObservedAt.After(now) || !slices.Contains([]string{"linux", "macos"}, fact.Platform) || len(fact.Limitations) > 32 {
			return false
		}
		for _, limitation := range fact.Limitations {
			if !ValidMechanismKey(limitation) {
				return false
			}
		}
	}
	return true
}
