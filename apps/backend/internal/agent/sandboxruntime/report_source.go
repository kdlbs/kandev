package sandboxruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
)

const sandboxAccessRead = "read"

func (p *Prepared) readReportData(ctx context.Context) (policy.ReportData, error) {
	data := policy.ReportData{Status: "unsupported", Integrity: pluginsdk.SandboxReportUnverified, SupportedClasses: slices.Clone(p.description.ReportClasses)}
	if len(data.SupportedClasses) == 0 {
		return data, nil
	}
	data.Status, data.Integrity, data.Complete = "available", "complete", true
	for _, source := range p.description.RuntimeSources {
		if source.Kind != "audit" {
			continue
		}
		files, err := contract.ReadRuntimeSource(ctx, p.files.Directories.State, contract.RuntimeSource{
			Key: source.Key, Kind: source.Kind, Directory: source.RelativeDirectory, Suffix: source.FilenameSuffix,
			MaxDepth: source.MaxDepth, MaxFiles: source.MaxFiles, MaxBytes: source.MaxBytes})
		if err != nil {
			return policy.ReportData{}, err
		}
		for _, file := range files {
			if err = p.appendReportSource(ctx, &data, source.Key, file); err != nil {
				return policy.ReportData{}, err
			}
		}
	}
	if len(data.Sources) == 0 {
		data.Status, data.Integrity, data.Complete = "unavailable", pluginsdk.SandboxReportUnverified, false
	}
	return data, nil
}

func (p *Prepared) appendReportSource(ctx context.Context, data *policy.ReportData, schema string, file contract.RuntimeRecord) error {
	response, err := p.manager.providers.ReadSandboxReport(ctx, p.binding, &pluginsdk.ReadSandboxReportRequest{
		Context: p.operation(), Policy: p.policy, Audit: file.Content})
	if err != nil {
		return err
	}
	sourceID := reportHash([]byte(schema + "\x00" + file.Name))
	source := policy.ReportSource{ID: sourceID, Schema: schema, Digest: reportHash(file.Content),
		Integrity: response.Integrity, NextCursor: response.NextCursor, Complete: response.Complete}
	data.Sources = append(data.Sources, source)
	data.Complete = data.Complete && source.Complete && source.NextCursor == ""
	if response.Integrity == pluginsdk.SandboxReportUnverified || (response.Integrity == "partial" && data.Integrity == "complete") {
		data.Integrity = response.Integrity
	}
	if err := p.appendDenials(data, sourceID, response.Records); err != nil {
		return err
	}

	raw, err := json.Marshal(data)
	if err != nil || len(raw) > 240<<10 || len(data.Records) > 1000 || len(data.Sources) > 32 {
		return policy.ErrIneligible
	}
	return nil
}

func (p *Prepared) appendDenials(data *policy.ReportData, sourceID string, records []*pluginsdk.SandboxDenialRecord) error {
	for _, record := range records {
		if record.AttemptId != p.launch.AttemptID || record.PolicyDigest != p.policy.Digest {
			return policy.ErrConflict
		}
		kind := denialKind(record.Operation)
		if kind == "" || !slices.Contains(data.SupportedClasses, kind) {
			return policy.ErrIneligible
		}
		data.Records = append(data.Records, policy.DenialRecord{ID: reportHash([]byte(sourceID + "\x00" + record.RecordId)),
			SourceID: sourceID, ProviderRecordID: record.RecordId, Kind: kind, Operation: record.Operation,
			Target: record.Target, OccurredAt: time.UnixMilli(record.OccurredAtUnixMs).UTC()})
	}
	return nil
}

func denialKind(operation string) string {
	switch operation {
	case sandboxAccessRead, "write":
		return "filesystem"
	case "connect":
		return "network"
	case "execute":
		return "command"
	default:
		return ""
	}
}

func reportHash(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
