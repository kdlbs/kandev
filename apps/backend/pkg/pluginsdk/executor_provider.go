package pluginsdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const MaxExecutorRuntimeArtifactChunkBytes = 256 << 10

type ExecutorProviderRequestContext = pluginv1.ExecutorProviderRequestContext
type ExecutorProfileSnapshot = pluginv1.ExecutorProfileSnapshot
type ExecutorProviderError = pluginv1.ExecutorProviderError
type ExecutorProviderFieldError = pluginv1.ExecutorProviderFieldError
type ExecutorProviderCapabilities = pluginv1.ExecutorProviderCapabilities
type ExecutorResourceDescriptor = pluginv1.ExecutorResourceDescriptor
type ExecutorConnectionLease = pluginv1.ExecutorConnectionLease
type ExecutorRuntimeArtifactChunk = pluginv1.ExecutorRuntimeArtifactChunk

type ValidateExecutorProfileRequest = pluginv1.ValidateExecutorProfileRequest
type ValidateExecutorProfileResponse = pluginv1.ValidateExecutorProfileResponse
type ProvisionExecutorEnvironmentRequest = pluginv1.ProvisionExecutorEnvironmentRequest
type ProvisionExecutorEnvironmentResponse = pluginv1.ProvisionExecutorEnvironmentResponse
type RecoverExecutorOperationRequest = pluginv1.RecoverExecutorOperationRequest
type RecoverExecutorOperationResponse = pluginv1.RecoverExecutorOperationResponse
type AttachExecutorEnvironmentRequest = pluginv1.AttachExecutorEnvironmentRequest
type AttachExecutorEnvironmentResponse = pluginv1.AttachExecutorEnvironmentResponse
type InspectExecutorEnvironmentRequest = pluginv1.InspectExecutorEnvironmentRequest
type InspectExecutorEnvironmentResponse = pluginv1.InspectExecutorEnvironmentResponse
type ResolveExecutorConnectionRequest = pluginv1.ResolveExecutorConnectionRequest
type ResolveExecutorConnectionResponse = pluginv1.ResolveExecutorConnectionResponse
type DestroyExecutorEnvironmentRequest = pluginv1.DestroyExecutorEnvironmentRequest
type DestroyExecutorEnvironmentResponse = pluginv1.DestroyExecutorEnvironmentResponse
type CheckpointExecutorResourceRequest = pluginv1.CheckpointExecutorResourceRequest
type CheckpointExecutorResourceResponse = pluginv1.CheckpointExecutorResourceResponse
type ReportExecutorProgressRequest = pluginv1.ReportExecutorProgressRequest
type ReportExecutorProgressResponse = pluginv1.ReportExecutorProgressResponse
type ReadExecutorRuntimeArtifactRequest = pluginv1.ReadExecutorRuntimeArtifactRequest

// ExecutorProviderPlugin is an optional all-or-nothing extension. The host
// treats a partial implementation as unsupported so a manifest cannot expose
// a provider whose recovery or cleanup methods are missing.
type ExecutorProviderPlugin interface {
	ValidateExecutorProfile(context.Context, *ValidateExecutorProfileRequest) (*ValidateExecutorProfileResponse, error)
	ProvisionExecutorEnvironment(context.Context, *ProvisionExecutorEnvironmentRequest) (*ProvisionExecutorEnvironmentResponse, error)
	RecoverExecutorOperation(context.Context, *RecoverExecutorOperationRequest) (*RecoverExecutorOperationResponse, error)
	AttachExecutorEnvironment(context.Context, *AttachExecutorEnvironmentRequest) (*AttachExecutorEnvironmentResponse, error)
	InspectExecutorEnvironment(context.Context, *InspectExecutorEnvironmentRequest) (*InspectExecutorEnvironmentResponse, error)
	ResolveExecutorConnection(context.Context, *ResolveExecutorConnectionRequest) (*ResolveExecutorConnectionResponse, error)
	DestroyExecutorEnvironment(context.Context, *DestroyExecutorEnvironmentRequest) (*DestroyExecutorEnvironmentResponse, error)
}

// ExecutorProviderHost is an optional Host extension available to providers.
// Its callbacks are scoped to the host-admitted operation.
type ExecutorProviderHost interface {
	ExecutorProvider() ExecutorProviderHostAPI
}

// ExecutorProviderHostAPI contains the operation-bound callbacks a provider
// can use during launch and bootstrap.
type ExecutorProviderHostAPI interface {
	CheckpointExecutorResource(context.Context, *CheckpointExecutorResourceRequest) (*CheckpointExecutorResourceResponse, error)
	ReportExecutorProgress(context.Context, *ReportExecutorProgressRequest) (*ReportExecutorProgressResponse, error)
	ReadExecutorRuntimeArtifact(context.Context, *ReadExecutorRuntimeArtifactRequest, func(*ExecutorRuntimeArtifactChunk) error) error
}

var (
	_ ExecutorProviderPlugin = (*RemotePlugin)(nil)
	_ ExecutorProviderHost   = (*grpcHostClient)(nil)
)

func (r *RemotePlugin) ValidateExecutorProfile(ctx context.Context, req *ValidateExecutorProfileRequest) (*ValidateExecutorProfileResponse, error) {
	return r.client.ValidateExecutorProfile(ctx, req)
}

func (r *RemotePlugin) ProvisionExecutorEnvironment(ctx context.Context, req *ProvisionExecutorEnvironmentRequest) (*ProvisionExecutorEnvironmentResponse, error) {
	return r.client.ProvisionExecutorEnvironment(ctx, req)
}

func (r *RemotePlugin) RecoverExecutorOperation(ctx context.Context, req *RecoverExecutorOperationRequest) (*RecoverExecutorOperationResponse, error) {
	return r.client.RecoverExecutorOperation(ctx, req)
}

func (r *RemotePlugin) AttachExecutorEnvironment(ctx context.Context, req *AttachExecutorEnvironmentRequest) (*AttachExecutorEnvironmentResponse, error) {
	return r.client.AttachExecutorEnvironment(ctx, req)
}

func (r *RemotePlugin) InspectExecutorEnvironment(ctx context.Context, req *InspectExecutorEnvironmentRequest) (*InspectExecutorEnvironmentResponse, error) {
	return r.client.InspectExecutorEnvironment(ctx, req)
}

func (r *RemotePlugin) ResolveExecutorConnection(ctx context.Context, req *ResolveExecutorConnectionRequest) (*ResolveExecutorConnectionResponse, error) {
	return r.client.ResolveExecutorConnection(ctx, req)
}

func (r *RemotePlugin) DestroyExecutorEnvironment(ctx context.Context, req *DestroyExecutorEnvironmentRequest) (*DestroyExecutorEnvironmentResponse, error) {
	return r.client.DestroyExecutorEnvironment(ctx, req)
}

func (s *grpcPluginServer) ValidateExecutorProfile(ctx context.Context, req *pluginv1.ValidateExecutorProfileRequest) (*pluginv1.ValidateExecutorProfileResponse, error) {
	provider, ok := s.impl.(ExecutorProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete executor provider")
	}
	response, err := provider.ValidateExecutorProfile(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: executor profile validator returned nil response")
	}
	return response, nil
}

func (s *grpcPluginServer) ProvisionExecutorEnvironment(ctx context.Context, req *pluginv1.ProvisionExecutorEnvironmentRequest) (*pluginv1.ProvisionExecutorEnvironmentResponse, error) {
	provider, ok := s.impl.(ExecutorProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete executor provider")
	}
	response, err := provider.ProvisionExecutorEnvironment(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: executor provisioner returned nil response")
	}
	return response, nil
}

func (s *grpcPluginServer) RecoverExecutorOperation(ctx context.Context, req *pluginv1.RecoverExecutorOperationRequest) (*pluginv1.RecoverExecutorOperationResponse, error) {
	provider, ok := s.impl.(ExecutorProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete executor provider")
	}
	response, err := provider.RecoverExecutorOperation(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: executor recovery handler returned nil response")
	}
	return response, nil
}

func (s *grpcPluginServer) AttachExecutorEnvironment(ctx context.Context, req *pluginv1.AttachExecutorEnvironmentRequest) (*pluginv1.AttachExecutorEnvironmentResponse, error) {
	provider, ok := s.impl.(ExecutorProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete executor provider")
	}
	response, err := provider.AttachExecutorEnvironment(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: executor attachment handler returned nil response")
	}
	return response, nil
}

func (s *grpcPluginServer) InspectExecutorEnvironment(ctx context.Context, req *pluginv1.InspectExecutorEnvironmentRequest) (*pluginv1.InspectExecutorEnvironmentResponse, error) {
	provider, ok := s.impl.(ExecutorProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete executor provider")
	}
	response, err := provider.InspectExecutorEnvironment(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: executor inspector returned nil response")
	}
	return response, nil
}

func (s *grpcPluginServer) ResolveExecutorConnection(ctx context.Context, req *pluginv1.ResolveExecutorConnectionRequest) (*pluginv1.ResolveExecutorConnectionResponse, error) {
	provider, ok := s.impl.(ExecutorProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete executor provider")
	}
	response, err := provider.ResolveExecutorConnection(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: executor connection resolver returned nil response")
	}
	return response, nil
}

func (s *grpcPluginServer) DestroyExecutorEnvironment(ctx context.Context, req *pluginv1.DestroyExecutorEnvironmentRequest) (*pluginv1.DestroyExecutorEnvironmentResponse, error) {
	provider, ok := s.impl.(ExecutorProviderPlugin)
	if !ok {
		return nil, unsupportedPluginExtension("complete executor provider")
	}
	response, err := provider.DestroyExecutorEnvironment(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, status.Error(codes.Internal, "pluginsdk: executor destroyer returned nil response")
	}
	return response, nil
}

func (c *grpcHostClient) ExecutorProvider() ExecutorProviderHostAPI {
	return grpcExecutorProviderHost{client: c.client}
}

type grpcExecutorProviderHost struct {
	client pluginv1.HostClient
}

func (h grpcExecutorProviderHost) CheckpointExecutorResource(ctx context.Context, req *CheckpointExecutorResourceRequest) (*CheckpointExecutorResourceResponse, error) {
	return h.client.CheckpointExecutorResource(ctx, req)
}

func (h grpcExecutorProviderHost) ReportExecutorProgress(ctx context.Context, req *ReportExecutorProgressRequest) (*ReportExecutorProgressResponse, error) {
	return h.client.ReportExecutorProgress(ctx, req)
}

func (h grpcExecutorProviderHost) ReadExecutorRuntimeArtifact(ctx context.Context, req *ReadExecutorRuntimeArtifactRequest, consume func(*ExecutorRuntimeArtifactChunk) error) error {
	if consume == nil {
		return status.Error(codes.InvalidArgument, "pluginsdk: artifact consumer is required")
	}
	stream, err := h.client.ReadExecutorRuntimeArtifact(ctx, req)
	if err != nil {
		return err
	}
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := consume(chunk); err != nil {
			return err
		}
	}
}

func (s *grpcHostServer) CheckpointExecutorResource(ctx context.Context, req *pluginv1.CheckpointExecutorResourceRequest) (*pluginv1.CheckpointExecutorResourceResponse, error) {
	provider, ok := s.impl.(ExecutorProviderHost)
	if !ok {
		return nil, errUnimplementedHostData("executor_provider")
	}
	return provider.ExecutorProvider().CheckpointExecutorResource(ctx, req)
}

func (s *grpcHostServer) ReportExecutorProgress(ctx context.Context, req *pluginv1.ReportExecutorProgressRequest) (*pluginv1.ReportExecutorProgressResponse, error) {
	provider, ok := s.impl.(ExecutorProviderHost)
	if !ok {
		return nil, errUnimplementedHostData("executor_provider")
	}
	return provider.ExecutorProvider().ReportExecutorProgress(ctx, req)
}

func (s *grpcHostServer) ReadExecutorRuntimeArtifact(req *pluginv1.ReadExecutorRuntimeArtifactRequest, stream pluginv1.Host_ReadExecutorRuntimeArtifactServer) error {
	provider, ok := s.impl.(ExecutorProviderHost)
	if !ok {
		return errUnimplementedHostData("executor_provider")
	}
	if req == nil || req.GetArtifactId() == "" {
		return status.Error(codes.InvalidArgument, "pluginsdk: artifact id is required")
	}
	validator := newExecutorArtifactStreamValidator(req.GetArtifactId(), stream.Send)
	if err := provider.ExecutorProvider().ReadExecutorRuntimeArtifact(stream.Context(), req, validator.accept); err != nil {
		return err
	}
	if !validator.complete() {
		return status.Error(codes.DataLoss, "pluginsdk: runtime artifact stream is incomplete")
	}
	return nil
}

type executorArtifactStreamValidator struct {
	artifactID string
	index      uint32
	total      uint64
	size       uint64
	metadata   string
	hash       hash.Hash
	sendChunk  func(*pluginv1.ExecutorRuntimeArtifactChunk) error
	final      bool
}

func newExecutorArtifactStreamValidator(artifactID string, send func(*pluginv1.ExecutorRuntimeArtifactChunk) error) *executorArtifactStreamValidator {
	return &executorArtifactStreamValidator{artifactID: artifactID, sendChunk: send, hash: sha256.New()}
}

func (v *executorArtifactStreamValidator) accept(chunk *ExecutorRuntimeArtifactChunk) error {
	if err := v.validateChunkSequence(chunk); err != nil {
		return err
	}
	if v.index == 0 {
		if err := v.captureDescriptor(chunk); err != nil {
			return err
		}
	}
	if artifactChunkMetadata(chunk) != v.metadata {
		return status.Error(codes.InvalidArgument, "pluginsdk: runtime artifact descriptor changed during transfer")
	}
	if err := v.addChunk(chunk); err != nil {
		return err
	}
	if err := v.sendChunk(chunk); err != nil {
		return err
	}
	v.index++
	return nil
}

func (v *executorArtifactStreamValidator) validateChunkSequence(chunk *ExecutorRuntimeArtifactChunk) error {
	if chunk == nil || v.final || chunk.GetArtifactId() != v.artifactID || chunk.GetChunkIndex() != v.index {
		return status.Error(codes.InvalidArgument, "pluginsdk: invalid runtime artifact chunk sequence")
	}
	if len(chunk.GetData()) > MaxExecutorRuntimeArtifactChunkBytes {
		return status.Error(codes.ResourceExhausted, "pluginsdk: runtime artifact chunk exceeds the size limit")
	}
	return nil
}

func (v *executorArtifactStreamValidator) captureDescriptor(chunk *ExecutorRuntimeArtifactChunk) error {
	if chunk.GetTotalSizeBytes() == 0 && len(chunk.GetData()) != 0 {
		return status.Error(codes.InvalidArgument, "pluginsdk: invalid empty runtime artifact descriptor")
	}
	if chunk.GetTotalSizeBytes() > 512<<20 {
		return status.Error(codes.ResourceExhausted, "pluginsdk: runtime artifact exceeds the size limit")
	}
	if len(chunk.GetSha256()) != sha256.Size*2 {
		return status.Error(codes.InvalidArgument, "pluginsdk: runtime artifact digest is invalid")
	}
	if _, err := hex.DecodeString(chunk.GetSha256()); err != nil {
		return status.Error(codes.InvalidArgument, "pluginsdk: runtime artifact digest is invalid")
	}
	v.total = chunk.GetTotalSizeBytes()
	v.metadata = artifactChunkMetadata(chunk)
	return nil
}

func (v *executorArtifactStreamValidator) addChunk(chunk *ExecutorRuntimeArtifactChunk) error {
	v.size += uint64(len(chunk.GetData()))
	if v.size > v.total {
		return status.Error(codes.DataLoss, "pluginsdk: runtime artifact exceeds its declared size")
	}
	if _, err := v.hash.Write(chunk.GetData()); err != nil {
		return fmt.Errorf("pluginsdk: hash runtime artifact: %w", err)
	}
	if chunk.GetFinal() {
		if v.size != v.total || hex.EncodeToString(v.hash.Sum(nil)) != chunk.GetSha256() {
			return status.Error(codes.DataLoss, "pluginsdk: runtime artifact digest or size mismatch")
		}
		v.final = true
	} else if v.size == v.total {
		return status.Error(codes.DataLoss, "pluginsdk: final runtime artifact chunk is missing")
	}
	return nil
}

func (v *executorArtifactStreamValidator) complete() bool {
	return v.final
}

func artifactChunkMetadata(chunk *pluginv1.ExecutorRuntimeArtifactChunk) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s", chunk.GetArtifactId(), chunk.GetVersion(), chunk.GetTotalSizeBytes(), chunk.GetSha256())
}
