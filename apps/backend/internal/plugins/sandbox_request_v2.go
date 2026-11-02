package plugins

import (
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/proto"
	"strings"
)

func validSandboxV2Request(request proto.Message) bool {
	switch value := request.(type) {
	case *pluginsdk.DescribeSandboxProviderRequest:
		return true
	case *pluginsdk.CompileSandboxPolicyRequest:
		return validSandboxWirePolicyV2(value.Policy) && sandboxPublicReferencesV2(value.References)
	case *pluginsdk.NormalizeSandboxApprovalRequest:
		return len(value.VerifiedEvent) > 0 && len(value.VerifiedEvent) <= 64<<10 && (value.ServiceBindingV2 == nil || validSandboxServiceBindingV2(value.ServiceBindingV2))
	case *pluginsdk.NormalizeSandboxStatusRequest:
		return validSandboxStatusRequestV2(value)
	case *pluginsdk.ReadSandboxReportRequest:
		return validSandboxReportRequestV2(value)
	case *pluginsdk.ProposeSandboxRevisionRequest:
		return validSandboxWirePolicyV2(value.Policy) && validSandboxSelectionV2(value) && sandboxPublicReferencesV2(value.References)
	default:
		return false
	}
}

func validSandboxReportRequestV2(value *pluginsdk.ReadSandboxReportRequest) bool {
	if value.VerifyAuditV2 && (value.ChunkV2 == nil || value.ChunkV2.SourceCount == 0) {
		return false
	}
	if value.ChunkV2 != nil && (len(value.Audit) != 0 || value.Cursor != "" || !validSandboxChunkV2(value.ChunkV2)) {
		return false
	}
	return value.TrustV2 == nil || validSandboxTrustV2(value.TrustV2)
}

func sandboxPublicReferencesV2(values []*pluginsdk.SandboxRuntimeReference) bool {
	for _, value := range values {
		if value == nil || privateSandboxReference(value.Key) {
			return false
		}
	}
	return true
}

func validSandboxWirePolicyV2(value *pluginsdk.SandboxPolicySnapshot) bool {
	if value == nil {
		return false
	}
	if value.PolicyV2 == nil {
		return value.Grants != nil
	}
	return value.Grants == nil && validSandboxPolicyV2(value.PolicyV2)
}

func validSandboxSelectionV2(value *pluginsdk.ProposeSandboxRevisionRequest) bool {
	if value.Policy.PolicyV2 == nil {
		return len(value.SelectedEventsV2) == 0 && len(value.SelectedRecords) > 0 && len(value.SelectedRecords) <= 1000
	}
	return len(value.SelectedRecords) == 0 && len(value.SelectedEventsV2) > 0 && len(value.SelectedEventsV2) <= 1000
}

func validSandboxStatusSourceV2(value *pluginsdk.NormalizeSandboxStatusRequest) bool {
	if value.SourceRelativePathV2 == "" && value.ProviderSessionIdV2 == "" {
		return true
	}
	return validSandboxArtifactName(value.SourceRelativePathV2) && validSandboxArtifactName(value.ProviderSessionIdV2) && !strings.ContainsAny(value.ProviderSessionIdV2, "/\\")
}

func validSandboxStatusRequestV2(value *pluginsdk.NormalizeSandboxStatusRequest) bool {
	return len(value.Evidence) <= 1<<20 && validSandboxText(value.EvidenceSchema, 128) && validSandboxStatusSourceV2(value)
}
