package pluginsdk

import (
	"context"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const MaxSandboxMessageBytes = 1 << 20

const (
	SandboxRuntimeReferenceKey = "sandbox.runtime"
	SandboxArtifactProfile     = "profile"
	SandboxArtifactConfig      = "config"
	SandboxOperationFilesystem = "filesystem"
	SandboxOperationNetwork    = "network"
	SandboxDecisionDeny        = "deny"
	SandboxDecisionAllowOnce   = "allow_once"
	SandboxReportComplete      = "complete"
	SandboxReportPartial       = "partial"
	SandboxReportUnverified    = "unverified"
)

type SandboxRequestContext = pluginv1.SandboxRequestContext
type SandboxResponseContext = pluginv1.SandboxResponseContext
type SandboxFilesystemGrant = pluginv1.SandboxFilesystemGrant
type SandboxNetworkGrant = pluginv1.SandboxNetworkGrant
type SandboxGrants = pluginv1.SandboxGrants
type SandboxPolicySource = pluginv1.SandboxPolicySource
type SandboxPolicySnapshot = pluginv1.SandboxPolicySnapshot
type SandboxRuntimeFacts = pluginv1.SandboxRuntimeFacts
type SandboxRuntimeReference = pluginv1.SandboxRuntimeReference
type SandboxEndpoint = pluginv1.SandboxEndpoint
type SandboxArgument = pluginv1.SandboxArgument
type SandboxArgument_Literal = pluginv1.SandboxArgument_Literal
type SandboxArgument_RuntimeReference = pluginv1.SandboxArgument_RuntimeReference
type SandboxArgument_ArtifactReference = pluginv1.SandboxArgument_ArtifactReference
type SandboxArtifact = pluginv1.SandboxArtifact
type SandboxArtifactBinding = pluginv1.SandboxArtifactBinding
type SandboxOverrideConstraints = pluginv1.SandboxOverrideConstraints
type SandboxCompiledPolicy = pluginv1.SandboxCompiledPolicy
type SandboxProviderDescription = pluginv1.SandboxProviderDescription
type SandboxRuntimeArtifact = pluginv1.SandboxRuntimeArtifact
type SandboxRuntimeSource = pluginv1.SandboxRuntimeSource
type SandboxRuntimeIdentity = pluginv1.SandboxRuntimeIdentity
type SandboxApprovalWire = pluginv1.SandboxApprovalWire
type SandboxApprovalOperation = pluginv1.SandboxApprovalOperation
type SandboxProcessIdentity = pluginv1.SandboxProcessIdentity
type SandboxStatusEvidence = pluginv1.SandboxStatusEvidence
type SandboxDenialRecord = pluginv1.SandboxDenialRecord
type DescribeSandboxProviderRequest = pluginv1.DescribeSandboxProviderRequest
type DescribeSandboxProviderResponse = pluginv1.DescribeSandboxProviderResponse
type CompileSandboxPolicyRequest = pluginv1.CompileSandboxPolicyRequest
type CompileSandboxPolicyResponse = pluginv1.CompileSandboxPolicyResponse
type NormalizeSandboxApprovalRequest = pluginv1.NormalizeSandboxApprovalRequest
type NormalizeSandboxApprovalResponse = pluginv1.NormalizeSandboxApprovalResponse
type NormalizeSandboxStatusRequest = pluginv1.NormalizeSandboxStatusRequest
type NormalizeSandboxStatusResponse = pluginv1.NormalizeSandboxStatusResponse
type ReadSandboxReportRequest = pluginv1.ReadSandboxReportRequest
type ReadSandboxReportResponse = pluginv1.ReadSandboxReportResponse
type ProposeSandboxRevisionRequest = pluginv1.ProposeSandboxRevisionRequest
type ProposeSandboxRevisionResponse = pluginv1.ProposeSandboxRevisionResponse

// SandboxProviderPlugin is optional and complete. It supplies policy data;
// the host owns launch, approval authority, and durable policy revisions.
type SandboxProviderPlugin interface {
	DescribeSandboxProvider(context.Context, *DescribeSandboxProviderRequest) (*DescribeSandboxProviderResponse, error)
	CompileSandboxPolicy(context.Context, *CompileSandboxPolicyRequest) (*CompileSandboxPolicyResponse, error)
	NormalizeSandboxApproval(context.Context, *NormalizeSandboxApprovalRequest) (*NormalizeSandboxApprovalResponse, error)
	NormalizeSandboxStatus(context.Context, *NormalizeSandboxStatusRequest) (*NormalizeSandboxStatusResponse, error)
	ReadSandboxReport(context.Context, *ReadSandboxReportRequest) (*ReadSandboxReportResponse, error)
	ProposeSandboxRevision(context.Context, *ProposeSandboxRevisionRequest) (*ProposeSandboxRevisionResponse, error)
}

var _ SandboxProviderPlugin = (*RemotePlugin)(nil)

func (r *RemotePlugin) DescribeSandboxProvider(ctx context.Context, req *DescribeSandboxProviderRequest) (*DescribeSandboxProviderResponse, error) {
	return r.client.DescribeSandboxProvider(ctx, req, grpc.MaxCallRecvMsgSize(MaxSandboxMessageBytes), grpc.MaxCallSendMsgSize(MaxSandboxMessageBytes))
}

func (s *grpcPluginServer) DescribeSandboxProvider(ctx context.Context, req *DescribeSandboxProviderRequest) (*DescribeSandboxProviderResponse, error) {
	provider, ok := s.impl.(SandboxProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete sandbox provider")
	}
	response, err := provider.DescribeSandboxProvider(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: sandbox provider returned nil response")
	}
	return response, nil
}

func (r *RemotePlugin) CompileSandboxPolicy(ctx context.Context, req *CompileSandboxPolicyRequest) (*CompileSandboxPolicyResponse, error) {
	return r.client.CompileSandboxPolicy(ctx, req, grpc.MaxCallRecvMsgSize(MaxSandboxMessageBytes), grpc.MaxCallSendMsgSize(MaxSandboxMessageBytes))
}

func (s *grpcPluginServer) CompileSandboxPolicy(ctx context.Context, req *CompileSandboxPolicyRequest) (*CompileSandboxPolicyResponse, error) {
	provider, ok := s.impl.(SandboxProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete sandbox provider")
	}
	response, err := provider.CompileSandboxPolicy(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: sandbox provider returned nil response")
	}
	return response, nil
}

func (r *RemotePlugin) NormalizeSandboxApproval(ctx context.Context, req *NormalizeSandboxApprovalRequest) (*NormalizeSandboxApprovalResponse, error) {
	return r.client.NormalizeSandboxApproval(ctx, req, grpc.MaxCallRecvMsgSize(MaxSandboxMessageBytes), grpc.MaxCallSendMsgSize(MaxSandboxMessageBytes))
}

func (s *grpcPluginServer) NormalizeSandboxApproval(ctx context.Context, req *NormalizeSandboxApprovalRequest) (*NormalizeSandboxApprovalResponse, error) {
	provider, ok := s.impl.(SandboxProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete sandbox provider")
	}
	response, err := provider.NormalizeSandboxApproval(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: sandbox provider returned nil response")
	}
	return response, nil
}

func (r *RemotePlugin) NormalizeSandboxStatus(ctx context.Context, req *NormalizeSandboxStatusRequest) (*NormalizeSandboxStatusResponse, error) {
	return r.client.NormalizeSandboxStatus(ctx, req, grpc.MaxCallRecvMsgSize(MaxSandboxMessageBytes), grpc.MaxCallSendMsgSize(MaxSandboxMessageBytes))
}

func (s *grpcPluginServer) NormalizeSandboxStatus(ctx context.Context, req *NormalizeSandboxStatusRequest) (*NormalizeSandboxStatusResponse, error) {
	provider, ok := s.impl.(SandboxProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete sandbox provider")
	}
	response, err := provider.NormalizeSandboxStatus(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: sandbox provider returned nil response")
	}
	return response, nil
}

func (r *RemotePlugin) ReadSandboxReport(ctx context.Context, req *ReadSandboxReportRequest) (*ReadSandboxReportResponse, error) {
	return r.client.ReadSandboxReport(ctx, req, grpc.MaxCallRecvMsgSize(MaxSandboxMessageBytes), grpc.MaxCallSendMsgSize(MaxSandboxMessageBytes))
}

func (s *grpcPluginServer) ReadSandboxReport(ctx context.Context, req *ReadSandboxReportRequest) (*ReadSandboxReportResponse, error) {
	provider, ok := s.impl.(SandboxProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete sandbox provider")
	}
	response, err := provider.ReadSandboxReport(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: sandbox provider returned nil response")
	}
	return response, nil
}

func (r *RemotePlugin) ProposeSandboxRevision(ctx context.Context, req *ProposeSandboxRevisionRequest) (*ProposeSandboxRevisionResponse, error) {
	return r.client.ProposeSandboxRevision(ctx, req, grpc.MaxCallRecvMsgSize(MaxSandboxMessageBytes), grpc.MaxCallSendMsgSize(MaxSandboxMessageBytes))
}

func (s *grpcPluginServer) ProposeSandboxRevision(ctx context.Context, req *ProposeSandboxRevisionRequest) (*ProposeSandboxRevisionResponse, error) {
	provider, ok := s.impl.(SandboxProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete sandbox provider")
	}
	response, err := provider.ProposeSandboxRevision(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: sandbox provider returned nil response")
	}
	return response, nil
}
