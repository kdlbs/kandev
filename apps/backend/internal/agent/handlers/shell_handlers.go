// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"fmt"
	"sync"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agent/lifecycle"
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

	// Track active shell streams per task
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
	TaskID string `json:"task_id"`
}

// ShellInputRequest for shell.input action
type ShellInputRequest struct {
	TaskID string `json:"task_id"`
	Data   string `json:"data"`
}

// wsShellStatus returns the status of a shell session for a task
func (h *ShellHandlers) wsShellStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	// Get the agent instance for this task
	instance, ok := h.lifecycleMgr.GetInstanceByTaskID(req.TaskID)
	if !ok {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "no agent running for this task",
		})
	}

	// Get shell status from agentctl
	client := instance.GetAgentCtlClient()
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
	TaskID string `json:"task_id"`
}

// wsShellSubscribe subscribes to shell output for a task and starts the shell stream if needed
func (h *ShellHandlers) wsShellSubscribe(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	// Verify the agent instance exists for this task
	instance, ok := h.lifecycleMgr.GetInstanceByTaskID(req.TaskID)
	if !ok {
		return nil, fmt.Errorf("no agent running for task %s", req.TaskID)
	}

	// Start the shell stream (idempotent - does nothing if already running)
	if err := h.StartShellStream(ctx, req.TaskID); err != nil {
		return nil, fmt.Errorf("failed to start shell stream: %w", err)
	}

	// Get buffered output to include in response
	// This ensures client gets current shell state without duplicate broadcasts
	buffer := ""
	if client := instance.GetAgentCtlClient(); client != nil {
		if b, err := client.ShellBuffer(ctx); err == nil {
			buffer = b
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
		"buffer":  buffer,
	})
}

// wsShellInput sends input to a shell session
func (h *ShellHandlers) wsShellInput(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellInputRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	// Verify the agent instance exists for this task
	if _, ok := h.lifecycleMgr.GetInstanceByTaskID(req.TaskID); !ok {
		return nil, fmt.Errorf("no agent running for task %s", req.TaskID)
	}

	// Send input via the shell stream
	if err := h.SendShellInput(req.TaskID, req.Data); err != nil {
		return nil, err
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}

// StartShellStream starts streaming shell output for a task using the configured hub.
// This implements the lifecycle.ShellStreamStarter interface.
func (h *ShellHandlers) StartShellStream(ctx context.Context, taskID string) error {
	if h.hub == nil {
		return fmt.Errorf("hub not configured")
	}
	return h.StartShellStreamWithHub(ctx, taskID, h.hub)
}

// StartShellStreamWithHub starts streaming shell output for a task to a specific hub
func (h *ShellHandlers) StartShellStreamWithHub(ctx context.Context, taskID string, hub ShellOutputBroadcaster) error {
	h.mu.Lock()
	if _, exists := h.activeStreams[taskID]; exists {
		h.mu.Unlock()
		return nil // Already streaming
	}

	streamCtx, cancel := context.WithCancel(context.Background()) // Use background context so stream survives request
	h.activeStreams[taskID] = cancel
	h.mu.Unlock()

	// Use a channel to wait for stream setup to complete
	readyCh := make(chan error, 1)
	go h.runShellStreamWithReady(streamCtx, taskID, hub, readyCh)

	// Wait for stream to be ready (or fail)
	select {
	case err := <-readyCh:
		return err
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

// StopShellStream stops the shell stream for a task
func (h *ShellHandlers) StopShellStream(taskID string) {
	h.mu.Lock()
	if cancel, exists := h.activeStreams[taskID]; exists {
		cancel()
		delete(h.activeStreams, taskID)
	}
	h.mu.Unlock()
}

// ShellOutputBroadcaster interface for broadcasting shell output
type ShellOutputBroadcaster interface {
	BroadcastToTask(taskID string, msg *ws.Message)
}

// runShellStreamWithReady runs the shell output stream for a task and signals when ready
func (h *ShellHandlers) runShellStreamWithReady(ctx context.Context, taskID string, hub ShellOutputBroadcaster, readyCh chan<- error) {
	defer func() {
		h.mu.Lock()
		delete(h.activeStreams, taskID)
		h.mu.Unlock()
	}()

	instance, ok := h.lifecycleMgr.GetInstanceByTaskID(taskID)
	if !ok {
		h.logger.Debug("no agent instance for shell stream", zap.String("task_id", taskID))
		readyCh <- fmt.Errorf("no agent instance for task %s", taskID)
		return
	}

	client := instance.GetAgentCtlClient()
	if client == nil {
		h.logger.Debug("no client for shell stream", zap.String("task_id", taskID))
		readyCh <- fmt.Errorf("no agent client for task %s", taskID)
		return
	}

	outputCh, inputCh, err := client.StreamShell(ctx)
	if err != nil {
		h.logger.Error("failed to start shell stream", zap.String("task_id", taskID), zap.Error(err))
		readyCh <- fmt.Errorf("failed to start shell stream: %w", err)
		return
	}

	// Store input channel for sending input
	h.storeInputChannel(taskID, inputCh)
	defer h.removeInputChannel(taskID)

	h.logger.Info("shell stream started", zap.String("task_id", taskID))

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
			// Broadcast shell output to task subscribers
			notification, err := ws.NewNotification(ws.ActionShellOutput, map[string]interface{}{
				"task_id": taskID,
				"type":    msg.Type,
				"data":    msg.Data,
				"code":    msg.Code,
			})
			if err != nil {
				h.logger.Debug("failed to create shell output notification", zap.Error(err))
				continue
			}
			hub.BroadcastToTask(taskID, notification)
		}
	}
}

func (h *ShellHandlers) storeInputChannel(taskID string, ch chan<- agentctl.ShellMessage) {
	h.inputMu.Lock()
	h.inputChannels[taskID] = ch
	h.inputMu.Unlock()
}

func (h *ShellHandlers) removeInputChannel(taskID string) {
	h.inputMu.Lock()
	if ch, exists := h.inputChannels[taskID]; exists {
		close(ch)
		delete(h.inputChannels, taskID)
	}
	h.inputMu.Unlock()
}

// SendShellInput sends input to an active shell stream
func (h *ShellHandlers) SendShellInput(taskID, data string) error {
	h.inputMu.RLock()
	ch, exists := h.inputChannels[taskID]
	h.inputMu.RUnlock()

	if !exists {
		return fmt.Errorf("no active shell stream for task %s", taskID)
	}

	select {
	case ch <- agentctl.ShellMessage{Type: "input", Data: data}:
		return nil
	default:
		return fmt.Errorf("shell input channel full")
	}
}

