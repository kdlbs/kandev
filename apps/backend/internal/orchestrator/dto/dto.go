// Package dto provides Data Transfer Objects for the orchestrator module.
package dto

import "time"

// GetStatusRequest is the request for orchestrator.status
type GetStatusRequest struct{}

// StatusResponse is the response for orchestrator.status
type StatusResponse struct {
	Running       bool `json:"running"`
	ActiveAgents  int  `json:"active_agents"`
	QueuedTasks   int  `json:"queued_tasks"`
	MaxConcurrent int  `json:"max_concurrent"`
}

// GetQueueRequest is the request for orchestrator.queue
type GetQueueRequest struct{}

// QueuedTaskDTO represents a task in the queue
type QueuedTaskDTO struct {
	TaskID   string `json:"task_id"`
	Priority int    `json:"priority"`
	QueuedAt string `json:"queued_at"`
}

// QueueResponse is the response for orchestrator.queue
type QueueResponse struct {
	Tasks []QueuedTaskDTO `json:"tasks"`
	Total int             `json:"total"`
}

// TriggerTaskRequest is the payload for orchestrator.trigger
type TriggerTaskRequest struct {
	TaskID string `json:"task_id"`
}

// TriggerTaskResponse is the response for orchestrator.trigger
type TriggerTaskResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	TaskID  string `json:"task_id"`
}

// StartTaskRequest is the payload for orchestrator.start
type StartTaskRequest struct {
	TaskID    string `json:"task_id"`
	AgentType string `json:"agent_type,omitempty"`
	Priority  int    `json:"priority,omitempty"`
}

// StartTaskResponse is the response for orchestrator.start
type StartTaskResponse struct {
	Success         bool   `json:"success"`
	TaskID          string `json:"task_id"`
	AgentInstanceID string `json:"agent_instance_id"`
	Status          string `json:"status"`
}

// StopTaskRequest is the payload for orchestrator.stop
type StopTaskRequest struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

// SuccessResponse is a generic success response
type SuccessResponse struct {
	Success bool `json:"success"`
}

// PromptTaskRequest is the payload for orchestrator.prompt
type PromptTaskRequest struct {
	TaskID string `json:"task_id"`
	Prompt string `json:"prompt"`
}

// PromptTaskResponse is the response for orchestrator.prompt
type PromptTaskResponse struct {
	Success    bool   `json:"success"`
	StopReason string `json:"stop_reason"`
}

// CompleteTaskRequest is the payload for orchestrator.complete
type CompleteTaskRequest struct {
	TaskID string `json:"task_id"`
}

// CompleteTaskResponse is the response for orchestrator.complete
type CompleteTaskResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetTaskLogsRequest is the payload for task.logs
type GetTaskLogsRequest struct {
	TaskID string `json:"task_id"`
	Limit  int    `json:"limit,omitempty"`
}

// LogEntryDTO represents a single log entry
type LogEntryDTO struct {
	Type      string                 `json:"type"`
	Timestamp string                 `json:"timestamp"`
	AgentID   string                 `json:"agent_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// GetTaskLogsResponse is the response for task.logs
type GetTaskLogsResponse struct {
	TaskID string        `json:"task_id"`
	Logs   []LogEntryDTO `json:"logs"`
	Total  int           `json:"total"`
}

// PermissionRespondRequest is the payload for permission.respond
type PermissionRespondRequest struct {
	TaskID    string `json:"task_id"`
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// PermissionRespondResponse is the response for permission.respond
type PermissionRespondResponse struct {
	Success   bool   `json:"success"`
	TaskID    string `json:"task_id"`
	PendingID string `json:"pending_id"`
}

// FromQueuedTask converts a queue.QueuedTask to QueuedTaskDTO
func FromQueuedTask(qt interface{ GetTaskID() string; GetPriority() int; GetQueuedAt() time.Time }) QueuedTaskDTO {
	return QueuedTaskDTO{
		TaskID:   qt.GetTaskID(),
		Priority: qt.GetPriority(),
		QueuedAt: qt.GetQueuedAt().Format("2006-01-02T15:04:05Z"),
	}
}

