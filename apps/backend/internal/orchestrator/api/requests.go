// Package api provides REST API handlers for the orchestrator service.
package api

import "time"

// TriggerTaskRequest for manually triggering a task
type TriggerTaskRequest struct {
	TaskID string `json:"task_id" binding:"required"`
	Force  bool   `json:"force"`
}

// StartTaskRequest for starting agent execution
type StartTaskRequest struct {
	AgentType string `json:"agent_type,omitempty"`
	Priority  int    `json:"priority"`
}

// StopTaskRequest for stopping agent execution
type StopTaskRequest struct {
	Reason string `json:"reason,omitempty"`
	Force  bool   `json:"force"`
}

// PromptTaskRequest for sending a follow-up prompt to a running agent
type PromptTaskRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

// PromptTaskResponse for prompt endpoint
type PromptTaskResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// CompleteTaskRequest for explicitly completing a task
type CompleteTaskRequest struct {
	Message string `json:"message,omitempty"`
}

// TaskStatusResponse for task execution status
type TaskStatusResponse struct {
	TaskID              string     `json:"task_id"`
	State               string     `json:"state"`
	AgentInstanceID     string     `json:"agent_instance_id,omitempty"`
	AgentType           string     `json:"agent_type,omitempty"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	Progress            int        `json:"progress"`
	CurrentOperation    string     `json:"current_operation,omitempty"`
	EstimatedCompletion *time.Time `json:"estimated_completion,omitempty"`
}

// QueuedTaskResponse for queue listing
type QueuedTaskResponse struct {
	TaskID         string     `json:"task_id"`
	Priority       int        `json:"priority"`
	QueuedAt       time.Time  `json:"queued_at"`
	EstimatedStart *time.Time `json:"estimated_start,omitempty"`
}

// QueueResponse for queue endpoint
type QueueResponse struct {
	Tasks []QueuedTaskResponse `json:"tasks"`
	Total int                  `json:"total"`
}

// LogEntry for log listing
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// LogsResponse for logs endpoint
type LogsResponse struct {
	Logs  []LogEntry `json:"logs"`
	Total int        `json:"total"`
}

// ArtifactResponse for artifact listing
type ArtifactResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	DownloadURL string    `json:"download_url"`
}

// ArtifactsResponse for artifacts endpoint
type ArtifactsResponse struct {
	Artifacts []ArtifactResponse `json:"artifacts"`
}

