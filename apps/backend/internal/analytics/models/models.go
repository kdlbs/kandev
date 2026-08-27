package models

import "time"

// TaskStats represents aggregated statistics for a single task
type TaskStats struct {
	TaskID           string     `json:"task_id"`
	TaskTitle        string     `json:"task_title"`
	WorkspaceID      string     `json:"workspace_id"`
	WorkflowID       string     `json:"workflow_id"`
	State            string     `json:"state"`
	SessionCount     int        `json:"session_count"`
	TurnCount        int        `json:"turn_count"`
	MessageCount     int        `json:"message_count"`
	UserMessageCount int        `json:"user_message_count"`
	ToolCallCount    int        `json:"tool_call_count"`
	TotalDurationMs  int64      `json:"total_duration_ms"`
	ActiveDurationMs int64      `json:"active_duration_ms"`
	ElapsedSpanMs    int64      `json:"elapsed_span_ms"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// GlobalStats represents workspace-wide aggregated statistics
type GlobalStats struct {
	TotalTasks           int     `json:"total_tasks"`
	CompletedTasks       int     `json:"completed_tasks"`
	InProgressTasks      int     `json:"in_progress_tasks"`
	TotalSessions        int     `json:"total_sessions"`
	TotalTurns           int     `json:"total_turns"`
	TotalMessages        int     `json:"total_messages"`
	TotalUserMessages    int     `json:"total_user_messages"`
	TotalToolCalls       int     `json:"total_tool_calls"`
	TotalDurationMs      int64   `json:"total_duration_ms"`
	AvgTurnsPerTask      float64 `json:"avg_turns_per_task"`
	AvgMessagesPerTask   float64 `json:"avg_messages_per_task"`
	AvgDurationMsPerTask int64   `json:"avg_duration_ms_per_task"`
	AvgTurnDurationMs    int64   `json:"avg_turn_duration_ms"`
	AvgMessagesPerTurn   float64 `json:"avg_messages_per_turn"`
}

// DailyActivity represents activity statistics for a single day
type DailyActivity struct {
	Date         string `json:"date"` // YYYY-MM-DD format
	TurnCount    int    `json:"turn_count"`
	MessageCount int    `json:"message_count"`
	TaskCount    int    `json:"task_count"`
}

// CompletedTaskActivity represents completed task counts for a day
type CompletedTaskActivity struct {
	Date           string `json:"date"` // YYYY-MM-DD format
	CompletedTasks int    `json:"completed_tasks"`
}

// ModelUsage represents usage statistics for a single model.
//
// Usage is keyed by model rather than by agent profile because a profile is
// mutable: it gets renamed and repointed at other models, so its sessions carry
// several different snapshots and no single one of them describes the group.
// A model string never changes meaning, so every row is exactly true and a
// short range's counts are always contained in the longer range around it.
type ModelUsage struct {
	Model           string `json:"model"`
	SessionCount    int    `json:"session_count"`
	TurnCount       int    `json:"turn_count"`
	TotalDurationMs int64  `json:"total_duration_ms"`
}

// RepositoryStats represents aggregated statistics for a repository in a workspace
type RepositoryStats struct {
	RepositoryID      string `json:"repository_id"`
	RepositoryName    string `json:"repository_name"`
	TotalTasks        int    `json:"total_tasks"`
	CompletedTasks    int    `json:"completed_tasks"`
	InProgressTasks   int    `json:"in_progress_tasks"`
	SessionCount      int    `json:"session_count"`
	TurnCount         int    `json:"turn_count"`
	MessageCount      int    `json:"message_count"`
	UserMessageCount  int    `json:"user_message_count"`
	ToolCallCount     int    `json:"tool_call_count"`
	TotalDurationMs   int64  `json:"total_duration_ms"`
	TotalCommits      int    `json:"total_commits"`
	TotalFilesChanged int    `json:"total_files_changed"`
	TotalInsertions   int    `json:"total_insertions"`
	TotalDeletions    int    `json:"total_deletions"`
}

// GitStats represents aggregated git statistics for a workspace
type GitStats struct {
	TotalCommits      int `json:"total_commits"`
	TotalFilesChanged int `json:"total_files_changed"`
	TotalInsertions   int `json:"total_insertions"`
	TotalDeletions    int `json:"total_deletions"`
}

// SessionCodeStats is the per-session lines-of-code aggregation: committed
// sums from task_session_commits, plus the PEAK pending-diff seen across the
// session's task_session_git_snapshots (not the latest snapshot, which is
// usually a clean tree after a commit/merge/archive). Committed and pending
// are reported separately and are never summed together here — a caller that
// wants a single "effective lines" number (the larger of the two, to avoid
// double-counting work that was later committed) computes that itself, the
// same way the source plugin's effectiveLines helper does.
//
// LinesAddedCommitted / LinesDeletedCommitted are nil, not 0, for a session
// that predates commit-capture activation (see commit_capture_activated_at in
// kandev_meta) and has no observed commits: capture wasn't running yet for
// that session, so "zero" cannot be distinguished from "unknown" and must not
// be asserted as a real measurement. A session with at least one observed
// commit, or one that started after activation, always gets a real int64.
type SessionCodeStats struct {
	SessionID               string `json:"session_id"`
	LinesAddedCommitted     *int64 `json:"lines_added_committed"`
	LinesDeletedCommitted   *int64 `json:"lines_deleted_committed"`
	LinesAddedPeakPending   int64  `json:"lines_added_peak_pending"`
	LinesDeletedPeakPending int64  `json:"lines_deleted_peak_pending"`
}

// SessionCodeStatsFilter narrows ListSessionCodeStats to a subset of
// sessions. Each non-empty list field is an OR-list (standard SQL IN
// semantics); the fields themselves are ANDed together. All fields are
// optional; an entirely empty filter matches every session, bounded by
// Limit/Offset.
type SessionCodeStatsFilter struct {
	SessionIDs   []string
	TaskIDs      []string
	WorkspaceIDs []string
	// States filters by task_sessions.state (e.g. "COMPLETED", "RUNNING").
	States []string
	// Limit caps the number of returned rows. <= 0 uses the repository's
	// default page size.
	Limit int
	// Offset skips this many rows (after ORDER BY), for simple pagination.
	Offset int
}
