package sandboxruntime

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kdlbs/kandev/pluginsdk"
)

func (p *Prepared) typedReports() bool {
	return p.binding.ProtocolVersion() == 2 && slices.Contains(p.description.GetCapabilitiesV2().GetReportSchemas(), pluginsdk.SandboxReportSchemaV2)
}

func (p *Prepared) readTypedReport(ctx context.Context, attempt *policy.Attempt, previous *policy.Report) (policy.ReportData, error) {
	data := policy.ReportData{Status: contract.FactUnavailable, Integrity: pluginsdk.SandboxReportUnverified, SupportedClasses: []string{reportClassFilesystem}, V2: &policy.TypedReport{SchemaVersion: 2, Integrity: reportAuditUnchecked, Finalization: "unknown", Coverage: unavailableCoverage()}}
	if attempt.Owner == nil || attempt.Owner.ProviderSessionID == "" {
		return data, nil
	}
	terminal := attempt.State == policy.AttemptStopped || attempt.State == policy.AttemptFailed
	archive, err := contract.FreezeAuditSources(ctx, p.files.Directories.State, p.files.Directories.Temp, p.auditDeclarations(terminal), previousAuditSources(previous))
	if err != nil {
		return data, err
	}
	defer func() { _ = archive.Close() }()
	if len(archive.Sources) == 0 {
		return data, nil
	}
	if terminal {
		data.FinalAudit, err = p.retainFinalAudit(ctx, attempt, archive)
		if err != nil {
			return data, err
		}
	}
	data.V2.ArtifactSetDigest = archive.Digest
	for _, source := range archive.Sources {
		data.V2.Sources = append(data.V2.Sources, policy.TypedReportSource{ID: source.ID, Schema: source.Schema, Digest: source.Digest, Identity: source.Identity, Bytes: source.Size})
		data.Sources = append(data.Sources, policy.ReportSource{ID: source.ID, Schema: source.Schema, Digest: source.Digest, Integrity: pluginsdk.SandboxReportPartial})
	}
	if err = p.consumeTypedArchive(ctx, attempt.Owner.ProviderSessionID, archive, &data); err != nil {
		return data, err
	}
	data.Status, data.Integrity = reportStatusAvailable, pluginsdk.SandboxReportPartial
	if data.V2.Integrity == "invalid" {
		return data, contract.ErrInvalidPolicy
	}
	// Byte integrity does not establish complete collection coverage.
	for _, coverage := range data.V2.Coverage {
		if coverage.Coverage == contract.FactUnavailable {
			data.Status = contract.FactUnavailable
		}
	}
	return data, nil
}

func (p *Prepared) auditDeclarations(terminal bool) []contract.RuntimeSource {
	var sources []contract.RuntimeSource
	for _, source := range p.description.RuntimeSources {
		if (source.Kind != contract.RuntimeSourceAudit && source.Kind != contract.RuntimeSourceAuditFinal) || (source.Kind == contract.RuntimeSourceAuditFinal && !terminal) {
			continue
		}
		sources = append(sources, contract.RuntimeSource{Key: source.Key, Kind: source.Kind, Directory: source.RelativeDirectory, Suffix: source.FilenameSuffix, MaxDepth: source.MaxDepth, MaxFiles: source.MaxFiles, MaxBytes: source.MaxBytes})
	}
	return sources
}

func previousAuditSources(previous *policy.Report) []contract.AuditSourceReceipt {
	var sources []contract.AuditSourceReceipt
	if previous != nil && previous.Data.V2 != nil {
		for _, source := range previous.Data.V2.Sources {
			sources = append(sources, contract.AuditSourceReceipt{ID: source.ID, Schema: source.Schema, Digest: source.Digest, Identity: source.Identity, Size: source.Bytes})
		}
	}
	return sources
}

func (p *Prepared) consumeTypedArchive(ctx context.Context, session string, archive *contract.AuditArchive, data *policy.ReportData) error {
	handle := uuid.NewString()
	var sequence uint64
	for index, source := range archive.Sources {
		var offset int64
		for {
			content, final, err := archive.ReadChunk(ctx, index, offset, 256<<10)
			if err != nil {
				return err
			}
			chunk := &pluginsdk.SandboxAuditChunkV2{OperationHandle: handle, ArtifactSetDigest: archive.Digest, SourceId: source.ID, Sequence: sequence, Content: content, Final: final, TotalBytes: uint64(source.Size), SourceDigest: source.Digest, SourceSchema: source.Schema, SourceRelativePath: source.Name, ProviderSessionId: session, SourceIndex: uint32(index), SourceCount: uint32(len(archive.Sources))}
			response, err := p.manager.providers.ReadSandboxReport(ctx, p.binding, &pluginsdk.ReadSandboxReportRequest{Context: p.operation(), Policy: p.policy, ChunkV2: chunk})
			if err != nil {
				return err
			}
			if response.ReportV2 == nil {
				return contract.ErrInvalidPolicy
			}
			if err = appendTypedResponse(data, response.ReportV2); err != nil {
				return err
			}
			offset += int64(len(content))
			sequence++
			if final {
				break
			}
		}
	}
	return nil
}

func appendTypedResponse(data *policy.ReportData, response *pluginsdk.SandboxReportV2) error {
	data.V2.Integrity, data.V2.Finalization = response.Integrity, response.Finalization
	data.V2.Coverage = nil
	for _, coverage := range response.Coverage {
		data.V2.Coverage = append(data.V2.Coverage, policy.ReportCoverage{Kind: coverage.OperationKind, Coverage: coverage.Coverage})
	}
	if len(data.V2.Coverage) != 4 {
		return contract.ErrInvalidPolicy
	}
	for _, event := range response.Events {
		appendTypedEvent(data, event)
	}
	raw, err := json.Marshal(data)
	if err != nil || len(raw) > 240<<10 || len(data.V2.Records) > 1000 {
		return policy.ErrIneligible
	}
	return nil
}

func unavailableCoverage() []policy.ReportCoverage {
	var result []policy.ReportCoverage
	for _, kind := range []string{reportClassFilesystem, reportClassNetwork, reportClassCommand, "http_endpoint"} {
		result = append(result, policy.ReportCoverage{Kind: kind, Coverage: contract.FactUnavailable})
	}
	return result
}

func reportRefreshError(err error) string {
	if errors.Is(err, plugins.ErrSandboxReportUnsupported) {
		return reportStatusUnsupported
	}
	if errors.Is(err, contract.ErrInvalidPolicy) {
		return "invalid_evidence"
	}
	if errors.Is(err, policy.ErrIneligible) {
		return "limit_exceeded"
	}
	return contract.FactUnavailable
}

func appendTypedEvent(data *policy.ReportData, event *pluginsdk.SandboxAuditEventV2) {
	record := policy.ReportEvent{ID: reportHash([]byte(event.SourceId + "\x00" + event.RecordId)), SourceID: event.SourceId, ProviderRecordID: event.RecordId, Sequence: event.Sequence, Decision: event.Decision, ReasonCode: event.ReasonCode, OccurredAt: time.UnixMilli(event.OccurredAtUnixMs).UTC(), ExactTarget: event.ExactTarget}
	record.Kind, record.Operation = projectReportOperation(event.Operation)
	switch {
	case !record.ExactTarget:
		record.LearningBlocker = "incomplete_identity"
	case record.Decision != policy.ApprovalDenied && record.Decision != reportApprovalDenied:
		record.LearningBlocker = "not_denied"
	case record.Kind != reportClassFilesystem:
		record.LearningBlocker = "unsupported_policy"
	}
	if record.LearningBlocker == "" && record.Operation.Filesystem != nil {
		access := record.Operation.Filesystem.Access
		if access == "read_write" {
			access = "write"
		}
		record.LegacyRecordID = record.ID
		data.Records = append(data.Records, policy.DenialRecord{ID: record.ID, SourceID: record.SourceID, ProviderRecordID: record.ProviderRecordID, Kind: record.Kind, Operation: access, Target: record.Operation.Filesystem.Path, OccurredAt: record.OccurredAt})
	}
	data.V2.Records = append(data.V2.Records, record)
}
