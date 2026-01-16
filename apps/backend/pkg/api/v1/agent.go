package v1

import "time"

// AgentStatus represents the status of an agent execution
type AgentStatus string

const (
	AgentStatusPending   AgentStatus = "PENDING"
	AgentStatusStarting  AgentStatus = "STARTING"
	AgentStatusRunning   AgentStatus = "RUNNING" // Agent is processing a prompt
	AgentStatusReady     AgentStatus = "READY"   // Agent finished prompt, waiting for next
	AgentStatusCompleted AgentStatus = "COMPLETED"
	AgentStatusFailed    AgentStatus = "FAILED"
	AgentStatusStopped   AgentStatus = "STOPPED"
)

// ResourceLimits defines container resource limits
type ResourceLimits struct {
	CPULimit    string `json:"cpu_limit"`
	MemoryLimit string `json:"memory_limit"`
	DiskLimit   string `json:"disk_limit"`
}

// AgentExecution represents a running or completed agent container
type AgentExecution struct {
	ID             string         `json:"id"`
	TaskID         string         `json:"task_id"`
	AgentProfileID string         `json:"agent_profile_id"`
	ContainerID    *string        `json:"container_id,omitempty"`
	ContainerName  *string        `json:"container_name,omitempty"`
	Status         AgentStatus    `json:"status"`
	ImageName      string         `json:"image_name"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	StoppedAt      *time.Time     `json:"stopped_at,omitempty"`
	ExitCode       *int           `json:"exit_code,omitempty"`
	ErrorMessage   *string        `json:"error_message,omitempty"`
	ResourceLimits ResourceLimits `json:"resource_limits"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// AgentType represents a registered agent type
type AgentType struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	DockerImage      string            `json:"docker_image"`
	DockerTag        string            `json:"docker_tag"`
	DefaultResources ResourceLimits    `json:"default_resources"`
	EnvironmentVars  map[string]string `json:"environment_vars,omitempty"`
	Capabilities     []string          `json:"capabilities,omitempty"`
	Enabled          bool              `json:"enabled"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// AgentLog represents a log entry from an agent
type AgentLog struct {
	ID               int64                  `json:"id"`
	AgentExecutionID string                 `json:"agent_execution_id"`
	LogLevel        string                 `json:"log_level"`
	Message         string                 `json:"message"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}
