package lifecycle

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
	corev1 "k8s.io/api/core/v1"
	"time"
	"unicode/utf8"
)

func kubernetesExecutorObservation(pod *corev1.Pod, main string, checkedAt time.Time) *models.ExecutorObservation {
	state, _, ready, restarts, reason, message := kubernetesRemotePodState(pod, main)
	observation := &models.ExecutorObservation{Outcome: models.ExecutorOutcomeUnknown, Runtime: "k8s", ResourceKey: string(pod.UID), ObservedAt: checkedAt, Reason: boundedExecutorText(reason, 128), Message: boundedExecutorText(message, 768), PodPhase: string(pod.Status.Phase), ContainerReady: ready, Restarts: restarts, Workspace: models.ExecutorOutcomeUnknown}
	switch state {
	case kubernetesStatusRunning:
		observation.Outcome = models.ExecutorOutcomeHealthy
	case kubernetesStatusFailed, kubernetesStatusCompleted:
		observation.Outcome = models.ExecutorOutcomeTerminated
	}
	observation.PodConditions = kubernetesPodConditionEvidence(pod)
	statuses := append([]corev1.ContainerStatus(nil), pod.Status.ContainerStatuses...)
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	for _, status := range statuses {
		if len(observation.Containers) == 8 {
			break
		}
		evidence := models.ExecutorContainerEvidence{Name: boundedExecutorText(status.Name, 64), Ready: status.Ready, Restarts: status.RestartCount, State: models.ExecutorOutcomeUnknown}
		switch {
		case status.State.Running != nil:
			evidence.State = string(PrepareStepRunning)
			evidence.StartedAt = nonZeroTimePtr(status.State.Running.StartedAt.Time)
		case status.State.Waiting != nil:
			evidence.State = "waiting"
			evidence.Reason = boundedExecutorText(status.State.Waiting.Reason, 64)
		case status.State.Terminated != nil:
			term := status.State.Terminated
			evidence.State = models.ExecutorOutcomeTerminated
			evidence.Reason = boundedExecutorText(term.Reason, 64)
			code, signal := term.ExitCode, term.Signal
			evidence.ExitCode = &code
			evidence.Signal = &signal
			evidence.StartedAt = nonZeroTimePtr(term.StartedAt.Time)
			evidence.FinishedAt = nonZeroTimePtr(term.FinishedAt.Time)
		}
		if last := status.LastTerminationState.Terminated; last != nil {
			code := last.ExitCode
			evidence.LastExitCode = &code
			evidence.LastFinishedAt = nonZeroTimePtr(last.FinishedAt.Time)
		}
		if status.Name == main {
			observation.OccurredAt = evidence.FinishedAt
			if observation.OccurredAt == nil {
				observation.OccurredAt = evidence.LastFinishedAt
			}
		}
		observation.Containers = append(observation.Containers, evidence)
	}
	return observation
}

func (r *KubernetesExecutor) kubernetesWorkspaceRetention(ctx context.Context, instance *ExecutorInstance) string {
	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil || recorded.pvcName == "" {
		return models.ExecutorOutcomeUnknown
	}
	runtime, err := r.kubernetesRuntimeForInstance(instance)
	if err != nil {
		return models.ExecutorOutcomeUnknown
	}
	pvc, err := runtime.resources.GetPersistentVolumeClaim(ctx, recorded.namespace, recorded.pvcName)
	if err != nil {
		return models.ExecutorOutcomeUnknown
	}
	if verifyKubernetesRecordedPVC(ctx, runtime.resources, recorded, identity) != nil {
		return models.ExecutorOutcomeUnknown
	}
	if pvc.Status.Phase == corev1.ClaimBound {
		return models.WorkspaceRetained
	}
	return models.ExecutorOutcomeUnknown
}

// ExecutorUnavailableError carries the resource cause through cleanup/launch wrappers.
type ExecutorUnavailableError struct {
	Observation *models.ExecutorObservation
	Cause       error
}

func (e *ExecutorUnavailableError) Error() string {
	message := fmt.Sprintf("executor unavailable (%s): %s: %s", e.Observation.Outcome, e.Observation.Reason, e.Observation.Message)
	for _, secondary := range e.Observation.Secondary {
		message += "; " + secondary.Operation + ": " + secondary.Reason
	}
	return message
}
func (e *ExecutorUnavailableError) Unwrap() error { return e.Cause }
func (e *ExecutorUnavailableError) ExecutorObservation() *models.ExecutorObservation {
	return e.Observation
}

func boundedExecutorText(value string, maxBytes int) string {
	value = routingerr.Sanitize(value)
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
