package lifecycle

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

const (
	kubernetesStatusStarting  = "starting"
	kubernetesStatusRunning   = "running"
	kubernetesStatusCompleted = "completed"
	kubernetesStatusFailed    = "failed"
	kubernetesStatusUnknown   = "unknown"
	kubernetesStatusPVCName   = "pvc_name"
	kubernetesStatusWorkspace = "workspace_mode"
)

func (r *KubernetesExecutor) GetRemoteStatus(
	ctx context.Context,
	instance *ExecutorInstance,
) (*RemoteStatus, error) {
	checkedAt := time.Now().UTC()
	if instance == nil {
		return nil, fmt.Errorf("kubernetes lifecycle: instance is nil")
	}
	unlock := r.lockInstance(instance.InstanceID)
	defer unlock()

	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil {
		return nil, err
	}
	if recorded.podUID == "" {
		return &RemoteStatus{
			RuntimeName: r.Name(), State: kubernetesStatusStarting, LastCheckedAt: checkedAt,
			Details: map[string]interface{}{
				MetadataKeyKubernetesConfigNamespace: recorded.namespace,
				kubernetesStatusWorkspace:            string(recorded.workspaceMode),
				kubernetesStatusPVCName:              recorded.pvcName,
				"inventory_state":                    recorded.inventoryState,
			},
		}, nil
	}
	runtime, err := r.kubernetesRuntimeForInstance(instance)
	if err != nil {
		return nil, err
	}
	pod, err := runtime.resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if apierrors.IsNotFound(err) {
		return &RemoteStatus{
			RuntimeName: r.Name(), RemoteName: recorded.podName, State: "missing",
			LastCheckedAt: checkedAt,
			Details: map[string]interface{}{
				MetadataKeyKubernetesConfigNamespace: recorded.namespace,
				"pod_name":                           recorded.podName,
				kubernetesStatusWorkspace:            string(recorded.workspaceMode),
				kubernetesStatusPVCName:              recorded.pvcName,
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf(
			"kubernetes lifecycle: query recorded Pod status: %w",
			routingerr.SanitizeError(err),
		)
	}
	if err := verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity); err != nil {
		return nil, err
	}
	state, containerState, ready, restarts, reason, message := kubernetesRemotePodState(pod, recorded.mainContainer)
	details := map[string]interface{}{
		MetadataKeyKubernetesConfigNamespace: recorded.namespace,
		"pod_name":                           recorded.podName,
		"pod_phase":                          string(pod.Status.Phase),
		"main_container":                     recorded.mainContainer,
		"container_state":                    containerState,
		"container_ready":                    ready,
		"restart_count":                      restarts,
		kubernetesStatusWorkspace:            string(recorded.workspaceMode),
		kubernetesStatusPVCName:              recorded.pvcName,
	}
	if reason != "" {
		details["reason"] = reason
	}
	if message != "" {
		details["message"] = message
	}
	var createdAt *time.Time
	if !pod.CreationTimestamp.IsZero() {
		value := pod.CreationTimestamp.UTC()
		createdAt = &value
	}
	return &RemoteStatus{
		RuntimeName: r.Name(), RemoteName: recorded.podName, State: state,
		CreatedAt: createdAt, LastCheckedAt: checkedAt, Details: details,
	}, nil
}

func (r *KubernetesExecutor) kubernetesRuntimeForInstance(
	instance *ExecutorInstance,
) (*kubernetesRuntimeClient, error) {
	r.mu.Lock()
	session := r.sessions[instance.InstanceID]
	r.mu.Unlock()
	if session != nil && session.runtime != nil {
		return session.runtime, nil
	}
	if r.clientFactory == nil {
		return nil, fmt.Errorf("kubernetes lifecycle: status client factory is not configured")
	}
	config, err := kubernetesExecutorConfigFromMetadata(instance.Metadata)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: status configuration: %w", err)
	}
	runtime, err := r.clientFactory(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes lifecycle: status client: %w", routingerr.SanitizeError(err))
	}
	if runtime == nil || runtime.resources == nil {
		return nil, fmt.Errorf("kubernetes lifecycle: status resource client is unavailable")
	}
	return runtime, nil
}

func kubernetesRemotePodState(
	pod *corev1.Pod,
	mainContainer string,
) (state, containerState string, ready bool, restarts int32, reason, message string) {
	projection := kubernetesPodPhaseProjection(pod)
	for index := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[index]
		if status.Name == mainContainer {
			projection = kubernetesContainerProjection(pod.DeletionTimestamp != nil, status, projection)
			break
		}
	}
	return projection.state, projection.containerState, projection.ready,
		projection.restarts, routingerr.Sanitize(projection.reason), routingerr.Sanitize(projection.message)
}

type kubernetesPodStatusProjection struct {
	state          string
	containerState string
	ready          bool
	restarts       int32
	reason         string
	message        string
}

func kubernetesPodPhaseProjection(pod *corev1.Pod) kubernetesPodStatusProjection {
	projection := kubernetesPodStatusProjection{
		state: kubernetesStatusUnknown, reason: pod.Status.Reason, message: pod.Status.Message,
	}
	if pod.DeletionTimestamp != nil {
		projection.state = "stopping"
		return projection
	}
	switch pod.Status.Phase {
	case corev1.PodPending, corev1.PodRunning:
		projection.state = kubernetesStatusStarting
	case corev1.PodSucceeded:
		projection.state = kubernetesStatusCompleted
	case corev1.PodFailed:
		projection.state = kubernetesStatusFailed
	}
	return projection
}

func kubernetesContainerProjection(
	podDeleting bool,
	status *corev1.ContainerStatus,
	projection kubernetesPodStatusProjection,
) kubernetesPodStatusProjection {
	projection.ready, projection.restarts = status.Ready, status.RestartCount
	switch {
	case status.State.Running != nil:
		projection.containerState = kubernetesStatusRunning
		if !podDeleting {
			projection.state = kubernetesStatusStarting
			if status.Ready {
				projection.state = kubernetesStatusRunning
			}
		}
	case status.State.Waiting != nil:
		projection.containerState = "waiting"
		projection.reason, projection.message = status.State.Waiting.Reason, status.State.Waiting.Message
		if !podDeleting {
			projection.state = kubernetesStatusStarting
		}
	case status.State.Terminated != nil:
		projection.containerState = "terminated"
		projection.reason, projection.message = status.State.Terminated.Reason, status.State.Terminated.Message
		if !podDeleting && status.State.Terminated.ExitCode == 0 {
			projection.state = kubernetesStatusCompleted
		} else if !podDeleting {
			projection.state = kubernetesStatusFailed
		}
	default:
		projection.containerState = kubernetesStatusUnknown
	}
	return projection
}

var _ RemoteStatusProvider = (*KubernetesExecutor)(nil)
