package retention

// StatusClass distinguishes a row eligible for age-based deletion from a row
// a live decision still reads, and from a row whose status this package does
// not recognize at all — a status classification failure, not a bare bool,
// so an unrecognized status can be told apart from a recognized live-state
// one (AC-OFFICE-RUN-HISTORY-RETENTION-001.10).
type StatusClass int

const (
	// StatusUnknown is a status belonging to neither a table's history set
	// nor its live-state set. Treated as live state and warned about.
	StatusUnknown StatusClass = iota
	StatusHistory
	StatusLiveState
)

// RoutineRunHistoryStatuses are the office_routine_runs statuses eligible
// for age-based deletion (AC-OFFICE-RUN-HISTORY-RETENTION-001.1).
var RoutineRunHistoryStatuses = []string{"skipped", "coalesced", "failed", "done", "cancelled"}

// RoutineRunLiveStatuses are the office_routine_runs statuses that are
// never age-pruned, at any age (AC-OFFICE-RUN-HISTORY-RETENTION-001.1).
var RoutineRunLiveStatuses = []string{"received", "task_created"}

// RunHistoryStatuses are the runs statuses eligible for age-based deletion
// (AC-OFFICE-RUN-HISTORY-RETENTION-001.2). cancelled is history: its only
// writer, CancelRunsWhere, moves a row there from queued/claimed and stamps
// finished_at in the same statement.
var RunHistoryStatuses = []string{"finished", "failed", "cancelled"}

// RunLiveStatuses are the runs statuses that are never age-pruned, at any
// age, including a run parked for a future routing retry
// (AC-OFFICE-RUN-HISTORY-RETENTION-001.2).
var RunLiveStatuses = []string{"queued", "claimed"}

// ClassifyRoutineRunStatus classifies an office_routine_runs.status value.
func ClassifyRoutineRunStatus(status string) StatusClass {
	return classify(status, RoutineRunHistoryStatuses, RoutineRunLiveStatuses)
}

// ClassifyRunStatus classifies a runs.status value.
func ClassifyRunStatus(status string) StatusClass {
	return classify(status, RunHistoryStatuses, RunLiveStatuses)
}

func classify(status string, history, live []string) StatusClass {
	for _, s := range history {
		if s == status {
			return StatusHistory
		}
	}
	for _, s := range live {
		if s == status {
			return StatusLiveState
		}
	}
	return StatusUnknown
}
