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
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/workspacedescriptor"
	"github.com/kandev/kandev/internal/runtimeprobe"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	pluginExecutorInventoryMaxBytes     = 32 << 10
	pluginExecutorRuntimePort           = 8765
	pluginExecutorWorkspacePath         = "/workspace"
	pluginExecutorAgentctlPurpose       = "agentctl"
	pluginExecutorPhaseAllocating       = "allocating"
	pluginExecutorPhaseArtifactStaging  = "artifact_staging"
	pluginExecutorPhaseBootstrapping    = "bootstrapping"
	pluginExecutorPhaseProvisioned      = "provisioned"
	pluginExecutorPhaseReady            = "ready"
	pluginExecutorPhaseCleanupPending   = "cleanup_pending"
	pluginExecutorPhaseExpired          = "expired"
	pluginExecutorPhaseAbsent           = "absent"
	pluginExecutorPhaseCleanupProgress  = "cleanup_progress"
	pluginExecutorPhaseRecoveryProgress = "recovery_progress"
	pluginExecutorStateRunning          = "running"
	pluginExecutorStateSuspended        = "suspended"
	pluginExecutorStateTerminated       = "terminated"
	pluginExecutorStateUnknown          = "unknown"
	pluginExecutorStateUnavailable      = "unavailable"
	pluginExecutorRetentionBounded      = "bounded"
	pluginExecutorCleanupReasonTask     = "task_cleanup"
	pluginExecutorCleanupReasonLaunch   = "launch_failed"
)

type PluginExecutorLaunch struct {
	Profile models.ExecutorProviderLaunchProfile
	// Repositories and PlainEnv feed the provider's repository intent and
	// agent_env; they are read only when the provider requires that capability.
	Repositories []PluginRepositorySource
	PlainEnv     map[string]string
}

type PluginExecutorProviderOperations interface {
	ProvisionExecutorEnvironment(context.Context, *pluginsdk.ProvisionExecutorEnvironmentRequest) (*pluginsdk.ProvisionExecutorEnvironmentResponse, error)
	RecoverExecutorOperation(context.Context, *pluginsdk.RecoverExecutorOperationRequest) (*pluginsdk.RecoverExecutorOperationResponse, error)
	AttachExecutorEnvironment(context.Context, *pluginsdk.AttachExecutorEnvironmentRequest) (*pluginsdk.AttachExecutorEnvironmentResponse, error)
	InspectExecutorEnvironment(context.Context, *pluginsdk.InspectExecutorEnvironmentRequest) (*pluginsdk.InspectExecutorEnvironmentResponse, error)
	ResolveExecutorConnection(context.Context, *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error)
	DestroyExecutorEnvironment(context.Context, *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error)
}

// PluginExecutorTunnelOperations is implemented by a provider host that can open
// host-initiated tunnel streams. A launch whose lease asks for host_tunnel
// fails closed when the operations value does not implement it.
type PluginExecutorTunnelOperations interface {
	OpenExecutorTunnel(ctx context.Context, requestContext *pluginsdk.ExecutorProviderRequestContext, resource *pluginsdk.ExecutorResourceDescriptor, port uint32, allowedPorts []uint32, profile *pluginsdk.ExecutorProfileSnapshot) (net.Conn, error)
	CloseExecutorTunnels(installationID, environmentID string)
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

type pluginControlClientFactory func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string) (*agentctl.ControlClient, string, error)
type pluginAgentctlClientFactory func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.Client, error)
type pluginRecoveredAgentctlInstanceReader func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.InstanceInfo, error)
type pluginAgentctlReadinessCheck func(context.Context, *agentctl.Client) error

type PluginRemoteExecutor struct {
	operations                  PluginExecutorProviderOperations
	logger                      *logger.Logger
	profileLoader               PluginExecutorProfileLoader
	inventoryStore              PluginExecutorInventoryStore
	newAgentctlControlClient    pluginControlClientFactory
	newAgentctlClient           pluginAgentctlClientFactory
	recoveredAgentctlInfoReader pluginRecoveredAgentctlInstanceReader
	ready                       pluginAgentctlReadinessCheck
	drainRuntime                pluginRuntimeDrainer
	runtimeSecrets              PluginExecutorRuntimeSecrets
	hostAPIURL                  string
	hostAPIProbes               *runtimeprobe.Registry
	hostAPIProbeInterval        time.Duration
	cleanupClaimWait            time.Duration
	launching                   sync.Map
	cleanupClaimRetry           time.Duration
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
	NegotiatedCapabilities []string                              `json:"negotiated_capabilities,omitempty"`
	WorkspaceDescriptor    *workspacedescriptor.Descriptor       `json:"workspace_descriptor,omitempty"`
	WorkspacePreparation   string                                `json:"workspace_preparation,omitempty"`
	Repositories           []pluginExecutorRepository            `json:"repositories,omitempty"`
	ProfileID              string                                `json:"profile_id"`
	ProfileConfig          map[string]string                     `json:"profile_config,omitempty"`
	SecretReferences       map[string]string                     `json:"secret_references,omitempty"`
	OperationID            string                                `json:"operation_id"`
	InputDigest            string                                `json:"input_digest"`
	RuntimeIdentity        string                                `json:"runtime_identity,omitempty"`
	InstancePort           uint32                                `json:"instance_port,omitempty"`
	Phase                  string                                `json:"phase"`
	Resource               *pluginsdk.ExecutorResourceDescriptor `json:"resource,omitempty"`
	Platform               string                                `json:"platform,omitempty"`
	Capabilities           models.ExecutorProviderCapabilities   `json:"capabilities"`
	ExpiresAt              string                                `json:"expires_at,omitempty"`
	ExpiryCheckAt          string                                `json:"expiry_check_at,omitempty"`
	LastInspectionState    string                                `json:"last_inspection_state,omitempty"`
	Claim                  *pluginExecutorClaimState             `json:"claim,omitempty"`
	DrainOutcome           string                                `json:"drain_outcome,omitempty"`
	DrainReceiptID         string                                `json:"drain_receipt_id,omitempty"`
	DrainClaim             string                                `json:"drain_claim,omitempty"`
	HandshakeStarted       bool                                  `json:"handshake_started,omitempty"`
	PublishPending         bool                                  `json:"publish_pending,omitempty"`
	CleanupRequested       bool                                  `json:"cleanup_requested,omitempty"`
	Revision               uint64                                `json:"revision"`
	Generation             uint64                                `json:"generation"`
	EnvironmentGeneration  int64                                 `json:"environment_generation"`
}

func NewPluginRemoteExecutor(operations PluginExecutorProviderOperations, log *logger.Logger) *PluginRemoteExecutor {
	if log == nil {
		log = logger.Default()
	}
	runtime := &PluginRemoteExecutor{operations: operations, logger: log.WithFields(zap.String("runtime", string(executor.NamePluginRemote)))}
	runtime.cleanupClaimWait, runtime.cleanupClaimRetry = pluginExecutorCleanupClaimWait, pluginExecutorCleanupClaimRetry
	runtime.newAgentctlControlClient = createPluginEndpointControlClient
	runtime.drainRuntime = defaultPluginRuntimeDrainer
	runtime.newAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		return agentctl.NewEndpointClient(ctx, resolver, log,
			agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token))
	}
	runtime.recoveredAgentctlInfoReader = func(
		ctx context.Context,
		resolver agentctl.ConnectionLeaseResolver,
		log *logger.Logger,
		executionID string,
		token string,
	) (*agentctl.InstanceInfo, error) {
		control, err := agentctl.NewEndpointControlClient(ctx, resolver, log, agentctl.WithControlAuthToken(token))
		if err != nil {
			return nil, err
		}
		defer control.Close()
		return control.GetInstance(ctx, executionID)
	}
	runtime.ready = func(ctx context.Context, client *agentctl.Client) error { return client.Health(ctx) }
	runtime.hostAPIProbeInterval = hostAPIProbeInterval
	return runtime
}

// createPluginEndpointControlClient performs the bootstrap handshake with agentctl's
// control server and returns the credential its instances accept.
func createPluginEndpointControlClient(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, nonce string) (*agentctl.ControlClient, string, error) {
	control, err := agentctl.NewEndpointControlClient(ctx, resolver, log)
	if err != nil {
		return nil, "", err
	}
	token, err := control.Handshake(ctx, nonce)
	if err != nil {
		control.Close()
		return nil, "", err
	}
	return control, token, nil
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
	r.launching.Store(req.InstanceID, struct{}{})
	defer r.launching.Delete(req.InstanceID)
	resource, err := r.provisionPluginExecutor(ctx, launch)
	if err != nil {
		return nil, err
	}
	client, token, err := r.startPluginExecutorAgentctl(ctx, launch, resource)
	if err != nil {
		return nil, err
	}
	r.uploadPluginExecutorAgentCredentials(ctx, client, launch.request)
	r.applyPluginExecutorGitIdentity(ctx, client, launch.request)
	return r.finishPluginExecutorLaunch(ctx, launch, resource, client, token)
}

type pluginExecutorLaunchState struct {
	request          *ExecutorCreateRequest
	profile          models.ExecutorProviderLaunchProfile
	operationContext *pluginsdk.ExecutorProviderRequestContext
	snapshot         *pluginsdk.ExecutorProfileSnapshot
	apiURL           string
	hostAPIURL       string
	nonce            string
	inventory        pluginExecutorInventory
	intent           []*pluginsdk.ExecutorRepositoryIntent
	agentEnv         map[string]string
}

func (r *PluginRemoteExecutor) preparePluginExecutorLaunch(ctx context.Context, req *ExecutorCreateRequest) (*pluginExecutorLaunchState, error) {
	if err := r.validateLaunch(req); err != nil {
		return nil, err
	}
	profile := req.PluginExecutor.Profile
	apiURL, hostAPIURL, err := r.resolvePluginExecutorAPIEndpoints(profile.Provider, req.Env)
	if err != nil {
		return nil, err
	}
	if err := r.rejectExpiredPluginExecutorEnvironment(ctx, req.TaskEnvironmentID, profile.OwnershipGeneration); err != nil {
		return nil, err
	}
	if err := r.confirmPluginExecutorCapabilities(ctx, profile); err != nil {
		return nil, err
	}
	intent, agentEnv, err := pluginRepositoryHandoff(req, profile.Provider)
	if err != nil {
		return nil, err
	}
	operationID := strings.TrimSpace(req.InstanceID)
	digest, err := pluginExecutorInputDigestWith(profile, intent, agentEnv)
	if err != nil {
		return nil, err
	}
	nonce, err := newPluginExecutorNonce()
	if err != nil {
		return nil, errors.New("plugin executor bootstrap could not be initialized")
	}
	operationContext := pluginExecutorRequestContext(ctx, req, profile, operationID, digest)
	inventory := pluginExecutorLaunchInventory(req, profile, operationID, digest)
	snapshot, err := pluginExecutorProfileSnapshot(profile, inventory.NegotiatedCapabilities)
	if err != nil {
		return nil, err
	}
	if pluginExecutorDurableBootstrap(inventory) {
		if err := r.persistDurableNonce(ctx, req, nonce); err != nil {
			return nil, err
		}
	}
	if err := checkpointPluginExecutor(ctx, req, inventory); err != nil {
		r.settlePluginExecutorSecrets(ctx, inventory, req.InstanceID)
		return nil, fmt.Errorf("persist plugin executor allocation intent: %w", err)
	}
	return &pluginExecutorLaunchState{
		request: req, profile: profile, operationContext: operationContext, snapshot: snapshot,
		apiURL: apiURL, hostAPIURL: hostAPIURL, nonce: nonce, inventory: inventory,
		intent: intent, agentEnv: agentEnv,
	}, nil
}

func pluginExecutorLaunchInventory(req *ExecutorCreateRequest, profile models.ExecutorProviderLaunchProfile, operationID, digest string) pluginExecutorInventory {
	return pluginExecutorInventory{
		PluginID: profile.Provider.PluginID, InstallationID: profile.Provider.InstallationID,
		ProviderKey: profile.Provider.Key, ProviderIdentity: profile.Provider.Identity,
		EnvironmentID:          req.TaskEnvironmentID,
		ContractVersion:        profile.Provider.ContractVersion,
		SupportedStateVersions: append([]int(nil), profile.Provider.SupportedStateVersions...),
		NegotiatedCapabilities: negotiatedCapabilities(profile.Provider),
		WorkspacePreparation:   pluginWorkspacePreparationMode(profile.Provider),
		ProfileID:              profile.ProfileID, ProfileConfig: clonePluginExecutorStringMap(profile.Config),
		SecretReferences: clonePluginExecutorStringMap(profile.SecretReferences),
		OperationID:      operationID, InputDigest: digest, RuntimeIdentity: req.InstanceID, Phase: pluginExecutorPhaseAllocating,
		Capabilities: profile.Provider.Capabilities, EnvironmentGeneration: profile.OwnershipGeneration,
	}
}

// pluginExecutorBootstrapJSON is the provider-facing bootstrap. A provider that
// requires drain_receipt also receives idle_timeout_seconds 0, which the
// provider maps to a disabled agentctl idle reaper so a retained environment is
// never reaped from inside; other providers keep agentctl's default.
func pluginExecutorBootstrapJSON(launch *pluginExecutorLaunchState) (string, error) {
	fields := map[string]any{
		"nonce": launch.nonce, "runtime_port": pluginExecutorRuntimePort,
		"execution_id": launch.request.InstanceID, "artifact_id": pluginExecutorAgentctlPurpose,
	}
	if launch.hostAPIURL != "" {
		fields["host_api_url"] = launch.hostAPIURL
	} else {
		fields["api_url"] = launch.apiURL
	}
	if negotiatedDeclares(launch.inventory.NegotiatedCapabilities, pluginExecutorCapabilityDrainReceipt) {
		fields["idle_timeout_seconds"] = 0
	}
	encoded, err := json.Marshal(fields)
	return string(encoded), err
}

func (r *PluginRemoteExecutor) provisionPluginExecutor(ctx context.Context, launch *pluginExecutorLaunchState) (*pluginsdk.ExecutorResourceDescriptor, error) {
	bootstrap, err := pluginExecutorBootstrapJSON(launch)
	if err != nil {
		return nil, errors.New("plugin executor bootstrap could not be encoded")
	}
	provisioned, err := r.operations.ProvisionExecutorEnvironment(ctx, &pluginsdk.ProvisionExecutorEnvironmentRequest{
		Context:       launch.operationContext,
		Profile:       launch.snapshot,
		BootstrapJson: bootstrap,
		Repositories:  launch.intent,
		AgentEnv:      launch.agentEnv,
	})
	if err != nil || provisioned == nil || provisioned.GetResource() == nil || provisioned.GetResource().GetResourceHandle() == "" {
		if err == nil && provisioned.GetError() != nil {
			err = pluginExecutorProviderError(provisioned.GetError())
		}
		return nil, r.recoverOrRetainFailedProvision(ctx, launch.request, launch.operationContext, launch.profile, launch.inventory, err)
	}
	resource := provisioned.GetResource()
	r.reachCrashBarrier(ctx, BarrierAfterSideEffect)
	if provisioned.GetError() != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			"provision_rejected", pluginExecutorProviderError(provisioned.GetError()))
	}
	if err := validatePluginExecutorResourceForProvider(launch.profile.Provider, resource); err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			"resource_descriptor", err)
	}
	if err := rejectSecretInProviderState(launch.profile.SecretValues, resource); err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			"secret_in_state", err)
	}
	descriptor, err := resolvePluginExecutorWorkspace(launch.inventory.NegotiatedCapabilities, launch.inventory.WorkspaceDescriptor, resource)
	if err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			"workspace_descriptor", err)
	}
	launch.inventory.WorkspaceDescriptor = descriptor
	repositories, err := checkPluginExecutorRepositories(launch.inventory, descriptor, resource, launch.intent)
	if err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			cleanupStageRepositoryInventory, err)
	}
	launch.inventory.Repositories = repositories
	launch.inventory.Resource = withoutPluginExecutorRepositories(resource)
	launch.inventory.StateVersion = resource.GetStateVersion()
	launch.inventory.Platform = resource.GetPlatform()
	launch.inventory.ExpiresAt = resource.GetExpiresAt()
	launch.inventory.Capabilities = intersectPluginExecutorCapabilities(launch.profile.Provider.Capabilities, resource.GetCapabilities(), resource.GetRetention())
	launch.inventory.Phase = pluginExecutorPhaseProvisioned
	if err := checkPluginExecutorEnvelopeBudget(launch.inventory, pluginExecutorStateBudget(launch.profile.Provider)); err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			"state_budget", err)
	}
	if err := checkpointPluginExecutor(ctx, launch.request, launch.inventory); err != nil {
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			"provision_checkpoint", err)
	}
	return resource, nil
}

// startPluginExecutorAgentctl handshakes with agentctl's control server, creates the
// session's instance, and returns a client bound to the instance port.
func (r *PluginRemoteExecutor) startPluginExecutorAgentctl(ctx context.Context, launch *pluginExecutorLaunchState, resource *pluginsdk.ExecutorResourceDescriptor) (*agentctl.Client, string, error) {
	fail := func(stage string, err error) (*agentctl.Client, string, error) {
		return nil, "", r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource, stage, err)
	}
	durable := pluginExecutorDurableBootstrap(launch.inventory)
	if durable {
		launch.inventory.HandshakeStarted = true
		if err := checkpointPluginExecutor(ctx, launch.request, launch.inventory); err != nil {
			return fail("handshake_checkpoint", err)
		}
	}
	control, token, err := r.newAgentctlControlClient(ctx, r.connectionResolver(launch.operationContext, resource, pluginExecutorRuntimePort, launch.snapshot), r.logger, launch.nonce)
	if err != nil {
		return fail("control_handshake", err)
	}
	if durable {
		r.reachCrashBarrier(ctx, BarrierAfterHandshake)
		if err := r.persistDurableToken(ctx, launch.request, token); err != nil {
			control.Close()
			return fail("token_persist", err)
		}
		if err := checkpointPluginExecutor(ctx, launch.request, launch.inventory); err != nil {
			control.Close()
			return fail("token_checkpoint", err)
		}
	}
	if launch.hostAPIURL != "" {
		if err := r.probeForwardedHostAPI(ctx, launch, control); err != nil {
			control.Close()
			return fail(cleanupReasonAPIUnreachable, err)
		}
	}
	instancePort, err := createPluginAgentctlInstance(ctx, control, launch.request, launch.inventory.workspacePath())
	control.Close()
	if err != nil {
		return fail("instance_create", err)
	}
	launch.inventory.InstancePort = instancePort
	client, err := r.newAgentctlClient(ctx, r.connectionResolver(launch.operationContext, resource, instancePort, launch.snapshot), r.logger,
		launch.request.InstanceID, token)
	if err != nil {
		return fail("instance_connect", err)
	}
	if err := r.ready(ctx, client); err != nil {
		client.Close()
		return fail("agentctl_health", err)
	}
	return client, token, nil
}

func createPluginAgentctlInstance(ctx context.Context, control *agentctl.ControlClient, req *ExecutorCreateRequest, workspacePath string) (uint32, error) {
	instanceReq := agentctlInstanceRequest(req, workspacePath)
	created, err := control.CreateInstance(ctx, &instanceReq)
	if err != nil {
		return 0, err
	}
	if created.Port <= 0 || created.Port > 65535 {
		return 0, errors.New("agentctl created an instance without a usable port")
	}
	return uint32(created.Port), nil
}

func (r *PluginRemoteExecutor) finishPluginExecutorLaunch(ctx context.Context, launch *pluginExecutorLaunchState, resource *pluginsdk.ExecutorResourceDescriptor, client *agentctl.Client, token string) (*ExecutorInstance, error) {
	launch.inventory.Phase = pluginExecutorPhaseReady
	launch.inventory.Generation++
	delete(launch.request.Metadata, MetadataKeyBootstrapNonceSecret)
	if err := checkpointPluginExecutor(ctx, launch.request, launch.inventory); err != nil {
		client.Close()
		return nil, r.cleanupAfterPluginExecutorFailure(ctx, launch.request, launch.operationContext, launch.inventory, resource,
			"ready_checkpoint", err)
	}
	if pluginExecutorDurableBootstrap(launch.inventory) {
		r.settlePluginExecutorNonce(ctx, launch.request.InstanceID)
	}
	metadata := clonePluginExecutorMetadata(launch.request.Metadata)
	metadata[MetadataKeyPluginExecutor] = launch.inventory
	return &ExecutorInstance{
		InstanceID: launch.request.InstanceID, TaskID: launch.request.TaskID, SessionID: launch.request.SessionID,
		RuntimeName: agentruntime.RuntimePluginRemote, Client: client,
		WorkspacePath: launch.inventory.workspacePath(), Metadata: metadata, AuthToken: token,
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
	// Archive and delete stops only stop agentctl. The task-environment cleanup
	// job is the single authority that drains and destroys the environment; a
	// stop that raced it for the cleanup claim could only be fenced by it.
	if instance.StopReason == StopReasonLaunchRollback {
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
	return pluginExecutorInputDigestWith(profile, nil, nil)
}

// pluginExecutorInputDigestWith also covers the repository intent and agent
// environment; both are omitted when empty so a launch without them keeps the
// digest it always had.
func pluginExecutorInputDigestWith(profile models.ExecutorProviderLaunchProfile, intent []*pluginsdk.ExecutorRepositoryIntent, agentEnv map[string]string) (string, error) {
	type repository struct {
		ID, Locator, Branch, BaseBranch, CheckoutRef string
	}
	var repositories []repository
	for _, entry := range intent {
		repositories = append(repositories, repository{entry.GetId(), entry.GetLocator(), entry.GetBranch(), entry.GetBaseBranch(), entry.GetCheckoutRef()})
	}
	data, err := json.Marshal(struct {
		ProviderIdentity string            `json:"provider_identity"`
		ProfileID        string            `json:"profile_id"`
		Config           map[string]string `json:"config"`
		SecretReferences map[string]string `json:"secret_references"`
		Repositories     []repository      `json:"repositories,omitempty"`
		AgentEnv         map[string]string `json:"agent_env,omitempty"`
	}{profile.Provider.Identity, profile.ProfileID, profile.Config, profile.SecretReferences, repositories, agentEnv})
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
	if err := checkPluginExecutorEnvelopeBudget(inventory, pluginExecutorMaxStateBudget); err != nil {
		return err
	}
	data, err := json.Marshal(inventory)
	if err != nil || len(data) > pluginExecutorInventoryMaxBytes {
		return errors.New("plugin executor inventory exceeds the 32 KiB limit")
	}
	return req.CheckpointRuntimeInventory(ctx, map[string]interface{}{MetadataKeyPluginExecutor: inventory})
}

func (r *PluginRemoteExecutor) connectionResolver(operationContext *pluginsdk.ExecutorProviderRequestContext, resource *pluginsdk.ExecutorResourceDescriptor, runtimePort uint32, snapshot *pluginsdk.ExecutorProfileSnapshot) agentctl.ConnectionLeaseResolver {
	return func(ctx context.Context) (*agentctl.ConnectionLease, error) {
		requestContext := proto.Clone(operationContext).(*pluginsdk.ExecutorProviderRequestContext)
		requestContext.Deadline = time.Now().Add(15 * time.Second).UTC().Format(time.RFC3339Nano)
		response, err := r.operations.ResolveExecutorConnection(ctx, &pluginsdk.ResolveExecutorConnectionRequest{
			Context: requestContext, Resource: resource, Purpose: pluginExecutorAgentctlPurpose, RuntimePort: runtimePort, Profile: snapshot,
		})
		if err != nil || response == nil {
			return nil, errors.New("provider connection lease is unavailable")
		}
		if response.GetError() != nil {
			return nil, fmt.Errorf("provider connection lease was rejected: %w", pluginExecutorProviderError(response.GetError()))
		}
		if response.GetLease() == nil {
			return nil, errors.New("provider connection lease is unavailable")
		}
		lease := response.GetLease()
		expiresAt, err := time.Parse(time.RFC3339Nano, lease.GetExpiresAt())
		if err != nil {
			return nil, errors.New("provider connection lease expiry is invalid")
		}
		dial, err := r.tunnelDialer(lease.GetTransport(), operationContext, resource, runtimePort, snapshot)
		if err != nil {
			return nil, err
		}
		return &agentctl.ConnectionLease{
			Transport: lease.GetTransport(), Dial: dial,
			BaseURL: lease.GetBaseUrl(), HTTPHeaders: stringMapToHeader(lease.GetHttpHeaders()),
			WebSocketHeaders:      stringMapToHeader(lease.GetWebsocketHeaders()),
			WebSocketSubprotocols: append([]string(nil), lease.GetWebsocketSubprotocols()...),
			ExpiresAt:             expiresAt, Generation: lease.GetGeneration(),
		}, nil
	}
}

// tunnelDialer binds a host_tunnel lease to the environment, ownership
// generation and port of the call that resolved it. The allowed ports are the
// agentctl control port and the instance port being resolved.
func (r *PluginRemoteExecutor) tunnelDialer(transport string, operationContext *pluginsdk.ExecutorProviderRequestContext, resource *pluginsdk.ExecutorResourceDescriptor, port uint32, snapshot *pluginsdk.ExecutorProfileSnapshot) (agentctl.TunnelDialer, error) {
	if transport != agentctl.LeaseTransportHostTunnel {
		return nil, nil
	}
	tunnels, ok := r.operations.(PluginExecutorTunnelOperations)
	if !ok {
		return nil, errors.New("provider connection lease requires a host tunnel that is unavailable")
	}
	allowed := []uint32{pluginExecutorRuntimePort, port}
	return func(ctx context.Context) (net.Conn, error) {
		return tunnels.OpenExecutorTunnel(ctx, proto.Clone(operationContext).(*pluginsdk.ExecutorProviderRequestContext), resource, port, allowed, snapshot)
	}, nil
}

// closePluginExecutorTunnels closes every open tunnel stream of an environment
// before its provider resource is destroyed.
func (r *PluginRemoteExecutor) closePluginExecutorTunnels(operationContext *pluginsdk.ExecutorProviderRequestContext) {
	if tunnels, ok := r.operations.(PluginExecutorTunnelOperations); ok && operationContext != nil {
		tunnels.CloseExecutorTunnels(operationContext.GetInstallationId(), operationContext.GetEnvironmentId())
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
	snapshot, snapshotErr := pluginExecutorProfileSnapshot(profile, inventory.NegotiatedCapabilities)
	var recovery *pluginsdk.RecoverExecutorOperationResponse
	err := snapshotErr
	if snapshotErr == nil {
		recovery, err = r.operations.RecoverExecutorOperation(ctx, &pluginsdk.RecoverExecutorOperationRequest{Context: operationContext, Profile: snapshot})
	}
	if err != nil || recovery == nil || recovery.GetOutcome() == pluginExecutorStateUnknown {
		inventory.Phase = pluginExecutorPhaseCleanupPending
		_ = checkpointPluginExecutor(ctx, req, inventory)
		return withPluginExecutorProviderRefusal(provisionErr, errors.New("plugin executor allocation outcome is unknown; cleanup inventory was retained"))
	}
	if recovery.GetOutcome() == pluginExecutorPhaseAbsent {
		claim, claimErr := r.acquirePluginExecutorCleanupClaim(ctx, req.TaskID, req.SessionID, inventory)
		if claimErr != nil {
			inventory.Phase = pluginExecutorPhaseCleanupPending
			_ = checkpointPluginExecutor(ctx, req, inventory)
			return withPluginExecutorProviderRefusal(provisionErr, errors.New("plugin executor allocation is absent but inventory cleanup is fenced"))
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
		r.settlePluginExecutorSecrets(ctx, inventory, req.InstanceID)
		return withPluginExecutorProviderRefusal(provisionErr, errors.New("plugin executor allocation failed before creating a resource"))
	}
	resource := recovery.GetResource()
	if resource == nil || resource.GetResourceHandle() == "" {
		inventory.Phase = pluginExecutorPhaseCleanupPending
		_ = checkpointPluginExecutor(ctx, req, inventory)
		return withPluginExecutorProviderRefusal(provisionErr, errors.New("plugin executor allocation outcome is unknown; cleanup inventory was retained"))
	}
	if err := validatePluginExecutorResourceForProvider(profile.Provider, resource); err != nil {
		inventory.Phase = pluginExecutorPhaseCleanupPending
		_ = checkpointPluginExecutor(ctx, req, inventory)
		return withPluginExecutorProviderRefusal(provisionErr, fmt.Errorf("plugin executor recovered resource was rejected and cleanup was retained: %w", err))
	}
	return r.cleanupAfterPluginExecutorFailure(ctx, req, operationContext, inventory, resource, "provision_recovered", provisionErr)
}

// cleanupAfterPluginExecutorFailure logs the failed launch stage and removes the
// provisioned resource. The returned error names only the cleanup outcome.
func (r *PluginRemoteExecutor) cleanupAfterPluginExecutorFailure(
	ctx context.Context,
	req *ExecutorCreateRequest,
	operationContext *pluginsdk.ExecutorProviderRequestContext,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
	stage string,
	cause error,
) error {
	if cause != nil {
		r.logger.Warn("plugin executor launch gate failed",
			zap.String("stage", stage),
			zap.String("execution_id", req.InstanceID),
			zap.String("session_id", req.SessionID),
			zap.String("cause_type", fmt.Sprintf("%T", cause)))
	}
	retained := resource
	if errors.Is(cause, models.ErrExecutorStateTooLarge) || errors.Is(cause, models.ErrExecutorSecretInState) {
		retained = fixedSizePluginExecutorResource(resource)
	}
	claim, claimErr := r.acquireLaunchCleanupClaim(context.WithoutCancel(ctx), req, inventory)
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if claimErr != nil {
		inventory.Resource = retained
		inventory.Phase = pluginExecutorPhaseCleanupPending
		if err := checkpointPluginExecutor(cleanupCtx, req, inventory); err != nil {
			r.logger.Warn("plugin executor launch cleanup is fenced and its retry information was not retained",
				zap.String("execution_id", req.InstanceID), zap.Error(err))
			return r.destroyUnrecordedPluginExecutorResource(ctx, req, operationContext, inventory, resource, stage, cause)
		}
		return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; cleanup is fenced and retry information was retained"))
	}
	defer r.releasePluginExecutorCleanupClaim(claim)
	cleanupCtx = recoveryclaim.WithClaim(cleanupCtx, claim)
	cleanupContext := proto.Clone(operationContext).(*pluginsdk.ExecutorProviderRequestContext)
	cleanupContext.Deadline = time.Now().Add(30 * time.Second).UTC().Format(time.RFC3339Nano)
	r.closePluginExecutorTunnels(cleanupContext)
	snapshot, snapshotErr := pluginExecutorProfileSnapshot(req.PluginExecutor.Profile, inventory.NegotiatedCapabilities)
	if snapshotErr != nil {
		inventory.Resource = retained
		inventory.Phase = pluginExecutorPhaseCleanupPending
		_ = checkpointPluginExecutor(cleanupCtx, req, inventory)
		return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; recorded profile is unavailable and cleanup remains pending"))
	}
	request := &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: cleanupContext, Resource: resource, CleanupReason: launchCleanupReason(stage), CleanupClaim: claim.OperationID,
		Profile: snapshot,
	}
	if negotiatedDeclares(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDrainReceipt) {
		inventory.DrainClaim = claim.OperationID
		inventory.DrainOutcome = pluginDrainOutcomeNotAttempted
		inventory.DrainReceiptID = ""
		if checkpointErr := checkpointPluginExecutor(cleanupCtx, req, inventory); checkpointErr != nil {
			inventory.Resource = retained
			inventory.Phase = pluginExecutorPhaseCleanupPending
			if retainErr := checkpointPluginExecutor(cleanupCtx, req, inventory); retainErr != nil {
				return r.destroyUnrecordedPluginExecutorResource(ctx, req, operationContext, inventory, resource, stage, cause)
			}
			return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; drain outcome was not persisted and cleanup remains pending"))
		}
		pluginDrainDecision{outcome: pluginDrainOutcomeNotAttempted}.applyTo(request)
	}
	response, err := r.operations.DestroyExecutorEnvironment(cleanupCtx, request)
	if err == nil && response != nil && response.GetConfirmedAbsent() {
		inventory.Resource = retained
		inventory.Phase = pluginExecutorPhaseAbsent
		if checkpointErr := checkpointPluginExecutor(cleanupCtx, req, inventory); checkpointErr != nil {
			return fmt.Errorf("remote launch failed and absence checkpoint failed: %w", checkpointErr)
		}
		r.reachCrashBarrier(cleanupCtx, BarrierAfterDestroyConfirm)
		if releaseErr := req.ReleaseRuntimeInventory(cleanupCtx); releaseErr != nil {
			return fmt.Errorf("remote launch failed and inventory release failed: %w", releaseErr)
		}
		r.settlePluginExecutorSecrets(ctx, inventory, req.InstanceID)
		return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; allocated resource was removed"))
	}
	inventory.Resource = retained
	inventory.Phase = pluginExecutorPhaseCleanupPending
	if checkpointErr := checkpointPluginExecutor(cleanupCtx, req, inventory); checkpointErr != nil {
		return fmt.Errorf("remote launch failed and cleanup inventory checkpoint failed: %w", checkpointErr)
	}
	r.logger.Warn("plugin executor cleanup remains pending", zap.String("execution_id", req.InstanceID))
	return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; resource cleanup remains pending"))
}

// pluginExecutorProviderFailure is a provider-declared refusal. Only its
// stable code and message id are kept.
type pluginExecutorProviderFailure struct{ code, messageID string }

func (e *pluginExecutorProviderFailure) Error() string {
	return fmt.Sprintf("provider error %q (%s)", e.code, e.messageID)
}

// acquireLaunchCleanupClaim takes the cleanup claim for a failed launch. An
// archive or a cascade cleanup can hold the claim at that moment; it releases
// once its own pass ends, so a busy claim is retried for a bounded time before
// the launch gives up and leaves the retry information behind.
func (r *PluginRemoteExecutor) acquireLaunchCleanupClaim(ctx context.Context, req *ExecutorCreateRequest, inventory pluginExecutorInventory) (*models.TaskEnvironmentRecoveryClaim, error) {
	waitCtx, cancel := context.WithTimeout(ctx, r.cleanupClaimWait)
	defer cancel()
	for {
		claim, err := r.acquirePluginExecutorCleanupClaim(ctx, req.TaskID, req.SessionID, inventory)
		if !errors.Is(err, recoveryclaim.ErrBusy) {
			return claim, err
		}
		timer := time.NewTimer(r.cleanupClaimRetry)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return nil, err
		case <-timer.C:
		}
	}
}

// destroyUnrecordedPluginExecutorResource removes a resource that has neither a
// durable inventory row nor a cleanup claim to hold it, as when an archive
// rotated the execution after the provider returned. The launch is the only
// holder of the handle, so dropping it would leak the resource.
func (r *PluginRemoteExecutor) destroyUnrecordedPluginExecutorResource(
	ctx context.Context,
	req *ExecutorCreateRequest,
	operationContext *pluginsdk.ExecutorProviderRequestContext,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
	stage string,
	cause error,
) error {
	notRemoved := withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; allocated resource was not recorded and was not removed"))
	snapshot, err := pluginExecutorProfileSnapshot(req.PluginExecutor.Profile, inventory.NegotiatedCapabilities)
	if err != nil {
		r.logger.Error("plugin executor resource is unrecorded and its profile is unavailable", zap.String("execution_id", req.InstanceID))
		return notRemoved
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	cleanupContext := proto.Clone(operationContext).(*pluginsdk.ExecutorProviderRequestContext)
	cleanupContext.Deadline = time.Now().Add(30 * time.Second).UTC().Format(time.RFC3339Nano)
	r.closePluginExecutorTunnels(cleanupContext)
	request := &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: cleanupContext, Resource: resource, CleanupReason: launchCleanupReason(stage),
		CleanupClaim: pluginExecutorClaimOperationID(inventory.OperationID, models.PluginClaimKindCleanup), Profile: snapshot,
	}
	if negotiatedDeclares(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDrainReceipt) {
		pluginDrainDecision{outcome: pluginDrainOutcomeNotAttempted}.applyTo(request)
	}
	response, err := r.operations.DestroyExecutorEnvironment(cleanupCtx, request)
	if err != nil || response == nil || !response.GetConfirmedAbsent() {
		r.logger.Error("plugin executor resource is unrecorded and could not be removed", zap.String("execution_id", req.InstanceID), zap.String("stage", stage))
		return notRemoved
	}
	r.settlePluginExecutorSecrets(ctx, inventory, req.InstanceID)
	return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; allocated resource was not recorded and was removed"))
}

func pluginExecutorProviderError(providerErr *pluginsdk.ExecutorProviderError) error {
	return &pluginExecutorProviderFailure{code: providerErr.GetCode(), messageID: providerErr.GetMessageId()}
}

// withPluginExecutorProviderRefusal keeps the provider's refusal visible
// through the host's allocation-outcome wording.
func withPluginExecutorProviderRefusal(cause, failure error) error {
	var refusal *pluginExecutorProviderFailure
	if errors.As(cause, &refusal) {
		return fmt.Errorf("%w: %v", refusal, failure)
	}
	return failure
}

var _ ExecutorBackend = (*PluginRemoteExecutor)(nil)
