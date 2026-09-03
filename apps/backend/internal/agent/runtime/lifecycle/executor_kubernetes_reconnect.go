package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

func (r *KubernetesExecutor) reconnect(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	executorConfig kubeexecutor.ExecutorConfig,
) (*ExecutorInstance, error) {
	recorded, identity, err := kubernetesRecordedInventory(req, true)
	if err != nil {
		return nil, err
	}
	req = kubernetesRequestWithRecordedIdentity(req, identity)
	pod, err := runtime.resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.recreateMissingKubernetesPod(ctx, runtime, req, executorConfig, recorded, identity)
		}
		return nil, fmt.Errorf("kubernetes lifecycle: inspect recorded Pod: %w", err)
	}
	if err := verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity); err != nil {
		return nil, err
	}
	if err := verifyKubernetesRecordedPVC(ctx, runtime.resources, recorded, identity); err != nil {
		return nil, err
	}

	client, forward, token, remotePort, err := r.reconnectKubernetesAgentctl(ctx, runtime, req, pod, recorded)
	if err != nil {
		return nil, err
	}
	r.replaceSession(req.InstanceID, &kubernetesSession{
		runtime: runtime, forward: forward, client: client,
		request:      cloneKubernetesCreateRequest(req),
		restartCount: kubernetesMainContainerRestartCount(pod, recorded.mainContainer),
	})
	metadata := cloneKubernetesMetadata(req.Metadata)
	metadata[MetadataKeyKubernetesAgentctlRemotePort] = strconv.Itoa(remotePort)
	metadata[MetadataKeyKubernetesAgentctlInstanceID] = recorded.agentctlInstanceID
	metadata[MetadataKeyKubernetesContainerRestartCount] = strconv.FormatInt(
		int64(kubernetesMainContainerRestartCount(pod, recorded.mainContainer)), 10,
	)
	metadata[MetadataKeyKubernetesResourceInstanceID] = identity.InstanceID
	metadata[MetadataKeyKubernetesExecutorConfigHash] = kubernetesConfigHash(executorConfig)
	metadata[MetadataKeyIsRemote] = true
	return &ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: r.Name(), Client: client, WorkspacePath: kubernetesWorkspacePath,
		Metadata: metadata, AuthToken: token, BootstrapNonce: req.BootstrapNonce,
	}, nil
}

type kubernetesRecordedState struct {
	inventoryState     string
	namespace          string
	podName            string
	podUID             types.UID
	mainContainer      string
	workspaceMode      kubeexecutor.WorkspaceMode
	pvcName            string
	pvcUID             types.UID
	pvcCreated         bool
	remotePort         int
	agentctlInstanceID string
}

func kubernetesRecordedInventory(
	req *ExecutorCreateRequest,
	validateConnectionIdentity bool,
) (kubernetesRecordedState, kubeexecutor.ResourceIdentity, error) {
	recorded, err := decodeKubernetesRecordedState(req.Metadata)
	if err != nil {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, err
	}
	identity, err := kubernetesRecordedResourceIdentity(req, validateConnectionIdentity)
	if err != nil {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, err
	}
	if recorded.agentctlInstanceID != identity.InstanceID {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, errors.New(
			"kubernetes lifecycle: recorded agentctl instance does not match resource identity",
		)
	}
	if err := validateKubernetesRecordedPod(recorded); err != nil {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, err
	}
	if err := validateKubernetesRecordedWorkspace(recorded); err != nil {
		return kubernetesRecordedState{}, kubeexecutor.ResourceIdentity{}, err
	}
	return recorded, identity, nil
}

func decodeKubernetesRecordedState(metadata map[string]interface{}) (kubernetesRecordedState, error) {
	recorded := decodeKubernetesRecordedStateBase(metadata)
	remotePort, err := strconv.Atoi(getMetadataString(metadata, MetadataKeyKubernetesAgentctlRemotePort))
	if err != nil || remotePort < 1 || remotePort > 65535 {
		return kubernetesRecordedState{}, errors.New("kubernetes lifecycle: recorded agentctl port is invalid")
	}
	recorded.remotePort = remotePort
	return recorded, nil
}

func decodeKubernetesRecordedStateBase(metadata map[string]interface{}) kubernetesRecordedState {
	return kubernetesRecordedState{
		inventoryState:     getMetadataString(metadata, MetadataKeyKubernetesInventoryState),
		namespace:          getMetadataString(metadata, MetadataKeyKubernetesNamespace),
		podName:            getMetadataString(metadata, MetadataKeyKubernetesPodName),
		podUID:             types.UID(getMetadataString(metadata, MetadataKeyKubernetesPodUID)),
		mainContainer:      getMetadataString(metadata, MetadataKeyKubernetesMainContainer),
		workspaceMode:      kubeexecutor.WorkspaceMode(getMetadataString(metadata, MetadataKeyKubernetesRuntimeWorkspaceMode)),
		pvcName:            getMetadataString(metadata, MetadataKeyKubernetesPVCName),
		pvcUID:             types.UID(getMetadataString(metadata, MetadataKeyKubernetesPVCUID)),
		pvcCreated:         getMetadataBool(metadata, MetadataKeyKubernetesPVCCreated),
		agentctlInstanceID: getMetadataString(metadata, MetadataKeyKubernetesAgentctlInstanceID),
	}
}

func kubernetesRecordedResourceIdentity(
	req *ExecutorCreateRequest,
	validateConnectionIdentity bool,
) (kubeexecutor.ResourceIdentity, error) {
	identity := kubeexecutor.ResourceIdentity{
		ExecutorID:    getMetadataString(req.Metadata, MetadataKeyKubernetesResourceExecutorID),
		ProfileID:     getMetadataString(req.Metadata, MetadataKeyKubernetesResourceProfileID),
		InstanceID:    getMetadataString(req.Metadata, MetadataKeyKubernetesResourceInstanceID),
		TaskID:        getMetadataString(req.Metadata, MetadataKeyKubernetesResourceTaskID),
		SessionID:     getMetadataString(req.Metadata, MetadataKeyKubernetesResourceSessionID),
		EnvironmentID: getMetadataString(req.Metadata, MetadataKeyKubernetesResourceEnvironmentID),
	}
	if identity.ExecutorID == "" || identity.ProfileID == "" || identity.InstanceID == "" ||
		identity.TaskID == "" || identity.SessionID == "" || identity.EnvironmentID == "" {
		return kubeexecutor.ResourceIdentity{}, errors.New("kubernetes lifecycle: recorded resource identity is incomplete")
	}
	if _, labelErr := kubeexecutor.OwnershipLabels(identity); labelErr != nil {
		return kubeexecutor.ResourceIdentity{}, labelErr
	}
	if err := validateKubernetesRecordedIdentityField("task", req.TaskID, identity.TaskID); err != nil {
		return kubeexecutor.ResourceIdentity{}, err
	}
	if err := validateKubernetesRecordedIdentityField("session", req.SessionID, identity.SessionID); err != nil {
		return kubeexecutor.ResourceIdentity{}, err
	}
	if validateConnectionIdentity {
		if err := validateKubernetesRecordedIdentityField(
			"executor", getMetadataString(req.Metadata, "executor_id"), identity.ExecutorID,
		); err != nil {
			return kubeexecutor.ResourceIdentity{}, err
		}
		if err := validateKubernetesRecordedIdentityField(
			"profile", getMetadataString(req.Metadata, MetadataKeyExecutorProfileID), identity.ProfileID,
		); err != nil {
			return kubeexecutor.ResourceIdentity{}, err
		}
	}
	return identity, nil
}

func validateKubernetesRecordedIdentityField(kind, requestValue, recordedValue string) error {
	if requestValue == "" || requestValue != recordedValue {
		return fmt.Errorf("kubernetes lifecycle: recorded resource %s identity does not match request", kind)
	}
	return nil
}

func kubernetesRequestWithRecordedIdentity(
	req *ExecutorCreateRequest,
	identity kubeexecutor.ResourceIdentity,
) *ExecutorCreateRequest {
	recorded := *req
	recorded.TaskID = identity.TaskID
	recorded.SessionID = identity.SessionID
	recorded.TaskEnvironmentID = identity.EnvironmentID
	return &recorded
}

func validateKubernetesRecordedPod(
	recorded kubernetesRecordedState,
) error {
	if recorded.namespace == "" ||
		recorded.podName == "" || recorded.podUID == "" || recorded.mainContainer == "" ||
		recorded.agentctlInstanceID == "" {
		return errors.New("kubernetes lifecycle: recorded Pod inventory is incomplete")
	}
	return nil
}

func validateKubernetesRecordedWorkspace(recorded kubernetesRecordedState) error {
	switch recorded.workspaceMode {
	case kubeexecutor.WorkspaceModeEmptyDir:
		if recorded.pvcName != "" || recorded.pvcUID != "" || recorded.pvcCreated {
			return errors.New("kubernetes lifecycle: emptyDir inventory unexpectedly records a PVC")
		}
	case kubeexecutor.WorkspaceModeManagedPVC, kubeexecutor.WorkspaceModeExistingClaim:
		if recorded.pvcName == "" || recorded.pvcUID == "" {
			return errors.New("kubernetes lifecycle: recorded PVC inventory is incomplete")
		}
		if recorded.workspaceMode == kubeexecutor.WorkspaceModeManagedPVC && !recorded.pvcCreated {
			return errors.New("kubernetes lifecycle: managed PVC inventory does not record ownership")
		}
		if recorded.workspaceMode == kubeexecutor.WorkspaceModeExistingClaim && recorded.pvcCreated {
			return errors.New("kubernetes lifecycle: existing PVC inventory records Kandev ownership")
		}
	default:
		return errors.New("kubernetes lifecycle: recorded workspace mode is invalid")
	}
	return nil
}

func verifyKubernetesRecordedPVC(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) error {
	if recorded.workspaceMode == kubeexecutor.WorkspaceModeEmptyDir {
		return nil
	}
	pvc, err := resources.GetPersistentVolumeClaim(ctx, recorded.namespace, recorded.pvcName)
	if err != nil {
		return fmt.Errorf("kubernetes lifecycle: inspect recorded PVC: %w", err)
	}
	if pvc == nil || pvc.Namespace != recorded.namespace || pvc.Name != recorded.pvcName || pvc.UID != recorded.pvcUID {
		return errors.New("kubernetes lifecycle: PVC UID or name does not match recorded inventory")
	}
	if recorded.workspaceMode == kubeexecutor.WorkspaceModeManagedPVC {
		labels, labelErr := kubeexecutor.OwnershipLabels(identity)
		if labelErr != nil {
			return labelErr
		}
		if labelErr := kubeexecutor.ValidateOwnershipLabels(pvc.Labels, labels); labelErr != nil {
			return fmt.Errorf("kubernetes lifecycle: PVC ownership labels: %w", labelErr)
		}
	}
	return nil
}

func (r *KubernetesExecutor) reconnectKubernetesAgentctl(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	pod *corev1.Pod,
	recorded kubernetesRecordedState,
) (*agentctl.Client, kubeexecutor.PortForwardSession, string, int, error) {
	if req.AuthToken == "" {
		return nil, nil, "", 0, errors.New("kubernetes lifecycle: recorded agentctl auth token is unavailable")
	}
	client, forward, err := r.reattachKubernetesAgentctl(ctx, runtime, req, pod, recorded)
	if err == nil {
		return client, forward, req.AuthToken, recorded.remotePort, nil
	}
	if req.BootstrapNonce == "" {
		return nil, nil, "", 0, errors.New("kubernetes lifecycle: agentctl restart detected but bootstrap nonce is unavailable")
	}
	return r.connectRestartedKubernetesAgentctl(ctx, runtime, req, pod, recorded.agentctlInstanceID)
}

func (r *KubernetesExecutor) reattachKubernetesAgentctl(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	pod *corev1.Pod,
	recorded kubernetesRecordedState,
) (*agentctl.Client, kubeexecutor.PortForwardSession, error) {
	forward, err := startKubernetesForward(ctx, runtime.streams, pod, uint16(recorded.remotePort))
	if err != nil {
		return nil, nil, err
	}
	client := newKubernetesAgentctlClient(r.logger, req, recorded.agentctlInstanceID, req.AuthToken, forward.LocalPort())
	if healthErr := client.Health(ctx); healthErr != nil {
		client.Close()
		_ = forward.Close()
		return nil, nil, fmt.Errorf("kubernetes lifecycle: recorded instance health: %w", healthErr)
	}
	return client, forward, nil
}

func (r *KubernetesExecutor) connectRestartedKubernetesAgentctl(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	pod *corev1.Pod,
	remoteInstanceID string,
) (*agentctl.Client, kubeexecutor.PortForwardSession, string, int, error) {
	controlForward, control, err := r.connectHealthyKubernetesControl(ctx, runtime, pod)
	if err != nil {
		return nil, nil, "", 0, err
	}
	defer func() { _ = controlForward.Close() }()
	token, err := control.Handshake(ctx, req.BootstrapNonce)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: restarted nonce handshake: %w", err)
	}
	createRequest := buildReconnectCreateInstanceRequest(req, remoteInstanceID)
	response, err := createOrReconcileKubernetesAgentctlInstance(ctx, control, createRequest)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: recreate agentctl instance: %w", err)
	}
	if response.Port < 1 || response.Port > 65535 {
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: invalid restarted agentctl port %d", response.Port)
	}
	forward, err := startKubernetesForward(ctx, runtime.streams, pod, uint16(response.Port))
	if err != nil {
		return nil, nil, "", 0, err
	}
	client := newKubernetesAgentctlClient(r.logger, req, remoteInstanceID, token, forward.LocalPort())
	if err := client.Health(ctx); err != nil {
		client.Close()
		_ = forward.Close()
		return nil, nil, "", 0, fmt.Errorf("kubernetes lifecycle: restarted instance health: %w", err)
	}
	return client, forward, token, response.Port, nil
}

func newKubernetesAgentctlClient(
	log *logger.Logger,
	req *ExecutorCreateRequest,
	remoteInstanceID, token string,
	localPort uint16,
) *agentctl.Client {
	return agentctl.NewClient(kubernetesControlHost, int(localPort), log,
		agentctl.WithExecutionID(remoteInstanceID), agentctl.WithSessionID(req.SessionID), agentctl.WithAuthToken(token))
}

func cloneKubernetesMetadata(metadata map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func getMetadataBool(metadata map[string]interface{}, key string) bool {
	if metadata == nil {
		return false
	}
	switch value := metadata[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(value)
		return parsed
	default:
		return false
	}
}

func (r *KubernetesExecutor) recreateMissingKubernetesPod(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	executorConfig kubeexecutor.ExecutorConfig,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) (_ *ExecutorInstance, returnedErr error) {
	profile, err := kubernetesProfileFromRecordedSnapshot(req.Metadata)
	if err != nil {
		return nil, err
	}
	if err := r.validateKubernetesPodReplacement(ctx, runtime, req, profile, recorded, identity); err != nil {
		return nil, err
	}
	desired, err := composeKubernetesLifecyclePod(
		profile, identity, recorded.podName, recorded.namespace, recorded.pvcName,
	)
	if err != nil {
		return nil, err
	}
	podWaitCtx, cancelPodWait := kubernetesRequestContext(ctx, executorConfig.RequestTimeoutSeconds)
	created, pod, createErr := createAndWaitKubernetesPod(
		podWaitCtx, runtime.resources, desired, identity, recorded.mainContainer, "replacement Pod",
		func(state string, pod *corev1.Pod) error {
			metadata := cloneKubernetesMetadata(req.Metadata)
			metadata[MetadataKeyKubernetesInventoryState] = state
			metadata[MetadataKeyKubernetesPodName] = pod.Name
			metadata[MetadataKeyKubernetesPodUID] = string(pod.UID)
			return checkpointKubernetesRuntimeInventory(ctx, req, metadata)
		},
	)
	cancelPodWait()
	defer func() {
		if returnedErr != nil && created != nil {
			rollbackCtx, cancel := kubernetesDetachedRequestContext(ctx, executorConfig.RequestTimeoutSeconds)
			defer cancel()
			rollbackErr := rollbackCreatedKubernetesPod(rollbackCtx, runtime.resources, created)
			returnedErr = errors.Join(returnedErr, rollbackErr)
		}
	}()
	if createErr != nil {
		return nil, createErr
	}
	binary, err := r.resolveBinary(profile.Platform)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: resolve replacement agentctl: %w", err)
	}
	if err := r.bootstrapPod(ctx, runtime, req, pod, profile, req.BootstrapNonce, binary); err != nil {
		return nil, err
	}
	client, forward, token, remotePort, err := r.connectRestartedKubernetesAgentctl(
		ctx, runtime, req, pod, recorded.agentctlInstanceID,
	)
	if err != nil {
		return nil, err
	}
	metadata := cloneKubernetesMetadata(req.Metadata)
	metadata[MetadataKeyKubernetesPodUID] = string(pod.UID)
	metadata[MetadataKeyKubernetesAgentctlRemotePort] = strconv.Itoa(remotePort)
	metadata[MetadataKeyKubernetesContainerRestartCount] = strconv.FormatInt(
		int64(kubernetesMainContainerRestartCount(pod, recorded.mainContainer)), 10,
	)
	metadata[MetadataKeyKubernetesExecutorConfigHash] = kubernetesConfigHash(executorConfig)
	metadata[MetadataKeyKubernetesInventoryState] = KubernetesInventoryStateReady
	metadata[MetadataKeyIsRemote] = true
	if err := checkpointKubernetesRuntimeInventory(ctx, req, metadata); err != nil {
		client.Close()
		_ = forward.Close()
		return nil, err
	}
	r.replaceSession(req.InstanceID, &kubernetesSession{
		runtime: runtime, forward: forward, client: client,
		request:      cloneKubernetesCreateRequest(req),
		restartCount: kubernetesMainContainerRestartCount(pod, recorded.mainContainer),
	})
	return &ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: r.Name(), Client: client, WorkspacePath: kubernetesWorkspacePath,
		Metadata: metadata, AuthToken: token, BootstrapNonce: req.BootstrapNonce,
	}, nil
}

func (r *KubernetesExecutor) validateKubernetesPodReplacement(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	profile kubeexecutor.ProfileConfig,
	recorded kubernetesRecordedState,
	identity kubeexecutor.ResourceIdentity,
) error {
	if recorded.workspaceMode == kubeexecutor.WorkspaceModeEmptyDir {
		return errors.New("kubernetes lifecycle: recorded Pod is missing; emptyDir workspace is unrecoverable")
	}
	if profile.Workspace.Mode != recorded.workspaceMode || profile.MainContainer != recorded.mainContainer {
		return errors.New("kubernetes lifecycle: recorded Pod profile does not match replacement configuration")
	}
	if err := verifyKubernetesRecordedPVC(ctx, runtime.resources, recorded, identity); err != nil {
		return err
	}
	if req.BootstrapNonce == "" {
		return errors.New("kubernetes lifecycle: cannot replace Pod without recorded bootstrap nonce")
	}
	if r.resolveBinary == nil {
		return errors.New("kubernetes lifecycle: agentctl resolver is not configured")
	}
	return nil
}

func kubernetesProfileFromRecordedSnapshot(metadata map[string]interface{}) (kubeexecutor.ProfileConfig, error) {
	raw := getMetadataString(metadata, MetadataKeyKubernetesProfileSnapshot)
	if raw == "" {
		return kubeexecutor.ProfileConfig{}, errors.New("kubernetes lifecycle: recorded workload profile snapshot is missing")
	}
	var profile kubeexecutor.ProfileConfig
	if err := json.Unmarshal([]byte(raw), &profile); err != nil {
		return kubeexecutor.ProfileConfig{}, fmt.Errorf("kubernetes lifecycle: decode workload profile snapshot: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return kubeexecutor.ProfileConfig{}, fmt.Errorf("kubernetes lifecycle: validate workload profile snapshot: %w", err)
	}
	if recordedHash := getMetadataString(metadata, MetadataKeyKubernetesProfileConfigHash); recordedHash == "" ||
		recordedHash != kubernetesConfigHash(profile) {
		return kubeexecutor.ProfileConfig{}, errors.New("kubernetes lifecycle: workload profile snapshot hash does not match")
	}
	if templateHash := getMetadataString(metadata, MetadataKeyKubernetesTemplateHash); templateHash == "" ||
		templateHash != kubernetesStringHash(profile.PodTemplateYAML) {
		return kubeexecutor.ProfileConfig{}, errors.New("kubernetes lifecycle: workload template snapshot hash does not match")
	}
	return profile, nil
}

// ValidateKubernetesResumeMetadata validates the durable runtime authority a
// caller must establish before stopping an active Kubernetes execution for an
// exact-Pod reconnect. It intentionally performs no API calls.
func ValidateKubernetesResumeMetadata(
	metadata map[string]interface{},
	taskID, sessionID string,
	executorConfigValues map[string]string,
) error {
	if _, err := kubeexecutor.ParseExecutorConfig(executorConfigValues); err != nil {
		return fmt.Errorf("kubernetes lifecycle: validate current executor config: %w", err)
	}
	req := &ExecutorCreateRequest{TaskID: taskID, SessionID: sessionID, Metadata: metadata}
	recorded, _, err := kubernetesRecordedInventory(req, true)
	if err != nil {
		return err
	}
	if recorded.inventoryState != KubernetesInventoryStateReady {
		return errors.New("kubernetes lifecycle: recorded runtime inventory is not ready")
	}
	profile, err := kubernetesProfileFromRecordedSnapshot(metadata)
	if err != nil {
		return err
	}
	if profile.MainContainer != recorded.mainContainer || profile.Workspace.Mode != recorded.workspaceMode {
		return errors.New("kubernetes lifecycle: recorded workload snapshot does not match runtime inventory")
	}
	if getMetadataString(metadata, MetadataKeyAuthTokenSecret) == "" ||
		getMetadataString(metadata, MetadataKeyBootstrapNonceSecret) == "" {
		return errors.New("kubernetes lifecycle: recorded runtime secret references are incomplete")
	}
	return nil
}
