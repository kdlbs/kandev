package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

type StartProcessRequest struct {
	RequestID      string
	SessionID      string
	ExecutionID    string
	Kind           string
	ScriptName     string
	Command        string
	WorkingDir     string
	Env            map[string]string
	BufferMaxBytes int64
	Timeout        time.Duration
}

func (m *Manager) StartProcess(ctx context.Context, req StartProcessRequest) (*agentctl.ProcessInfo, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoExecutionForSession, req.SessionID)
	}
	if req.ExecutionID != "" && execution.ID != req.ExecutionID {
		return nil, fmt.Errorf("%w: session %s is bound to execution %s", ErrExecutionNotFound, req.SessionID, execution.ID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for session %s", req.SessionID)
	}

	lease, err := m.acquireActivity(ctx, processActivityKind(req.Kind))
	if err != nil {
		return nil, err
	}
	startCtx, cancelStart := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancelStart()
	process, err := client.StartProcess(startCtx, agentctl.StartProcessRequest{
		RequestID:      req.RequestID,
		SessionID:      req.SessionID,
		Kind:           agentctl.ProcessKind(req.Kind),
		ScriptName:     req.ScriptName,
		Command:        req.Command,
		WorkingDir:     req.WorkingDir,
		Env:            processEnvironment(execution, req.Env),
		BufferMaxBytes: req.BufferMaxBytes,
		Timeout:        req.Timeout,
	})
	if err != nil {
		lease.Release()
		return nil, err
	}
	if isTerminalProcessStatus(process.Status) {
		lease.Release()
	} else {
		m.trackActivity(processActivityKey(process.ID), lease)
	}
	return process, nil
}

// WorkspaceProcessRequest is the validated request for a command bound to one
// exact agent execution workspace.
type WorkspaceProcessRequest struct {
	RunID          string
	SessionID      string
	ExecutionID    string
	Command        string
	WorkingDir     string
	Timeout        time.Duration
	Kind           string
	BufferMaxBytes int64
}

// StartWorkspaceProcess starts a managed command in the execution's prepared
// workspace. The caller cannot choose a different execution or directory.
func (m *Manager) StartWorkspaceProcess(ctx context.Context, req WorkspaceProcessRequest) (*agentctl.ProcessInfo, error) {
	execution, err := m.workspaceProcessExecution(req.SessionID, req.ExecutionID, req.WorkingDir)
	if err != nil {
		return nil, err
	}
	if req.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if req.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if req.Timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}
	workingDir := execution.WorkspacePath
	if req.Kind == "" {
		req.Kind = string(agentctltypes.ProcessKindWorkflowScript)
	}
	return m.StartProcess(ctx, StartProcessRequest{
		RequestID: req.RunID, SessionID: req.SessionID, ExecutionID: execution.ID,
		Kind: req.Kind, Command: req.Command, WorkingDir: workingDir,
		BufferMaxBytes: req.BufferMaxBytes, Timeout: req.Timeout,
	})
}

func (m *Manager) workspaceProcessExecution(sessionID, executionID, workingDir string) (*AgentExecution, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if executionID == "" {
		return nil, fmt.Errorf("execution_id is required")
	}
	execution, ok := m.executionStore.Get(executionID)
	if !ok || execution == nil {
		return nil, fmt.Errorf("%w: %s", ErrExecutionNotFound, executionID)
	}
	if execution.SessionID != sessionID {
		return nil, fmt.Errorf("%w: execution %s is bound to session %s", ErrExecutionNotFound, executionID, execution.SessionID)
	}
	if execution.WorkspacePath == "" {
		return nil, fmt.Errorf("workspace path is unavailable for execution %s", executionID)
	}
	if workingDir != "" && filepath.Clean(workingDir) != filepath.Clean(execution.WorkspacePath) {
		return nil, fmt.Errorf("workspace path does not match execution %s", executionID)
	}
	return execution, nil
}

// GetWorkspaceProcess reads process state through the exact execution that
// owns it and verifies the returned session before exposing it.
func (m *Manager) GetWorkspaceProcess(ctx context.Context, executionID, processID string, includeOutput bool) (*agentctl.ProcessInfo, error) {
	execution, err := m.workspaceProcessExecutionByID(executionID)
	if err != nil {
		return nil, err
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for execution %s", executionID)
	}
	process, err := client.GetProcess(ctx, processID, includeOutput)
	if err != nil {
		return nil, err
	}
	if process.SessionID != execution.SessionID {
		return nil, fmt.Errorf("process %s is not owned by session %s", processID, execution.SessionID)
	}
	return process, nil
}

// GetWorkspaceProcessByRequestID reads an admitted process through the exact
// execution that owns it. The request identity is used only for recovery; it
// never starts a replacement process.
func (m *Manager) GetWorkspaceProcessByRequestID(ctx context.Context, executionID, requestID string, includeOutput bool) (*agentctl.ProcessInfo, error) {
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	execution, err := m.workspaceProcessExecutionByID(executionID)
	if err != nil {
		return nil, err
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for execution %s", executionID)
	}
	process, err := client.GetProcessByRequestID(ctx, requestID, includeOutput)
	if err != nil {
		return nil, err
	}
	if process.SessionID != execution.SessionID {
		return nil, fmt.Errorf("process request %s is not owned by session %s", requestID, execution.SessionID)
	}
	return process, nil
}

// StopWorkspaceProcess stops a process only after proving its execution and
// session ownership.
func (m *Manager) StopWorkspaceProcess(ctx context.Context, executionID, processID string) error {
	execution, err := m.workspaceProcessExecutionByID(executionID)
	if err != nil {
		return err
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("agentctl client not available for execution %s", executionID)
	}
	process, err := client.GetProcess(ctx, processID, false)
	if err != nil {
		return err
	}
	if process.SessionID != execution.SessionID {
		return fmt.Errorf("process %s is not owned by session %s", processID, execution.SessionID)
	}
	return client.StopProcess(ctx, processID)
}

func (m *Manager) workspaceProcessExecutionByID(executionID string) (*AgentExecution, error) {
	if executionID == "" {
		return nil, fmt.Errorf("execution_id is required")
	}
	execution, ok := m.executionStore.Get(executionID)
	if !ok || execution == nil {
		return nil, fmt.Errorf("%w: %s", ErrExecutionNotFound, executionID)
	}
	return execution, nil
}

// WaitForAgentctlReadyForSession waits for agentctl to be ready for a session.
func (m *Manager) WaitForAgentctlReadyForSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("agentctl client not available for session %s", sessionID)
	}
	return client.WaitForReady(ctx, 10*time.Second)
}

func (m *Manager) StopProcess(ctx context.Context, processID string) error {
	if processID == "" {
		return fmt.Errorf("process_id is required")
	}
	for _, exec := range m.executionStore.List() {
		client, releaseClient := exec.AcquireAgentCtlClient()
		if client == nil {
			releaseClient()
			continue
		}
		if _, err := client.GetProcess(ctx, processID, false); err != nil {
			releaseClient()
			continue
		}
		if err := client.StopProcess(ctx, processID); err != nil {
			releaseClient()
			return err
		}
		releaseClient()
		m.releaseActivity(processActivityKey(processID))
		return nil
	}
	return fmt.Errorf("process not found: %s", processID)
}

func processEnvironment(execution *AgentExecution, requested map[string]string) map[string]string {
	managedPath := execution.metadataString(managedGoCacheMetadataKey)
	if managedPath == "" {
		return requested
	}
	env := make(map[string]string, len(requested)+1)
	for key, value := range requested {
		env[key] = value
	}
	env["GOCACHE"] = managedPath
	return env
}

func isTerminalProcessStatus(status agentctl.ProcessStatus) bool {
	switch status {
	case agentctltypes.ProcessStatusExited,
		agentctltypes.ProcessStatusFailed,
		agentctltypes.ProcessStatusStopped,
		agentctltypes.ProcessStatusTimedOut:
		return true
	default:
		return false
	}
}

// StopProcessForSession stops a running process by ID within a specific session.
func (m *Manager) StopProcessForSession(ctx context.Context, sessionID, processID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if processID == "" {
		return fmt.Errorf("process_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("agentctl client not available for session %s", sessionID)
	}
	if err := client.StopProcess(ctx, processID); err != nil {
		return err
	}
	m.releaseActivity(processActivityKey(processID))
	return nil
}

func (m *Manager) ListProcesses(ctx context.Context, sessionID string) ([]agentctl.ProcessInfo, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for session %s", sessionID)
	}
	return client.ListProcesses(ctx, sessionID)
}

func (m *Manager) GetProcess(ctx context.Context, processID string, includeOutput bool) (*agentctl.ProcessInfo, error) {
	if processID == "" {
		return nil, fmt.Errorf("process_id is required")
	}
	for _, exec := range m.executionStore.List() {
		client, releaseClient := exec.AcquireAgentCtlClient()
		if client == nil {
			releaseClient()
			continue
		}
		proc, err := client.GetProcess(ctx, processID, includeOutput)
		releaseClient()
		if err == nil {
			return proc, nil
		}
	}
	return nil, fmt.Errorf("process not found: %s", processID)
}

// StopAllProcesses attempts to stop all running processes across all executions.
func (m *Manager) StopAllProcesses(ctx context.Context) error {
	executions := m.executionStore.List()
	var errs []error
	for _, exec := range executions {
		client, releaseClient := exec.AcquireAgentCtlClient()
		if client == nil {
			releaseClient()
			continue
		}
		procs, err := client.ListProcesses(ctx, exec.SessionID)
		if err != nil {
			errs = append(errs, err)
			releaseClient()
			continue
		}
		for _, proc := range procs {
			if err := client.StopProcess(ctx, proc.ID); err != nil {
				errs = append(errs, err)
			} else {
				m.releaseActivity(processActivityKey(proc.ID))
			}
		}
		releaseClient()
	}
	return errors.Join(errs...)
}
