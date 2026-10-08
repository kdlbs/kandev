package sqlite

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/task/models"
	"sort"
)

func safeExecutorObservation(observation *models.ExecutorObservation) *models.ExecutorObservation {
	safe := observation.Clone()
	safe.ResourceKey = ""
	safe.Runtime = boundedExecutorDiagnostic(safe.Runtime, 64)
	safe.PodPhase = boundedExecutorDiagnostic(safe.PodPhase, 32)
	safe.Reason = boundedExecutorDiagnostic(safe.Reason, 128)
	safe.Message = boundedExecutorDiagnostic(safe.Message, 768)
	if safe.Workspace != models.WorkspaceRetained {
		safe.Workspace = models.ExecutorOutcomeUnknown
	}
	safe.PodConditions = safeExecutorPodConditions(safe.PodConditions)
	safe.Secondary = mergeExecutorSecondary(nil, safe.Secondary)
	if len(safe.Containers) > 8 {
		safe.Containers = safe.Containers[:8]
	}
	for i := range safe.Containers {
		safe.Containers[i].Name = boundedExecutorDiagnostic(safe.Containers[i].Name, 64)
		safe.Containers[i].Reason = boundedExecutorDiagnostic(safe.Containers[i].Reason, 64)
		safe.Containers[i].State = boundedExecutorDiagnostic(safe.Containers[i].State, 32)
	}
	boundExecutorEvidence(safe)
	return safe
}

// Operation blockers survive subsequent physical checks. Repeated observations
// of the same blocker retain its first timestamp without emitting another episode.
func mergeExecutorSecondary(prior, next []models.ExecutorOperationEvidence) []models.ExecutorOperationEvidence {
	var result []models.ExecutorOperationEvidence
	for _, group := range [][]models.ExecutorOperationEvidence{prior, next} {
		for _, evidence := range group {
			if evidence.Operation != "cleanup" || evidence.OccurredAt.IsZero() {
				continue
			}
			switch evidence.Reason {
			case "cleanup_timeout", "cleanup_unauthorized", "cleanup_failed":
			default:
				continue
			}
			duplicate := false
			for _, existing := range result {
				if existing.Reason == evidence.Reason {
					duplicate = true
					break
				}
			}
			if !duplicate {
				result = append(result, evidence)
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].OccurredAt.Before(result[j].OccurredAt) })
	return result
}

// Detailed container evidence yields to the primary cause and operation blockers
// so a large diagnostic payload cannot suppress the durable failure itself.
func boundExecutorEvidence(observation *models.ExecutorObservation) {
	for len(observation.Containers) > 0 {
		raw, err := json.Marshal(observation)
		if err != nil || len(raw) <= 4096 {
			return
		}
		observation.Containers = observation.Containers[:len(observation.Containers)-1]
	}
}

func safeExecutorFailureObservation(observation *models.ExecutorObservation, prior *models.ExecutorFailureEpisode) *models.ExecutorObservation {
	safe := safeExecutorObservation(observation)
	if prior != nil {
		if observation.Outcome == models.ExecutorOutcomeMissing && prior.Observation.Outcome != models.ExecutorOutcomeMissing {
			safe = prior.Observation.Clone()
		}
		safe.Secondary = mergeExecutorSecondary(prior.Observation.Secondary, observation.Secondary)
		boundExecutorEvidence(safe)
	}
	return safe
}

func safeExecutorPodConditions(conditions []models.ExecutorPodConditionEvidence) []models.ExecutorPodConditionEvidence {
	var result []models.ExecutorPodConditionEvidence
	for _, condition := range conditions {
		if condition.Type != "Ready" && condition.Type != "DisruptionTarget" {
			continue
		}
		if condition.Status != "True" && condition.Status != "False" && condition.Status != "Unknown" {
			continue
		}
		condition.Reason = boundedExecutorDiagnostic(condition.Reason, 128)
		condition.Message = boundedExecutorDiagnostic(condition.Message, 256)
		result = append(result, condition)
		if len(result) == 4 {
			break
		}
	}
	return result
}
