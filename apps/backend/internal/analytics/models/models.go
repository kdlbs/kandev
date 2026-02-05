package models

import "time"

// TaskStats represents aggregated statistics for a single task
type TaskStats struct {
	TaskID           string     `json:"task_id"`
	TaskTitle        string     `json:"task_title"`
	WorkspaceID      string     `json:"workspace_id"`
	BoardID          string     `json:"board_id"`
	State            string     `json:"state"`
	SessionCount     int        `json:"session_count"`
	TurnCount        int        `json:"turn_count"`
	MessageCount     int        `json:"message_count"`
	UserMessageCount int        `json:"user_message_count"`
	ToolCallCount    int        `json:"tool_call_count"`
	TotalDurationMs  int64      `json:"total_duration_ms"`
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

// AgentUsage represents usage statistics for a single agent profile
type AgentUsage struct {
	AgentProfileID   string `json:"agent_profile_id"`
	AgentProfileName string `json:"agent_profile_name"`
	AgentModel       string `json:"agent_model"`
	SessionCount     int    `json:"session_count"`
	TurnCount        int    `json:"turn_count"`
	TotalDurationMs  int64  `json:"total_duration_ms"`
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
