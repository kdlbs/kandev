package lifecycle

import (
	"context"
	"errors"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
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
		return nil, fmt.Errorf("no execution found for session %s", req.SessionID)
	}
	client := execution.GetAgentCtlClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for session %s", req.SessionID)
	}

	return client.StartProcess(ctx, agentctl.StartProcessRequest{
		SessionID:  req.SessionID,
		Kind:       agentctl.ProcessKind(req.Kind),
		ScriptName: req.ScriptName,
		Command:    req.Command,
		WorkingDir: req.WorkingDir,
		Env:        req.Env,
	})
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
		if err := client.StopProcess(ctx, processID); err == nil {
			return nil
		}
	}
	return fmt.Errorf("process not found: %s", processID)
}

func (m *Manager) ListProcesses(ctx context.Context, sessionID string) ([]agentctl.ProcessInfo, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		return nil, fmt.Errorf("no execution found for session %s", sessionID)
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
			}
		}
	}
	return errors.Join(errs...)
}
