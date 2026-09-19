package recoveryclaim

import "github.com/kandev/kandev/internal/task/models"

// ClassifyAdmission applies the fail-closed durable predicate used before a
// task environment recovery claim is acquired.
func ClassifyAdmission(snapshot models.TaskEnvironmentAdmissionSnapshot) models.TaskEnvironmentAdmissionClass {
	if snapshot.MaterializationSessionID != "" {
		return models.TaskEnvironmentAdmissionLiveBlocker
	}
	for _, consumer := range snapshot.Consumers {
		if consumerBlocksAdmission(consumer) {
			return models.TaskEnvironmentAdmissionLiveBlocker
		}
	}
	return models.TaskEnvironmentAdmissionInactivePreserved
}

func consumerBlocksAdmission(consumer models.TaskEnvironmentAdmissionConsumer) bool {
	if consumer.HasActiveTurn {
		return true
	}
	if consumer.HasExecutor && consumer.ExecutorStatus != models.ExecutorRunningStatusStopped {
		return true
	}
	switch consumer.SessionState {
	case models.TaskSessionStateIdle,
		models.TaskSessionStateWaitingForInput:
		return !consumer.HasExecutor
	case models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled:
		return false
	default:
		return true
	}
}
