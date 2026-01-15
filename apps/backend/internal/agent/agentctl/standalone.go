// Package agentctl provides a client for communicating with agentctl.
// This file contains the StandaloneCtl client for the multi-instance control API.
package agentctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// StandaloneCtl is a client for the agentctl control API in standalone mode.
// It manages creation and deletion of agent instances.
type StandaloneCtl struct {
	baseURL    string
	httpClient *http.Client
	logger     *logger.Logger
}

// CreateInstanceRequest contains the parameters for creating a new agent instance.
type CreateInstanceRequest struct {
	ID            string            `json:"id,omitempty"`
	WorkspacePath string            `json:"workspace_path"`
	AgentCommand  string            `json:"agent_command,omitempty"`
	Protocol      string            `json:"protocol,omitempty"`       // Protocol adapter to use (acp, rest, mcp)
	WorkspaceFlag string            `json:"workspace_flag,omitempty"` // CLI flag for workspace path (e.g., "--workspace-root")
	Env           map[string]string `json:"env,omitempty"`
	AutoStart     bool              `json:"auto_start,omitempty"`
}

// CreateInstanceResponse contains the result of creating a new agent instance.
type CreateInstanceResponse struct {
	ID   string `json:"id"`
	Port int    `json:"port"`
}

// InstanceInfo contains information about an agent instance.
type InstanceInfo struct {
	ID            string            `json:"id"`
	Port          int               `json:"port"`
	Status        string            `json:"status"`
	WorkspacePath string            `json:"workspace_path"`
	AgentCommand  string            `json:"agent_command"`
	Env           map[string]string `json:"env,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

// NewStandaloneCtl creates a new StandaloneCtl client.
func NewStandaloneCtl(host string, port int, log *logger.Logger) *StandaloneCtl {
	return &StandaloneCtl{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log.WithFields(zap.String("component", "standalone-agentctl")),
	}
}

// Health checks if the standalone agentctl is healthy.
func (s *StandaloneCtl) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}
	return nil
}

// CreateInstance creates a new agent instance.
func (s *StandaloneCtl) CreateInstance(ctx context.Context, req *CreateInstanceRequest) (*CreateInstanceResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/v1/instances", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %w", err)
		}
		return nil, fmt.Errorf("failed to create instance: %s (status %d)", errResp.Error, resp.StatusCode)
	}

	var result CreateInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	s.logger.Info("created agent instance",
		zap.String("instance_id", result.ID),
		zap.Int("port", result.Port))

	return &result, nil
}

// DeleteInstance stops and removes an agent instance.
func (s *StandaloneCtl) DeleteInstance(ctx context.Context, instanceID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", s.baseURL+"/api/v1/instances/"+instanceID, nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return fmt.Errorf("failed to decode error response: %w", err)
		}
		return fmt.Errorf("failed to delete instance: %s (status %d)", errResp.Error, resp.StatusCode)
	}

	s.logger.Info("deleted agent instance", zap.String("instance_id", instanceID))
	return nil
}

// GetInstance gets information about a specific instance.
func (s *StandaloneCtl) GetInstance(ctx context.Context, instanceID string) (*InstanceInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/api/v1/instances/"+instanceID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("instance %q not found", instanceID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get instance: status %d", resp.StatusCode)
	}

	var info InstanceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &info, nil
}

// ListInstances lists all running agent instances.
func (s *StandaloneCtl) ListInstances(ctx context.Context) ([]*InstanceInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/api/v1/instances", nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list instances: status %d", resp.StatusCode)
	}

	var result struct {
		Instances []*InstanceInfo `json:"instances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result.Instances, nil
}
