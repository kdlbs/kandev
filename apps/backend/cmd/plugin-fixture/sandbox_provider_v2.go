package main

import (
	"context"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (p *fixturePlugin) proposeSandboxV2(ctx context.Context, req *pluginsdk.ProposeSandboxRevisionRequest, reply *pluginsdk.SandboxResponseContext) (*pluginsdk.ProposeSandboxRevisionResponse, error) {
	if req.Policy == nil || req.Policy.PolicyV2 == nil || len(req.SelectedEventsV2) == 0 {
		return nil, status.Error(codes.InvalidArgument, "policy and selected events are required")
	}
	added := &pluginsdk.SandboxPermissionsV2{}
	ids := make([]string, 0, len(req.SelectedEventsV2))
	for _, event := range req.SelectedEventsV2 {
		if event == nil || event.Operation == nil || !event.ExactTarget || event.PolicyDigest != req.Policy.Digest {
			return nil, status.Error(codes.InvalidArgument, "invalid fixture event")
		}
		if err := fixtureSandboxAdditionV2(added, event.Operation); err != nil {
			return nil, err
		}
		ids = append(ids, event.RecordId)
	}
	candidate := proto.Clone(req.Policy).(*pluginsdk.SandboxPolicySnapshot)
	if candidate.PolicyV2.StaticGrants == nil {
		return nil, status.Error(codes.InvalidArgument, "missing static grants")
	}
	proto.Merge(candidate.PolicyV2.StaticGrants, added)
	compiled, err := p.CompileSandboxPolicy(ctx, &pluginsdk.CompileSandboxPolicyRequest{Context: req.Context, Policy: candidate})
	if err != nil {
		return nil, err
	}
	return &pluginsdk.ProposeSandboxRevisionResponse{Context: reply, SourceRecordIds: ids, AdditionsV2: &pluginsdk.SandboxRevisionAdditionsV2{StaticGrants: added, ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}, Compiled: compiled.Compiled}, nil
}

func fixtureSandboxAdditionV2(added *pluginsdk.SandboxPermissionsV2, value *pluginsdk.SandboxOperationV2) error {
	switch operation := value.Value.(type) {
	case *pluginsdk.SandboxOperationV2_Filesystem:
		if operation.Filesystem == nil {
			return status.Error(codes.InvalidArgument, "missing filesystem operation")
		}
		added.Filesystem = append(added.Filesystem, &pluginsdk.SandboxFilesystemPermissionV2{Path: operation.Filesystem.Path, Writable: operation.Filesystem.Access != "read"})
	case *pluginsdk.SandboxOperationV2_Tcp:
		added.Tcp = append(added.Tcp, operation.Tcp)
	case *pluginsdk.SandboxOperationV2_Command:
		added.Commands = append(added.Commands, operation.Command)
	case *pluginsdk.SandboxOperationV2_HttpEndpoint:
		added.HttpEndpoints = append(added.HttpEndpoints, operation.HttpEndpoint)
	default:
		return status.Error(codes.InvalidArgument, "unsupported fixture operation")
	}
	return nil
}
