package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/codex"
	"go.uber.org/zap"
)

// CodexAdapter implements AgentAdapter for agents using the OpenAI Codex app-server protocol.
// Codex uses a JSON-RPC 2.0 variant over stdio (omitting the jsonrpc field).
type CodexAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Subprocess stdin/stdout (managed externally)
	stdin  io.Writer
	stdout io.Reader

	// Codex client for JSON-RPC communication
	client *codex.Client

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Session state - Thread maps to Session, Turn maps to Operation
	threadID string // session ID
	turnID   string // operation ID

	// Agent info (populated after Initialize)
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan SessionUpdate

	// Permission handler
	permissionHandler PermissionHandler

	// Accumulators for streaming content
	messageBuffer   string
	reasoningBuffer string
	summaryBuffer   string

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// NewCodexAdapter creates a new Codex protocol adapter.
// stdin and stdout are the subprocess's stdin/stdout pipes (managed by process.Manager).
func NewCodexAdapter(stdin io.Writer, stdout io.Reader, cfg *Config, log *logger.Logger) *CodexAdapter {
	ctx, cancel := context.WithCancel(context.Background())
	return &CodexAdapter{
		cfg:       cfg,
		logger:    log.WithFields(zap.String("adapter", "codex")),
		stdin:     stdin,
		stdout:    stdout,
		ctx:       ctx,
		cancel:    cancel,
		updatesCh: make(chan SessionUpdate, 100),
	}
}

// Initialize establishes the Codex connection with the agent subprocess.
func (a *CodexAdapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing Codex adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create Codex client
	a.client = codex.NewClient(a.stdin, a.stdout, a.logger)
	a.client.SetNotificationHandler(a.handleNotification)
	a.client.SetRequestHandler(a.handleRequest)

	// Start reading from stdout with the adapter's context
	// The readLoop needs to stay alive for the entire lifecycle of the adapter,
	// not just the initialize HTTP request. It will be cancelled when Close() is called.
	a.client.Start(a.ctx)

	// Perform Codex initialize handshake
	resp, err := a.client.Call(ctx, codex.MethodInitialize, &codex.InitializeParams{
		ClientInfo: &codex.ClientInfo{
			Name:    "kandev-agentctl",
			Title:   "Kandev Agent Controller",
			Version: "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("codex initialize handshake failed: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("codex initialize error: %s", resp.Error.Message)
	}

	// Parse initialize result
	var initResult codex.InitializeResult
	if resp.Result != nil {
		if err := json.Unmarshal(resp.Result, &initResult); err != nil {
			a.logger.Warn("failed to parse initialize result", zap.Error(err))
		}
	}

	// Send initialized notification
	if err := a.client.Notify(codex.MethodInitialized, nil); err != nil {
		return fmt.Errorf("failed to send initialized notification: %w", err)
	}

	// Store agent info
	a.agentInfo = &AgentInfo{
		Name:    "codex",
		Version: initResult.UserAgent,
	}

	a.logger.Info("Codex adapter initialized",
		zap.String("user_agent", initResult.UserAgent))

	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *CodexAdapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new Codex thread (session).
func (a *CodexAdapter) NewSession(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	resp, err := a.client.Call(ctx, codex.MethodThreadStart, &codex.ThreadStartParams{
		Cwd:            a.cfg.WorkDir,
		ApprovalPolicy: "never",             // Valid values: untrusted, on-failure, on-request, never
		Sandbox:        "danger-full-access", // Disable sandbox to avoid landlock permission issues
	})
	if err != nil {
		return "", fmt.Errorf("failed to start thread: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("thread start error: %s", resp.Error.Message)
	}

	var result codex.ThreadStartResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse thread start result: %w", err)
	}

	a.threadID = result.Thread.ID
	a.logger.Info("created new thread", zap.String("thread_id", a.threadID))

	return a.threadID, nil
}

// LoadSession resumes an existing Codex thread.
func (a *CodexAdapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	resp, err := a.client.Call(ctx, codex.MethodThreadResume, &codex.ThreadResumeParams{
		ThreadID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to resume thread: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("thread resume error: %s", resp.Error.Message)
	}

	a.threadID = sessionID
	a.logger.Info("resumed thread", zap.String("thread_id", a.threadID))

	return nil
}

// Prompt sends a prompt to the agent, starting a new turn.
func (a *CodexAdapter) Prompt(ctx context.Context, message string) error {
	a.mu.Lock()
	threadID := a.threadID
	// Reset accumulators for new turn
	a.messageBuffer = ""
	a.reasoningBuffer = ""
	a.summaryBuffer = ""
	a.mu.Unlock()

	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("sending prompt", zap.String("thread_id", threadID))

	params := &codex.TurnStartParams{
		ThreadID: threadID,
		Input: []codex.UserInput{
			{Type: "text", Text: message},
		},
	}

	resp, err := a.client.Call(ctx, codex.MethodTurnStart, params)
	if err != nil {
		return fmt.Errorf("failed to start turn: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("turn start error: %s", resp.Error.Message)
	}

	var result codex.TurnStartResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		a.logger.Warn("failed to parse turn start result", zap.Error(err), zap.String("raw", string(resp.Result)))
	}

	turnID := ""
	if result.Turn != nil {
		turnID = result.Turn.ID
	}

	a.mu.Lock()
	a.turnID = turnID
	a.mu.Unlock()

	if result.Turn != nil {
		a.logger.Info("started turn", zap.String("turn_id", turnID), zap.String("status", result.Turn.Status))
	} else {
		a.logger.Info("started turn", zap.String("turn_id", turnID))
	}

	return nil
}

// Cancel interrupts the current turn.
func (a *CodexAdapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	threadID := a.threadID
	turnID := a.turnID
	a.mu.RUnlock()

	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling turn", zap.String("thread_id", threadID), zap.String("turn_id", turnID))

	// Codex uses turn/interrupt to cancel
	_, err := a.client.Call(ctx, codex.MethodTurnInterrupt, map[string]string{
		"threadId": threadID,
		"turnId":   turnID,
	})
	return err
}

// Updates returns the channel for session updates.
func (a *CodexAdapter) Updates() <-chan SessionUpdate {
	return a.updatesCh
}

// GetSessionID returns the current thread ID (session).
func (a *CodexAdapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.threadID
}

// GetOperationID returns the current turn ID (operation).
func (a *CodexAdapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.turnID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *CodexAdapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// Close releases resources held by the adapter.
func (a *CodexAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing Codex adapter")

	// Cancel the context to stop the read loop goroutine
	if a.cancel != nil {
		a.cancel()
	}

	// Stop the client
	if a.client != nil {
		a.client.Stop()
	}

	// Close update channel
	close(a.updatesCh)

	return nil
}

// sendUpdate safely sends an update to the updates channel.
func (a *CodexAdapter) sendUpdate(update SessionUpdate) {
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping notification")
	}
}

// handleNotification processes Codex notifications and emits SessionUpdates.
func (a *CodexAdapter) handleNotification(method string, params json.RawMessage) {
	a.mu.RLock()
	threadID := a.threadID
	turnID := a.turnID
	a.mu.RUnlock()

	switch method {
	// Standard notifications
	case codex.NotifyItemAgentMessageDelta:
		var p codex.AgentMessageDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse agent message delta", zap.Error(err))
			return
		}
		a.mu.Lock()
		a.messageBuffer += p.Delta
		a.mu.Unlock()
		a.sendUpdate(SessionUpdate{
			Type:        UpdateTypeMessageChunk,
			SessionID:   threadID,
			OperationID: turnID,
			Text:        p.Delta,
		})

	case codex.NotifyItemReasoningTextDelta:
		var p codex.ReasoningDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse reasoning text delta", zap.Error(err))
			return
		}
		a.mu.Lock()
		a.reasoningBuffer += p.Delta
		a.mu.Unlock()
		a.sendUpdate(SessionUpdate{
			Type:          UpdateTypeReasoning,
			SessionID:     threadID,
			OperationID:   turnID,
			ReasoningText: p.Delta,
		})

	case codex.NotifyItemReasoningSummaryDelta:
		var p codex.ReasoningDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse reasoning summary delta", zap.Error(err))
			return
		}
		a.mu.Lock()
		a.summaryBuffer += p.Delta
		a.mu.Unlock()
		a.sendUpdate(SessionUpdate{
			Type:             UpdateTypeReasoning,
			SessionID:        threadID,
			OperationID:      turnID,
			ReasoningSummary: p.Delta,
		})

	case codex.NotifyTurnCompleted:
		var p codex.TurnCompletedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse turn completed", zap.Error(err))
			return
		}
		update := SessionUpdate{
			Type:        UpdateTypeComplete,
			SessionID:   threadID,
			OperationID: p.TurnID,
		}
		if !p.Success && p.Error != "" {
			update.Type = UpdateTypeError
			update.Error = p.Error
		}
		a.sendUpdate(update)

	case codex.NotifyTurnDiffUpdated:
		var p codex.TurnDiffUpdatedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse turn diff updated", zap.Error(err))
			return
		}
		a.sendUpdate(SessionUpdate{
			Type:        UpdateTypeMessageChunk,
			SessionID:   threadID,
			OperationID: p.TurnID,
			Diff:        p.Diff,
		})

	case codex.NotifyTurnPlanUpdated:
		var p codex.TurnPlanUpdatedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse turn plan updated", zap.Error(err))
			return
		}
		entries := make([]PlanEntry, len(p.Plan))
		for i, e := range p.Plan {
			entries[i] = PlanEntry{
				Description: e.Description,
				Status:      e.Status,
			}
		}
		a.sendUpdate(SessionUpdate{
			Type:        UpdateTypePlan,
			SessionID:   threadID,
			OperationID: p.TurnID,
			PlanEntries: entries,
		})

	case codex.NotifyError:
		var p codex.ErrorParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse error notification", zap.Error(err))
			return
		}
		a.sendUpdate(SessionUpdate{
			Type:      UpdateTypeError,
			SessionID: threadID,
			Error:     p.Message,
		})

	case codex.NotifyItemStarted:
		var p codex.ItemStartedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse item started", zap.Error(err))
			return
		}
		if p.Item == nil {
			return
		}
		// Map Codex item types to tool call updates
		// Item types: "userMessage", "agentMessage", "commandExecution", "fileChange", "reasoning"
		switch p.Item.Type {
		case "commandExecution":
			a.sendUpdate(SessionUpdate{
				Type:        UpdateTypeToolCall,
				SessionID:   threadID,
				OperationID: turnID,
				ToolCallID:  p.Item.ID,
				ToolName:    "commandExecution",
				ToolTitle:   p.Item.Command,
				ToolStatus:  "running",
				ToolArgs: map[string]interface{}{
					"command": p.Item.Command,
					"cwd":     p.Item.Cwd,
				},
			})
		case "fileChange":
			// Build title from file paths
			var title string
			if len(p.Item.Changes) > 0 {
				title = p.Item.Changes[0].Path
				if len(p.Item.Changes) > 1 {
					title += fmt.Sprintf(" (+%d more)", len(p.Item.Changes)-1)
				}
			}
			a.sendUpdate(SessionUpdate{
				Type:        UpdateTypeToolCall,
				SessionID:   threadID,
				OperationID: turnID,
				ToolCallID:  p.Item.ID,
				ToolName:    "fileChange",
				ToolTitle:   title,
				ToolStatus:  "running",
			})
		}

	case codex.NotifyItemCompleted:
		var p codex.ItemCompletedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse item completed", zap.Error(err))
			return
		}
		if p.Item == nil {
			return
		}
		// Only send updates for tool-like items
		if p.Item.Type == "commandExecution" || p.Item.Type == "fileChange" {
			status := "complete"
			if p.Item.Status == "failed" {
				status = "error"
			}
			update := SessionUpdate{
				Type:        UpdateTypeToolUpdate,
				SessionID:   threadID,
				OperationID: turnID,
				ToolCallID:  p.Item.ID,
				ToolStatus:  status,
			}
			// Include output for commands
			if p.Item.Type == "commandExecution" && p.Item.AggregatedOutput != "" {
				update.ToolResult = p.Item.AggregatedOutput
			}
			// Include diff for file changes
			if p.Item.Type == "fileChange" && len(p.Item.Changes) > 0 {
				var diffs []string
				for _, c := range p.Item.Changes {
					if c.Diff != "" {
						diffs = append(diffs, c.Diff)
					}
				}
				if len(diffs) > 0 {
					update.Diff = strings.Join(diffs, "\n")
				}
			}
			a.sendUpdate(update)
		}

	case codex.NotifyItemCmdExecOutputDelta:
		var p codex.CommandOutputDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse command output delta", zap.Error(err))
			return
		}
		a.sendUpdate(SessionUpdate{
			Type:        UpdateTypeToolUpdate,
			SessionID:   threadID,
			OperationID: turnID,
			ToolCallID:  p.ItemID,
			ToolResult:  p.Delta,
		})

	default:
		// Log unhandled notifications at debug level
		a.logger.Debug("unhandled notification", zap.String("method", method))
	}
}

// handleRequest processes Codex requests (approval requests) and calls permissionHandler.
func (a *CodexAdapter) handleRequest(id interface{}, method string, params json.RawMessage) {
	a.mu.RLock()
	handler := a.permissionHandler
	threadID := a.threadID
	a.mu.RUnlock()

	switch method {
	case codex.NotifyItemCmdExecRequestApproval:
		var p codex.CommandApprovalParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse command approval request", zap.Error(err))
			if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.InvalidParams, Message: "invalid params"}); err != nil {
				a.logger.Warn("failed to send invalid params response", zap.Error(err))
			}
			return
		}
		a.handleApprovalRequest(id, handler, threadID, p.ItemID, types.ActionTypeCommand, p.Command, map[string]interface{}{
			"command":   p.Command,
			"cwd":       p.Cwd,
			"reasoning": p.Reasoning,
		}, p.Options)

	case codex.NotifyItemFileChangeRequestApproval:
		var p codex.FileChangeApprovalParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse file change approval request", zap.Error(err))
			if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.InvalidParams, Message: "invalid params"}); err != nil {
				a.logger.Warn("failed to send invalid params response", zap.Error(err))
			}
			return
		}
		a.handleApprovalRequest(id, handler, threadID, p.ItemID, types.ActionTypeFileWrite, p.Path, map[string]interface{}{
			"path":      p.Path,
			"diff":      p.Diff,
			"reasoning": p.Reasoning,
		}, p.Options)

	default:
		a.logger.Warn("unhandled request", zap.String("method", method))
		if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.MethodNotFound, Message: "method not found"}); err != nil {
			a.logger.Warn("failed to send method not found response", zap.Error(err))
		}
	}
}

// handleApprovalRequest handles permission request logic for both command and file change approvals.
func (a *CodexAdapter) handleApprovalRequest(
	id interface{},
	handler PermissionHandler,
	sessionID string,
	itemID string,
	actionType string,
	title string,
	details map[string]interface{},
	optionStrings []string,
) {
	// Build permission options from Codex options
	options := make([]PermissionOption, len(optionStrings))
	for i, opt := range optionStrings {
		kind := "allow_once"
		switch opt {
		case "approveAlways":
			kind = "allow_always"
		case "reject":
			kind = "reject_once"
		}
		options[i] = PermissionOption{
			OptionID: opt,
			Name:     opt,
			Kind:     kind,
		}
	}

	// If no options provided, use defaults
	if len(options) == 0 {
		options = []PermissionOption{
			{OptionID: "approve", Name: "Approve", Kind: "allow_once"},
			{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
		}
	}

	req := &PermissionRequest{
		SessionID:     sessionID,
		ToolCallID:    itemID,
		Title:         title,
		Options:       options,
		ActionType:    actionType,
		ActionDetails: details,
	}

	if handler == nil {
		// Auto-approve if no handler
		a.logger.Info("auto-approving request (no handler)", zap.String("item_id", itemID))
		if err := a.client.SendResponse(id, &codex.ApprovalResponse{
			ItemID:   itemID,
			Decision: "approve",
		}, nil); err != nil {
			a.logger.Warn("failed to send approval response", zap.Error(err))
		}
		return
	}

	// Call the permission handler
	ctx := context.Background()
	resp, err := handler(ctx, req)
	if err != nil {
		a.logger.Error("permission handler error", zap.Error(err))
		if err := a.client.SendResponse(id, &codex.ApprovalResponse{
			ItemID:   itemID,
			Decision: "reject",
		}, nil); err != nil {
			a.logger.Warn("failed to send reject response", zap.Error(err))
		}
		return
	}

	decision := "approve"
	if resp.Cancelled {
		decision = "reject"
	} else if resp.OptionID != "" {
		decision = resp.OptionID
	}

	if err := a.client.SendResponse(id, &codex.ApprovalResponse{
		ItemID:   itemID,
		Decision: decision,
	}, nil); err != nil {
		a.logger.Warn("failed to send approval response", zap.Error(err))
	}
}

// Verify interface implementation
var _ AgentAdapter = (*CodexAdapter)(nil)
