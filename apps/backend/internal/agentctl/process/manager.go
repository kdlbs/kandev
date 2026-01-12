// Package process manages the agent subprocess lifecycle
package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/acp"
	"github.com/kandev/kandev/internal/agentctl/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Compile-time check that we export our client
var _ acp.Client = (*acpclient.Client)(nil)

// Status represents the agent process status
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusPaused   Status = "paused"
	StatusStopping Status = "stopping"
	StatusError    Status = "error"
)

// errorWrapper wraps an error so it can be stored in atomic.Value (which cannot store nil)
type errorWrapper struct {
	err error
}

// PendingPermission represents a permission request waiting for user response
type PendingPermission struct {
	ID         string
	Request    *acpclient.PermissionRequest
	ResponseCh chan *acpclient.PermissionResponse
	CreatedAt  time.Time
}

// PermissionNotification is sent when the agent requests permission
type PermissionNotification struct {
	PendingID  string                       `json:"pending_id"`
	SessionID  string                       `json:"session_id"`
	ToolCallID string                       `json:"tool_call_id"`
	Title      string                       `json:"title"`
	Options    []acpclient.PermissionOption `json:"options"`
	CreatedAt  time.Time                    `json:"created_at"`
}

// Manager manages the agent subprocess
type Manager struct {
	cfg    *config.Config
	logger *logger.Logger

	// Process state
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   io.ReadCloser
	status   atomic.Value // Status
	exitCode atomic.Int32
	exitErr  atomic.Value // error

	// Workspace tracker for git status and file changes
	workspaceTracker *WorkspaceTracker

	// ACP SDK connection
	acpClient *acpclient.Client
	acpConn   *acp.ClientSideConnection
	sessionID acp.SessionId

	// Session update notifications
	updatesCh chan acp.SessionNotification

	// Permission request notifications (sent to backend)
	permissionCh chan *PermissionNotification

	// Pending permission requests waiting for user response
	pendingPermissions map[string]*PendingPermission
	permissionMu       sync.RWMutex

	// Synchronization
	mu       sync.RWMutex
	wg       sync.WaitGroup
	stopCh   chan struct{}
	doneCh   chan struct{}
	startMu  sync.Mutex
}

// NewManager creates a new process manager
func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	m := &Manager{
		cfg:                cfg,
		logger:             log.WithFields(zap.String("component", "process-manager")),
		workspaceTracker:   NewWorkspaceTracker(cfg.WorkDir, log),
		updatesCh:          make(chan acp.SessionNotification, 100),
		permissionCh:       make(chan *PermissionNotification, 10),
		pendingPermissions: make(map[string]*PendingPermission),
	}
	m.status.Store(StatusStopped)
	m.exitCode.Store(-1)
	return m
}

// Status returns the current process status
func (m *Manager) Status() Status {
	return m.status.Load().(Status)
}

// ExitCode returns the exit code (-1 if not exited)
func (m *Manager) ExitCode() int {
	return int(m.exitCode.Load())
}

// ExitError returns the exit error if any
func (m *Manager) ExitError() error {
	if v := m.exitErr.Load(); v != nil {
		if w, ok := v.(errorWrapper); ok {
			return w.err
		}
	}
	return nil
}

// GetWorkspaceTracker returns the workspace tracker for git status and file monitoring
func (m *Manager) GetWorkspaceTracker() *WorkspaceTracker {
	return m.workspaceTracker
}

// Start starts the agent process
func (m *Manager) Start(ctx context.Context) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	if m.Status() == StatusRunning || m.Status() == StatusStarting {
		return fmt.Errorf("agent is already running")
	}

	m.logger.Info("starting agent process",
		zap.Strings("args", m.cfg.AgentArgs),
		zap.String("workdir", m.cfg.WorkDir))

	m.status.Store(StatusStarting)
	m.exitCode.Store(-1)
	m.exitErr.Store(errorWrapper{err: nil})

	// Create command
	if len(m.cfg.AgentArgs) == 0 {
		m.status.Store(StatusError)
		return fmt.Errorf("no agent command configured")
	}

	// NOTE: We intentionally don't use exec.CommandContext here because we don't want
	// the HTTP request context to kill the agent process when the request completes.
	m.cmd = exec.Command(m.cfg.AgentArgs[0], m.cfg.AgentArgs[1:]...)
	m.cmd.Dir = m.cfg.WorkDir
	m.cmd.Env = m.cfg.AgentEnv

	// Set up pipes
	var err error
	m.stdin, err = m.cmd.StdinPipe()
	if err != nil {
		m.status.Store(StatusError)
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	m.stdout, err = m.cmd.StdoutPipe()
	if err != nil {
		m.status.Store(StatusError)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	m.stderr, err = m.cmd.StderrPipe()
	if err != nil {
		m.status.Store(StatusError)
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := m.cmd.Start(); err != nil {
		m.status.Store(StatusError)
		return fmt.Errorf("failed to start agent: %w", err)
	}

	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})

	// Create ACP client that will handle agent requests
	m.acpClient = acpclient.NewClient(
		acpclient.WithLogger(m.logger.Zap()),
		acpclient.WithWorkspaceRoot(m.cfg.WorkDir),
		acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
			select {
			case m.updatesCh <- n:
			default:
				m.logger.Warn("updates channel full, dropping notification")
			}
		}),
		acpclient.WithPermissionHandler(m.handlePermissionRequest),
	)

	// Create ACP SDK connection - it handles JSON-RPC communication
	m.acpConn = acp.NewClientSideConnection(m.acpClient, m.stdin, m.stdout)
	m.acpConn.SetLogger(slog.Default().With("component", "acp-conn"))

	// Start stderr reader and exit waiter
	m.wg.Add(2)
	go m.readStderr()
	go m.waitForExit()

	// Start workspace tracker with background context (not tied to HTTP request)
	m.workspaceTracker.Start(context.Background())

	m.status.Store(StatusRunning)
	m.logger.Info("agent process started", zap.Int("pid", m.cmd.Process.Pid))

	return nil
}

// GetUpdates returns the channel for session update notifications
func (m *Manager) GetUpdates() <-chan acp.SessionNotification {
	return m.updatesCh
}

// GetConnection returns the ACP connection (for advanced usage)
func (m *Manager) GetConnection() *acp.ClientSideConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acpConn
}

// GetSessionID returns the current session ID
func (m *Manager) GetSessionID() acp.SessionId {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionID
}

// SetSessionID sets the current session ID
func (m *Manager) SetSessionID(id acp.SessionId) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionID = id
}

// Stop stops the agent process
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := m.Status()
	if status == StatusStopped || status == StatusStopping {
		return nil
	}

	m.logger.Info("stopping agent process")
	m.status.Store(StatusStopping)

	// Stop workspace tracker
	if m.workspaceTracker != nil {
		m.workspaceTracker.Stop()
	}

	// Close stop channel to signal readers
	if m.stopCh != nil {
		close(m.stopCh)
	}

	// Close stdin to signal EOF to agent
	if m.stdin != nil {
		m.stdin.Close()
	}

	// Wait for process to exit with timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("agent process stopped gracefully")
	case <-ctx.Done():
		// Force kill
		if m.cmd != nil && m.cmd.Process != nil {
			m.logger.Warn("force killing agent process")
			m.cmd.Process.Kill()
		}
	}

	m.status.Store(StatusStopped)
	return nil
}



// readStderr reads and logs stderr from the agent
func (m *Manager) readStderr() {
	defer m.wg.Done()

	scanner := bufio.NewScanner(m.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		m.logger.Debug("agent stderr", zap.String("line", line))
	}

	if err := scanner.Err(); err != nil {
		m.logger.Debug("stderr reader error", zap.Error(err))
	}
}

// waitForExit waits for the process to exit
func (m *Manager) waitForExit() {
	defer m.wg.Done()
	defer close(m.doneCh)

	err := m.cmd.Wait()

	if err != nil {
		m.exitErr.Store(errorWrapper{err: err})
		if exitErr, ok := err.(*exec.ExitError); ok {
			m.exitCode.Store(int32(exitErr.ExitCode()))
		}
		m.logger.Info("agent process exited with error", zap.Error(err))
	} else {
		m.exitCode.Store(0)
		m.logger.Info("agent process exited successfully")
	}

	m.status.Store(StatusStopped)
}



// GetProcessInfo returns information about the process
func (m *Manager) GetProcessInfo() map[string]interface{} {
	info := map[string]interface{}{
		"status":    string(m.Status()),
		"exit_code": m.ExitCode(),
	}

	if m.cmd != nil && m.cmd.Process != nil {
		info["pid"] = m.cmd.Process.Pid
	}

	if err := m.ExitError(); err != nil {
		info["exit_error"] = err.Error()
	}

	return info
}

// handlePermissionRequest handles permission requests from the agent
// It stores the pending request and waits for a response from the backend
func (m *Manager) handlePermissionRequest(ctx context.Context, req *acpclient.PermissionRequest) (*acpclient.PermissionResponse, error) {
	// Generate a unique ID for this permission request
	pendingID := fmt.Sprintf("%s-%s-%d", req.SessionID, req.ToolCallID, time.Now().UnixNano())

	m.logger.Info("handling permission request",
		zap.String("pending_id", pendingID),
		zap.String("session_id", req.SessionID),
		zap.String("tool_call_id", req.ToolCallID),
		zap.String("title", req.Title),
		zap.Bool("auto_approve", m.cfg.AutoApprovePermissions))

	// If auto-approve is enabled, immediately approve with the first "allow" option
	if m.cfg.AutoApprovePermissions {
		return m.autoApprovePermission(req)
	}

	// Create pending permission with response channel
	pending := &PendingPermission{
		ID:         pendingID,
		Request:    req,
		ResponseCh: make(chan *acpclient.PermissionResponse, 1),
		CreatedAt:  time.Now(),
	}

	// Store pending permission
	m.permissionMu.Lock()
	m.pendingPermissions[pendingID] = pending
	m.permissionMu.Unlock()

	// Clean up when done
	defer func() {
		m.permissionMu.Lock()
		delete(m.pendingPermissions, pendingID)
		m.permissionMu.Unlock()
	}()

	// Send notification to backend via updates channel
	// We use a custom notification type that the backend will recognize
	m.sendPermissionNotification(pending)

	// Wait for response with timeout
	select {
	case resp := <-pending.ResponseCh:
		m.logger.Info("received permission response",
			zap.String("pending_id", pendingID),
			zap.String("option_id", resp.OptionID),
			zap.Bool("cancelled", resp.Cancelled))
		return resp, nil
	case <-ctx.Done():
		m.logger.Warn("permission request context cancelled",
			zap.String("pending_id", pendingID))
		return &acpclient.PermissionResponse{Cancelled: true}, nil
	case <-time.After(5 * time.Minute):
		m.logger.Warn("permission request timed out",
			zap.String("pending_id", pendingID))
		return &acpclient.PermissionResponse{Cancelled: true}, nil
	}
}

// autoApprovePermission automatically approves a permission request
// by selecting the first "allow" option, or the first option if no allow option exists
func (m *Manager) autoApprovePermission(req *acpclient.PermissionRequest) (*acpclient.PermissionResponse, error) {
	if len(req.Options) == 0 {
		m.logger.Warn("no options available for auto-approve, cancelling")
		return &acpclient.PermissionResponse{Cancelled: true}, nil
	}

	// Find the first "allow" option
	var selectedOption *acpclient.PermissionOption
	for i := range req.Options {
		opt := &req.Options[i]
		if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
			selectedOption = opt
			break
		}
	}

	// If no allow option, use the first option
	if selectedOption == nil {
		selectedOption = &req.Options[0]
	}

	m.logger.Info("auto-approving permission request",
		zap.String("option_id", selectedOption.OptionID),
		zap.String("option_name", selectedOption.Name),
		zap.String("kind", selectedOption.Kind))

	return &acpclient.PermissionResponse{
		OptionID: selectedOption.OptionID,
	}, nil
}

// sendPermissionNotification sends a permission request notification to the backend
func (m *Manager) sendPermissionNotification(pending *PendingPermission) {
	notification := &PermissionNotification{
		PendingID:  pending.ID,
		SessionID:  pending.Request.SessionID,
		ToolCallID: pending.Request.ToolCallID,
		Title:      pending.Request.Title,
		Options:    pending.Request.Options,
		CreatedAt:  pending.CreatedAt,
	}

	m.logger.Info("sending permission notification to backend",
		zap.String("pending_id", pending.ID),
		zap.String("title", pending.Request.Title))

	select {
	case m.permissionCh <- notification:
		// Sent successfully
	default:
		m.logger.Warn("permission channel full, dropping notification",
			zap.String("pending_id", pending.ID))
	}
}

// GetPermissionRequests returns the channel for permission request notifications
func (m *Manager) GetPermissionRequests() <-chan *PermissionNotification {
	return m.permissionCh
}

// RespondToPermission responds to a pending permission request
func (m *Manager) RespondToPermission(pendingID string, optionID string, cancelled bool) error {
	m.permissionMu.RLock()
	pending, ok := m.pendingPermissions[pendingID]
	m.permissionMu.RUnlock()

	if !ok {
		return fmt.Errorf("pending permission not found: %s", pendingID)
	}

	m.logger.Info("responding to permission request",
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	// Send response (non-blocking since channel is buffered)
	select {
	case pending.ResponseCh <- &acpclient.PermissionResponse{
		OptionID:  optionID,
		Cancelled: cancelled,
	}:
		return nil
	default:
		return fmt.Errorf("response channel full for pending permission: %s", pendingID)
	}
}

// GetPendingPermissions returns all pending permission requests
func (m *Manager) GetPendingPermissions() []*PendingPermission {
	m.permissionMu.RLock()
	defer m.permissionMu.RUnlock()

	result := make([]*PendingPermission, 0, len(m.pendingPermissions))
	for _, p := range m.pendingPermissions {
		result = append(result, p)
	}
	return result
}

// GetPendingPermission returns a specific pending permission request
func (m *Manager) GetPendingPermission(pendingID string) (*PendingPermission, bool) {
	m.permissionMu.RLock()
	defer m.permissionMu.RUnlock()
	p, ok := m.pendingPermissions[pendingID]
	return p, ok
}
