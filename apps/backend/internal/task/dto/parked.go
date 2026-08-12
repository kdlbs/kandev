package dto

// ParkedProjectionProvider surfaces the orchestrator's runtime
// parked-on-background-work projection for a session. Boolean-only (D-2): V1
// ships no epoch, no revision, no resolveParkedTriple — the wire carries
// exactly one new field per DTO. A nil provider leaves the field at false,
// the correct serialization when the projection is not wired.
type ParkedProjectionProvider interface {
	ParkedProjectionSnapshot(sessionID string) bool
}

// TaskParkedProjectionProvider surfaces the task-level OR of all session
// parked states. Boolean-only (D-2), mirroring ParkedProjectionProvider.
type TaskParkedProjectionProvider interface {
	TaskParkedProjectionSnapshot(taskID string) bool
}

// EnrichParked stamps the current runtime parked projection onto a full
// session DTO. A nil provider leaves the field at false, which is the
// correct serialization when the projection is not wired.
func EnrichParked(session *TaskSessionDTO, provider ParkedProjectionProvider) {
	if session == nil || provider == nil {
		return
	}
	session.ParkedOnBackgroundWork = provider.ParkedProjectionSnapshot(session.ID)
}

// EnrichParkedSummary is the summary DTO equivalent of EnrichParked.
func EnrichParkedSummary(session *TaskSessionSummaryDTO, provider ParkedProjectionProvider) {
	if session == nil || provider == nil {
		return
	}
	session.ParkedOnBackgroundWork = provider.ParkedProjectionSnapshot(session.ID)
}

// EnrichTaskParked stamps the task-level parked projection onto a task DTO. A
// nil provider leaves the field at false.
func EnrichTaskParked(task *TaskDTO, provider TaskParkedProjectionProvider) {
	if task == nil || provider == nil {
		return
	}
	task.ParkedOnBackgroundWork = provider.TaskParkedProjectionSnapshot(task.ID)
}
