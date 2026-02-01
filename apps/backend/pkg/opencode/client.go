package opencode

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Client manages HTTP communication with OpenCode server
type Client struct {
	baseURL    string
	directory  string
	password   string
	httpClient *http.Client
	logger     *logger.Logger

	// Event handling
	eventHandler EventHandler
	controlCh    chan ControlEvent

	// SSE connection tracking - prevents multiple concurrent connections
	sseCancel context.CancelFunc
	sseActive bool

	mu     sync.RWMutex
	closed bool
}

// EventHandler is called for each SDK event from the SSE stream
type EventHandler func(event *SDKEventEnvelope)

// ControlEvent represents control flow events
type ControlEvent struct {
	Type    string // "idle", "auth_required", "session_error", "disconnected"
	Message string
}

// NewClient creates a new OpenCode HTTP client
func NewClient(baseURL, directory, password string, log *logger.Logger) *Client {
	return &Client{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		directory: directory,
		password:  password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:    log,
		controlCh: make(chan ControlEvent, 10),
	}
}

// GenerateServerPassword generates a cryptographically secure random password
func GenerateServerPassword() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a timestamp-based string if random fails
		return fmt.Sprintf("opencode-%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// SetEventHandler sets the handler for SDK events
func (c *Client) SetEventHandler(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandler = handler
}

// ControlChannel returns the channel for control events
func (c *Client) ControlChannel() <-chan ControlEvent {
	return c.controlCh
}

// buildAuthHeader creates the Basic auth header value
func (c *Client) buildAuthHeader() string {
	credentials := base64.StdEncoding.EncodeToString([]byte("opencode:" + c.password))
	return "Basic " + credentials
}

// doRequest performs an HTTP request with auth headers
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	if strings.Contains(path, "?") {
		url += "&directory=" + c.directory
	} else {
		url += "?directory=" + c.directory
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", c.buildAuthHeader())
	req.Header.Set("X-OpenCode-Directory", c.directory)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// doPromptRequest performs an HTTP request with a longer timeout suitable for prompts.
// Prompts can take minutes to complete, so we use a 60-minute timeout.
func (c *Client) doPromptRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	if strings.Contains(path, "?") {
		url += "&directory=" + c.directory
	} else {
		url += "?directory=" + c.directory
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", c.buildAuthHeader())
	req.Header.Set("X-OpenCode-Directory", c.directory)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Use a client with extended timeout for prompts
	promptClient := &http.Client{
		Timeout: 60 * time.Minute,
	}
	return promptClient.Do(req)
}

// WaitForHealth waits for the OpenCode server to be healthy
func (c *Client) WaitForHealth(ctx context.Context) error {
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := c.doRequest(ctx, http.MethodGet, "/global/health", nil)
		if err != nil {
			lastErr = err
			c.logger.Debug("health check request failed", zap.Error(err))
			time.Sleep(150 * time.Millisecond)
			continue
		}

		// Read body for logging and parsing
		bodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read health response: %w", err)
			time.Sleep(150 * time.Millisecond)
			continue
		}

		c.logger.Debug("health check response",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(bodyBytes)))

		// Check HTTP status first
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("health check HTTP %d: %s", resp.StatusCode, string(bodyBytes))
			time.Sleep(150 * time.Millisecond)
			continue
		}

		var health HealthResponse
		if err := json.Unmarshal(bodyBytes, &health); err != nil {
			lastErr = fmt.Errorf("parse health response (got: %q): %w", string(bodyBytes), err)
			time.Sleep(150 * time.Millisecond)
			continue
		}

		if health.Healthy {
			c.logger.Info("OpenCode server healthy", zap.String("version", health.Version))
			return nil
		}

		lastErr = fmt.Errorf("server unhealthy (version %s)", health.Version)
		time.Sleep(150 * time.Millisecond)
	}

	if lastErr != nil {
		return fmt.Errorf("health check timeout: %w", lastErr)
	}
	return fmt.Errorf("health check timeout")
}

// CreateSession creates a new OpenCode session
func (c *Client) CreateSession(ctx context.Context) (string, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/session", strings.NewReader("{}"))
	if err != nil {
		return "", fmt.Errorf("create session request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create session failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var session SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", fmt.Errorf("parse session response: %w", err)
	}

	return session.ID, nil
}

// ForkSession forks an existing session for follow-up prompts
func (c *Client) ForkSession(ctx context.Context, sessionID string) (string, error) {
	path := fmt.Sprintf("/session/%s/fork", sessionID)
	resp, err := c.doRequest(ctx, http.MethodPost, path, strings.NewReader("{}"))
	if err != nil {
		return "", fmt.Errorf("fork session request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fork session failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var session SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", fmt.Errorf("parse session response: %w", err)
	}

	return session.ID, nil
}

// SendPrompt sends a prompt to the session
func (c *Client) SendPrompt(ctx context.Context, sessionID, prompt string, model *ModelSpec, agent, variant string) error {
	req := PromptRequest{
		Model:   model,
		Agent:   agent,
		Variant: variant,
		Parts: []TextPartInput{
			{Type: "text", Text: prompt},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal prompt request: %w", err)
	}

	path := fmt.Sprintf("/session/%s/message", sessionID)
	// Use a dedicated client with long timeout for prompts - they can take minutes
	resp, err := c.doPromptRequest(ctx, http.MethodPost, path, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("send prompt request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read and validate response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read prompt response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prompt failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Validate response structure
	trimmed := strings.TrimSpace(string(respBody))
	if trimmed == "" {
		return fmt.Errorf("prompt returned empty response")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return fmt.Errorf("parse prompt response: %w", err)
	}

	// Check for success response: { info, parts }
	if _, hasInfo := parsed["info"]; hasInfo {
		if _, hasParts := parsed["parts"]; hasParts {
			return nil
		}
	}

	// Check for error response: { name, data }
	if name, ok := parsed["name"].(string); ok {
		message := "unknown error"
		if data, ok := parsed["data"].(map[string]any); ok {
			if msg, ok := data["message"].(string); ok {
				message = msg
			}
		}
		return fmt.Errorf("prompt error: %s: %s", name, message)
	}

	return nil
}

// Abort sends an abort request to stop the current operation
func (c *Client) Abort(ctx context.Context, sessionID string) error {
	path := fmt.Sprintf("/session/%s/abort", sessionID)

	// Use a short timeout for abort
	abortCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	resp, err := c.doRequest(abortCtx, http.MethodPost, path, nil)
	if err != nil {
		return nil // Ignore abort errors
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain body
	_, _ = io.ReadAll(resp.Body)
	return nil
}

// ReplyPermission sends a permission reply
func (c *Client) ReplyPermission(ctx context.Context, requestID, reply string, message *string) error {
	payload := PermissionReplyRequest{
		Reply: reply,
	}
	if message != nil {
		payload.Message = *message
	} else if reply == PermissionReplyReject {
		// If rejecting without message, provide default
		payload.Message = "User denied this tool use request"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal permission reply: %w", err)
	}

	path := fmt.Sprintf("/permission/%s/reply", requestID)
	resp, err := c.doRequest(ctx, http.MethodPost, path, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("permission reply request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain body
	_, _ = io.ReadAll(resp.Body)
	return nil
}

// StartEventStream starts the SSE event stream and processes events.
// It ensures only one SSE connection is active at a time to prevent duplicate events.
func (c *Client) StartEventStream(ctx context.Context, sessionID string) error {
	c.mu.Lock()
	// Check if already connected - prevent multiple concurrent SSE connections
	// which would cause duplicate event processing
	if c.sseActive {
		c.mu.Unlock()
		c.logger.Debug("SSE stream already active, skipping duplicate connection",
			zap.String("session_id", sessionID))
		return nil
	}
	c.sseActive = true
	c.mu.Unlock()

	url := c.baseURL + "/event?directory=" + c.directory

	// Create a cancellable context for this SSE connection
	sseCtx, sseCancel := context.WithCancel(ctx)

	// Store the cancel function so we can close this connection later if needed
	c.mu.Lock()
	c.sseCancel = sseCancel
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(sseCtx, http.MethodGet, url, nil)
	if err != nil {
		c.mu.Lock()
		c.sseActive = false
		c.sseCancel = nil
		c.mu.Unlock()
		sseCancel()
		return fmt.Errorf("create event stream request: %w", err)
	}

	req.Header.Set("Authorization", c.buildAuthHeader())
	req.Header.Set("X-OpenCode-Directory", c.directory)
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for SSE
	sseClient := &http.Client{}
	resp, err := sseClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.sseActive = false
		c.sseCancel = nil
		c.mu.Unlock()
		sseCancel()
		return fmt.Errorf("connect event stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		c.mu.Lock()
		c.sseActive = false
		c.sseCancel = nil
		c.mu.Unlock()
		sseCancel()
		return fmt.Errorf("event stream failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug("SSE stream connected",
		zap.String("session_id", sessionID))

	// Process events in background
	go c.processEventStream(sseCtx, sessionID, resp.Body)

	return nil
}

// processEventStream reads and processes SSE events
func (c *Client) processEventStream(ctx context.Context, sessionID string, body io.ReadCloser) {
	defer func() {
		_ = body.Close()
		// Mark SSE as inactive when stream ends
		c.mu.Lock()
		c.sseActive = false
		c.sseCancel = nil
		c.mu.Unlock()
		c.logger.Debug("SSE stream ended", zap.String("session_id", sessionID))
	}()

	scanner := bufio.NewScanner(body)
	// Increase buffer size for potentially large events
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var dataBuffer strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// SSE format: "data: {...}"
		if strings.HasPrefix(line, "data: ") {
			dataBuffer.WriteString(strings.TrimPrefix(line, "data: "))
			continue
		}

		// Empty line signals end of event
		if line == "" && dataBuffer.Len() > 0 {
			data := strings.TrimSpace(dataBuffer.String())
			dataBuffer.Reset()

			if data == "" {
				continue
			}

			event, err := ParseSDKEvent([]byte(data))
			if err != nil {
				c.logger.Warn("failed to parse SDK event", zap.Error(err))
				continue
			}

			// Filter events for this session
			if !c.eventMatchesSession(event, sessionID) {
				continue
			}

			// Process control events
			c.processControlEvent(event, sessionID)

			// Call event handler
			c.mu.RLock()
			handler := c.eventHandler
			c.mu.RUnlock()

			if handler != nil {
				handler(event)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		c.logger.Error("event stream error", zap.Error(err))
	}

	// Notify disconnection (check if closed first to avoid panic)
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if !closed {
		select {
		case c.controlCh <- ControlEvent{Type: "disconnected"}:
		default:
		}
	}
}

// eventMatchesSession checks if an event belongs to the specified session
func (c *Client) eventMatchesSession(event *SDKEventEnvelope, sessionID string) bool {
	// Try to extract sessionID from properties
	var props map[string]any
	if event.Properties != nil {
		if err := json.Unmarshal(event.Properties, &props); err != nil {
			return true // If we can't parse, let it through
		}
	}

	// Check various paths where sessionID might be
	extractedID := ""

	switch event.Type {
	case SDKEventMessageUpdated:
		if info, ok := props["info"].(map[string]any); ok {
			if id, ok := info["sessionID"].(string); ok {
				extractedID = id
			}
		}
	case SDKEventMessagePartUpdated:
		if part, ok := props["part"].(map[string]any); ok {
			if id, ok := part["sessionID"].(string); ok {
				extractedID = id
			}
		}
	default:
		if id, ok := props["sessionID"].(string); ok {
			extractedID = id
		}
	}

	if extractedID == "" {
		return true // No sessionID in event, let it through
	}

	return extractedID == sessionID
}

// processControlEvent handles control flow events
func (c *Client) processControlEvent(event *SDKEventEnvelope, sessionID string) {
	// Check if closed first to avoid panic on closed channel
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return
	}

	switch event.Type {
	case SDKEventSessionIdle:
		select {
		case c.controlCh <- ControlEvent{Type: "idle"}:
		default:
		}

	case SDKEventSessionError:
		props, err := ParseSessionError(event.Properties)
		if err != nil {
			return
		}

		if props.Error != nil {
			kind := props.Error.GetKind()
			message := props.Error.GetMessage()

			if kind == "ProviderAuthError" {
				select {
				case c.controlCh <- ControlEvent{Type: "auth_required", Message: message}:
				default:
				}
			} else {
				select {
				case c.controlCh <- ControlEvent{Type: "session_error", Message: message}:
				default:
				}
			}
		}
	}
}

// Close closes the client and terminates any active SSE connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}
	c.closed = true

	// Cancel any active SSE connection
	if c.sseCancel != nil {
		c.sseCancel()
		c.sseCancel = nil
	}
	c.sseActive = false

	close(c.controlCh)
}
