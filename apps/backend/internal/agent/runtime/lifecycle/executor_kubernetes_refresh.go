package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"go.uber.org/zap"
)

// RefreshRemoteInstance stages a fresh agentctl instance connection when the
// tracked Pod's main container restarted or the tracked client died. Kubernetes
// resources and the durable bootstrap nonce remain unchanged.
func (r *KubernetesExecutor) RefreshRemoteInstance(
	ctx context.Context,
	instance *ExecutorInstance,
) (*RemoteInstanceRefresh, error) {
	if instance == nil || instance.InstanceID == "" {
		return nil, errKubernetesLifecycleRequestIncomplete
	}
	unlock := r.lockInstance(instance.InstanceID)
	defer unlock()
	current := r.currentKubernetesSession(instance.InstanceID)
	if current == nil || current.runtime == nil || current.client == nil || current.request == nil {
		return nil, nil
	}
	freshRuntime, err := r.newKubernetesRefreshRuntime(instance)
	if err != nil {
		return nil, err
	}
	inspection, refreshNeeded, err := r.inspectKubernetesRefresh(ctx, instance, current, freshRuntime)
	if err != nil {
		return nil, err
	}
	if !refreshNeeded {
		if err := r.publishKubernetesRefreshRuntime(instance.InstanceID, current, freshRuntime); err != nil {
			return nil, err
		}
		return nil, nil
	}
	req := kubernetesActiveRefreshRequest(current.request, instance, inspection.identity)
	client, forward, token, remotePort, err := r.connectKubernetesRefresh(
		ctx, freshRuntime, current, req, inspection,
	)
	if err != nil {
		return nil, err
	}
	metadata := cloneKubernetesMetadata(instance.Metadata)
	metadata[MetadataKeyKubernetesAgentctlRemotePort] = strconv.Itoa(remotePort)
	metadata[MetadataKeyKubernetesContainerRestartCount] = strconv.FormatInt(int64(inspection.restarts), 10)
	metadata[MetadataKeyKubernetesInventoryState] = KubernetesInventoryStateReady
	metadata[MetadataKeyIsRemote] = true
	staged := &kubernetesSession{
		runtime: freshRuntime, forward: forward, client: client,
		request: req, restartCount: inspection.restarts,
	}
	commit, abort := r.kubernetesRefreshFinalizers(instance.InstanceID, current, staged)
	return &RemoteInstanceRefresh{
		Instance: &ExecutorInstance{
			InstanceID:  instance.InstanceID,
			TaskID:      inspection.identity.TaskID,
			SessionID:   inspection.identity.SessionID,
			RuntimeName: r.Name(), Client: client, WorkspacePath: kubernetesWorkspacePath,
			Metadata: metadata, AuthToken: token, BootstrapNonce: instance.BootstrapNonce,
		},
		AgentConfig:            current.request.AgentConfig,
		McpServers:             append([]McpServerConfig(nil), current.request.McpServers...),
		AutoApprovePermissions: current.request.AutoApprovePermissions,
		ProcessRestarted:       inspection.restarted,
		Commit:                 commit,
		Abort:                  abort,
	}, nil
}

type kubernetesRefreshInspection struct {
	recorded  kubernetesRecordedState
	identity  kubeexecutor.ResourceIdentity
	pod       *corev1.Pod
	restarts  int32
	restarted bool
}

func (r *KubernetesExecutor) currentKubernetesSession(instanceID string) *kubernetesSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[instanceID]
}

func (r *KubernetesExecutor) inspectKubernetesRefresh(
	ctx context.Context,
	instance *ExecutorInstance,
	current *kubernetesSession,
	freshRuntime *kubernetesRuntimeClient,
) (kubernetesRefreshInspection, bool, error) {
	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil {
		return kubernetesRefreshInspection{}, false, err
	}
	pod, err := freshRuntime.resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return kubernetesRefreshInspection{recorded: recorded, identity: identity}, false, nil
		}
		return kubernetesRefreshInspection{}, false, fmt.Errorf(
			"kubernetes lifecycle: inspect active Pod for refresh: %w", err,
		)
	}
	if err := verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity); err != nil {
		return kubernetesRefreshInspection{}, false, err
	}
	if err := verifyKubernetesRecordedPVC(ctx, freshRuntime.resources, recorded, identity); err != nil {
		return kubernetesRefreshInspection{}, false, err
	}
	restarts := kubernetesMainContainerRestartCount(pod, recorded.mainContainer)
	restarted := restarts != current.restartCount
	inspection := kubernetesRefreshInspection{
		recorded: recorded, identity: identity, pod: pod, restarts: restarts, restarted: restarted,
	}
	if !restarted && current.client.Health(ctx) == nil {
		return inspection, false, nil
	}
	if restarted && instance.BootstrapNonce == "" {
		return kubernetesRefreshInspection{}, false, errors.New(
			"kubernetes lifecycle: active restart detected but bootstrap nonce is unavailable",
		)
	}
	return inspection, true, nil
}

func (r *KubernetesExecutor) newKubernetesRefreshRuntime(
	instance *ExecutorInstance,
) (*kubernetesRuntimeClient, error) {
	if r.clientFactory == nil {
		return nil, errors.New("kubernetes lifecycle: refresh client factory is not configured")
	}
	config, err := kubernetesExecutorConfigFromMetadata(instance.Metadata)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: refresh configuration: %w", err)
	}
	runtime, err := r.clientFactory(config)
	if err != nil {
		return nil, fmt.Errorf(
			"kubernetes lifecycle: refresh client: %w", routingerr.SanitizeError(err),
		)
	}
	if runtime == nil || runtime.resources == nil || runtime.streams == nil {
		return nil, errors.New("kubernetes lifecycle: refresh client is incomplete")
	}
	return runtime, nil
}

func (r *KubernetesExecutor) publishKubernetesRefreshRuntime(
	instanceID string,
	current *kubernetesSession,
	freshRuntime *kubernetesRuntimeClient,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[instanceID] != current {
		return errors.New("kubernetes lifecycle: active session changed during credential refresh")
	}
	current.runtime = freshRuntime
	return nil
}

func kubernetesActiveRefreshRequest(
	current *ExecutorCreateRequest,
	instance *ExecutorInstance,
	identity kubeexecutor.ResourceIdentity,
) *ExecutorCreateRequest {
	req := cloneKubernetesCreateRequest(current)
	req.InstanceID = instance.InstanceID
	req.TaskID = identity.TaskID
	req.SessionID = identity.SessionID
	req.TaskEnvironmentID = identity.EnvironmentID
	req.Metadata = cloneKubernetesMetadata(instance.Metadata)
	req.AuthToken = instance.AuthToken
	req.BootstrapNonce = instance.BootstrapNonce
	return req
}

func (r *KubernetesExecutor) connectKubernetesRefresh(
	ctx context.Context,
	freshRuntime *kubernetesRuntimeClient,
	current *kubernetesSession,
	req *ExecutorCreateRequest,
	inspection kubernetesRefreshInspection,
) (*agentctl.Client, kubeexecutor.PortForwardSession, string, int, error) {
	if inspection.restarted {
		return r.connectRestartedKubernetesAgentctl(
			ctx, freshRuntime, req, inspection.pod, inspection.recorded.agentctlInstanceID,
		)
	}
	client, forward, err := r.reattachKubernetesAgentctl(
		ctx, freshRuntime, req, inspection.pod, inspection.recorded,
	)
	return client, forward, req.AuthToken, inspection.recorded.remotePort, err
}

func (r *KubernetesExecutor) kubernetesRefreshFinalizers(
	instanceID string,
	current, staged *kubernetesSession,
) (func(func()) error, func()) {
	var finalizeMu sync.Mutex
	finalized := false
	commit := func(publish func()) error {
		finalizeMu.Lock()
		defer finalizeMu.Unlock()
		if finalized {
			return errors.New("kubernetes lifecycle: remote refresh was already finalized")
		}
		commitUnlock := r.lockInstance(instanceID)
		defer commitUnlock()
		r.mu.Lock()
		if r.sessions[instanceID] != current {
			r.mu.Unlock()
			return errors.New("kubernetes lifecycle: active session changed during remote refresh")
		}
		r.sessions[instanceID] = staged
		r.mu.Unlock()
		finalized = true
		if publish != nil {
			publish()
		}
		if closeErr := closeKubernetesSessionResources(current); closeErr != nil {
			r.logger.Warn("failed to close replaced Kubernetes session resources",
				zap.String("instance_id", instanceID), zap.Error(closeErr))
		}
		return nil
	}
	abort := func() {
		finalizeMu.Lock()
		defer finalizeMu.Unlock()
		if finalized {
			return
		}
		finalized = true
		_ = closeKubernetesSessionResources(staged)
	}
	return commit, abort
}
