package main

import (
	"context"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const fixtureSandboxLimitation = "No confinement. Test fixture only."

var _ pluginsdk.SandboxProviderPlugin = (*fixturePlugin)(nil)

func fixtureSandboxReply(value *pluginsdk.SandboxRequestContext) (*pluginsdk.SandboxResponseContext, error) {
	if value == nil || value.ProtocolMajor != 1 || (value.ProviderKey != "fixture-alpha" && value.ProviderKey != "fixture-beta") {
		return nil, status.Error(codes.InvalidArgument, "unsupported fixture sandbox contract")
	}
	return &pluginsdk.SandboxResponseContext{SchemaVersion: 1, OperationId: value.OperationId}, nil
}

func (*fixturePlugin) DescribeSandboxProvider(_ context.Context, req *pluginsdk.DescribeSandboxProviderRequest) (*pluginsdk.DescribeSandboxProviderResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	return &pluginsdk.DescribeSandboxProviderResponse{Context: reply, Description: &pluginsdk.SandboxProviderDescription{
		DisplayName: req.Context.ProviderKey, ProviderVersion: "1.0.0", RuntimeVersion: "fixture-1",
		SupportedTransports: []string{"local", "worktree"}, Limitations: []string{fixtureSandboxLimitation},
	}}, nil
}

func (*fixturePlugin) CompileSandboxPolicy(_ context.Context, req *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
	}
	if req.Policy == nil || req.Policy.Grants == nil {
		return nil, status.Error(codes.InvalidArgument, "policy is required")
	}
	content, err := protojson.Marshal(req.Policy)
	if err != nil {
		return nil, err
	}
	return &pluginsdk.CompileSandboxPolicyResponse{Context: reply, Compiled: &pluginsdk.SandboxCompiledPolicy{
		Prefix: []*pluginsdk.SandboxArgument{
			{Value: &pluginsdk.SandboxArgument_RuntimeReference{RuntimeReference: pluginsdk.SandboxRuntimeReferenceKey}},
			{Value: &pluginsdk.SandboxArgument_Literal{Literal: "--"}},
		},
		Artifacts:       []*pluginsdk.SandboxArtifact{{Name: "fixture-profile.json", Kind: pluginsdk.SandboxArtifactProfile, ContentUtf8: string(content)}},
		EffectiveGrants: proto.Clone(req.Policy.Grants).(*pluginsdk.SandboxGrants),
	}}, nil
}

func (*fixturePlugin) NormalizeSandboxApproval(_ context.Context, req *pluginsdk.NormalizeSandboxApprovalRequest) (*pluginsdk.NormalizeSandboxApprovalResponse, error) {
	reply, err := fixtureSandboxReply(req.GetContext())
	if err != nil {
		return nil, err
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
