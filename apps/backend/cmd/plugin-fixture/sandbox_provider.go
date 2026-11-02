package main

import (
	"context"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const fixtureSandboxAlpha = "fixture-alpha"
const fixtureSandboxBeta = "fixture-beta"

const fixtureSandboxLimitation = "No confinement. Test fixture only."

var _ pluginsdk.SandboxProviderPlugin = (*fixturePlugin)(nil)

func fixtureSandboxReply(value *pluginsdk.SandboxRequestContext) (*pluginsdk.SandboxResponseContext, error) {
	if value == nil || (value.ProtocolMajor != 1 && value.ProtocolMajor != 2) || (value.ProviderKey != fixtureSandboxAlpha && value.ProviderKey != fixtureSandboxBeta) {
		return nil, status.Error(codes.InvalidArgument, "unsupported fixture sandbox contract")
	}
	return &pluginsdk.SandboxResponseContext{SchemaVersion: value.ProtocolMajor, OperationId: value.OperationId}, nil
}

func (*fixturePlugin) DescribeSandboxProvider(_ context.Context, req *pluginsdk.DescribeSandboxProviderRequest) (*pluginsdk.DescribeSandboxProviderResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	result := &pluginsdk.DescribeSandboxProviderResponse{Context: reply, Description: &pluginsdk.SandboxProviderDescription{
		DisplayName: req.Context.ProviderKey, ProviderVersion: "1.0.0", RuntimeVersion: "fixture-1",
		SupportedTransports: []string{"local", "worktree"}, Limitations: []string{fixtureSandboxLimitation},
	}}
	if req.Context.ProtocolMajor == 2 {
		result.Description.CapabilitiesV2 = &pluginsdk.SandboxCapabilitiesV2{PolicySchemas: []uint32{2}, OperationKinds: []string{"filesystem", "network", "command", "http_endpoint"}, ActorKinds: []string{"child_process", "runtime_service"}, NetworkMode: "none"}
	}
	return result, nil
}

func (*fixturePlugin) CompileSandboxPolicy(_ context.Context, req *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	if req.Policy == nil || (req.Policy.Grants == nil && (req.Context.ProtocolMajor != 2 || req.Policy.PolicyV2 == nil)) {
		return nil, status.Error(codes.InvalidArgument, "policy is required")
	}
	content, err := protojson.Marshal(req.Policy)
	if err != nil {
		return nil, err
	}
	result := &pluginsdk.CompileSandboxPolicyResponse{Context: reply, Compiled: &pluginsdk.SandboxCompiledPolicy{
		Prefix: []*pluginsdk.SandboxArgument{
			{Value: &pluginsdk.SandboxArgument_RuntimeReference{RuntimeReference: pluginsdk.SandboxRuntimeReferenceKey}},
			{Value: &pluginsdk.SandboxArgument_Literal{Literal: "--"}},
		},
		Artifacts:       []*pluginsdk.SandboxArtifact{{Name: "fixture-profile.json", Kind: pluginsdk.SandboxArtifactProfile, ContentUtf8: string(content)}},
		EffectiveGrants: req.Policy.Grants,
	}}
	if req.Policy.Grants != nil {
		result.Compiled.EffectiveGrants = proto.Clone(req.Policy.Grants).(*pluginsdk.SandboxGrants)
	}
	if req.Policy.PolicyV2 != nil {
		result.Compiled.EffectivePolicyV2 = proto.Clone(req.Policy.PolicyV2).(*pluginsdk.SandboxPolicyV2)
	}
	return result, nil
}

func (*fixturePlugin) NormalizeSandboxApproval(_ context.Context, req *pluginsdk.NormalizeSandboxApprovalRequest) (*pluginsdk.NormalizeSandboxApprovalResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	if req.Context.ProtocolMajor == 2 {
		approval := &pluginsdk.SandboxApprovalV2{}
		if err := protojson.Unmarshal(req.VerifiedEvent, approval); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid fixture event")
		}
		return &pluginsdk.NormalizeSandboxApprovalResponse{Context: reply, ApprovalV2: approval}, nil
	}
	operation := &pluginsdk.SandboxApprovalOperation{}
	if err := protojson.Unmarshal(req.VerifiedEvent, operation); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid fixture event")
	}
	return &pluginsdk.NormalizeSandboxApprovalResponse{Context: reply, Operation: operation}, nil
}

func (*fixturePlugin) NormalizeSandboxStatus(_ context.Context, req *pluginsdk.NormalizeSandboxStatusRequest) (*pluginsdk.NormalizeSandboxStatusResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	if req.Context.ProtocolMajor == 2 {
		return &pluginsdk.NormalizeSandboxStatusResponse{Context: reply, ObservationsV2: []*pluginsdk.SandboxRuntimeObservationV2{{SourceId: req.EvidenceSchema, ObservedAtUnixMs: time.Now().UnixMilli(), Platform: "linux", Limitations: []string{fixtureSandboxLimitation}}}}, nil
	}
	return &pluginsdk.NormalizeSandboxStatusResponse{Context: reply, Evidence: &pluginsdk.SandboxStatusEvidence{
		SchemaVersion: 1, Mechanism: "fixture", Process: req.Process,
		AttemptId: req.Context.AttemptId, Generation: req.Context.Generation, Limitations: []string{fixtureSandboxLimitation},
	}}, nil
}

func (*fixturePlugin) ReadSandboxReport(_ context.Context, req *pluginsdk.ReadSandboxReportRequest) (*pluginsdk.ReadSandboxReportResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	if req.Context.ProtocolMajor == 2 && req.ChunkV2 != nil {
		report := &pluginsdk.SandboxReportV2{}
		if err := protojson.Unmarshal(req.ChunkV2.Content, report); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid fixture report")
		}
		report.OperationHandle = req.ChunkV2.OperationHandle
		report.ArtifactSetDigest = req.ChunkV2.ArtifactSetDigest
		report.NextSequence = req.ChunkV2.Sequence + 1
		return &pluginsdk.ReadSandboxReportResponse{Context: reply, ReportV2: report}, nil
	}
	result := &pluginsdk.ReadSandboxReportResponse{}
	if err := protojson.Unmarshal(req.Audit, result); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid fixture report")
	}
	result.Context = reply
	return result, nil
}

func (p *fixturePlugin) ProposeSandboxRevision(ctx context.Context, req *pluginsdk.ProposeSandboxRevisionRequest) (*pluginsdk.ProposeSandboxRevisionResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	if req.Context.ProtocolMajor == 2 {
		return p.proposeSandboxV2(ctx, req, reply)
	}
	if req.Policy == nil || req.Policy.Grants == nil || len(req.SelectedRecords) == 0 {
		return nil, status.Error(codes.InvalidArgument, "policy and selected records are required")
	}
	added := &pluginsdk.SandboxGrants{}
	ids := make([]string, 0, len(req.SelectedRecords))
	for _, record := range req.SelectedRecords {
		if record == nil || record.Operation != "read" || record.PolicyDigest != req.Policy.Digest {
			return nil, status.Error(codes.InvalidArgument, "unsupported fixture revision")
		}
		added.Filesystem = append(added.Filesystem, &pluginsdk.SandboxFilesystemGrant{Path: record.Target})
		ids = append(ids, record.RecordId)
	}
	candidate := proto.Clone(req.Policy).(*pluginsdk.SandboxPolicySnapshot)
	proto.Merge(candidate.Grants, added)
	compiled, err := p.CompileSandboxPolicy(ctx, &pluginsdk.CompileSandboxPolicyRequest{Context: req.Context, Policy: candidate})
	if err != nil {
		return nil, err
	}
	return &pluginsdk.ProposeSandboxRevisionResponse{Context: reply, AddedGrants: added, SourceRecordIds: ids, Compiled: compiled.Compiled}, nil
}
