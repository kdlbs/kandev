package dto

// TaskStatsDTO represents aggregated statistics for a single task
type TaskStatsDTO struct {
	TaskID           string  `json:"task_id"`
	TaskTitle        string  `json:"task_title"`
	WorkspaceID      string  `json:"workspace_id"`
	BoardID          string  `json:"board_id"`
	State            string  `json:"state"`
	SessionCount     int     `json:"session_count"`
	TurnCount        int     `json:"turn_count"`
	MessageCount     int     `json:"message_count"`
	UserMessageCount int     `json:"user_message_count"`
	ToolCallCount    int     `json:"tool_call_count"`
	TotalDurationMs  int64   `json:"total_duration_ms"`
	CreatedAt        string  `json:"created_at"`
	CompletedAt      *string `json:"completed_at,omitempty"`
}

// GlobalStatsDTO represents workspace-wide aggregated statistics
type GlobalStatsDTO struct {
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

// DailyActivityDTO represents activity statistics for a single day
type DailyActivityDTO struct {
	Date         string `json:"date"`
	TurnCount    int    `json:"turn_count"`
	MessageCount int    `json:"message_count"`
	TaskCount    int    `json:"task_count"`
}

// CompletedTaskActivityDTO represents completed task counts for a day
type CompletedTaskActivityDTO struct {
	Date           string `json:"date"`
	CompletedTasks int    `json:"completed_tasks"`
}

// AgentUsageDTO represents usage statistics for a single agent profile
type AgentUsageDTO struct {
	AgentProfileID   string `json:"agent_profile_id"`
	AgentProfileName string `json:"agent_profile_name"`
	AgentModel       string `json:"agent_model"`
	SessionCount     int    `json:"session_count"`
	TurnCount        int    `json:"turn_count"`
	TotalDurationMs  int64  `json:"total_duration_ms"`
}

// RepositoryStatsDTO represents usage statistics for a repository
type RepositoryStatsDTO struct {
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

// GitStatsDTO represents aggregated git statistics
type GitStatsDTO struct {
	TotalCommits      int `json:"total_commits"`
	TotalFilesChanged int `json:"total_files_changed"`
	TotalInsertions   int `json:"total_insertions"`
	TotalDeletions    int `json:"total_deletions"`
}

// StatsResponse represents the full stats API response
type StatsResponse struct {
	Global            GlobalStatsDTO             `json:"global"`
	TaskStats         []TaskStatsDTO             `json:"task_stats"`
	DailyActivity     []DailyActivityDTO         `json:"daily_activity"`
	CompletedActivity []CompletedTaskActivityDTO `json:"completed_activity"`
	AgentUsage        []AgentUsageDTO            `json:"agent_usage"`
	RepositoryStats   []RepositoryStatsDTO       `json:"repository_stats"`
	GitStats          GitStatsDTO                `json:"git_stats"`
}
