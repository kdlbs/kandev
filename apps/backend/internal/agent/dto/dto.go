// Package dto provides Data Transfer Objects for the agent module.
package dto

import "time"

// ListAgentsRequest is the request for agent.list
type ListAgentsRequest struct{}

// AgentDTO represents an agent instance
type AgentDTO struct {
	ID             string            `json:"id"`
	TaskID         string            `json:"task_id"`
	AgentProfileID string            `json:"agent_profile_id"`
	ContainerID    string            `json:"container_id,omitempty"`
	Status         string            `json:"status"`
	StartedAt      string            `json:"started_at"`
	FinishedAt     string            `json:"finished_at,omitempty"`
	ExitCode       *int              `json:"exit_code,omitempty"`
	Error          string            `json:"error,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ListAgentsResponse is the response for agent.list
type ListAgentsResponse struct {
	Agents []AgentDTO `json:"agents"`
	Total  int        `json:"total"`
}

// LaunchAgentRequest is the payload for agent.launch
type LaunchAgentRequest struct {
	TaskID         string            `json:"task_id"`
	AgentProfileID string            `json:"agent_profile_id"`
	WorkspacePath  string            `json:"workspace_path"`
	Env            map[string]string `json:"env,omitempty"`
}

// LaunchAgentResponse is the response for agent.launch
type LaunchAgentResponse struct {
	Success bool   `json:"success"`
	AgentID string `json:"agent_id"`
	TaskID  string `json:"task_id"`
}

// GetAgentStatusRequest is the payload for agent.status
type GetAgentStatusRequest struct {
	AgentID string `json:"agent_id"`
}

// GetAgentLogsRequest is the payload for agent.logs
type GetAgentLogsRequest struct {
	AgentID string `json:"agent_id"`
}

// GetAgentLogsResponse is the response for agent.logs
type GetAgentLogsResponse struct {
	AgentID string   `json:"agent_id"`
	Logs    []string `json:"logs"`
	Message string   `json:"message,omitempty"`
}

// StopAgentRequest is the payload for agent.stop
type StopAgentRequest struct {
	AgentID string `json:"agent_id"`
}

// SuccessResponse is a generic success response
type SuccessResponse struct {
	Success bool `json:"success"`
}

// ListAgentTypesRequest is the request for agent.types
type ListAgentTypesRequest struct{}

// AgentTypeDTO represents an agent type
type AgentTypeDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image"`
	Enabled     bool   `json:"enabled"`
}

// ListAgentTypesResponse is the response for agent.types
type ListAgentTypesResponse struct {
	Types []AgentTypeDTO `json:"types"`
	Total int            `json:"total"`
}

// FromAgentExecution converts a lifecycle.AgentExecution to AgentDTO
func FromAgentExecution(execution *AgentExecutionData) AgentDTO {
	agent := AgentDTO{
		ID:             execution.ID,
		TaskID:         execution.TaskID,
		AgentProfileID: execution.AgentProfileID,
		ContainerID:    execution.ContainerID,
		Status:         execution.Status,
		StartedAt:      execution.StartedAt.Format("2006-01-02T15:04:05Z"),
	}
	if execution.FinishedAt != nil && !execution.FinishedAt.IsZero() {
		agent.FinishedAt = execution.FinishedAt.Format("2006-01-02T15:04:05Z")
	}
	if execution.ExitCode != nil {
		agent.ExitCode = execution.ExitCode
	}
	if execution.ErrorMessage != "" {
		agent.Error = execution.ErrorMessage
	}
	return agent
}

// AgentExecutionData is the input data for FromAgentExecution (avoids circular import)
type AgentExecutionData struct {
	ID             string
	TaskID         string
	AgentProfileID string
	ContainerID    string
	Status         string
	StartedAt      time.Time
	FinishedAt     *time.Time
	ExitCode       *int
	ErrorMessage   string
}

// FromAgentType converts a registry.AgentType to AgentTypeDTO
func FromAgentType(t *AgentTypeData) AgentTypeDTO {
	return AgentTypeDTO{
		ID:           t.ID,
		Name:        t.Name,
		Description: t.Description,
		Image:       t.Image,
		Enabled:     t.Enabled,
	}
}

// AgentTypeData is the input data for FromAgentType (avoids circular import)
type AgentTypeData struct {
	ID          string
	Name        string
	Description string
	Image       string
	Enabled     bool
}

