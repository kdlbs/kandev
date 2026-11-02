package sandboxruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
)

func initialFacts(attempt *policy.Attempt, checkedAt time.Time) *contract.ExecutionFacts {
	facts := &contract.ExecutionFacts{SchemaVersion: 1, Process: contract.ProcessFact{State: contract.StatusUnknown}, LaunchChecks: contract.LaunchChecksFact{State: contract.FactUnavailable}, Runtime: contract.RuntimeFacts{State: reportStatusUnsupported}, Audit: contract.AuditFacts{Integrity: reportAuditUnchecked, Finalization: "unknown", Signer: reportAuditUnchecked}}
	if receipt := attempt.Snapshot.LaunchChecks; receipt != nil {
		facts.LaunchChecks = contract.LaunchChecksFact{State: "passed", Receipt: receipt}
	}
	if attempt.State == policy.AttemptStopped {
		now := checkedAt
		facts.Process = contract.ProcessFact{State: contract.StatusStopped, CheckedAt: &now}
	}
	return facts
}

func (m *Manager) observeProcess(ctx context.Context, attempt *policy.Attempt, result *Observation) {
	if attempt.State != policy.AttemptRunning || attempt.Owner == nil {
		return
	}
	prepared, err := m.retained(attempt.Snapshot.OrgID, attempt.ID)
	if err != nil || prepared.verifyProcesses(ctx, attempt.Owner) != nil {
		return
	}
	now := time.Now().UTC()
	until := now.Add(30 * time.Second)
	result.CheckedAt = now
	result.Facts.Process = contract.ProcessFact{State: "running", CheckedAt: &now, ValidUntil: &until}
}

func (p *Prepared) readStatusFacts(ctx context.Context, attempt *policy.Attempt, result *Observation) (*pluginsdk.SandboxStatusEvidence, error) {
	var matched *pluginsdk.SandboxStatusEvidence
	for _, source := range p.description.RuntimeSources {
		legacy := source.Kind == "status" && slices.Contains(p.description.EvidenceSchemas, source.Key)
		typed := p.binding.ProtocolVersion() == 2 && slices.Contains(p.description.GetCapabilitiesV2().GetObservationSchemas(), source.Key)
		if !legacy && !typed {
			continue
		}
		if typed && result.Facts.Runtime.State != contract.RuntimeReported {
			result.Facts.Runtime.State = contract.RuntimeMissing
		}
		records, err := contract.ReadRuntimeSource(ctx, p.files.Directories.State, contract.RuntimeSource{Key: source.Key, Kind: source.Kind, Directory: source.RelativeDirectory, Suffix: source.FilenameSuffix, MaxDepth: source.MaxDepth, MaxFiles: source.MaxFiles, MaxBytes: min(source.MaxBytes, 1<<20)})
		if err != nil {
			result.Facts.Runtime.State = contract.FactUnavailable
			return nil, err
		}
		for _, record := range records {
			request := &pluginsdk.NormalizeSandboxStatusRequest{Context: p.operation(), Policy: p.policy, Process: &pluginsdk.SandboxProcessIdentity{Pid: int64(attempt.Owner.SupervisorPID), BirthId: attempt.Owner.SupervisorBirth}, Evidence: record.Content, EvidenceSchema: source.Key}
			if typed {
				request.SourceRelativePathV2 = record.Name
				request.ProviderSessionIdV2 = attempt.Owner.ProviderSessionID
			}
			response, err := p.manager.providers.NormalizeSandboxStatus(ctx, p.binding, request)
			if err != nil {
				result.Facts.Runtime.State = contract.FactUnavailable
				return nil, err
			}
			if typed {
				appendRuntimeFacts(result, response.ObservationsV2, record.Content)
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

func appendRuntimeFacts(result *Observation, observations []*pluginsdk.SandboxRuntimeObservationV2, content []byte) {
	digest := sha256.Sum256(content)
	for _, fact := range observations {
		if len(result.Facts.Runtime.Observations) >= 32 || fact.ObservedAtUnixMs > time.Now().UnixMilli() {
			continue
		}
		observation := contract.RuntimeObservation{SourceID: fact.SourceId, EvidenceDigest: hex.EncodeToString(digest[:]), ObservedAt: time.UnixMilli(fact.ObservedAtUnixMs).UTC(), Platform: fact.Platform, LandlockABI: fact.LandlockAbi, ExecuteEnforced: fact.ExecuteEnforced, CommandMediationActive: fact.CommandMediationActive}
		// Public runtime limitations are closed identifiers, never provider log text.
		for _, limitation := range fact.Limitations {
			if contract.ValidMechanismKey(limitation) {
				observation.Limitations = append(observation.Limitations, limitation)
			}
		}
		result.Facts.Runtime.Observations = append(result.Facts.Runtime.Observations, observation)
		result.Facts.Runtime.State = "reported"
	}
}
