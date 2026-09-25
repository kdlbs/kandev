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
		return launchQueueReplayErrorSummary(task)
	}

	queuedAt := launchQueueQueuedAt(task, deferral)
	destinationID := models.CeilingDeferralSessionID(task, deferral)
	queue := &LaunchQueueSummary{
		SessionID:      destinationID,
		AgentProfileID: launchQueueStringField(deferral.Payload, "agent_profile_id"),
		WorkflowStepID: launchQueueStringField(deferral.Payload, "workflow_step_id"),
		QueuedAt:       queuedAt,
		Reason:         LaunchQueueReasonSessionCapacity,
		Retrying:       true,
	}
	if binding, present, bindingErr := models.ReadCeilingWorkflowEntryBinding(deferral.Payload); bindingErr == nil && present && queue.WorkflowStepID == "" {
		queue.WorkflowStepID = binding.DestinationStepID
	}
	// Workflow-origin records are bound to the task's committed route. A
	// malformed binding or a binding that no longer matches that route is not a
	// capacity wait: showing it as retryable would advertise work that replay
	// must not dispatch. Keep the row visible with an actionable ownership state
	// while the authoritative lifecycle path disposes or replaces the record.
	if launchQueueOwnershipUnavailable(task, deferral, queue.WorkflowStepID, destinationID) {
		queue.Reason = LaunchQueueReasonOwnershipUnavailable
		queue.Retrying = false
		return queue
	}
	if queue.WorkflowStepID == "" {
		if route, ok := models.LoadWorkflowSessionRoute(task.Metadata); ok {
			queue.WorkflowStepID = route.DestinationStepID
		}
	}
	queue.Capacity = launchQueueCapacity(deferral, observation, queuedAt)
	return queue
}

func launchQueueReplayErrorSummary(task *models.Task) *LaunchQueueSummary {
	queuedAt := task.UpdatedAt.UTC()
	if queuedAt.IsZero() {
		queuedAt = time.Now().UTC()
	}
	return &LaunchQueueSummary{QueuedAt: queuedAt, Reason: LaunchQueueReasonReplayError, Retrying: true}
}

func launchQueueQueuedAt(task *models.Task, deferral models.CeilingDeferral) time.Time {
	queuedAt := deferral.QueuedAt.UTC()
	if queuedAt.IsZero() {
		queuedAt = task.UpdatedAt.UTC()
	}
	if queuedAt.IsZero() {
		queuedAt = time.Now().UTC()
	}
	return queuedAt
}

func launchQueueOwnershipUnavailable(
	task *models.Task,
	deferral models.CeilingDeferral,
	workflowStepID, destinationID string,
) bool {
	binding, bindingPresent, bindingErr := models.ReadCeilingWorkflowEntryBinding(deferral.Payload)
	workflowOrigin := bindingPresent || workflowStepID != "" ||
		launchQueueInt64Field(deferral.Payload, "workflow_entry_id") > 0
	if deferral.Kind == models.CeilingLaunchStart {
		if _, routePresent := models.LoadWorkflowSessionRoute(task.Metadata); routePresent {
			workflowOrigin = true
		}
	}
	if bindingErr != nil {
		return true
	}
	if !workflowOrigin {
		return false
	}
	if destinationID == "" {
		return false
	}
	if !bindingPresent || models.CeilingDeferralTargetsSession(task, deferral, destinationID) {
		return false
	}
	// Direct-profile workflow entries have no workflow_session_route. Their
	// binding is still useful to the projection because the task's current
	// workflow and destination step identify the selected lane. Replay applies
	// the stricter latest-entry-ledger check before dispatch.
	if _, routePresent := models.LoadWorkflowSessionRoute(task.Metadata); !routePresent &&
		binding.WorkflowID != "" && task.WorkflowID == binding.WorkflowID &&
		task.WorkflowStepID == binding.DestinationStepID &&
		binding.DestinationSessionID == destinationID {
		return false
	}
	return true
}

func launchQueueCapacity(
	deferral models.CeilingDeferral,
	observation *LaunchQueueCapacityObservation,
	queuedAt time.Time,
) *LaunchQueueCapacity {
	if observation != nil {
		if observation.Known && observation.InUse >= 0 && observation.Limit >= 0 && !observation.ObservedAt.IsZero() {
			return &LaunchQueueCapacity{
				InUse:      observation.InUse,
				Limit:      observation.Limit,
				ObservedAt: observation.ObservedAt.UTC(),
			}
		}
		return nil
	}
	if !deferral.PopulationKnown && deferral.Ceiling <= 0 {
		return nil
	}
	return &LaunchQueueCapacity{
		InUse:      deferral.Population,
		Limit:      deferral.Ceiling,
		ObservedAt: queuedAt,
	}
}

func launchQueueStringField(payload map[string]interface{}, key string) string {
	value, _ := payload[key].(string)
	return value
}

func launchQueueInt64Field(payload map[string]interface{}, key string) int64 {
	switch value := payload[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}
