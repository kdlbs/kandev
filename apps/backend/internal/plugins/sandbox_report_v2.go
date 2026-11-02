package plugins

import (
	"slices"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/proto"
)

func validateSandboxReportV2(req *pluginsdk.ReadSandboxReportRequest, value *pluginsdk.SandboxReportV2) error {
	if !validSandboxReportHeaderV2(value) {
		return errSandboxProviderReply
	}
	if chunk := req.ChunkV2; chunk != nil {
		if !validSandboxChunkV2(chunk) || value.OperationHandle != chunk.OperationHandle || value.ArtifactSetDigest != chunk.ArtifactSetDigest || value.NextSequence != chunk.Sequence+1 {
			return errSandboxProviderReply
		}
	}
	if !validSandboxCoverageV2(value.Coverage) {
		return errSandboxProviderReply
	}
	ids := map[string]bool{}
	for _, event := range value.Events {
		if !validSandboxEventV2(req, event) || ids[event.RecordId] {
			return errSandboxProviderReply
		}
		ids[event.RecordId] = true
	}
	if value.Verification != nil {
		return validateSandboxVerificationV2(req, value)
	}
	return nil
}

func validSandboxChunkV2(value *pluginsdk.SandboxAuditChunkV2) bool {
	return value != nil && validSandboxText(value.OperationHandle, 256) && sandboxDigestV2(value.ArtifactSetDigest) && validSandboxText(value.SourceId, 128) && sandboxDigestV2(value.SourceDigest) && len(value.Content) <= 256<<10 && value.TotalBytes > 0 && value.TotalBytes <= 128<<20 && uint64(len(value.Content)) <= value.TotalBytes && value.Sequence < 65536
}

func validSandboxCoverageV2(values []*pluginsdk.SandboxReportCoverageV2) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if value == nil || !slices.Contains([]string{pluginsdk.SandboxOperationFilesystem, pluginsdk.SandboxOperationNetwork, sandboxOperationCommand, sandboxOperationHTTPEndpoint}, value.OperationKind) || seen[value.OperationKind] || !slices.Contains([]string{sandboxV2Supported, pluginsdk.SandboxReportPartial, sandboxV2Unsupported, sandboxV2Unavailable}, value.Coverage) {
			return false
		}
		seen[value.OperationKind] = true
	}
	return true
}

func validSandboxEventV2(req *pluginsdk.ReadSandboxReportRequest, event *pluginsdk.SandboxAuditEventV2) bool {
	return event != nil && req.Context != nil && req.Policy != nil && validSandboxText(event.RecordId, 256) && validSandboxOperationV2(event.Operation) && validSandboxText(event.SourceId, 128) && event.OccurredAtUnixMs > 0 && event.PolicyDigest == req.Policy.Digest && event.AttemptId == req.Context.AttemptId && event.Generation == req.Context.Generation && slices.Contains([]string{sandboxEventAllowed, sandboxEventDenied, sandboxV2ApprovalRequested, sandboxV2ApprovalGranted, sandboxV2ApprovalDenied, sandboxV2Error}, event.Decision)
}

func validateSandboxVerificationV2(req *pluginsdk.ReadSandboxReportRequest, report *pluginsdk.SandboxReportV2) error {
	result := report.Verification
	if req.ChunkV2 == nil || !req.ChunkV2.Final || result.ArtifactSetDigest != req.ChunkV2.ArtifactSetDigest || !validSandboxText(result.VerifierVersion, 128) || !validSandboxText(result.SessionId, 256) || !sandboxDigestV2(result.EventRoot) || !slices.Contains([]string{sandboxV2Unchecked, sandboxV2Valid, sandboxIntegrityInvalid}, result.ChainState) || result.Finalization != report.Finalization {
		return errSandboxProviderReply
	}
	if !slices.Contains([]string{sandboxV2Unsigned, sandboxV2Untrusted, sandboxV2Verified, sandboxIntegrityInvalid, sandboxV2Unsupported}, result.SignatureState) || !slices.Contains([]string{sandboxV2Unavailable, sandboxV2Valid, sandboxIntegrityInvalid}, result.LedgerState) {
		return errSandboxProviderReply
	}
	if result.SignatureState == sandboxV2Verified && !validSandboxVerificationTrustV2(req.TrustV2, result, report.Finalization) {
		return errSandboxProviderReply
	}
	return nil
}

func validSandboxTrustV2(trust *pluginsdk.SandboxAuditTrustV2) bool {
	return trust != nil && len(trust.ExpectedPublicKeyPem) > 0 && len(trust.ExpectedPublicKeyPem) <= 16<<10 && sandboxDigestV2(trust.ExpectedKeyFingerprint) && validSandboxText(trust.TrustRevision, 128)
}

func validateSandboxRevisionV2(req *pluginsdk.ProposeSandboxRevisionRequest, result *pluginsdk.ProposeSandboxRevisionResponse) error {
	if req.Policy != nil && req.Policy.PolicyV2 == nil && result.AdditionsV2 == nil {
		return validateSandboxRevision(req, result)
	}
	if !validSandboxRevisionAdditionsV2(req, result) {
		return errSandboxProviderReply
	}
	selected := map[string]bool{}
	for _, event := range req.SelectedEventsV2 {
		if event == nil || !event.ExactTarget {
			return errSandboxProviderReply
		}
		selected[event.RecordId] = true
	}
	for _, id := range result.SourceRecordIds {
		if !selected[id] {
			return errSandboxProviderReply
		}
	}
	candidate := proto.Clone(req.Policy).(*pluginsdk.SandboxPolicySnapshot)
	proto.Merge(candidate.PolicyV2.StaticGrants, result.AdditionsV2.StaticGrants)
	proto.Merge(candidate.PolicyV2.ApprovalEnvelope, result.AdditionsV2.ApprovalEnvelope)
	if !proto.Equal(candidate.PolicyV2, result.GetCompiled().GetEffectivePolicyV2()) {
		return errSandboxProviderReply
	}
	return validateSandboxCompilation(&pluginsdk.CompileSandboxPolicyRequest{Context: req.Context, Policy: candidate, Runtime: req.Runtime, References: req.References, Endpoints: req.Endpoints}, result.Compiled)
}

func validSandboxReportHeaderV2(value *pluginsdk.SandboxReportV2) bool {
	return value != nil && len(value.Events) <= 1000 && len(value.Coverage) <= 4 && slices.Contains([]string{sandboxV2Unchecked, sandboxV2Valid, sandboxIntegrityInvalid}, value.Integrity) && slices.Contains([]string{sandboxV2Live, sandboxFinalizationFinal, sandboxV2Interrupted}, value.Finalization)
}

func validSandboxVerificationTrustV2(trust *pluginsdk.SandboxAuditTrustV2, result *pluginsdk.SandboxAuditVerificationV2, finalization string) bool {
	return validSandboxTrustV2(trust) && trust.ExpectedKeyFingerprint == result.ExpectedKeyFingerprint && trust.TrustRevision == result.TrustRevision && result.ChainState == sandboxV2Valid && finalization == sandboxFinalizationFinal
}

func validSandboxRevisionAdditionsV2(req *pluginsdk.ProposeSandboxRevisionRequest, result *pluginsdk.ProposeSandboxRevisionResponse) bool {
	return req.Policy != nil && validSandboxPolicyV2(req.Policy.PolicyV2) && result.AdditionsV2 != nil && result.AddedGrants == nil && validSandboxPermissionsV2(result.AdditionsV2.StaticGrants) && validSandboxPermissionsV2(result.AdditionsV2.ApprovalEnvelope) && sandboxUniqueTextV2(result.SourceRecordIds, 1000, 256) && len(result.SourceRecordIds) > 0
}
