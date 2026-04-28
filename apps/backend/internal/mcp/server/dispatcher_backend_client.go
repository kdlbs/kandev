package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Dispatcher is the subset of *ws.Dispatcher used by DispatcherBackendClient.
// Defined as an interface so tests can supply lightweight fakes.
type Dispatcher interface {
	Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error)
}

// DispatcherBackendClient implements BackendClient by calling a ws.Dispatcher
// in-process — no channels, no WebSocket round-trip. Used by the backend's
// external MCP endpoint where handlers and MCP server live in the same process.
type DispatcherBackendClient struct {
	dispatcher Dispatcher
	logger     *logger.Logger
}

// NewDispatcherBackendClient creates a BackendClient backed by a ws.Dispatcher.
func NewDispatcherBackendClient(d Dispatcher, log *logger.Logger) *DispatcherBackendClient {
	clientLogger := logger.Default()
	if log != nil {
		clientLogger = log
	}
	return &DispatcherBackendClient{
		dispatcher: d,
		logger:     clientLogger.WithFields(zap.String("component", "mcp-dispatcher-backend-client")),
	}
}

// RequestPayload dispatches the request to the in-process handler and unmarshals the response.
func (c *DispatcherBackendClient) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	id := uuid.New().String()
	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.dispatcher.Dispatch(ctx, msg)
	if err != nil {
		return fmt.Errorf("dispatch failed for %s: %w", action, err)
	}
	if resp == nil {
		return fmt.Errorf("dispatcher returned nil response for %s", action)
	}
	if resp.Type == ws.MessageTypeError {
		var ep struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(resp.Payload, &ep) == nil {
			return fmt.Errorf("backend error [%s]: %s", ep.Code, ep.Message)
		}
		return fmt.Errorf("backend error: %s", string(resp.Payload))
	}
	if result != nil && len(resp.Payload) > 0 {
		if err := json.Unmarshal(resp.Payload, result); err != nil {
			return fmt.Errorf("failed to unmarshal response for %s: %w", action, err)
		}
	}
	return nil
}
