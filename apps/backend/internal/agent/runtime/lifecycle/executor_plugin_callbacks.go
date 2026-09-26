package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

const maxPluginExecutorArtifactBytes = 128 << 20

func (m *Manager) registerPluginExecutorCallbacks(req *ExecutorCreateRequest) error {
	if req == nil || req.PluginExecutor == nil || req.InstanceID == "" {
		return errors.New("plugin executor callback identity is incomplete")
	}
	m.pluginExecutorCallbackMu.Lock()
	defer m.pluginExecutorCallbackMu.Unlock()
	if m.pluginExecutorCallbacks == nil {
		m.pluginExecutorCallbacks = make(map[string]*ExecutorCreateRequest)
	}
	if _, exists := m.pluginExecutorCallbacks[req.InstanceID]; exists {
		return errors.New("plugin executor callback operation is already active")
	}
	m.pluginExecutorCallbacks[req.InstanceID] = req
	return nil
}

func (m *Manager) unregisterPluginExecutorCallbacks(executionID string) {
	m.pluginExecutorCallbackMu.Lock()
	delete(m.pluginExecutorCallbacks, executionID)
	m.pluginExecutorCallbackMu.Unlock()
}

func (m *Manager) pluginExecutorCallbackRequest(ctx context.Context, request *pluginsdk.ExecutorProviderRequestContext) (*ExecutorCreateRequest, error) {
	if err := validatePluginExecutorCallbackLease(request); err != nil {
		return nil, err
	}
	req := m.activePluginExecutorCallback(request.GetExecutionId())
	if !pluginExecutorCallbackMatchesLaunch(req, request) {
		return nil, errors.New("plugin executor callback no longer owns this execution")
	}
	if !pluginExecutorCallbackMatchesProvider(req, request) {
		return nil, errors.New("plugin executor callback identity does not match the active launch")
	}
	if err := m.validatePluginExecutorOwnership(ctx, req, request); err != nil {
		return nil, err
	}
	return req, nil
}

func validatePluginExecutorCallbackLease(request *pluginsdk.ExecutorProviderRequestContext) error {
	if request == nil || request.GetOperationId() == "" || request.GetDispatchGeneration() == 0 {
		return errors.New("plugin executor callback lease is incomplete")
	}
	return nil
}

func (m *Manager) activePluginExecutorCallback(executionID string) *ExecutorCreateRequest {
	m.pluginExecutorCallbackMu.Lock()
	defer m.pluginExecutorCallbackMu.Unlock()
	return m.pluginExecutorCallbacks[executionID]
}

func pluginExecutorCallbackMatchesLaunch(req *ExecutorCreateRequest, request *pluginsdk.ExecutorProviderRequestContext) bool {
	return req != nil && req.PluginExecutor != nil && req.InstanceID == request.GetExecutionId() &&
		req.TaskID == request.GetTaskId() && req.SessionID == request.GetSessionId() &&
		req.TaskEnvironmentID == request.GetEnvironmentId() && req.InstanceID == request.GetOperationId()
}

func pluginExecutorCallbackMatchesProvider(req *ExecutorCreateRequest, request *pluginsdk.ExecutorProviderRequestContext) bool {
	provider := req.PluginExecutor.Profile.Provider
	return provider.PluginID == request.GetPluginId() && provider.InstallationID == request.GetInstallationId() &&
		provider.Key == request.GetProviderKey() && int32(provider.ContractVersion) == request.GetContractVersion() &&
		uint64(req.PluginExecutor.Profile.OwnershipGeneration) == request.GetEnvironmentGeneration()
}

func (m *Manager) validatePluginExecutorOwnership(ctx context.Context, req *ExecutorCreateRequest, request *pluginsdk.ExecutorProviderRequestContext) error {
	if m.executorProfileReader == nil {
		return errors.New("plugin executor ownership reader is unavailable")
	}
	session, err := m.executorProfileReader.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil || session.TaskID != req.TaskID || session.TaskEnvironmentID != req.TaskEnvironmentID {
		return errors.New("plugin executor session ownership changed")
	}
	environment, err := m.executorProfileReader.GetTaskEnvironment(ctx, req.TaskEnvironmentID)
	if err != nil || environment == nil || environment.TaskID != req.TaskID ||
		uint64(environment.OwnershipGeneration) != request.GetEnvironmentGeneration() {
		return errors.New("plugin executor environment ownership changed")
	}
	return nil
}

func (m *Manager) CheckpointExecutorResource(ctx context.Context, request *pluginsdk.CheckpointExecutorResourceRequest) error {
	req, err := m.pluginExecutorCallbackRequest(ctx, request.GetContext())
	if err != nil {
		return err
	}
	phase := strings.TrimSpace(request.GetPhase())
	if phase != pluginExecutorPhaseProvisioned && phase != pluginExecutorPhaseBootstrapping && phase != pluginExecutorPhaseArtifactStaging {
		return errors.New("plugin executor checkpoint phase is unsupported")
	}
	if err := validatePluginExecutorResourceForProvider(req.PluginExecutor.Profile.Provider, request.GetResource()); err != nil {
		return err
	}
	inventory := pluginExecutorInventoryForLaunch(req, request.GetContext())
	inventory.Phase = phase
	inventory.Resource = request.GetResource()
	inventory.StateVersion = request.GetResource().GetStateVersion()
	inventory.Platform = request.GetResource().GetPlatform()
	if existing, decodeErr := decodePluginExecutorInventory(req.Metadata); decodeErr == nil &&
		existing.EnvironmentID == inventory.EnvironmentID && existing.EnvironmentGeneration == inventory.EnvironmentGeneration {
		inventory.ExpiresAt = existing.ExpiresAt
	}
	inventory.ExpiresAt, err = pluginExecutorEarlierExpiry(inventory.ExpiresAt, request.GetResource().GetExpiresAt())
	if err != nil {
		return err
	}
	inventory.Capabilities = intersectPluginExecutorCapabilities(
		req.PluginExecutor.Profile.Provider.Capabilities,
		request.GetResource().GetCapabilities(),
		request.GetResource().GetRetention(),
	)
	return checkpointPluginExecutor(ctx, req, inventory)
}

func (m *Manager) ReportExecutorProgress(ctx context.Context, request *pluginsdk.ReportExecutorProgressRequest) error {
	req, err := m.pluginExecutorCallbackRequest(ctx, request.GetContext())
	if err != nil {
		return err
	}
	stage := strings.TrimSpace(request.GetStage())
	switch stage {
	case "provision", "artifact", "bootstrap", "readiness":
	default:
		return errors.New("plugin executor progress stage is unsupported")
	}
	total, completed := request.GetTotal(), request.GetCompleted()
	if total == 0 || total > 1000 || completed > total {
		return errors.New("plugin executor progress counts are invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.OnProgress == nil {
		return nil
	}
	status := PrepareStepRunning
	if completed == total {
		status = PrepareStepCompleted
	}
	req.OnProgress(PrepareStep{Name: stage, Status: status}, int(completed), int(total))
	return nil
}

func (m *Manager) ReadExecutorRuntimeArtifact(ctx context.Context, request *pluginsdk.ReadExecutorRuntimeArtifactRequest, send func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error {
	if _, err := m.pluginExecutorCallbackRequest(ctx, request.GetContext()); err != nil {
		return err
	}
	if request.GetArtifactId() != "agentctl" || send == nil {
		return errors.New("plugin executor runtime artifact request is invalid")
	}
	platform, err := parsePluginExecutorPlatform(request.GetPlatform())
	if err != nil {
		return err
	}
	path, err := NewAgentctlResolver(m.logger).ResolveRemoteBinary(platform)
	if err != nil {
		return errors.New("agentctl runtime artifact is unavailable for the requested platform")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > maxPluginExecutorArtifactBytes {
		return errors.New("agentctl runtime artifact could not be read")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	version := "sha256:" + hex.EncodeToString(digest[:])
	for offset, index := 0, uint32(0); offset < len(data); index++ {
		end := offset + pluginsdk.MaxExecutorRuntimeArtifactChunkBytes
		if end > len(data) {
			end = len(data)
		}
		if err := send(&pluginsdk.ExecutorRuntimeArtifactChunk{
			ArtifactId: request.GetArtifactId(), Version: version, TotalSizeBytes: uint64(len(data)),
			Sha256: hex.EncodeToString(digest[:]), ChunkIndex: index, Data: data[offset:end], Final: end == len(data),
		}); err != nil {
			return err
		}
		offset = end
	}
	return nil
}

func pluginExecutorInventoryForLaunch(req *ExecutorCreateRequest, request *pluginsdk.ExecutorProviderRequestContext) pluginExecutorInventory {
	profile := req.PluginExecutor.Profile
	return pluginExecutorInventory{
		PluginID: profile.Provider.PluginID, InstallationID: profile.Provider.InstallationID,
		ProviderKey: profile.Provider.Key, ProviderIdentity: profile.Provider.Identity,
		EnvironmentID:          req.TaskEnvironmentID,
		ContractVersion:        profile.Provider.ContractVersion,
		SupportedStateVersions: append([]int(nil), profile.Provider.SupportedStateVersions...),
		ProfileID:              profile.ProfileID, ProfileConfig: clonePluginExecutorStringMap(profile.Config),
		SecretReferences: clonePluginExecutorStringMap(profile.SecretReferences),
		OperationID:      request.GetOperationId(), InputDigest: request.GetInputDigest(),
		RuntimeIdentity:       request.GetExecutionId(),
		Capabilities:          profile.Provider.Capabilities,
		EnvironmentGeneration: int64(request.GetEnvironmentGeneration()),
	}
}

func parsePluginExecutorPlatform(raw string) (SSHRemotePlatform, error) {
	parts := strings.Split(strings.TrimSpace(raw), "-")
	if len(parts) != 2 {
		return SSHRemotePlatform{}, errors.New("plugin executor platform is unsupported")
	}
	platform := SSHRemotePlatform{GOOS: parts[0], GOARCH: parts[1]}
	if err := requireSupportedRemotePlatform(platform); err != nil {
		return SSHRemotePlatform{}, fmt.Errorf("plugin executor platform is unsupported: %w", err)
	}
	return platform, nil
}

var _ interface {
	CheckpointExecutorResource(context.Context, *pluginsdk.CheckpointExecutorResourceRequest) error
	ReportExecutorProgress(context.Context, *pluginsdk.ReportExecutorProgressRequest) error
	ReadExecutorRuntimeArtifact(context.Context, *pluginsdk.ReadExecutorRuntimeArtifactRequest, func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error
} = (*Manager)(nil)
