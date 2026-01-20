// Package lifecycle provides agent runtime abstractions.
package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Runtime abstracts the agent execution environment (Docker, Standalone, K8s, SSH, etc.)
// Each runtime is responsible for creating and managing agentctl instances.
// Agent subprocess launching is handled separately via agentctl client methods.
type Runtime interface {
	// Name returns the runtime identifier (e.g., "docker", "standalone", "k8s")
	Name() runtime.Name

	// HealthCheck verifies the runtime is available and operational
	HealthCheck(ctx context.Context) error

	// CreateInstance creates a new agentctl instance for a task.
	// This starts the agentctl process/container with workspace access (shell, git, files).
	// Agent subprocess is NOT started - use agentctl.Client.ConfigureAgent() + Start().
	CreateInstance(ctx context.Context, req *RuntimeCreateRequest) (*RuntimeInstance, error)

	// StopInstance stops an agentctl instance.
	StopInstance(ctx context.Context, instance *RuntimeInstance, force bool) error

	// RecoverInstances discovers and recovers instances that were running before a restart.
	// Returns recovered instances that can be re-tracked by the manager.
	RecoverInstances(ctx context.Context) ([]*RuntimeInstance, error)
}

// RuntimeCreateRequest contains parameters for creating an agentctl instance.
type RuntimeCreateRequest struct {
	InstanceID     string
	TaskID         string
	SessionID      string
	AgentProfileID string
	WorkspacePath  string
	Protocol       string
	Env            map[string]string
	Metadata       map[string]interface{}
	// Docker-specific
	AgentConfig    *registry.AgentTypeConfig
	MainRepoGitDir string
	WorktreeID     string
	WorktreeBranch string
}

// RuntimeInstance represents an agentctl instance created by a runtime.
// This is returned by the runtime and contains enough info to build an AgentExecution.
type RuntimeInstance struct {
	// Core identifiers
	InstanceID string
	TaskID     string
	SessionID  string

	// Agentctl client for communicating with this instance
	Client *agentctl.Client

	// Runtime-specific identifiers (only one set is populated)
	ContainerID          string // Docker
	ContainerIP          string // Docker
	StandaloneInstanceID string // Standalone
	StandalonePort       int    // Standalone

	// Common fields
	WorkspacePath string
	Metadata      map[string]interface{}
}

// ToAgentExecution converts a RuntimeInstance to an AgentExecution.
func (ri *RuntimeInstance) ToAgentExecution(req *RuntimeCreateRequest) *AgentExecution {
	metadata := req.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	// Merge runtime metadata
	for k, v := range ri.Metadata {
		metadata[k] = v
	}

	workspacePath := ri.WorkspacePath
	if workspacePath == "" {
		workspacePath = req.WorkspacePath
	}

	return &AgentExecution{
		ID:                   ri.InstanceID,
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		AgentProfileID:       req.AgentProfileID,
		ContainerID:          ri.ContainerID,
		ContainerIP:          ri.ContainerIP,
		WorkspacePath:        workspacePath,
		Status:               v1.AgentStatusRunning,
		StartedAt:            time.Now(),
		Progress:             0,
		Metadata:             metadata,
		agentctl:             ri.Client,
		standaloneInstanceID: ri.StandaloneInstanceID,
		standalonePort:       ri.StandalonePort,
	}
}
