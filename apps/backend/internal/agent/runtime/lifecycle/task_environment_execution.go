package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type taskPrimarySessionReader interface {
	GetPrimarySession(context.Context, string) (*models.TaskSession, error)
}

// GetOrEnsureExecutionForTaskEnvironment preserves task ownership when an
// ordinary terminal uses a shared environment owned by another task.
func (m *Manager) GetOrEnsureExecutionForTaskEnvironment(ctx context.Context, taskID, environmentID string) (*AgentExecution, error) {
	if taskID == "" || environmentID == "" {
		return nil, fmt.Errorf("task and task environment are required")
	}
	if err := m.CheckTaskEnvironmentAccess(ctx, taskID, environmentID); err != nil {
		return nil, err
	}
	if err := m.CheckEnvironmentExecAccess(ctx, environmentID); err != nil {
		return nil, err
	}
	session, err := m.terminalTaskSession(ctx, taskID, environmentID)
	if err != nil {
		return nil, err
	}
	execution, err := m.EnsureWorkspaceExecutionForSession(ctx, taskID, session.ID)
	if err != nil {
		return nil, err
	}
	if execution == nil || execution.TaskID != taskID || execution.SessionID != session.ID || execution.TaskEnvironmentID != environmentID {
		return nil, fmt.Errorf("%w: terminal execution owner changed", ErrSessionWorkspaceNotReady)
	}
	current, err := m.terminalTaskSession(ctx, taskID, environmentID)
	if err != nil {
		return nil, err
	}
	if current.ID != session.ID {
		return nil, fmt.Errorf("%w: terminal task primary changed", ErrSessionWorkspaceNotReady)
	}
	return execution, nil
}

func (m *Manager) terminalTaskSession(ctx context.Context, taskID, environmentID string) (*models.TaskSession, error) {
	reader, ok := m.workspaceInfoProvider.(taskPrimarySessionReader)
	if !ok || m.executorProfileReader == nil {
		return nil, fmt.Errorf("%w: terminal task owner unavailable", ErrSessionWorkspaceNotReady)
	}
	session, err := reader.GetPrimarySession(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.ID == "" || session.TaskID != taskID {
		return nil, fmt.Errorf("%w: terminal task session binding changed", ErrSessionWorkspaceNotReady)
	}
	environment, err := m.executorProfileReader.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	if environment == nil || environment.ID != environmentID || environment.IsRetired() {
		return nil, fmt.Errorf("%w: terminal task environment unavailable", ErrSessionWorkspaceNotReady)
	}
	if err := m.validateTerminalSessionEnvironment(ctx, session, environment); err != nil {
		return nil, err
	}
	return session, nil
}

func (m *Manager) validateTerminalSessionEnvironment(ctx context.Context, session *models.TaskSession, environment *models.TaskEnvironment) error {
	if session.TaskEnvironmentID == environment.ID {
		return nil
	}
	if session.TaskEnvironmentID != "" || environment.TaskID != session.TaskID {
		return fmt.Errorf("%w: terminal task environment binding changed", ErrSessionWorkspaceNotReady)
	}
	// Legacy unbound sessions may use only their own canonical environment,
	// positively resolved by the existing workspace provider.
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, session.TaskID, session.ID)
	if err != nil {
		return err
	}
	if info == nil || info.TaskID != session.TaskID || info.SessionID != session.ID || info.TaskEnvironmentID != environment.ID {
		return fmt.Errorf("%w: terminal legacy workspace binding unavailable", ErrSessionWorkspaceNotReady)
	}
	return nil
}
