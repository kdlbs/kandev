package statussummary

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// LaunchQueueCapacityObservation is a point-in-time admission reading supplied
// by the orchestrator. Known is false when the controller or its population
// store cannot be read; the queue remains visible but its count is omitted.
type LaunchQueueCapacityObservation struct {
	InUse      int
	Limit      int
	ObservedAt time.Time
	Known      bool
}

// LaunchQueueSummaryFromTask maps the durable automatic-launch record to the
// bounded task-list projection. The launch payload remains private to the
// orchestrator and is never copied into the status summary.
func LaunchQueueSummaryFromTask(task *models.Task) *LaunchQueueSummary {
	return launchQueueSummaryFromTask(task, nil)
}

// LaunchQueueSummaryFromTaskWithCapacity maps a durable queue entry while
// replacing its old refusal-time capacity with the latest controller reading.
// The durable record remains the source of queue identity and time.
func LaunchQueueSummaryFromTaskWithCapacity(
	task *models.Task,
	observation *LaunchQueueCapacityObservation,
) *LaunchQueueSummary {
	return launchQueueSummaryFromTask(task, observation)
}

func launchQueueSummaryFromTask(
	task *models.Task,
	observation *LaunchQueueCapacityObservation,
) *LaunchQueueSummary {
	if !models.HasCeilingDeferredIntent(task) {
		return nil
	}

	record, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	deferral, err := models.ReadCeilingDeferral(record)
	if err != nil {
		queuedAt := task.UpdatedAt.UTC()
		if queuedAt.IsZero() {
			queuedAt = time.Now().UTC()
		}
		return &LaunchQueueSummary{QueuedAt: queuedAt, Reason: LaunchQueueReasonReplayError, Retrying: true}
	}

	queuedAt := deferral.QueuedAt.UTC()
	if queuedAt.IsZero() {
		queuedAt = task.UpdatedAt.UTC()
	}
	if queuedAt.IsZero() {
		queuedAt = time.Now().UTC()
	}
	queue := &LaunchQueueSummary{
		SessionID:      launchQueueStringField(deferral.Payload, "session_id"),
		AgentProfileID: launchQueueStringField(deferral.Payload, "agent_profile_id"),
		WorkflowStepID: launchQueueStringField(deferral.Payload, "workflow_step_id"),
		QueuedAt:       queuedAt,
		Reason:         LaunchQueueReasonSessionCapacity,
		Retrying:       true,
	}
	if queue.WorkflowStepID == "" {
		if route, ok := models.LoadWorkflowSessionRoute(task.Metadata); ok {
			queue.WorkflowStepID = route.DestinationStepID
		}
	}
	if observation != nil {
		if observation.Known && observation.InUse >= 0 && observation.Limit >= 0 && !observation.ObservedAt.IsZero() {
			queue.Capacity = &LaunchQueueCapacity{
				InUse:      observation.InUse,
				Limit:      observation.Limit,
				ObservedAt: observation.ObservedAt.UTC(),
			}
		}
	} else if deferral.PopulationKnown || deferral.Ceiling > 0 {
		queue.Capacity = &LaunchQueueCapacity{
			InUse:      deferral.Population,
			Limit:      deferral.Ceiling,
			ObservedAt: queuedAt,
		}
	}
	return queue
}

func launchQueueStringField(payload map[string]interface{}, key string) string {
	value, _ := payload[key].(string)
	return value
}
