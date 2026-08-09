package dto

// ParkedProjectionProvider surfaces the orchestrator's runtime parked-on-background-work
// projection for a session. The provider is authoritative for all three values;
// a settled probe clears stale client state via the revision.
type ParkedProjectionProvider interface {
	ParkedProjectionSnapshot(sessionID string) (parked bool, epoch int64, revision uint64)
}

// TaskParkedProjectionProvider surfaces the task-level OR of all session parked states.
type TaskParkedProjectionProvider interface {
	TaskParkedProjectionSnapshot(taskID string) (parked bool, revision uint64)
}

// EnrichParked stamps the current runtime parked projection onto a full session DTO.
// The provider is authoritative for all three fields; nil provider leaves them at
// zero values (false, 0, 0), which is the correct serialization when the projection
// is not yet wired (wave-0 hardcoded state).
func EnrichParked(session *TaskSessionDTO, provider ParkedProjectionProvider) {
	if session == nil || provider == nil {
		return
	}
	session.ParkedOnBackgroundWork, session.ParkedEpoch, session.ParkedRevision =
		provider.ParkedProjectionSnapshot(session.ID)
}

// EnrichParkedSummary is the summary DTO equivalent of EnrichParked.
func EnrichParkedSummary(session *TaskSessionSummaryDTO, provider ParkedProjectionProvider) {
	if session == nil || provider == nil {
		return
	}
	session.ParkedOnBackgroundWork, session.ParkedEpoch, session.ParkedRevision =
		provider.ParkedProjectionSnapshot(session.ID)
}

// EnrichTaskParked stamps the task-level parked projection onto a task DTO.
// The provider is authoritative for both parked and revision; nil provider
// leaves them at zero values (false, 0).
func EnrichTaskParked(task *TaskDTO, provider TaskParkedProjectionProvider) {
	if task == nil || provider == nil {
		return
	}
	task.ParkedOnBackgroundWork, task.ParkedRevision =
		provider.TaskParkedProjectionSnapshot(task.ID)
}
