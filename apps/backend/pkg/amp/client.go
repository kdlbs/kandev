// Package amp provides a client wrapper for the Sourcegraph Amp CLI.
// This package handles communication with the Amp CLI via stream-json protocol.
package amp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Message types for Amp stream-json protocol.
// Amp claims Claude Code compatibility via --stream-json, so these match Claude Code's types.
const (
	MessageTypeSystem    = "system"
	MessageTypeAssistant = "assistant"
	MessageTypeResult    = "result"
	MessageTypeUser      = "user"
	MessageTypeRateLimit = "rate_limit_event"
)

// Content block types.
const (
	ContentTypeText       = "text"
	ContentTypeThinking   = "thinking"
	ContentTypeToolUse    = "tool_use"
	ContentTypeToolResult = "tool_result"
)

// Client wraps communication with Amp CLI via stdin/stdout.
type Client struct {
	stdin  io.Writer
	stdout io.Reader
	logger *logger.Logger

	scanner *bufio.Scanner

	// Message handler
	messageHandler func(*Message)
	handlerMu      sync.RWMutex

	// Context for lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// State
	threadID string
	mu       sync.RWMutex
}

// Message represents a message from Amp.
// Amp uses Claude Code-compatible stream-json format, so fields match both protocols.
type Message struct {
	Type            string   `json:"type"`
	ThreadID        string   `json:"thread_id,omitempty"`
	Message         *Content `json:"message,omitempty"`
	ParentToolUseID string   `json:"parent_tool_use_id,omitempty"`

	// Result fields (when Type == "result")
	IsError           bool              `json:"is_error,omitempty"`
	Error             string            `json:"error,omitempty"`   // Single error message
	Errors            []string          `json:"errors,omitempty"`  // Multiple errors (Claude Code format)
	Subtype           string            `json:"subtype,omitempty"` // e.g., "error_during_execution"
	CostUSD           float64           `json:"cost_usd,omitempty"`
	TotalCostUSD      float64           `json:"total_cost_usd,omitempty"` // Claude Code uses this field name
	DurationMS        int64             `json:"duration_ms,omitempty"`
	NumTurns          int               `json:"num_turns,omitempty"`
	TotalInputTokens  int64             `json:"total_input_tokens,omitempty"`
	TotalOutputTokens int64             `json:"total_output_tokens,omitempty"`
	Result            json.RawMessage   `json:"result,omitempty"`
	ModelUsage        map[string]*Usage `json:"model_usage,omitempty"`

	// Rate limit fields (when Type == "rate_limit_event")
	RateLimitInfo json.RawMessage `json:"rate_limit_info,omitempty"`
}

// GetCostUSD returns the cost, checking both total_cost_usd (Claude Code format)
// and cost_usd (Amp format).
func (m *Message) GetCostUSD() float64 {
	if m.TotalCostUSD != 0 {
		return m.TotalCostUSD
	}
	return m.CostUSD
}

// Content represents the message content.
type Content struct {
	Model        string         `json:"model,omitempty"`
	Content      []ContentBlock `json:"content,omitempty"`
	Usage        *TokenUsage    `json:"usage,omitempty"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence string         `json:"stop_sequence,omitempty"`
}

// ContentBlock represents a content block (text, tool_use, etc.).
type ContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

// TokenUsage represents token usage information.
type TokenUsage struct {
	InputTokens              int64 `json:"input_tokens,omitempty"`
	OutputTokens             int64 `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
}

// Usage represents model-specific usage stats.
type Usage struct {
	InputTokens   int64  `json:"input_tokens,omitempty"`
	OutputTokens  int64  `json:"output_tokens,omitempty"`
	ContextWindow *int64 `json:"context_window,omitempty"`
}

// ResultData represents the result data structure.
type ResultData struct {
	SessionID string `json:"session_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	Text      string `json:"text,omitempty"`
}

// UserMessage represents a user message to send (Claude Code-compatible format).
type UserMessage struct {
	Type    string          `json:"type"` // "user"
	Message UserMessageBody `json:"message"`
}

// UserMessageBody is the nested message body for user messages.
type UserMessageBody struct {
	Role    string             `json:"role"` // "user"
	Content []UserContentBlock `json:"content"`
}

// UserContentBlock represents a content block in user messages.
type UserContentBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// NewClient creates a new Amp client.
func NewClient(stdin io.Writer, stdout io.Reader, log *logger.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		stdin:  stdin,
		stdout: stdout,
		logger: log.WithFields(zap.String("component", "amp-client")),
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetMessageHandler sets the handler for incoming messages.
func (c *Client) SetMessageHandler(handler func(*Message)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.messageHandler = handler
}

// Start begins reading from stdout and dispatching messages.
func (c *Client) Start(ctx context.Context) {
	c.scanner = bufio.NewScanner(c.stdout)
	// Allow larger buffer for JSON messages
	c.scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	c.wg.Add(1)
	go c.readLoop(ctx)
}

// Stop stops the client and waits for goroutines to finish.
func (c *Client) Stop() {
	c.cancel()
	c.wg.Wait()
}

// readLoop reads JSON lines from stdout and dispatches them.
func (c *Client) readLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.ctx.Done():
			return
		default:
		}

		if !c.scanner.Scan() {
			if err := c.scanner.Err(); err != nil {
				c.logger.Error("scanner error", zap.Error(err))
			}
			return
		}

		line := c.scanner.Text()
		if line == "" {
			continue
		}

		c.handleLine(line)
	}
}

// handleLine parses and dispatches a JSON line.
func (c *Client) handleLine(line string) {
	var msg Message
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		c.logger.Debug("failed to parse message", zap.Error(err), zap.String("line", line))
		return
	}

	// Update thread ID if provided
	if msg.ThreadID != "" {
		c.mu.Lock()
		c.threadID = msg.ThreadID
		c.mu.Unlock()
	}

	c.handlerMu.RLock()
	handler := c.messageHandler
	c.handlerMu.RUnlock()

	if handler != nil {
		handler(&msg)
	}
}

// SendUserMessage sends a user message to Amp.
func (c *Client) SendUserMessage(message string) error {
	msg := &UserMessage{
		Type: "user",
		Message: UserMessageBody{
			Role: "user",
			Content: []UserContentBlock{
				{Type: "text", Text: message},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// GetThreadID returns the current thread ID.
func (c *Client) GetThreadID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.threadID
}

// SetThreadID sets the current thread ID.
func (c *Client) SetThreadID(threadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.threadID = threadID
}

// GetResultData extracts ResultData from a result message.
func (m *Message) GetResultData() *ResultData {
	if len(m.Result) == 0 {
		return nil
	}

	var data ResultData
	if err := json.Unmarshal(m.Result, &data); err != nil {
		return nil
	}

	return &data
}

// GetResultString extracts a string result (for error messages).
func (m *Message) GetResultString() string {
	if len(m.Result) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(m.Result, &s); err != nil {
		return ""
	}

	return s
}
