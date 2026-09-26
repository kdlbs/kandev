package lifecycle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	pluginExecutorInventoryMaxBytes    = 32 << 10
	pluginExecutorRuntimePort          = 8765
	pluginExecutorPhaseAllocating      = "allocating"
	pluginExecutorPhaseArtifactStaging = "artifact_staging"
	pluginExecutorPhaseBootstrapping   = "bootstrapping"
	pluginExecutorPhaseProvisioned     = "provisioned"
	pluginExecutorPhaseReady           = "ready"
	pluginExecutorPhaseCleanupPending  = "cleanup_pending"
	pluginExecutorPhaseExpired         = "expired"
	pluginExecutorPhaseAbsent          = "absent"
	pluginExecutorStateRunning         = "running"
	pluginExecutorStateSuspended       = "suspended"
	pluginExecutorStateTerminated      = "terminated"
	pluginExecutorStateUnknown         = "unknown"
	pluginExecutorStateUnavailable     = "unavailable"
	pluginExecutorRetentionBounded     = "bounded"
)

type PluginExecutorLaunch struct {
	Profile models.ExecutorProviderLaunchProfile
}

type PluginExecutorProviderOperations interface {
	ProvisionExecutorEnvironment(context.Context, *pluginsdk.ProvisionExecutorEnvironmentRequest) (*pluginsdk.ProvisionExecutorEnvironmentResponse, error)
	RecoverExecutorOperation(context.Context, *pluginsdk.RecoverExecutorOperationRequest) (*pluginsdk.RecoverExecutorOperationResponse, error)
	AttachExecutorEnvironment(context.Context, *pluginsdk.AttachExecutorEnvironmentRequest) (*pluginsdk.AttachExecutorEnvironmentResponse, error)
	InspectExecutorEnvironment(context.Context, *pluginsdk.InspectExecutorEnvironmentRequest) (*pluginsdk.InspectExecutorEnvironmentResponse, error)
	ResolveExecutorConnection(context.Context, *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error)
	DestroyExecutorEnvironment(context.Context, *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error)
}

type PluginExecutorProfileLoader interface {
	ExecutorProviderProfileForLaunch(context.Context, string, string) (*models.ExecutorProviderLaunchProfile, error)
	ExecutorProviderProfileForRecovery(context.Context, string, string, string, string, int64, map[string]string, map[string]string) (*models.ExecutorProviderLaunchProfile, error)
}

type PluginExecutorInventoryStore interface {
	CheckpointPluginExecutorInventory(context.Context, *models.ExecutorRunning) error
	DeletePluginExecutorInventoryIfCurrent(context.Context, string, string, int64) error
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	AcquireTaskEnvironmentRecoveryClaim(context.Context, models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error)
	ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	ListExecutorsRunningPluginRemote(context.Context) ([]*models.ExecutorRunning, error)
}

type pluginEndpointClientFactory func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.Client, string, error)
type pluginRecoveredAgentctlClientFactory func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.Client, error)
type pluginAgentctlReadinessCheck func(context.Context, *agentctl.Client) error

type PluginRemoteExecutor struct {
	operations                 PluginExecutorProviderOperations
	logger                     *logger.Logger
	profileLoader              PluginExecutorProfileLoader
	inventoryStore             PluginExecutorInventoryStore
	newAgentctlClient          pluginEndpointClientFactory
	newRecoveredAgentctlClient pluginRecoveredAgentctlClientFactory
	ready                      pluginAgentctlReadinessCheck
}

func (r *PluginRemoteExecutor) SetRecoveryDependencies(loader PluginExecutorProfileLoader, inventory PluginExecutorInventoryStore) {
	r.profileLoader = loader
	r.inventoryStore = inventory
}

type pluginExecutorInventory struct {
	PluginID               string                                `json:"plugin_id"`
	InstallationID         string                                `json:"installation_id"`
	ProviderKey            string                                `json:"provider_key"`
	ProviderIdentity       string                                `json:"provider_identity"`
	EnvironmentID          string                                `json:"environment_id"`
	ContractVersion        int                                   `json:"contract_version"`
	SupportedStateVersions []int                                 `json:"supported_state_versions"`
	StateVersion           uint32                                `json:"state_version,omitempty"`
	ProfileID              string                                `json:"profile_id"`
	ProfileConfig          map[string]string                     `json:"profile_config,omitempty"`
	SecretReferences       map[string]string                     `json:"secret_references,omitempty"`
	OperationID            string                                `json:"operation_id"`
	InputDigest            string                                `json:"input_digest"`
	RuntimeIdentity        string                                `json:"runtime_identity,omitempty"`
	Phase                  string                                `json:"phase"`
	Resource               *pluginsdk.ExecutorResourceDescriptor `json:"resource,omitempty"`
	Platform               string                                `json:"platform,omitempty"`
	Capabilities           models.ExecutorProviderCapabilities   `json:"capabilities"`
	ExpiresAt              string                                `json:"expires_at,omitempty"`
	ExpiryCheckAt          string                                `json:"expiry_check_at,omitempty"`
	LastInspectionState    string                                `json:"last_inspection_state,omitempty"`
	Revision               uint64                                `json:"revision"`
	Generation             uint64                                `json:"generation"`
	EnvironmentGeneration  int64                                 `json:"environment_generation"`
}

func NewPluginRemoteExecutor(operations PluginExecutorProviderOperations, log *logger.Logger) *PluginRemoteExecutor {
	if log == nil {
		log = logger.Default()
	}
	runtime := &PluginRemoteExecutor{operations: operations, logger: log.WithFields(zap.String("runtime", string(executor.NamePluginRemote)))}
	runtime.newAgentctlClient = createPluginEndpointClient
	runtime.newRecoveredAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		return agentctl.NewEndpointClient(ctx, resolver, log,
			agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token))
	}
	runtime.ready = func(ctx context.Context, client *agentctl.Client) error { return client.Health(ctx) }
	return runtime
}

func createPluginEndpointClient(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, nonce string) (*agentctl.Client, string, error) {
	client, err := agentctl.NewEndpointClient(ctx, resolver, log, agentctl.WithExecutionID(executionID))
	if err != nil {
		return nil, "", err
	}
	token, err := client.BootstrapHandshake(ctx, nonce)
	if err != nil {
		client.Close()
		return nil, "", err
	}
	return client, token, nil
}

func (r *PluginRemoteExecutor) Name() executor.Name { return executor.NamePluginRemote }

func (r *PluginRemoteExecutor) HealthCheck(context.Context) error {
	if r.operations == nil {
		return errors.New("plugin executor provider service is unavailable")
	}
	return nil
}

func (r *PluginRemoteExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	if req != nil && req.WorkspaceReuseRequired {
		return r.attachRetainedPluginExecutor(ctx, req)
	}
	launch, err := r.preparePluginExecutorLaunch(ctx, req)
	if err != nil {
		return nil, err
	}
	resource, err := r.provisionPluginExecutor(ctx, launch)
	if err != nil {
		return nil, err
	}
	client, token, err := r.startPluginExecutorAgentctl(ctx, launch, resource)
	if err != nil {
		return nil, err
	}
	return r.finishPluginExecutorLaunch(ctx, launch, resource, client, token)
}

type pluginExecutorLaunchState struct {
	request          *ExecutorCreateRequest
	profile          models.ExecutorProviderLaunchProfile
	operationContext *pluginsdk.ExecutorProviderRequestContext
	apiURL           string
	nonce            string
	inventory        pluginExecutorInventory
}

func (r *PluginRemoteExecutor) preparePluginExecutorLaunch(ctx context.Context, req *ExecutorCreateRequest) (*pluginExecutorLaunchState, error) {
	if err := r.validateLaunch(req); err != nil {
		return nil, err
	}
	apiURL, err := validatePluginRuntimeAPIURL(req.Env[envKeyKandevAPIURL])
	if err != nil {
		return nil, err
	}
	profile := req.PluginExecutor.Profile
	if err := r.rejectExpiredPluginExecutorEnvironment(ctx, req.TaskEnvironmentID, profile.OwnershipGeneration); err != nil {
		return nil, err
	}
	operationID := strings.TrimSpace(req.InstanceID)
	digest, err := pluginExecutorInputDigest(profile)
	if err != nil {
		return nil, err
	}
	nonce, err := newPluginExecutorNonce()
	if err != nil {
		return nil, errors.New("plugin executor bootstrap could not be initialized")
	}
	operationContext := pluginExecutorRequestContext(ctx, req, profile, operationID, digest)
	inventory := pluginExecutorLaunchInventory(req, profile, operationID, digest)
	if err := checkpointPluginExecutor(ctx, req, inventory); err != nil {
		return nil, fmt.Errorf("persist plugin executor allocation intent: %w", err)
	}
	return &pluginExecutorLaunchState{
		request: req, profile: profile, operationContext: operationContext,
		apiURL: apiURL, nonce: nonce, inventory: inventory,
	}, nil
}

func pluginExecutorLaunchInventory(req *ExecutorCreateRequest, profile models.ExecutorProviderLaunchProfile, operationID, digest string) pluginExecutorInventory {
	return pluginExecutorInventory{
		PluginID: profile.Provider.PluginID, InstallationID: profile.Provider.InstallationID,
		ProviderKey: profile.Provider.Key, ProviderIdentity: profile.Provider.Identity,
		EnvironmentID:          req.TaskEnvironmentID,
		ContractVersion:        profile.Provider.ContractVersion,
		SupportedStateVersions: append([]int(nil), profile.Provider.SupportedStateVersions...),
		ProfileID:              profile.ProfileID, ProfileConfig: clonePluginExecutorStringMap(profile.Config),
		SecretReferences: clonePluginExecutorStringMap(profile.SecretReferences),
		OperationID:      operationID, InputDigest: digest, RuntimeIdentity: req.InstanceID, Phase: pluginExecutorPhaseAllocating,
		Capabilities: profile.Provider.Capabilities, EnvironmentGeneration: profile.OwnershipGeneration,
	}
}

func (r *PluginRemoteExecutor) provisionPluginExecutor(ctx context.Context, launch *pluginExecutorLaunchState) (*pluginsdk.ExecutorResourceDescriptor, error) {
	bootstrap, err := json.Marshal(map[string]any{
		"nonce": launch.nonce, "runtime_port": pluginExecutorRuntimePort,
		"api_url": launch.apiURL, "execution_id": launch.request.InstanceID, "artifact_id": "agentctl",
	})
	if err != nil {
		return nil, errors.New("plugin executor bootstrap could not be encoded")
	}
	provisioned, err := r.operations.ProvisionExecutorEnvironment(ctx, &pluginsdk.ProvisionExecutorEnvironmentRequest{
		Context:       launch.operationContext,
		Profile:       &pluginsdk.ExecutorProfileSnapshot{ProfileId: launch.profile.ProfileID, Config: launch.profile.Config, SecretValues: launch.profile.SecretValues},
		BootstrapJson: string(bootstrap),
	})
	if err != nil || provisioned == nil || provisioned.GetResource() == nil || provisioned.GetResource().GetResourceHandle() == "" {
		return nil, r.recoverOrRetainFailedProvision(ctx, launch.request, launch.operationContext, launch.profile, launch.inventory, err)
	}
	resource := provisioned.GetResource()
	if provisioned.GetError() != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource)
	}
	if err := validatePluginExecutorResourceForProvider(launch.profile.Provider, resource); err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource)
	}
	launch.inventory.Resource = resource
	launch.inventory.StateVersion = resource.GetStateVersion()
	launch.inventory.Platform = resource.GetPlatform()
	launch.inventory.ExpiresAt = resource.GetExpiresAt()
	launch.inventory.Capabilities = intersectPluginExecutorCapabilities(launch.profile.Provider.Capabilities, resource.GetCapabilities(), resource.GetRetention())
	launch.inventory.Phase = pluginExecutorPhaseProvisioned
	if err := checkpointPluginExecutor(ctx, launch.request, launch.inventory); err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource)
	}
	return resource, nil
}

func (r *PluginRemoteExecutor) startPluginExecutorAgentctl(ctx context.Context, launch *pluginExecutorLaunchState, resource *pluginsdk.ExecutorResourceDescriptor) (*agentctl.Client, string, error) {
	connectionResolver := r.connectionResolver(launch.operationContext, resource)
	client, token, err := r.newAgentctlClient(ctx, connectionResolver, r.logger, launch.request.InstanceID, launch.nonce)
	if err != nil {
		return nil, "", r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource)
	}
	if err := r.ready(ctx, client); err != nil {
		client.Close()
		return nil, "", r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource)
	}
	return client, token, nil
}

func (r *PluginRemoteExecutor) finishPluginExecutorLaunch(ctx context.Context, launch *pluginExecutorLaunchState, resource *pluginsdk.ExecutorResourceDescriptor, client *agentctl.Client, token string) (*ExecutorInstance, error) {
	launch.inventory.Phase = pluginExecutorPhaseReady
	launch.inventory.Generation++
	if err := checkpointPluginExecutor(ctx, launch.request, launch.inventory); err != nil {
		client.Close()
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource)
	}
	metadata := clonePluginExecutorMetadata(launch.request.Metadata)
	metadata[MetadataKeyPluginExecutor] = launch.inventory
	return &ExecutorInstance{
		InstanceID: launch.request.InstanceID, TaskID: launch.request.TaskID, SessionID: launch.request.SessionID,
		RuntimeName: agentruntime.RuntimePluginRemote, Client: client,
		WorkspacePath: "/workspace", Metadata: metadata, AuthToken: token,
		ReleaseRuntimeInventory: launch.request.ReleaseRuntimeInventory,
	}, nil
}

func (r *PluginRemoteExecutor) validateLaunch(req *ExecutorCreateRequest) error {
	if r.operations == nil {
		return errors.New("plugin executor provider service is unavailable")
	}
	if !pluginExecutorLaunchIdentityComplete(req) {
		return errors.New("plugin executor launch identity is incomplete")
	}
	if req.WorkspaceReuseRequired || isPluginExecutorSharedWorkspace(req.Metadata) {
		return errors.New("plugin executor requires an isolated session workspace")
	}
	if req.CheckpointRuntimeInventory == nil || req.ReleaseRuntimeInventory == nil {
		return errors.New("plugin executor durable inventory callbacks are unavailable")
	}
	provider := req.PluginExecutor.Profile.Provider
	if !pluginExecutorProviderIdentityComplete(provider) {
		return errors.New("plugin executor provider is unavailable")
	}
	if req.PluginExecutor.Profile.ProfileID == "" {
		return errors.New("plugin executor profile is required")
	}
	return nil
}

func pluginExecutorLaunchIdentityComplete(req *ExecutorCreateRequest) bool {
	return req != nil && req.PluginExecutor != nil && req.InstanceID != "" && req.TaskID != "" &&
		req.SessionID != "" && req.TaskEnvironmentID != ""
}

func pluginExecutorProviderIdentityComplete(provider models.ExecutorProvider) bool {
	return provider.Available && provider.PluginID != "" && provider.InstallationID != "" &&
		provider.Key != "" && provider.Identity != ""
}

func (r *PluginRemoteExecutor) rejectExpiredPluginExecutorEnvironment(ctx context.Context, environmentID string, generation int64) error {
	if r.inventoryStore == nil {
		return nil
	}
	records, err := r.inventoryStore.ListExecutorsRunningPluginRemote(ctx)
	if err != nil {
		return fmt.Errorf("read plugin executor expiry inventory: %w", err)
	}
	for _, record := range records {
		if record == nil || record.Runtime != agentruntime.RuntimePluginRemote {
			continue
		}
		inventory, err := decodePluginExecutorInventory(record.Metadata)
		if err != nil {
			return err
		}
		if inventory.EnvironmentID == environmentID && inventory.EnvironmentGeneration == generation && inventory.Phase == pluginExecutorPhaseExpired {
			return errors.New("plugin executor environment expired; reset the environment before starting another run")
		}
	}
	return nil
}

func (r *PluginRemoteExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, _ bool) error {
	if instance == nil {
		return nil
	}
	if instance.StopReason == StopReasonBackendShutdown {
		if instance.Client != nil {
			instance.Client.Close()
		}
		return nil
	}
	var stopErr error
	if instance.Client != nil {
		stopErr = instance.Client.Stop(ctx)
		instance.Client.Close()
	}
	if shouldRunExecutorCleanup(instance.StopReason) {
		if err := r.cleanupPluginExecutorInstance(ctx, instance); err != nil {
			return errors.Join(stopErr, err)
		}
	}
	return stopErr
}

func (r *PluginRemoteExecutor) RecoverInstances(ctx context.Context, records []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	if r.operations == nil || r.profileLoader == nil || r.inventoryStore == nil {
		return nil, errors.New("plugin executor recovery dependencies are unavailable")
	}
	instances := make([]*ExecutorInstance, 0)
	for _, record := range records {
		if record == nil || record.Runtime != agentruntime.RuntimePluginRemote {
			continue
		}
		instance, err := r.recoverPluginExecutor(ctx, record)
		if err != nil {
			r.logger.Warn("plugin executor recovery deferred", zap.String("session_id", record.SessionID), zap.Error(err))
			continue
		}
		if instance != nil {
			instances = append(instances, instance)
		}
	}
	return instances, nil
}

func (r *PluginRemoteExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (r *PluginRemoteExecutor) RequiresCloneURL() bool                           { return true }
func (r *PluginRemoteExecutor) ShouldApplyPreferredShell() bool                  { return false }
func (r *PluginRemoteExecutor) IsAlwaysResumable() bool                          { return false }

func isPluginExecutorSharedWorkspace(metadata map[string]interface{}) bool {
	workspace, _ := metadata["workspace"].(map[string]interface{})
	mode, _ := workspace["mode"].(string)
	return mode == "shared_group" || mode == "inherit_parent"
}

func validatePluginRuntimeAPIURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("plugin executor requires a reachable HTTPS Kandev API URL")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && (ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsPrivate()) {
		return "", errors.New("plugin executor Kandev API URL cannot use a local or private address")
	}
	if strings.EqualFold(parsed.Hostname(), "localhost") || strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".localhost") {
		return "", errors.New("plugin executor Kandev API URL cannot use localhost")
	}
	return parsed.String(), nil
}

func pluginExecutorInputDigest(profile models.ExecutorProviderLaunchProfile) (string, error) {
	data, err := json.Marshal(struct {
		ProviderIdentity string            `json:"provider_identity"`
		ProfileID        string            `json:"profile_id"`
		Config           map[string]string `json:"config"`
		SecretReferences map[string]string `json:"secret_references"`
	}{profile.Provider.Identity, profile.ProfileID, profile.Config, profile.SecretReferences})
	if err != nil {
		return "", errors.New("plugin executor profile snapshot could not be hashed")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func pluginExecutorRequestContext(ctx context.Context, req *ExecutorCreateRequest, profile models.ExecutorProviderLaunchProfile, operationID, digest string) *pluginsdk.ExecutorProviderRequestContext {
	deadline := time.Now().Add(10 * time.Minute)
	if value, ok := ctx.Deadline(); ok {
		deadline = value
	}
	return &pluginsdk.ExecutorProviderRequestContext{
		PluginId: profile.Provider.PluginID, InstallationId: profile.Provider.InstallationID, ProviderKey: profile.Provider.Key,
		ContractVersion: int32(profile.Provider.ContractVersion), TaskId: req.TaskID, SessionId: req.SessionID,
		EnvironmentId: req.TaskEnvironmentID, ExecutionId: req.InstanceID, OperationId: operationID,
		Deadline: deadline.UTC().Format(time.RFC3339Nano), InputDigest: digest,
		EnvironmentGeneration: uint64(profile.OwnershipGeneration),
	}
}

func newPluginExecutorNonce() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validatePluginExecutorResource(resource *pluginsdk.ExecutorResourceDescriptor) error {
	if resource == nil || resource.GetResourceHandle() == "" {
		return errors.New("provider returned no resource handle")
	}
	state := resource.GetStateJson()
	if len(state) > pluginExecutorInventoryMaxBytes {
		return errors.New("provider resource state exceeds the inventory limit")
	}
	if state == "" {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(state), &object); err != nil || object == nil {
		return errors.New("provider resource state is invalid")
	}
	return nil
}

func validatePluginExecutorResourceForProvider(provider models.ExecutorProvider, resource *pluginsdk.ExecutorResourceDescriptor) error {
	if err := validatePluginExecutorResource(resource); err != nil {
		return err
	}
	for _, supported := range provider.SupportedStateVersions {
		if supported > 0 && uint32(supported) == resource.GetStateVersion() {
			return nil
		}
	}
	return errors.New("provider resource state version is unsupported")
}

func intersectPluginExecutorCapabilities(base models.ExecutorProviderCapabilities, reported *pluginsdk.ExecutorProviderCapabilities, resourceRetention ...string) models.ExecutorProviderCapabilities {
	retention := pluginExecutorEffectiveRetention(base.Retention, reported.GetRetention(), resourceRetention)
	maximum := pluginExecutorEffectiveMaximumLifetime(base.MaximumLifetimeSecs, reported.GetMaximumLifetimeSeconds())
	return models.ExecutorProviderCapabilities{
		Terminal: base.Terminal && reported.GetTerminal(), Files: base.Files && reported.GetFiles(),
		Git: base.Git && reported.GetGit(), EmbeddedEditor: base.EmbeddedEditor && reported.GetEmbeddedEditor(),
		Preview: base.Preview && reported.GetPreview(), Reattach: base.Reattach && reported.GetReattach(),
		Retention: retention, MaximumLifetimeSecs: maximum,
	}
}

func pluginExecutorEffectiveRetention(base, reported string, resourceRetention []string) string {
	retention := reported
	if len(resourceRetention) > 0 && resourceRetention[0] != "" {
		if retention == "" || pluginExecutorRetentionRank(resourceRetention[0]) < pluginExecutorRetentionRank(retention) {
			retention = resourceRetention[0]
		}
	}
	if retention == "" || base == pluginExecutorStateUnknown {
		retention = pluginExecutorStateUnknown
	} else if pluginExecutorRetentionRank(retention) > pluginExecutorRetentionRank(base) {
		retention = base
	}
	return retention
}

func pluginExecutorEffectiveMaximumLifetime(base int64, reported uint64) int64 {
	if reported > 0 && (base == 0 || int64(reported) < base) {
		return int64(reported)
	}
	return base
}

func pluginExecutorRetentionRank(retention string) int {
	switch retention {
	case "persistent":
		return 3
	case pluginExecutorRetentionBounded:
		return 2
	case "ephemeral":
		return 1
	default:
		return 0
	}
}

func checkpointPluginExecutor(ctx context.Context, req *ExecutorCreateRequest, inventory pluginExecutorInventory) error {
	data, err := json.Marshal(inventory)
	if err != nil || len(data) > pluginExecutorInventoryMaxBytes {
		return errors.New("plugin executor inventory exceeds the 32 KiB limit")
	}
	return req.CheckpointRuntimeInventory(ctx, map[string]interface{}{MetadataKeyPluginExecutor: inventory})
}

func (r *PluginRemoteExecutor) connectionResolver(operationContext *pluginsdk.ExecutorProviderRequestContext, resource *pluginsdk.ExecutorResourceDescriptor) agentctl.ConnectionLeaseResolver {
	return func(ctx context.Context) (*agentctl.ConnectionLease, error) {
		requestContext := proto.Clone(operationContext).(*pluginsdk.ExecutorProviderRequestContext)
		requestContext.Deadline = time.Now().Add(15 * time.Second).UTC().Format(time.RFC3339Nano)
		response, err := r.operations.ResolveExecutorConnection(ctx, &pluginsdk.ResolveExecutorConnectionRequest{
			Context: requestContext, Resource: resource, Purpose: "agentctl", RuntimePort: pluginExecutorRuntimePort,
		})
		if err != nil || response == nil || response.GetLease() == nil {
			return nil, errors.New("provider connection lease is unavailable")
		}
		if response.GetError() != nil {
			return nil, errors.New("provider connection lease was rejected")
		}
		lease := response.GetLease()
		expiresAt, err := time.Parse(time.RFC3339Nano, lease.GetExpiresAt())
		if err != nil {
			return nil, errors.New("provider connection lease expiry is invalid")
		}
		return &agentctl.ConnectionLease{
			BaseURL: lease.GetBaseUrl(), HTTPHeaders: stringMapToHeader(lease.GetHttpHeaders()),
			WebSocketHeaders:      stringMapToHeader(lease.GetWebsocketHeaders()),
			WebSocketSubprotocols: append([]string(nil), lease.GetWebsocketSubprotocols()...),
			ExpiresAt:             expiresAt, Generation: lease.GetGeneration(),
		}, nil
	}
}

func stringMapToHeader(values map[string]string) http.Header {
	header := make(http.Header, len(values))
	for key, value := range values {
		header.Set(key, value)
	}
	return header
}

func clonePluginExecutorStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func clonePluginExecutorMetadata(source map[string]interface{}) map[string]interface{} {
	return cloneKubernetesMetadata(source)
}

func (r *PluginRemoteExecutor) recoverOrRetainFailedProvision(
	ctx context.Context,
	req *ExecutorCreateRequest,
	operationContext *pluginsdk.ExecutorProviderRequestContext,
	profile models.ExecutorProviderLaunchProfile,
	inventory pluginExecutorInventory,
	provisionErr error,
) error {
	recovery, err := r.operations.RecoverExecutorOperation(ctx, &pluginsdk.RecoverExecutorOperationRequest{
		Context: operationContext,
		Profile: &pluginsdk.ExecutorProfileSnapshot{ProfileId: profile.ProfileID, Config: profile.Config, SecretValues: profile.SecretValues},
	})
	if err != nil || recovery == nil || recovery.GetOutcome() == pluginExecutorStateUnknown {
		inventory.Phase = pluginExecutorPhaseCleanupPending
		_ = checkpointPluginExecutor(ctx, req, inventory)
		return errors.New("plugin executor allocation outcome is unknown; cleanup inventory was retained")
	}
	if recovery.GetOutcome() == pluginExecutorPhaseAbsent {
		claim, claimErr := r.acquirePluginExecutorCleanupClaim(ctx, req.TaskID, req.SessionID, inventory)
		if claimErr != nil {
			inventory.Phase = pluginExecutorPhaseCleanupPending
			_ = checkpointPluginExecutor(ctx, req, inventory)
			return errors.New("plugin executor allocation is absent but inventory cleanup is fenced")
		}
		defer r.releasePluginExecutorCleanupClaim(claim)
		claimedCtx := recoveryclaim.WithClaim(ctx, claim)
		inventory.Phase = pluginExecutorPhaseAbsent
		if err := checkpointPluginExecutor(claimedCtx, req, inventory); err != nil {
			return err
		}
		if err := req.ReleaseRuntimeInventory(claimedCtx); err != nil {
			return fmt.Errorf("release absent plugin executor inventory: %w", err)
		}
		return errors.New("plugin executor allocation failed before creating a resource")
	}
	resource := recovery.GetResource()
	if resource == nil || resource.GetResourceHandle() == "" {
		inventory.Phase = pluginExecutorPhaseCleanupPending
		_ = checkpointPluginExecutor(ctx, req, inventory)
		return errors.New("plugin executor allocation outcome is unknown; cleanup inventory was retained")
	}
	return r.cleanupAfterPluginExecutorFailure(ctx, req, operationContext, inventory, resource)
}

func (r *PluginRemoteExecutor) cleanupAfterPluginExecutorFailure(
	ctx context.Context,
	req *ExecutorCreateRequest,
	operationContext *pluginsdk.ExecutorProviderRequestContext,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	claim, claimErr := r.acquirePluginExecutorCleanupClaim(cleanupCtx, req.TaskID, req.SessionID, inventory)
	if claimErr != nil {
		inventory.Resource = resource
		inventory.Phase = pluginExecutorPhaseCleanupPending
		_ = checkpointPluginExecutor(cleanupCtx, req, inventory)
		return errors.New("plugin executor launch failed; cleanup is fenced and retry information was retained")
	}
	defer r.releasePluginExecutorCleanupClaim(claim)
	cleanupCtx = recoveryclaim.WithClaim(cleanupCtx, claim)
	cleanupContext := proto.Clone(operationContext).(*pluginsdk.ExecutorProviderRequestContext)
	cleanupContext.Deadline = time.Now().Add(30 * time.Second).UTC().Format(time.RFC3339Nano)
	response, err := r.operations.DestroyExecutorEnvironment(cleanupCtx, &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: cleanupContext, Resource: resource, CleanupReason: "launch_failed", CleanupClaim: claim.OperationID,
	})
	if err == nil && response != nil && response.GetConfirmedAbsent() {
		inventory.Resource = resource
		inventory.Phase = pluginExecutorPhaseAbsent
		if checkpointErr := checkpointPluginExecutor(cleanupCtx, req, inventory); checkpointErr != nil {
			return fmt.Errorf("remote launch failed and absence checkpoint failed: %w", checkpointErr)
		}
		if releaseErr := req.ReleaseRuntimeInventory(cleanupCtx); releaseErr != nil {
			return fmt.Errorf("remote launch failed and inventory release failed: %w", releaseErr)
		}
		return errors.New("plugin executor launch failed; allocated resource was removed")
	}
	inventory.Resource = resource
	inventory.Phase = pluginExecutorPhaseCleanupPending
	if checkpointErr := checkpointPluginExecutor(cleanupCtx, req, inventory); checkpointErr != nil {
		return fmt.Errorf("remote launch failed and cleanup inventory checkpoint failed: %w", checkpointErr)
	}
	r.logger.Warn("plugin executor cleanup remains pending", zap.String("execution_id", req.InstanceID))
	return errors.New("plugin executor launch failed; resource cleanup remains pending")
}

var _ ExecutorBackend = (*PluginRemoteExecutor)(nil)
