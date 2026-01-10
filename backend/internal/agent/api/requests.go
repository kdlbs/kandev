// Package api provides HTTP handlers for the agent manager API.
package api

import "time"

// LaunchAgentRequest for launching an agent
type LaunchAgentRequest struct {
	TaskID        string                 `json:"task_id" binding:"required"`
	AgentType     string                 `json:"agent_type" binding:"required"`
	WorkspacePath string                 `json:"workspace_path" binding:"required"`
	Env           map[string]string      `json:"env,omitempty"`
	Priority      int                    `json:"priority"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// StopAgentRequest for stopping an agent
type StopAgentRequest struct {
	Force  bool   `json:"force"`
	Reason string `json:"reason,omitempty"`
}

// AgentInstanceResponse for agent status
type AgentInstanceResponse struct {
	ID           string                 `json:"id"`
	TaskID       string                 `json:"task_id"`
	AgentType    string                 `json:"agent_type"`
	ContainerID  string                 `json:"container_id"`
	Status       string                 `json:"status"`
	Progress     int                    `json:"progress"`
	StartedAt    time.Time              `json:"started_at"`
	FinishedAt   *time.Time             `json:"finished_at,omitempty"`
	ExitCode     *int                   `json:"exit_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// AgentTypeResponse for agent type listing
type AgentTypeResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Image        string   `json:"image"`
	Capabilities []string `json:"capabilities"`
	Enabled      bool     `json:"enabled"`
}

// AgentsListResponse for listing agents
type AgentsListResponse struct {
	Agents []AgentInstanceResponse `json:"agents"`
	Total  int                     `json:"total"`
}

// AgentTypesListResponse for listing agent types
type AgentTypesListResponse struct {
	Types []AgentTypeResponse `json:"types"`
	Total int                 `json:"total"`
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Stream    string    `json:"stream"` // stdout or stderr
}

// LogsResponse for agent logs
type LogsResponse struct {
	Logs  []LogEntry `json:"logs"`
	Total int        `json:"total"`
}

// HealthResponse for health check
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

