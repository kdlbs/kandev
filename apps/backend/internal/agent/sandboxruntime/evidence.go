package sandboxruntime

import (
	"context"
	"slices"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const ownershipUnavailableReason = "ownership_unavailable"

type Observation = contract.Observation

func (m *Manager) Observe(ctx context.Context, orgID, attemptID string) (*Observation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	attempt, err := m.policies.ReadAttempt(ctx, orgID, attemptID)
	if err != nil {
		return nil, err
	}
	result := initialObservation(attempt)
	if attempt.State == policy.AttemptStopped || attempt.State == policy.AttemptFailed {
		return result, nil
	}
	provider := attempt.Snapshot.Provider
	binding, err := m.selectProvider(ctx, provider.InstallationID, provider.Key)
	if err != nil || binding.Version != provider.Version || binding.PackageDigest != provider.PackageDigest {
		result.Status, result.ProviderHealth, result.Reason = contract.StatusUnknown, contract.HealthUnavailable, "provider_unavailable"
		return result, nil
	}
	result.ProviderHealth = contract.HealthAvailable
	if attempt.State != policy.AttemptRunning {
		return result, nil
	}
	return m.observeRunning(ctx, attempt, result)
}

func (m *Manager) observeRunning(ctx context.Context, attempt *policy.Attempt, result *Observation) (*Observation, error) {
	prepared, err := m.retained(attempt.Snapshot.OrgID, attempt.ID)
	if err != nil || m.VerifyLive(ctx, attempt) != nil {
		result.Reason = ownershipUnavailableReason
		return result, nil
	}
	evidence, err := prepared.readStatusEvidence(ctx, attempt)
	if err != nil || evidence == nil || ctx.Err() != nil {
		return result, nil
	}
	current, err := m.policies.ReadAttempt(ctx, attempt.Snapshot.OrgID, attempt.ID)
	if err != nil {
		return nil, err
	}
	if current.Version != attempt.Version {
		return initialObservation(current), nil
	}
	if m.VerifyLive(ctx, current) != nil || ctx.Err() != nil {
		result.Reason = ownershipUnavailableReason
		return result, nil
	}
	now := time.Now().UTC()
	if !boundConfinementEvidence(evidence, current, now) {
		return result, nil
	}
	observed := time.UnixMilli(evidence.ObservedAtUnixMs).UTC()
	until := observed.Add(30 * time.Second)
	result.Status, result.Reason, result.Mechanism = contract.StatusActive, "", evidence.Mechanism
	result.CheckedAt, result.EvidenceAt, result.ValidUntil = now, &observed, &until
	return result, nil
}

func boundConfinementEvidence(evidence *pluginsdk.SandboxStatusEvidence, attempt *policy.Attempt, now time.Time) bool {
	if evidence == nil || attempt == nil || attempt.State != policy.AttemptRunning || attempt.Owner == nil || attempt.Snapshot.Launch == nil {
		return false
	}
	if evidence.SchemaVersion != 1 || evidence.AttemptId != attempt.ID || evidence.Generation != attempt.Snapshot.Launch.Generation || evidence.PolicyDigest != attempt.Snapshot.Launch.PolicyDigest {
		return false
	}
	process := evidence.Process
	if process == nil || process.Pid != int64(attempt.Owner.SupervisorPID) || process.BirthId != attempt.Owner.SupervisorBirth {
		return false
	}
	return freshConfinementEvidence(evidence, now)
}

func freshConfinementEvidence(evidence *pluginsdk.SandboxStatusEvidence, now time.Time) bool {
	observed := time.UnixMilli(evidence.ObservedAtUnixMs)
	return evidence.ObservedAtUnixMs > 0 && !observed.After(now) && now.Before(observed.Add(30*time.Second)) && contract.ValidMechanismKey(evidence.Mechanism)
}

func initialObservation(attempt *policy.Attempt) *Observation {
	result := &Observation{AttemptID: attempt.ID, Generation: attempt.Snapshot.Launch.Generation,
		AttemptVersion: attempt.Version, Status: contract.StatusUnknown, ProviderHealth: contract.HealthUnknown, Reason: "evidence_unavailable", CheckedAt: time.Now().UTC()}
	switch attempt.State {
	case policy.AttemptPrepared:
		result.Status, result.Reason = contract.StatusConfigured, ""
	case policy.AttemptStarting:
		result.Status, result.Reason = contract.StatusStarting, ""
	case policy.AttemptStopped:
		result.Status, result.Reason = contract.StatusStopped, ""
	case policy.AttemptFailed:
		result.Status, result.Reason = contract.StatusFailed, "launch_failed"
	case policy.AttemptRecoveryRequired:
		result.Reason = ownershipUnavailableReason
	}
	return result
}

func (p *Prepared) readStatusEvidence(ctx context.Context, attempt *policy.Attempt) (*pluginsdk.SandboxStatusEvidence, error) {
	var matched *pluginsdk.SandboxStatusEvidence
	for _, source := range p.description.RuntimeSources {
		if source.Kind != "status" || !slices.Contains(p.description.EvidenceSchemas, source.Key) {
			continue
		}
		records, err := contract.ReadRuntimeSource(ctx, p.files.Directories.State, contract.RuntimeSource{
			Key: source.Key, Kind: source.Kind, Directory: source.RelativeDirectory, Suffix: source.FilenameSuffix,
			MaxDepth: source.MaxDepth, MaxFiles: source.MaxFiles, MaxBytes: source.MaxBytes})
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			response, err := p.manager.providers.NormalizeSandboxStatus(ctx, p.binding, &pluginsdk.NormalizeSandboxStatusRequest{
				Context: p.operation(), Policy: p.policy, Process: &pluginsdk.SandboxProcessIdentity{Pid: int64(attempt.Owner.SupervisorPID), BirthId: attempt.Owner.SupervisorBirth}, Evidence: record.Content, EvidenceSchema: source.Key})
			if err != nil {
				return nil, err
			}
			if !boundConfinementEvidence(response.Evidence, attempt, time.Now()) {
				continue
			}
			if matched != nil {
				return nil, policy.ErrConflict
			}
			matched = response.Evidence
		}
	}
	return matched, nil
}
