package lifecycle

import (
	"github.com/kandev/kandev/internal/task/models"
	corev1 "k8s.io/api/core/v1"
)

// Pod conditions override cached container readiness without requiring node RBAC.
func kubernetesPodAvailabilityReason(pod *corev1.Pod) string {
	reason := ""
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.DisruptionTarget && condition.Status == corev1.ConditionTrue && condition.Reason == "DeletionByTaintManager" {
			return "WorkerUnavailable"
		}
		if condition.Type != corev1.PodReady || condition.Status == corev1.ConditionTrue {
			continue
		}
		if condition.Reason == "NodeNotReady" || condition.Reason == "NodeStatusUnknown" {
			return "WorkerUnavailable"
		}
		reason = "PodNotReady"
	}
	return reason
}

func kubernetesPodConditionEvidence(pod *corev1.Pod) []models.ExecutorPodConditionEvidence {
	var evidence []models.ExecutorPodConditionEvidence
	for _, condition := range pod.Status.Conditions {
		if condition.Type != corev1.PodReady && condition.Type != corev1.DisruptionTarget {
			continue
		}
		if len(evidence) == 4 {
			break
		}
		evidence = append(evidence, models.ExecutorPodConditionEvidence{Type: string(condition.Type), Status: string(condition.Status), Reason: boundedExecutorText(condition.Reason, 128), Message: boundedExecutorText(condition.Message, 256), TransitionAt: nonZeroTimePtr(condition.LastTransitionTime.Time)})
	}
	return evidence
}
