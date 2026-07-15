package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

type StartProcessRequest struct {
	SessionID  string
	Kind       string
	ScriptName string
	Command    string
	WorkingDir string
	Env        map[string]string
}

func (m *Manager) StartProcess(ctx context.Context, req StartProcessRequest) (*agentctl.ProcessInfo, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoExecutionForSession, req.SessionID)
	}
	client := execution.GetAgentCtlClient()
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
		SessionID:  req.SessionID,
		Kind:       agentctl.ProcessKind(req.Kind),
		ScriptName: req.ScriptName,
		Command:    req.Command,
		WorkingDir: req.WorkingDir,
		Env:        processEnvironment(execution, req.Env),
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

// WaitForAgentctlReadyForSession waits for agentctl to be ready for a session.
func (m *Manager) WaitForAgentctlReadyForSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client := execution.GetAgentCtlClient()
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
		client := exec.GetAgentCtlClient()
		if client == nil {
			continue
		}
		if _, err := client.GetProcess(ctx, processID, false); err != nil {
			continue
		}
		if err := client.StopProcess(ctx, processID); err != nil {
			return err
		}
		m.releaseActivity(processActivityKey(processID))
		return nil
	}
	return fmt.Errorf("process not found: %s", processID)
}

func processEnvironment(execution *AgentExecution, requested map[string]string) map[string]string {
	managedPath, _ := execution.Metadata[managedGoCacheMetadataKey].(string)
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
	case agentctltypes.ProcessStatusExited, agentctltypes.ProcessStatusFailed, agentctltypes.ProcessStatusStopped:
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
	client := execution.GetAgentCtlClient()
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
	client := execution.GetAgentCtlClient()
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
		if exec.GetAgentCtlClient() == nil {
			continue
		}
		proc, err := exec.GetAgentCtlClient().GetProcess(ctx, processID, includeOutput)
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
		client := exec.GetAgentCtlClient()
		if client == nil {
			continue
		}
		procs, err := client.ListProcesses(ctx, exec.SessionID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, proc := range procs {
			if err := client.StopProcess(ctx, proc.ID); err != nil {
				errs = append(errs, err)
			} else {
				m.releaseActivity(processActivityKey(proc.ID))
			}
		}
	}
	return errors.Join(errs...)
}
