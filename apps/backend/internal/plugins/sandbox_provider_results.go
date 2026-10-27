package plugins

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/protobuf/proto"
)

const sandboxJSONContentType = "application/json"

func validateSandboxReplyPayload(request proto.Message, reply proto.Message) error {
	switch result := reply.(type) {
	case *pluginsdk.DescribeSandboxProviderResponse:
		return validateSandboxDescription(result.Description)
	case *pluginsdk.CompileSandboxPolicyResponse:
		req, ok := request.(*pluginsdk.CompileSandboxPolicyRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxCompilation(req, result.Compiled)
	case *pluginsdk.NormalizeSandboxApprovalResponse:
		return validateSandboxApproval(result.Operation)
	case *pluginsdk.NormalizeSandboxStatusResponse:
		req, ok := request.(*pluginsdk.NormalizeSandboxStatusRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxStatus(req, result)
	case *pluginsdk.ReadSandboxReportResponse:
		return validateSandboxReport(result)
	case *pluginsdk.ProposeSandboxRevisionResponse:
		req, ok := request.(*pluginsdk.ProposeSandboxRevisionRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxRevision(req, result)
	default:
		return errSandboxProviderReply
	}
}

func validSandboxText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit && utf8.ValidString(value) &&
		!strings.ContainsAny(value, "<>") && strings.IndexFunc(value, unicode.IsControl) == -1
}

func validateSandboxDescription(value *pluginsdk.SandboxProviderDescription) error {
	if value == nil || !validSandboxText(value.DisplayName, 100) ||
		!validSandboxText(value.ProviderVersion, 64) || !validSandboxText(value.RuntimeVersion, 64) {
		return errSandboxProviderReply
	}
	if value.RuntimeArtifact != nil && !validSandboxRuntimeArtifact(value.RuntimeArtifact) {
		return errSandboxProviderReply
	}
	if value.ApprovalWire != nil && !validSandboxApprovalWire(value.ApprovalWire) {
		return errSandboxProviderReply
	}
	if err := validateSandboxRuntimeSources(value.RuntimeSources); err != nil {
		return err
	}
	return validateSandboxDescriptionLists(value)
}

func validateSandboxDescriptionLists(value *pluginsdk.SandboxProviderDescription) error {
	for _, transport := range value.SupportedTransports {
		if transport != "local" && transport != "worktree" {
			return errSandboxProviderReply
		}
	}
	for _, values := range [][]string{value.SupportedFeatures, value.EvidenceSchemas, value.ReportClasses, value.Limitations} {
		if len(values) > 32 {
			return errSandboxProviderReply
		}
		for _, text := range values {
			if !validSandboxText(text, 512) {
				return errSandboxProviderReply
			}
		}
	}
	return nil
}

func validSandboxApprovalWire(wire *pluginsdk.SandboxApprovalWire) bool {
	if wire.ContentType != sandboxJSONContentType {
		return false
	}
	for _, body := range []string{wire.AllowOnceBodyUtf8, wire.DenyBodyUtf8, wire.TimeoutBodyUtf8} {
		if len(body) > 4096 || !json.Valid([]byte(body)) || !utf8.ValidString(body) {
			return false
		}
	}
	return true
}

func validateSandboxApproval(value *pluginsdk.SandboxApprovalOperation) error {
	if value == nil || !validSandboxText(value.RequestId, 256) || !validSandboxText(value.Target, 4096) ||
		len(value.SupportedDecisions) == 0 || len(value.SupportedDecisions) > 2 {
		return errSandboxProviderReply
	}
	if !validSandboxApprovalAccess(value) {
		return errSandboxProviderReply
	}
	seen := make(map[string]bool)
	for _, decision := range value.SupportedDecisions {
		if (decision != pluginsdk.SandboxDecisionDeny && decision != pluginsdk.SandboxDecisionAllowOnce) || seen[decision] {
			return errSandboxProviderReply
		}
		seen[decision] = true
	}
	if !seen[pluginsdk.SandboxDecisionDeny] {
		return errSandboxProviderReply
	}
	return nil
}

func validSandboxApprovalAccess(value *pluginsdk.SandboxApprovalOperation) bool {
	const sandboxAccessRead = "read"
	return (value.Kind == pluginsdk.SandboxOperationFilesystem && (value.Access == sandboxAccessRead || value.Access == "write")) ||
		(value.Kind == pluginsdk.SandboxOperationNetwork && value.Access == "connect")
}

func validateSandboxReport(value *pluginsdk.ReadSandboxReportResponse) error {
	if proto.Size(value) > 256<<10 || len(value.Records) > 1000 || len(value.NextCursor) > 1024 {
		return errSandboxProviderReply
	}
	switch value.Integrity {
	case pluginsdk.SandboxReportComplete, pluginsdk.SandboxReportPartial, pluginsdk.SandboxReportUnverified:
	default:
		return errSandboxProviderReply
	}
	seen := make(map[string]bool, len(value.Records))
	for _, record := range value.Records {
		if record == nil || !validSandboxText(record.RecordId, 256) || seen[record.RecordId] ||
			!validSandboxText(record.Operation, 64) || !validSandboxText(record.Target, 4096) {
			return errSandboxProviderReply
		}
		seen[record.RecordId] = true
	}
	return nil
}

func validateSandboxRevision(req *pluginsdk.ProposeSandboxRevisionRequest, result *pluginsdk.ProposeSandboxRevisionResponse) error {
	if req.Policy == nil || req.Policy.Grants == nil || result.AddedGrants == nil || len(result.SourceRecordIds) == 0 {
		return errSandboxProviderReply
	}
	selected := make(map[string]bool, len(req.SelectedRecords))
	for _, record := range req.SelectedRecords {
		if record != nil {
			selected[record.RecordId] = true
		}
	}
	for _, id := range result.SourceRecordIds {
		if !selected[id] {
			return errSandboxProviderReply
		}
		delete(selected, id)
	}
	candidate := proto.Clone(req.Policy).(*pluginsdk.SandboxPolicySnapshot)
	proto.Merge(candidate.Grants, result.AddedGrants)
	return validateSandboxCompilation(&pluginsdk.CompileSandboxPolicyRequest{
		Policy: candidate, Runtime: req.Runtime, References: req.References, Endpoints: req.Endpoints,
	}, result.Compiled)
}

func validSandboxRuntimeArtifact(artifact *pluginsdk.SandboxRuntimeArtifact) bool {
	if artifact == nil {
		return false
	}
	digest, err := hex.DecodeString(artifact.Sha256)
	return validSandboxArtifactName(artifact.RelativePath) && err == nil && len(digest) == 32 && artifact.Sha256 == strings.ToLower(artifact.Sha256)
}
