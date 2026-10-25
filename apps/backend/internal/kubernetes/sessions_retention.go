package kubernetes

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	kubernetesRetentionActive      = "active"
	kubernetesRetentionRetained    = "retained"
	kubernetesRetentionTerminating = "terminating"
	kubernetesRetentionTerminal    = "terminal"
	kubernetesRetentionMissing     = "missing"
	kubernetesRetentionUnknown     = "unknown"
)

// MainContainerRequests contains only configured requests from the verified
// Kandev main container. It does not represent observed usage or capacity.
type MainContainerRequests struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

func projectedTaskSessionState(state models.TaskSessionState) string {
	if isKnownTaskSessionState(state) {
		return string(state)
	}
	return kubernetesRetentionUnknown
}

func isKnownTaskSessionState(state models.TaskSessionState) bool {
	for _, known := range models.AllTaskSessionStates {
		if state == known {
			return true
		}
	}
	return false
}

func populatePodStatus(
	row *SessionRow,
	pod *corev1.Pod,
	mainContainer string,
	sessionState models.TaskSessionState,
) {
	row.PodPhase = string(pod.Status.Phase)
	row.ContainerState = "unknown"
	failureReason := pod.Status.Reason
	mainSpecFound := false
	for _, container := range pod.Spec.Containers {
		if container.Name != mainContainer {
			continue
		}
		mainSpecFound = true
		row.MainContainerRequests = requestsFromContainer(container)
		break
	}
	if !mainSpecFound {
		row.FailureReason = "Main container status is unavailable"
		return
	}

	mainStatusFound := false
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != mainContainer {
			continue
		}
		mainStatusFound = true
		row.Restarts = status.RestartCount
		switch {
		case status.State.Running != nil:
			row.ContainerState = "running"
		case status.State.Waiting != nil:
			row.ContainerState = "waiting"
			failureReason = status.State.Waiting.Reason
		case status.State.Terminated != nil:
			row.ContainerState = "terminated"
			failureReason = status.State.Terminated.Reason
		}
		break
	}
	if !mainStatusFound && failureReason == "" {
		failureReason = "Main container status is unavailable"
	}
	row.FailureReason = sanitizedPodReason(failureReason)
	row.RetentionState = kubernetesRetentionState(sessionState, pod)
}

func requestsFromContainer(container corev1.Container) *MainContainerRequests {
	requests := &MainContainerRequests{}
	if quantity, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
		requests.CPU = quantity.String()
	}
	if quantity, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
		requests.Memory = quantity.String()
	}
	if requests.CPU == "" && requests.Memory == "" {
		return nil
	}
	return requests
}

func kubernetesRetentionState(sessionState models.TaskSessionState, pod *corev1.Pod) string {
	if !isKnownTaskSessionState(sessionState) || pod == nil {
		return kubernetesRetentionUnknown
	}
	if pod.DeletionTimestamp != nil {
		return kubernetesRetentionTerminating
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return kubernetesRetentionTerminal
	case corev1.PodPending, corev1.PodRunning:
		if isRetainedTaskSessionState(sessionState) {
			return kubernetesRetentionRetained
		}
		if isActiveTaskSessionState(sessionState) {
			return kubernetesRetentionActive
		}
	}
	return kubernetesRetentionUnknown
}

func isRetainedTaskSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCancelled,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateIdle:
		return true
	default:
		return false
	}
}

func isActiveTaskSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}
