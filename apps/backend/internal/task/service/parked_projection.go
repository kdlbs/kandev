package service

// TaskParkedProvider surfaces the orchestrator's task-level
// parked_on_background_work OR-aggregate, its own monotonic revision counter,
// and the process epoch (spec: docs/specs/disambiguate-waiting/spec.md).
// Duplicated from internal/task/dto's identical interface rather than
// imported: dto depends on this package, so this package cannot depend on
// dto without a cycle (see ForegroundActivityProvider for precedent).
type TaskParkedProvider interface {
	// TaskParkedSnapshot returns a task's OR-aggregate across its sessions and
	// the task's own transition revision — never derived from member
	// sessions' revisions (see the spec's "Task-level projection").
	TaskParkedSnapshot(taskID string) (parked bool, revision uint64)
	// ParkedEpoch returns this process's start time in Unix nanoseconds,
	// fixed for the process's life (see "Revision epoch").
	ParkedEpoch() uint64
}

// SetTaskParkedProvider wires the task-level parked projection source used to
// stamp parked_on_background_work / parked_revision / parked_epoch onto
// task.updated events. Optional; when unset those fields are omitted from the
// event payload (clients apply D9's false/0/0 defaults).
func (s *Service) SetTaskParkedProvider(provider TaskParkedProvider) {
	s.taskParkedProvider = provider
}
