package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/kandev/kandev/internal/agent/executor"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	kubernetesWorkspacePath       = "/workspace"
	kubernetesAgentctlPath        = "/opt/kandev/agentctl"
	kubernetesRuntimeEnvPath      = "/opt/kandev/runtime.env"
	kubernetesAuthEnvPath         = "/run/kandev/auth.env"
	kubernetesAuthHomePath        = "/run/kandev/home"
	kubernetesPreparePath         = "/opt/kandev/prepare.sh"
	kubernetesStartPath           = "/opt/kandev/start"
	kubernetesControlHost         = "127.0.0.1"
	kubernetesEnvInstanceID       = "KANDEV_INSTANCE_ID"
	kubernetesEnvSessionID        = "KANDEV_SESSION_ID"
	kubernetesEnvTaskID           = "KANDEV_TASK_ID"
	kubernetesEnvEnvironmentID    = "KANDEV_TASK_ENVIRONMENT_ID"
	kubernetesEnvAgentProfile     = "KANDEV_AGENT_PROFILE_ID"
	kubernetesEnvExecutionProfile = "KANDEV_EXECUTION_PROFILE_ID"
	kubernetesRepositoryCloneURL  = "repository.clone_url"
)

var errKubernetesLifecycleRequestIncomplete = errors.New("kubernetes lifecycle request is incomplete")

// KubernetesExecutor owns Kubernetes Pod/PVC lifecycle and process-local
// forwards. Cluster clients are initialized lazily from the selected executor.
type KubernetesExecutor struct {
	agentctlResolver *AgentctlResolver
	logger           *logger.Logger
	clientFactory    kubernetesRuntimeClientFactory
	resolveBinary    kubernetesAgentctlBinaryResolver
	healthRetryDelay time.Duration

	mu       sync.Mutex
	sessions map[string]*kubernetesSession
	locks    map[string]*kubernetesInstanceLock
}

type kubernetesInstanceLock struct {
	mutex sync.Mutex
	refs  int
}

type kubernetesSession struct {
	runtime      *kubernetesRuntimeClient
	forward      kubeexecutor.PortForwardSession
	client       *agentctl.Client
	request      *ExecutorCreateRequest
	restartCount int32
}

const (
	KubernetesInventoryStatePVCCreated  = "pvc_created"
	KubernetesInventoryStatePVCAdmitted = "pvc_admitted"
	KubernetesInventoryStatePodCreated  = "pod_created"
	KubernetesInventoryStatePodAdmitted = "pod_admitted"
	KubernetesInventoryStateReady       = "ready"
)

func NewKubernetesExecutor(agentctlResolver *AgentctlResolver, log *logger.Logger) *KubernetesExecutor {
	runtime := &KubernetesExecutor{
		agentctlResolver: agentctlResolver,
		logger:           log,
		clientFactory:    newKubernetesRuntimeClient,
		healthRetryDelay: agentctlHealthRetryDelay,
		sessions:         make(map[string]*kubernetesSession),
		locks:            make(map[string]*kubernetesInstanceLock),
	}
	if agentctlResolver != nil {
		runtime.resolveBinary = func(platform kubeexecutor.Platform) ([]byte, error) {
			arch := strings.TrimPrefix(string(platform), "linux/")
			path, err := agentctlResolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "linux", GOARCH: arch})
			if err != nil {
				return nil, err
			}
			return os.ReadFile(path)
		}
	}
	return runtime
}

func (r *KubernetesExecutor) Name() executor.Name { return executor.NameKubernetes }

func (r *KubernetesExecutor) HealthCheck(context.Context) error { return nil }

func (r *KubernetesExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	if req == nil || req.InstanceID == "" {
		return nil, errKubernetesLifecycleRequestIncomplete
	}
	unlock := r.lockInstance(req.InstanceID)
	defer unlock()

	executorConfig, err := kubernetesExecutorConfigFromMetadata(req.Metadata)
	if err != nil {
		return nil, err
	}
	reconnect := getMetadataString(req.Metadata, MetadataKeyKubernetesPodName) != ""
	var profile kubeexecutor.ProfileConfig
	if !reconnect {
		profile, err = kubernetesProfileConfigFromMetadata(req.Metadata)
		if err != nil {
			return nil, err
		}
	}
	if r.clientFactory == nil {
		return nil, fmt.Errorf("kubernetes lifecycle: client factory is not configured")
	}
	runtime, err := r.clientFactory(executorConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: create client: %w", err)
	}
	if runtime == nil || runtime.resources == nil || runtime.streams == nil {
		return nil, fmt.Errorf("kubernetes lifecycle: client is incomplete")
	}

	if reconnect {
		return r.reconnect(ctx, runtime, req, executorConfig)
	}
	if r.resolveBinary == nil {
		return nil, fmt.Errorf("kubernetes lifecycle: agentctl resolver is not configured")
	}
	return r.createFresh(ctx, runtime, req, executorConfig, profile)
}

func (r *KubernetesExecutor) createFresh(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	executorConfig kubeexecutor.ExecutorConfig,
	profile kubeexecutor.ProfileConfig,
) (_ *ExecutorInstance, returnedErr error) {
	launch, err := newKubernetesFreshLaunch(r, runtime, req, executorConfig, profile)
	if err != nil {
		return nil, err
	}
	defer func() {
		returnedErr = launch.rollbackAfterFailure(ctx, returnedErr)
	}()
	if err := launch.provisionWorkspace(ctx); err != nil {
		return nil, err
	}
	runningPod, err := launch.createRunningPod(ctx)
	if err != nil {
		return nil, err
	}
	nonce, err := generateBootstrapNonce()
	if err != nil {
		return nil, err
	}
	binary, err := r.resolveBinary(profile.Platform)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: resolve agentctl for %s: %w", profile.Platform, err)
	}
	if err = r.bootstrapPod(ctx, runtime, req, runningPod, profile, nonce, binary); err != nil {
		return nil, err
	}
	client, finalForward, token, remotePort, err := r.connectNewAgentctl(ctx, runtime, req, runningPod, nonce)
	if err != nil {
		return nil, err
	}
	return launch.complete(ctx, runningPod, client, finalForward, token, nonce, remotePort)
}

type kubernetesFreshLaunch struct {
	executor       *KubernetesExecutor
	runtime        *kubernetesRuntimeClient
	req            *ExecutorCreateRequest
	executorConfig kubeexecutor.ExecutorConfig
	profile        kubeexecutor.ProfileConfig
	identity       kubeexecutor.ResourceIdentity
	podName        string
	pvcName        string
	workspace      kubernetesWorkspaceProvision
	createdPod     *corev1.Pod
}

func newKubernetesFreshLaunch(
	executor *KubernetesExecutor,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	executorConfig kubeexecutor.ExecutorConfig,
	profile kubeexecutor.ProfileConfig,
) (*kubernetesFreshLaunch, error) {
	identity, err := kubernetesIdentity(req)
	if err != nil {
		return nil, err
	}
	podName, pvcName := kubernetesResourceNames(req.InstanceID)
	return &kubernetesFreshLaunch{
		executor: executor, runtime: runtime, req: req, executorConfig: executorConfig,
		profile: profile, identity: identity, podName: podName, pvcName: pvcName,
	}, nil
}

func (l *kubernetesFreshLaunch) checkpoint(
	ctx context.Context,
	state string,
	pod *corev1.Pod,
	pvc *corev1.PersistentVolumeClaim,
	pvcCreated bool,
	remotePort int,
) error {
	metadata := kubernetesRuntimeMetadataAtState(
		l.executorConfig, l.profile, l.identity, pod, pvc, pvcCreated, remotePort, state,
	)
	return checkpointKubernetesRuntimeInventory(ctx, l.req, metadata)
}

func (l *kubernetesFreshLaunch) provisionWorkspace(ctx context.Context) error {
	workspace, err := provisionKubernetesWorkspace(
		ctx, l.runtime.resources, l.executorConfig, l.profile, l.identity, l.pvcName,
		func(created *corev1.PersistentVolumeClaim) error {
			return l.checkpoint(ctx, KubernetesInventoryStatePVCCreated, nil, created, true, 0)
		},
	)
	l.workspace = workspace
	if err != nil {
		return err
	}
	if workspace.createdClaim == nil {
		return nil
	}
	return l.checkpoint(ctx, KubernetesInventoryStatePVCAdmitted, nil, workspace.claim, true, 0)
}

func (l *kubernetesFreshLaunch) createRunningPod(ctx context.Context) (*corev1.Pod, error) {
	desiredPod, err := composeKubernetesLifecyclePod(
		l.profile, l.identity, l.podName, l.executorConfig.Namespace, l.workspace.claimName,
	)
	if err != nil {
		return nil, err
	}
	podWaitCtx, cancel := kubernetesRequestContext(ctx, l.executorConfig.RequestTimeoutSeconds)
	defer cancel()
	created, running, err := createAndWaitKubernetesPod(
		podWaitCtx, l.runtime.resources, desiredPod, l.identity, l.profile.MainContainer, "Pod",
		func(state string, pod *corev1.Pod) error {
			return l.checkpoint(
				ctx, state, pod, l.workspace.claim, l.workspace.createdClaim != nil, 0,
			)
		},
	)
	l.createdPod = created
	return running, err
}

func (l *kubernetesFreshLaunch) complete(
	ctx context.Context,
	runningPod *corev1.Pod,
	client *agentctl.Client,
	forward kubeexecutor.PortForwardSession,
	token, nonce string,
	remotePort int,
) (*ExecutorInstance, error) {
	metadata := kubernetesRuntimeMetadataAtState(
		l.executorConfig, l.profile, l.identity, runningPod, l.workspace.claim,
		l.workspace.createdClaim != nil, remotePort, KubernetesInventoryStateReady,
	)
	// A fresh lifecycle-managed launch already has provisional Pod/PVC inventory.
	// Manager persists the final ready row only after both required runtime secret
	// references are durable, avoiding a restart-visible ready row that cannot
	// reconnect. Standalone callers without a release callback keep the direct
	// ready checkpoint behavior.
	if l.req.ReleaseRuntimeInventory == nil {
		if err := checkpointKubernetesRuntimeInventory(ctx, l.req, metadata); err != nil {
			client.Close()
			_ = forward.Close()
			return nil, err
		}
	}
	l.executor.replaceSession(l.req.InstanceID, &kubernetesSession{
		runtime: l.runtime, forward: forward, client: client,
		request:      cloneKubernetesCreateRequest(l.req),
		restartCount: kubernetesMainContainerRestartCount(runningPod, l.profile.MainContainer),
	})
	return &ExecutorInstance{
		InstanceID: l.req.InstanceID, TaskID: l.req.TaskID, SessionID: l.req.SessionID,
		RuntimeName: l.executor.Name(), Client: client, WorkspacePath: kubernetesWorkspacePath,
		Metadata: metadata, AuthToken: token, BootstrapNonce: nonce,
		ReleaseRuntimeInventory: l.req.ReleaseRuntimeInventory,
	}, nil
}

func (l *kubernetesFreshLaunch) rollbackAfterFailure(ctx context.Context, launchErr error) error {
	if launchErr == nil {
		return nil
	}
	rollbackCtx, cancel := kubernetesDetachedRequestContext(ctx, l.executorConfig.RequestTimeoutSeconds)
	defer cancel()
	var rollbackErrors []error
	if l.createdPod != nil {
		if err := rollbackCreatedKubernetesPod(rollbackCtx, l.runtime.resources, l.createdPod); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if l.workspace.createdClaim != nil {
		if err := rollbackCreatedKubernetesPVC(rollbackCtx, l.runtime.resources, l.workspace.createdClaim); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if len(rollbackErrors) == 0 && l.req.ReleaseRuntimeInventory != nil {
		if err := l.req.ReleaseRuntimeInventory(rollbackCtx); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf(
				"kubernetes lifecycle: release rolled-back runtime inventory: %w", err,
			))
		}
	}
	return errors.Join(launchErr, errors.Join(rollbackErrors...))
}

func kubernetesDetachedRequestContext(ctx context.Context, requestTimeoutSeconds int) (context.Context, context.CancelFunc) {
	if requestTimeoutSeconds <= 0 {
		requestTimeoutSeconds = kubeexecutor.DefaultRequestTimeoutSeconds
	}
	return context.WithTimeout(
		context.WithoutCancel(ctx), time.Duration(requestTimeoutSeconds)*time.Second,
	)
}

func kubernetesAmbiguousCreateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return kubernetesDetachedRequestContext(ctx, kubeexecutor.DefaultRequestTimeoutSeconds)
}

func kubernetesRequestContext(ctx context.Context, requestTimeoutSeconds int) (context.Context, context.CancelFunc) {
	if requestTimeoutSeconds <= 0 {
		requestTimeoutSeconds = kubeexecutor.DefaultRequestTimeoutSeconds
	}
	return context.WithTimeout(ctx, time.Duration(requestTimeoutSeconds)*time.Second)
}

func (r *KubernetesExecutor) connectNewAgentctl(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	pod *corev1.Pod,
	nonce string,
) (*agentctl.Client, kubeexecutor.PortForwardSession, string, int, error) {
	controlForward, control, err := r.connectHealthyKubernetesControl(ctx, runtime, pod)
	if err != nil {
		return nil, nil, "", 0, err
	}
	defer func() { _ = controlForward.Close() }()
	token, err := control.Handshake(ctx, nonce)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: nonce handshake: %w", err)
	}
	createRequest := buildReconnectCreateInstanceRequest(req, req.InstanceID)
	createResponse, err := createOrReconcileKubernetesAgentctlInstance(ctx, control, createRequest)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: create agentctl instance: %w", err)
	}
	if createResponse.Port < 1 || createResponse.Port > 65535 {
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: invalid agentctl instance port %d", createResponse.Port)
	}
	forward, err := startKubernetesForward(ctx, runtime.streams, pod, uint16(createResponse.Port))
	if err != nil {
		return nil, nil, "", 0, err
	}
	client := agentctl.NewClient(kubernetesControlHost, int(forward.LocalPort()), r.logger,
		agentctl.WithExecutionID(req.InstanceID), agentctl.WithSessionID(req.SessionID), agentctl.WithAuthToken(token))
	if err := client.Health(ctx); err != nil {
		client.Close()
		_ = forward.Close()
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: instance health: %w", err)
	}
	return client, forward, token, createResponse.Port, nil
}

type kubernetesAgentctlInstanceControl interface {
	CreateInstance(context.Context, *agentctl.CreateInstanceRequest) (*agentctl.CreateInstanceResponse, error)
	GetInstance(context.Context, string) (*agentctl.InstanceInfo, error)
}

func createOrReconcileKubernetesAgentctlInstance(
	ctx context.Context,
	control kubernetesAgentctlInstanceControl,
	request *agentctl.CreateInstanceRequest,
) (*agentctl.CreateInstanceResponse, error) {
	response, createErr := control.CreateInstance(ctx, request)
	if createErr == nil {
		if err := validateKubernetesAgentctlInstanceResponse(response, request); err != nil {
			return nil, err
		}
		return response, nil
	}
	if !kubeexecutor.IsAmbiguousCreateError(createErr) {
		return nil, createErr
	}
	reconcileCtx, cancel := kubernetesAmbiguousCreateContext(ctx)
	defer cancel()
	info, reconcileErr := control.GetInstance(reconcileCtx, request.ID)
	if reconcileErr != nil {
		return nil, errors.Join(createErr, fmt.Errorf("reconcile ambiguous agentctl create: %w", reconcileErr))
	}
	response = &agentctl.CreateInstanceResponse{ID: info.ID, Port: info.Port}
	if err := validateKubernetesAgentctlInstanceResponse(response, request); err != nil {
		return nil, errors.Join(createErr, fmt.Errorf("reconcile ambiguous agentctl create: %w", err))
	}
	if info.WorkspacePath != request.WorkspacePath {
		return nil, errors.Join(createErr, errors.New(
			"reconcile ambiguous agentctl create: existing instance workspace does not match request",
		))
	}
	return response, nil
}

func validateKubernetesAgentctlInstanceResponse(
	response *agentctl.CreateInstanceResponse,
	request *agentctl.CreateInstanceRequest,
) error {
	if response == nil || request == nil || response.ID != request.ID {
		return errors.New("kubernetes lifecycle: agentctl instance identity does not match request")
	}
	if response.Port < 1 || response.Port > 65535 {
		return fmt.Errorf("kubernetes lifecycle: invalid agentctl instance port %d", response.Port)
	}
	return nil
}

func (r *KubernetesExecutor) connectHealthyKubernetesControl(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	pod *corev1.Pod,
) (kubeexecutor.PortForwardSession, *agentctl.ControlClient, error) {
	var lastForwardErr error
	for {
		forward, err := startKubernetesForward(
			ctx, runtime.streams, pod, uint16(kubeexecutor.DefaultAgentctlPort),
		)
		if err != nil {
			return nil, nil, errors.Join(lastForwardErr, err)
		}
		control := agentctl.NewControlClient(kubernetesControlHost, int(forward.LocalPort()), r.logger)
		stopped, healthErr := r.waitForKubernetesAgentctlHealthUntilForwardStops(ctx, control, forward.Done())
		if !stopped {
			if healthErr != nil {
				_ = forward.Close()
				return nil, nil, fmt.Errorf("kubernetes lifecycle: control health: %w", errors.Join(lastForwardErr, healthErr))
			}
			return forward, control, nil
		}
		_ = forward.Close()
		lastForwardErr = healthErr
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("kubernetes lifecycle: control health: %w", errors.Join(err, lastForwardErr))
		}
		if err := r.waitForKubernetesControlForwardRetry(ctx); err != nil {
			return nil, nil, fmt.Errorf("kubernetes lifecycle: control health: %w", errors.Join(err, lastForwardErr))
		}
	}
}

func (r *KubernetesExecutor) waitForKubernetesControlForwardRetry(ctx context.Context) error {
	delay := r.healthRetryDelay
	if delay <= 0 {
		delay = agentctlHealthRetryDelay
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *KubernetesExecutor) waitForKubernetesAgentctlHealth(ctx context.Context, ctl healthChecker) error {
	_, err := r.waitForKubernetesAgentctlHealthUntilForwardStops(ctx, ctl, nil)
	return err
}

func (r *KubernetesExecutor) waitForKubernetesAgentctlHealthUntilForwardStops(
	ctx context.Context,
	ctl healthChecker,
	forwardDone <-chan error,
) (bool, error) {
	delay := r.healthRetryDelay
	if delay <= 0 {
		delay = agentctlHealthRetryDelay
	}
	var lastErr error
	for {
		if err := ctl.Health(ctx); err == nil {
			if forwardDone != nil {
				select {
				case forwardErr := <-forwardDone:
					return true, kubernetesControlForwardStoppedError(forwardErr)
				default:
				}
			}
			return false, nil
		} else {
			lastErr = err
		}
		if ctx.Err() != nil {
			return false, fmt.Errorf("agentctl did not become healthy: %w", errors.Join(ctx.Err(), lastErr))
		}
		timer := time.NewTimer(delay)
		select {
		case forwardErr := <-forwardDone:
			timer.Stop()
			return true, kubernetesControlForwardStoppedError(forwardErr)
		case <-ctx.Done():
			timer.Stop()
			return false, fmt.Errorf("agentctl did not become healthy: %w", errors.Join(ctx.Err(), lastErr))
		case <-timer.C:
		}
	}
}

func kubernetesControlForwardStoppedError(err error) error {
	if err == nil {
		err = errors.New("port forward stopped")
	}
	return fmt.Errorf("control port forward stopped before agentctl became healthy: %w", err)
}

func startKubernetesForward(
	ctx context.Context,
	streams *kubeexecutor.StreamOperations,
	pod *corev1.Pod,
	remotePort uint16,
) (kubeexecutor.PortForwardSession, error) {
	forward, err := streams.Forward(context.WithoutCancel(ctx), kubeexecutor.PortForwardRequest{
		Namespace: pod.Namespace, Pod: pod.Name, LocalAddress: kubernetesControlHost,
		LocalPort: 0, RemotePort: remotePort,
	})
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: forward port %d: %w", remotePort, err)
	}
	select {
	case <-forward.Ready():
		if forward.LocalPort() == 0 {
			_ = forward.Close()
			return nil, fmt.Errorf("kubernetes lifecycle: port forward returned zero local port")
		}
		return forward, nil
	case doneErr := <-forward.Done():
		_ = forward.Close()
		if doneErr == nil {
			doneErr = errors.New("port forward stopped before becoming ready")
		}
		return nil, fmt.Errorf("kubernetes lifecycle: port forward startup: %w", doneErr)
	case <-ctx.Done():
		_ = forward.Close()
		return nil, ctx.Err()
	}
}

func kubernetesProfileConfigFromMetadata(metadata map[string]interface{}) (kubeexecutor.ProfileConfig, error) {
	values := map[string]string{
		MetadataKeyKubernetesProfilePlatform:       getMetadataString(metadata, MetadataKeyKubernetesProfilePlatform),
		MetadataKeyKubernetesProfileMainContainer:  getMetadataString(metadata, MetadataKeyKubernetesProfileMainContainer),
		MetadataKeyKubernetesPodTemplateYAML:       getMetadataString(metadata, MetadataKeyKubernetesPodTemplateYAML),
		MetadataKeyKubernetesWorkspaceMode:         getMetadataString(metadata, MetadataKeyKubernetesWorkspaceMode),
		MetadataKeyKubernetesWorkspaceSize:         getMetadataString(metadata, MetadataKeyKubernetesWorkspaceSize),
		MetadataKeyKubernetesWorkspaceStorageClass: getMetadataString(metadata, MetadataKeyKubernetesWorkspaceStorageClass),
		MetadataKeyKubernetesWorkspaceAccessModes:  getMetadataString(metadata, MetadataKeyKubernetesWorkspaceAccessModes),
		MetadataKeyKubernetesWorkspaceClaimName:    getMetadataString(metadata, MetadataKeyKubernetesWorkspaceClaimName),
	}
	return kubeexecutor.ParseProfileConfig(values)
}

func (r *KubernetesExecutor) RecoverInstances(context.Context) ([]*ExecutorInstance, error) {
	return nil, nil
}

func (r *KubernetesExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }

func (r *KubernetesExecutor) RequiresCloneURL() bool          { return true }
func (r *KubernetesExecutor) ShouldApplyPreferredShell() bool { return false }
func (r *KubernetesExecutor) IsAlwaysResumable() bool         { return true }

func kubernetesExecutorConfigFromMetadata(metadata map[string]interface{}) (kubeexecutor.ExecutorConfig, error) {
	values := map[string]string{
		MetadataKeyKubernetesAuthMode:              getMetadataString(metadata, MetadataKeyKubernetesAuthMode),
		MetadataKeyKubernetesKubeconfigPath:        getMetadataString(metadata, MetadataKeyKubernetesKubeconfigPath),
		MetadataKeyKubernetesKubeContext:           getMetadataString(metadata, MetadataKeyKubernetesKubeContext),
		MetadataKeyKubernetesConfigNamespace:       getMetadataString(metadata, MetadataKeyKubernetesConfigNamespace),
		MetadataKeyKubernetesRequestTimeoutSeconds: getMetadataString(metadata, MetadataKeyKubernetesRequestTimeoutSeconds),
	}
	return kubeexecutor.ParseExecutorConfig(values)
}

func kubernetesIdentity(req *ExecutorCreateRequest) (kubeexecutor.ResourceIdentity, error) {
	identity := kubeexecutor.ResourceIdentity{
		ExecutorID: getMetadataString(req.Metadata, "executor_id"), ProfileID: getMetadataString(req.Metadata, MetadataKeyExecutorProfileID),
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID, EnvironmentID: req.TaskEnvironmentID,
	}
	if _, err := kubeexecutor.OwnershipLabels(identity); err != nil {
		return kubeexecutor.ResourceIdentity{}, err
	}
	return identity, nil
}

func kubernetesResourceNames(instanceID string) (string, string) {
	sum := sha256.Sum256([]byte(instanceID))
	suffix := hex.EncodeToString(sum[:8])
	return "kandev-" + suffix, "kandev-" + suffix + "-workspace"
}

func kubernetesRuntimeMetadata(
	executorConfig kubeexecutor.ExecutorConfig,
	profile kubeexecutor.ProfileConfig,
	identity kubeexecutor.ResourceIdentity,
	pod *corev1.Pod,
	pvc *corev1.PersistentVolumeClaim,
	pvcCreated bool,
	remotePort int,
) map[string]interface{} {
	metadata := map[string]interface{}{
		MetadataKeyKubernetesNamespace:             executorConfig.Namespace,
		MetadataKeyKubernetesMainContainer:         profile.MainContainer,
		MetadataKeyKubernetesPlatform:              string(profile.Platform),
		MetadataKeyKubernetesRuntimeWorkspaceMode:  string(profile.Workspace.Mode),
		MetadataKeyKubernetesPVCCreated:            pvcCreated,
		MetadataKeyKubernetesAgentctlInstanceID:    identity.InstanceID,
		MetadataKeyKubernetesResourceInstanceID:    identity.InstanceID,
		MetadataKeyKubernetesResourceExecutorID:    identity.ExecutorID,
		MetadataKeyKubernetesResourceProfileID:     identity.ProfileID,
		MetadataKeyKubernetesResourceTaskID:        identity.TaskID,
		MetadataKeyKubernetesResourceSessionID:     identity.SessionID,
		MetadataKeyKubernetesResourceEnvironmentID: identity.EnvironmentID,
		MetadataKeyKubernetesExecutorConfigHash:    kubernetesConfigHash(executorConfig),
		MetadataKeyKubernetesProfileConfigHash:     kubernetesConfigHash(profile),
		MetadataKeyKubernetesTemplateHash:          kubernetesStringHash(profile.PodTemplateYAML),
		MetadataKeyKubernetesProfileSnapshot:       kubernetesProfileSnapshot(profile),
		MetadataKeyIsRemote:                        true,
		"executor_id":                              identity.ExecutorID,
		MetadataKeyExecutorProfileID:               identity.ProfileID,
	}
	if pod != nil {
		metadata[MetadataKeyKubernetesNamespace] = pod.Namespace
		metadata[MetadataKeyKubernetesPodName] = pod.Name
		metadata[MetadataKeyKubernetesPodUID] = string(pod.UID)
		metadata[MetadataKeyKubernetesContainerRestartCount] = strconv.FormatInt(
			int64(kubernetesMainContainerRestartCount(pod, profile.MainContainer)), 10,
		)
	}
	if pvc != nil {
		metadata[MetadataKeyKubernetesPVCName] = pvc.Name
		metadata[MetadataKeyKubernetesPVCUID] = string(pvc.UID)
	}
	if remotePort > 0 {
		metadata[MetadataKeyKubernetesAgentctlRemotePort] = strconv.Itoa(remotePort)
	}
	return metadata
}

func kubernetesRuntimeMetadataAtState(
	executorConfig kubeexecutor.ExecutorConfig,
	profile kubeexecutor.ProfileConfig,
	identity kubeexecutor.ResourceIdentity,
	pod *corev1.Pod,
	pvc *corev1.PersistentVolumeClaim,
	pvcCreated bool,
	remotePort int,
	state string,
) map[string]interface{} {
	metadata := kubernetesRuntimeMetadata(
		executorConfig, profile, identity, pod, pvc, pvcCreated, remotePort,
	)
	metadata[MetadataKeyKubernetesInventoryState] = state
	return metadata
}

func checkpointKubernetesRuntimeInventory(
	ctx context.Context,
	req *ExecutorCreateRequest,
	metadata map[string]interface{},
) error {
	if req == nil || req.CheckpointRuntimeInventory == nil {
		return nil
	}
	if err := req.CheckpointRuntimeInventory(ctx, metadata); err != nil {
		return fmt.Errorf("kubernetes lifecycle: checkpoint runtime inventory: %w", err)
	}
	return nil
}

func kubernetesProfileSnapshot(profile kubeexecutor.ProfileConfig) string {
	data, _ := json.Marshal(profile)
	return string(data)
}

func kubernetesConfigHash(value interface{}) string {
	data, _ := json.Marshal(value)
	return kubernetesStringHash(string(data))
}

func kubernetesStringHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (r *KubernetesExecutor) lockInstance(instanceID string) func() {
	r.mu.Lock()
	lock := r.locks[instanceID]
	if lock == nil {
		lock = &kubernetesInstanceLock{}
		r.locks[instanceID] = lock
	}
	lock.refs++
	r.mu.Unlock()
	lock.mutex.Lock()
	return func() {
		lock.mutex.Unlock()
		r.mu.Lock()
		lock.refs--
		if lock.refs == 0 && r.locks[instanceID] == lock {
			delete(r.locks, instanceID)
		}
		r.mu.Unlock()
	}
}

func (r *KubernetesExecutor) replaceSession(instanceID string, session *kubernetesSession) {
	r.mu.Lock()
	old := r.sessions[instanceID]
	r.sessions[instanceID] = session
	r.mu.Unlock()
	if old != nil {
		if old.client != nil {
			old.client.Close()
		}
		if old.forward != nil {
			_ = old.forward.Close()
		}
	}
}

func kubernetesMainContainerRestartCount(pod *corev1.Pod, mainContainer string) int32 {
	if pod == nil {
		return 0
	}
	for index := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[index].Name == mainContainer {
			return pod.Status.ContainerStatuses[index].RestartCount
		}
	}
	return 0
}

func cloneKubernetesCreateRequest(req *ExecutorCreateRequest) *ExecutorCreateRequest {
	if req == nil {
		return nil
	}
	cloned := *req
	cloned.Env = cloneStringMap(req.Env)
	cloned.Metadata = cloneKubernetesMetadata(req.Metadata)
	cloned.WorkspaceSourceRoots = append([]string(nil), req.WorkspaceSourceRoots...)
	cloned.ApprovedSecretEnvKeys = append([]string(nil), req.ApprovedSecretEnvKeys...)
	cloned.McpServers = append([]McpServerConfig(nil), req.McpServers...)
	cloned.McpProviders = append([]string(nil), req.McpProviders...)
	return &cloned
}

func kubernetesBootstrapCommand() string {
	return `set -eu
while [ ! -f /opt/kandev/start ]; do sleep 1; done
set -a
. /opt/kandev/runtime.env
. /run/kandev/auth.env
set +a
if [ ! -f /opt/kandev/prepared ]; then
  sh /opt/kandev/prepare.sh
  : > /opt/kandev/prepared
fi
exec /opt/kandev/agentctl`
}
