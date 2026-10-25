package dto

// ParkedProvider surfaces the orchestrator's session-level runtime
// parked_on_background_work projection and its process epoch, without
// coupling task serialization to the orchestrator package (spec:
// docs/specs/disambiguate-waiting/spec.md).
type ParkedProvider interface {
	// ParkedSnapshot returns a session's projection and its process-local
	// transition revision from one critical section (D1), mirroring
	// CancellationPendingSnapshotProvider.
	ParkedSnapshot(sessionID string) (parked bool, revision uint64)
	// ParkedEpoch returns this process's start time in Unix nanoseconds,
	// fixed for the process's life (see "Revision epoch").
	ParkedEpoch() uint64
}

// TaskParkedProvider is ParkedProvider's task-level sibling: the task's own
// OR-aggregate and monotonic counter, never derived from member sessions'
// revisions (see the spec's "Task-level projection").
type TaskParkedProvider interface {
	TaskParkedSnapshot(taskID string) (parked bool, revision uint64)
	ParkedEpoch() uint64
}

// EnrichParkedProjection stamps the session-level parked_on_background_work
// projection onto a full session DTO. The provider is authoritative for both
// true and false values, so a settled projection clears stale client state.
func EnrichParkedProjection(session *TaskSessionDTO, provider ParkedProvider) {
	if session == nil || provider == nil {
		return
	}
	session.ParkedOnBackgroundWork, session.Revision = provider.ParkedSnapshot(session.ID)
	session.ParkedEpoch = provider.ParkedEpoch()
}

// EnrichParkedProjectionSummary is the summary DTO equivalent of
// EnrichParkedProjection.
func EnrichParkedProjectionSummary(session *TaskSessionSummaryDTO, provider ParkedProvider) {
	if session == nil || provider == nil {
		return
	}
	session.ParkedOnBackgroundWork, session.Revision = provider.ParkedSnapshot(session.ID)
	session.ParkedEpoch = provider.ParkedEpoch()
}

// EnrichTaskParkedProjection stamps the task-level OR-aggregate and its own
// monotonic revision onto a full task DTO. Unlike EnrichTaskForegroundActivity
// this does not recompute from a session list — the aggregate and its
// revision are owned and incrementally maintained by the provider itself.
func EnrichTaskParkedProjection(dto *TaskDTO, provider TaskParkedProvider) {
	if dto == nil || provider == nil {
		return
	}
	dto.ParkedOnBackgroundWork, dto.ParkedRevision = provider.TaskParkedSnapshot(dto.ID)
	dto.ParkedEpoch = provider.ParkedEpoch()
}
