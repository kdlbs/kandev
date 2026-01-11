// Package agentctl provides a client for communicating with agentctl running inside containers
package agentctl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Client communicates with agentctl via HTTP
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *logger.Logger

	// WebSocket connections for streaming
	outputConn     *websocket.Conn
	acpConn        *websocket.Conn
	permissionConn *websocket.Conn
	mu             sync.RWMutex
}

// StatusResponse from agentctl
type StatusResponse struct {
	AgentStatus string                 `json:"agent_status"`
	ProcessInfo map[string]interface{} `json:"process_info"`
}

// OutputLine from agentctl
type OutputLine struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Content   string    `json:"content"`
}

// NewClient creates a new agentctl client
func NewClient(host string, port int, log *logger.Logger) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log.WithFields(zap.String("component", "agentctl-client")),
	}
}

// Health checks if agentctl is healthy
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}
	return nil
}

// GetStatus returns the agent status
func (c *Client) GetStatus(ctx context.Context) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

// Start starts the agent process
func (c *Client) Start(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/start", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("start failed: %s", result.Error)
	}
	return nil
}

// Stop stops the agent process
func (c *Client) Stop(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/stop", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("stop failed: %s", result.Error)
	}
	return nil
}

// WaitForReady waits until agentctl is ready to accept requests
func (c *Client) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for agentctl to be ready")
			}

			if err := c.Health(ctx); err == nil {
				c.logger.Info("agentctl is ready")
				return nil
			}
		}
	}
}

// BaseURL returns the base URL of the agentctl client
func (c *Client) BaseURL() string {
	return c.baseURL
}
