// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// SessionResumer is an interface for resuming task sessions
type SessionResumer interface {
	ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error
}

// ShellHandlers provides WebSocket handlers for shell terminal operations
type ShellHandlers struct {
	lifecycleMgr    *lifecycle.Manager
	logger          *logger.Logger
	hub             ShellOutputBroadcaster
	sessionResumer  SessionResumer

	// Track active shell streams per session
	activeStreams map[string]context.CancelFunc
	mu            sync.RWMutex

	// Input channels for sending input to shell streams
	inputChannels map[string]chan<- agentctl.ShellMessage
	inputMu       sync.RWMutex
}

// NewShellHandlers creates a new ShellHandlers instance
func NewShellHandlers(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *ShellHandlers {
	return &ShellHandlers{
		lifecycleMgr:  lifecycleMgr,
		logger:        log.WithFields(zap.String("component", "shell_handlers")),
		activeStreams: make(map[string]context.CancelFunc),
		inputChannels: make(map[string]chan<- agentctl.ShellMessage),
	}
}

// SetHub sets the hub for broadcasting shell output
func (h *ShellHandlers) SetHub(hub ShellOutputBroadcaster) {
	h.hub = hub
}

// SetSessionResumer sets the session resumer for auto-resuming sessions on shell subscribe
func (h *ShellHandlers) SetSessionResumer(resumer SessionResumer) {
	h.sessionResumer = resumer
}


// RegisterHandlers registers shell handlers with the WebSocket dispatcher
func (h *ShellHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionShellStatus, h.wsShellStatus)
	d.RegisterFunc(ws.ActionShellSubscribe, h.wsShellSubscribe)
	d.RegisterFunc(ws.ActionShellInput, h.wsShellInput)
}

// ShellStatusRequest for shell.status action
type ShellStatusRequest struct {
	SessionID string `json:"session_id"`
}

// ShellInputRequest for shell.input action
type ShellInputRequest struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"`
}

// wsShellStatus returns the status of a shell session for a session
func (h *ShellHandlers) wsShellStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get the agent execution for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "no agent running for this session",
		})
	}

	// Get shell status from agentctl
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "agent client not available",
		})
	}

	status, err := client.ShellStatus(ctx)
	if err != nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     err.Error(),
		})
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"available":  true,
		"running":    status.Running,
		"pid":        status.Pid,
		"shell":      status.Shell,
		"cwd":        status.Cwd,
		"started_at": status.StartedAt,
	})
}

// ShellSubscribeRequest for shell.subscribe action
type ShellSubscribeRequest struct {
	SessionID string `json:"session_id"`
}

// wsShellSubscribe subscribes to shell output for a session and starts the shell stream if needed
func (h *ShellHandlers) wsShellSubscribe(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Verify the agent execution exists for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("no agent running for session %s", req.SessionID)
	}

	// Start the shell stream (idempotent - does nothing if already running)
	if err := h.StartShellStream(ctx, req.SessionID); err != nil {
		return nil, fmt.Errorf("failed to start shell stream: %w", err)
	}

	// Get buffered output to include in response
	// This ensures client gets current shell state without duplicate broadcasts
	buffer := ""
	if client := execution.GetAgentCtlClient(); client != nil {
		if b, err := client.ShellBuffer(ctx); err == nil {
			buffer = b
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
		"buffer":     buffer,
	})
}

// wsShellInput sends input to a shell session
func (h *ShellHandlers) wsShellInput(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellInputRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Verify the agent execution exists for this session
	if _, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID); !ok {
		return nil, fmt.Errorf("no agent running for session %s", req.SessionID)
	}

	// Send input via the shell stream
	if err := h.SendShellInput(req.SessionID, req.Data); err != nil {
		return nil, err
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}

// StartShellStream starts streaming shell output for a session using the configured hub.
// This implements the lifecycle.ShellStreamStarter interface.
func (h *ShellHandlers) StartShellStream(ctx context.Context, sessionID string) error {
	if h.hub == nil {
		return fmt.Errorf("hub not configured")
	}
	return h.StartShellStreamWithHub(ctx, sessionID, h.hub)
}

// StartShellStreamWithHub starts streaming shell output for a session to a specific hub
func (h *ShellHandlers) StartShellStreamWithHub(ctx context.Context, sessionID string, hub ShellOutputBroadcaster) error {
	h.mu.Lock()
	if _, exists := h.activeStreams[sessionID]; exists {
		h.mu.Unlock()
		return nil // Already streaming
	}

	streamCtx, cancel := context.WithCancel(context.Background()) // Use background context so stream survives request
	h.activeStreams[sessionID] = cancel
	h.mu.Unlock()

	// Use a channel to wait for stream setup to complete
	readyCh := make(chan error, 1)
	go h.runShellStreamWithReady(streamCtx, sessionID, hub, readyCh)

	// Wait for stream to be ready (or fail)
	select {
	case err := <-readyCh:
		return err
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

// StopShellStream stops the shell stream for a session
func (h *ShellHandlers) StopShellStream(sessionID string) {
	h.mu.Lock()
	if cancel, exists := h.activeStreams[sessionID]; exists {
		cancel()
		delete(h.activeStreams, sessionID)
	}
	h.mu.Unlock()
}

// ShellOutputBroadcaster interface for broadcasting shell output
type ShellOutputBroadcaster interface {
	BroadcastToSession(sessionID string, msg *ws.Message)
}

// runShellStreamWithReady runs the shell output stream for a session and signals when ready
func (h *ShellHandlers) runShellStreamWithReady(ctx context.Context, sessionID string, hub ShellOutputBroadcaster, readyCh chan<- error) {
	defer func() {
		h.mu.Lock()
		delete(h.activeStreams, sessionID)
		h.mu.Unlock()
	}()

	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		h.logger.Debug("no agent execution for shell stream", zap.String("session_id", sessionID))
		readyCh <- fmt.Errorf("no agent execution for session %s", sessionID)
		return
	}

	client := execution.GetAgentCtlClient()
	if client == nil {
		h.logger.Debug("no client for shell stream", zap.String("session_id", sessionID))
		readyCh <- fmt.Errorf("no agent client for session %s", sessionID)
		return
	}

	if err := client.WaitForReady(ctx, 10*time.Second); err != nil {
		h.logger.Error("agentctl not ready for shell stream", zap.String("session_id", sessionID), zap.Error(err))
		readyCh <- fmt.Errorf("agentctl not ready: %w", err)
		return
	}

	outputCh, inputCh, err := client.StreamShell(ctx)
	if err != nil {
		h.logger.Error("failed to start shell stream", zap.String("session_id", sessionID), zap.Error(err))
		readyCh <- fmt.Errorf("failed to start shell stream: %w", err)
		return
	}

	// Store input channel for sending input
	h.storeInputChannel(sessionID, inputCh)
	defer h.removeInputChannel(sessionID)

	h.logger.Info("shell stream started", zap.String("session_id", sessionID))

	// Signal that we're ready - input channel is now stored
	readyCh <- nil

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-outputCh:
			if !ok {
				return
			}
			// Broadcast shell output to session subscribers
			notification, err := ws.NewNotification(ws.ActionShellOutput, map[string]interface{}{
				"session_id": sessionID,
				"type":       msg.Type,
				"data":       msg.Data,
				"code":       msg.Code,
			})
			if err != nil {
				h.logger.Debug("failed to create shell output notification", zap.Error(err))
				continue
			}
			hub.BroadcastToSession(sessionID, notification)
		}
	}
}


func (h *ShellHandlers) storeInputChannel(sessionID string, ch chan<- agentctl.ShellMessage) {
	h.inputMu.Lock()
	h.inputChannels[sessionID] = ch
	h.inputMu.Unlock()
}

func (h *ShellHandlers) removeInputChannel(sessionID string) {
	h.inputMu.Lock()
	if ch, exists := h.inputChannels[sessionID]; exists {
		close(ch)
		delete(h.inputChannels, sessionID)
	}
	h.inputMu.Unlock()
}

// SendShellInput sends input to an active shell stream
func (h *ShellHandlers) SendShellInput(sessionID, data string) error {
	h.inputMu.RLock()
	ch, exists := h.inputChannels[sessionID]
	h.inputMu.RUnlock()

	if !exists {
		return fmt.Errorf("no active shell stream for session %s", sessionID)
	}

	select {
	case ch <- agentctl.ShellMessage{Type: "input", Data: data}:
		return nil
	default:
		return fmt.Errorf("shell input channel full")
	}
}
