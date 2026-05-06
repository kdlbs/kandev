package service

import (
	"context"
	"encoding/json"
	"fmt"
)

// ExecutorConfig represents resolved executor configuration.
type ExecutorConfig struct {
	Type             string `json:"type"`
	Image            string `json:"image,omitempty"`
	ResourceLimits   string `json:"resource_limits,omitempty"`
	EnvironmentID    string `json:"environment_id,omitempty"`
	WorktreeStrategy string `json:"worktree_strategy,omitempty"`
}

// IsEmpty returns true if the config has no executor type set.
func (c ExecutorConfig) IsEmpty() bool {
	return c.Type == ""
}

// ResolveExecutor walks the resolution chain to determine executor config.
// Priority: task override > agent preference > project config > workspace default.
func (s *Service) ResolveExecutor(
	ctx context.Context,
	taskExecutionPolicy string,
	agentInstanceID string,
	projectID string,
	workspaceDefaultJSON string,
) (*ExecutorConfig, error) {
	// 1. Task-level override
	if cfg := extractExecutorFromPolicy(taskExecutionPolicy); !cfg.IsEmpty() {
		return &cfg, nil
	}

	// 2. Agent instance preference
	if agentInstanceID != "" {
		cfg, err := s.resolveFromAgent(ctx, agentInstanceID)
		if err != nil {
			return nil, err
		}
		if !cfg.IsEmpty() {
			return &cfg, nil
		}
	}

	// 3. Project executor config
	if projectID != "" {
		cfg, err := s.resolveFromProject(ctx, projectID)
		if err != nil {
			return nil, err
		}
		if !cfg.IsEmpty() {
			return &cfg, nil
		}
	}

	// 4. Workspace default
	if cfg := parseExecutorConfig(workspaceDefaultJSON); !cfg.IsEmpty() {
		return &cfg, nil
	}

	return nil, fmt.Errorf("no executor configuration found")
}

func (s *Service) resolveFromAgent(ctx context.Context, agentInstanceID string) (ExecutorConfig, error) {
	agent, err := s.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return ExecutorConfig{}, fmt.Errorf("get agent: %w", err)
	}
	return parseExecutorConfig(agent.ExecutorPreference), nil
}

func (s *Service) resolveFromProject(ctx context.Context, projectID string) (ExecutorConfig, error) {
	project, err := s.GetProjectFromConfig(ctx, projectID)
	if err != nil {
		return ExecutorConfig{}, fmt.Errorf("get project: %w", err)
	}
	return parseExecutorConfig(project.ExecutorConfig), nil
}

// extractExecutorFromPolicy pulls executor_config from an execution policy JSON.
func extractExecutorFromPolicy(policyJSON string) ExecutorConfig {
	if policyJSON == "" || policyJSON == "{}" {
		return ExecutorConfig{}
	}
	var policy struct {
		ExecutorConfig ExecutorConfig `json:"executor_config"`
	}
	if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
		return ExecutorConfig{}
	}
	return policy.ExecutorConfig
}

// parseExecutorConfig parses an ExecutorConfig from JSON.
func parseExecutorConfig(raw string) ExecutorConfig {
	if raw == "" || raw == "{}" {
		return ExecutorConfig{}
	}
	var cfg ExecutorConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return ExecutorConfig{}
	}
	return cfg
}
