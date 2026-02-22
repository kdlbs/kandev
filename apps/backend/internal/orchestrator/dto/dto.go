// Package dto provides Data Transfer Objects for the orchestrator module.
package dto

// GetStatusRequest is the request for orchestrator.status
type GetStatusRequest struct{}

// StatusResponse is the response for orchestrator.status
type StatusResponse struct {
	Running      bool `json:"running"`
	ActiveAgents int  `json:"active_agents"`
	QueuedTasks  int  `json:"queued_tasks"`
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
// If SessionID is provided, the existing session is resumed and prompted with workflow step config.
// If SessionID is not provided, a new session is created (requires AgentProfileID).
type StartTaskRequest struct {
	TaskID         string `json:"task_id"`
	SessionID      string `json:"session_id,omitempty"`       // If provided, resume this existing session instead of creating new
	AgentProfileID string `json:"agent_profile_id,omitempty"` // Required when creating new session (SessionID not provided)
	ExecutorID     string `json:"executor_id,omitempty"`      // Executor to use (determines runtime: local, worktree, local_docker, etc.)
	Priority       int    `json:"priority,omitempty"`
	Prompt         string `json:"prompt,omitempty"`           // Initial prompt to send to the agent
	WorkflowStepID string `json:"workflow_step_id,omitempty"` // If provided, prompt prefix/suffix and plan mode are applied from the step
}

// StartTaskResponse is the response for orchestrator.start
type StartTaskResponse struct {
	Success          bool    `json:"success"`
	TaskID           string  `json:"task_id"`
	AgentExecutionID string  `json:"agent_execution_id"`
	TaskSessionID    string  `json:"session_id,omitempty"`
	State            string  `json:"state"`
	WorktreePath     *string `json:"worktree_path,omitempty"`
	WorktreeBranch   *string `json:"worktree_branch,omitempty"`
}

// ResumeTaskSessionRequest is the payload for task.session.resume
type ResumeTaskSessionRequest struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
}

// ResumeTaskSessionResponse is the response for task.session.resume
type ResumeTaskSessionResponse struct {
	Success          bool    `json:"success"`
	TaskID           string  `json:"task_id"`
	AgentExecutionID string  `json:"agent_execution_id"`
	TaskSessionID    string  `json:"session_id,omitempty"`
	State            string  `json:"state"`
	WorktreePath     *string `json:"worktree_path,omitempty"`
	WorktreeBranch   *string `json:"worktree_branch,omitempty"`
}

// TaskSessionStatusRequest is the payload for task.session.status
type TaskSessionStatusRequest struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
}

// TaskSessionStatusResponse is the response for task.session.status
type TaskSessionStatusResponse struct {
	// Session metadata
	SessionID      string `json:"session_id"`
	TaskID         string `json:"task_id"`
	State          string `json:"state"`
	AgentProfileID string `json:"agent_profile_id,omitempty"`

	// Runtime status
	IsAgentRunning bool   `json:"is_agent_running"`        // Agent process is currently running
	IsResumable    bool   `json:"is_resumable"`            // Session can be resumed
	NeedsResume    bool   `json:"needs_resume"`            // Session needs resumption (page reload scenario)
	ResumeReason   string `json:"resume_reason,omitempty"` // Why resume is needed (e.g., "agent_not_running")

	// ACP session info
	ACPSessionID string `json:"acp_session_id,omitempty"`

	// Executor/runtime info
	ExecutorID       string `json:"executor_id,omitempty"`
	ExecutorType     string `json:"executor_type,omitempty"`
	ExecutorName     string `json:"executor_name,omitempty"`
	Runtime          string `json:"runtime,omitempty"`
	IsRemoteExecutor bool   `json:"is_remote_executor"`
	RemoteState      string `json:"remote_state,omitempty"`
	RemoteName       string `json:"remote_name,omitempty"`
	RemoteCreatedAt  string `json:"remote_created_at,omitempty"`
	RemoteCheckedAt  string `json:"remote_checked_at,omitempty"`
	RemoteStatusErr  string `json:"remote_status_error,omitempty"`

	// Worktree info
	WorktreePath   *string `json:"worktree_path,omitempty"`
	WorktreeBranch *string `json:"worktree_branch,omitempty"`

	// Error info
	Error string `json:"error,omitempty"`
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
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
	Prompt        string `json:"prompt"`
	Model         string `json:"model,omitempty"`     // Optional: switch to this model before processing prompt
	PlanMode      bool   `json:"plan_mode,omitempty"` // Optional: enable plan mode (analyze before executing)
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

// PermissionRespondRequest is the payload for permission.respond
type PermissionRespondRequest struct {
	SessionID string `json:"session_id"`
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// PermissionRespondResponse is the response for permission.respond
type PermissionRespondResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id"`
	PendingID string `json:"pending_id"`
}

// CancelAgentRequest is the payload for the agent.cancel WebSocket action.
type CancelAgentRequest struct {
	SessionID string `json:"session_id"`
}

// CancelAgentResponse is the response for the agent.cancel WebSocket action.
type CancelAgentResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id"`
}
