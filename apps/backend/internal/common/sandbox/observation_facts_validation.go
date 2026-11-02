package sandbox

import (
	"slices"
	"time"
)

func (f *ExecutionFacts) valid(observation Observation) bool {
	if f == nil {
		return true
	}
	if f.SchemaVersion != 1 || !f.Process.valid(observation) || !f.LaunchChecks.valid(observation.CheckedAt) || !f.Runtime.valid(observation.CheckedAt) {
		return false
	}
	return slices.Contains([]string{"unchecked", "valid", "invalid"}, f.Audit.Integrity) &&
		slices.Contains([]string{StatusUnknown, "live", "final", "interrupted"}, f.Audit.Finalization) &&
		slices.Contains([]string{"unchecked", "unsigned", "untrusted", "trusted", "invalid"}, f.Audit.Signer)
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
