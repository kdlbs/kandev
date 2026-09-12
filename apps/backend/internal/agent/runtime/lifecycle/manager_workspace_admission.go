package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type workspaceAdmissionSnapshot struct {
	session *models.TaskSession
	env     *models.TaskEnvironment
}

// ensureWorkspaceSessionAdmitted validates the durable identity that permits
// a workspace-only execution. Terminal session state is intentionally not part
// of this rule. Cleanup, ownership, archive, and binding changes are.
func (m *Manager) ensureWorkspaceSessionAdmitted(ctx context.Context, taskID string, info *WorkspaceInfo) error {
	if info == nil {
		return fmt.Errorf("workspace info is required")
	}
	if info.TaskArchived || info.WorkspaceOwnerArchived {
		return fmt.Errorf("%w: task workspace is archived", ErrSessionWorkspaceNotReady)
	}
	if m.executorProfileReader == nil || info.SessionID == "" {
		return nil
	}

	snapshot, err := m.readWorkspaceAdmission(ctx, info)
	if err != nil {
		return err
	}
	if err := validateWorkspaceAdmission(taskID, info, snapshot); err != nil {
		return err
	}
	if err := m.ensureWorkspaceTaskAdmission(ctx, snapshot.session); err != nil {
		return err
	}
	if err := m.ensureWorkspaceCleanupInactive(ctx, taskID, snapshot); err != nil {
		return err
	}

	latest, err := m.readWorkspaceAdmission(ctx, info)
	if err != nil {
		return fmt.Errorf("reverify workspace admission: %w", err)
	}
	if err := validateWorkspaceAdmission(taskID, info, latest); err != nil {
		return fmt.Errorf("reverify workspace admission: %w", err)
	}
	if err := m.ensureWorkspaceTaskAdmission(ctx, latest.session); err != nil {
		return fmt.Errorf("reverify workspace admission: %w", err)
	}
	return nil
}

func (m *Manager) readWorkspaceAdmission(ctx context.Context, info *WorkspaceInfo) (*workspaceAdmissionSnapshot, error) {
	session, err := m.executorProfileReader.GetTaskSession(ctx, info.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: verify workspace admission: %w", ErrSessionWorkspaceNotReady, err)
	}
	if session == nil {
		return nil, fmt.Errorf("%w: verify workspace admission: session %q not found", ErrSessionWorkspaceNotReady, info.SessionID)
	}
	snapshot := &workspaceAdmissionSnapshot{session: session}
	if info.TaskEnvironmentID == "" {
		return snapshot, nil
	}
	env, err := m.executorProfileReader.GetTaskEnvironment(ctx, info.TaskEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("%w: verify workspace environment: %w", ErrSessionWorkspaceNotReady, err)
	}
	if env == nil {
		return nil, fmt.Errorf("%w: task environment %q not found", ErrSessionWorkspaceNotReady, info.TaskEnvironmentID)
	}
	snapshot.env = env
	return snapshot, nil
}

func validateWorkspaceAdmission(taskID string, info *WorkspaceInfo, snapshot *workspaceAdmissionSnapshot) error {
	if snapshot == nil || snapshot.session == nil {
		return fmt.Errorf("verify workspace admission: session is unavailable")
	}
	session := snapshot.session
	if err := validateWorkspaceSessionIdentity(taskID, info, session); err != nil {
		return err
	}
	if info.TaskEnvironmentID == "" {
		return validateUnboundWorkspaceSession(info, session)
	}
	return validateBoundWorkspaceEnvironment(info, session, snapshot.env)
}

func validateWorkspaceSessionIdentity(taskID string, info *WorkspaceInfo, session *models.TaskSession) error {
	if session.ID == "" || session.ID != info.SessionID {
		return fmt.Errorf("%w: verify workspace admission: session identity changed", ErrSessionWorkspaceNotReady)
	}
	if taskID == "" || session.TaskID == "" || session.TaskID != taskID {
		return fmt.Errorf("%w: verify workspace admission: session belongs to another task", ErrSessionWorkspaceNotReady)
	}
	if info.TaskID == "" || session.TaskID != info.TaskID {
		return fmt.Errorf("%w: verify workspace admission: session task binding changed", ErrSessionWorkspaceNotReady)
	}
	return nil
}

func validateUnboundWorkspaceSession(info *WorkspaceInfo, session *models.TaskSession) error {
	if session.TaskEnvironmentID != "" {
		return fmt.Errorf("%w: session environment binding is unavailable", ErrSessionWorkspaceNotReady)
	}
	return nil
}

func validateBoundWorkspaceEnvironment(
	info *WorkspaceInfo,
	session *models.TaskSession,
	env *models.TaskEnvironment,
) error {
	if err := validateWorkspaceEnvironmentBinding(info, session, env); err != nil {
		return err
	}
	if info.ValidatedTaskEnvironmentID == "" {
		info.ValidatedTaskEnvironmentID = env.ID
	}
	if env.ID != info.ValidatedTaskEnvironmentID {
		return fmt.Errorf("%w: task environment identity changed", ErrSessionWorkspaceNotReady)
	}
	if info.ValidatedExecutorType == "" && env.ExecutorType != "" {
		info.ValidatedExecutorType = env.ExecutorType
	}
	if info.ValidatedExecutorType != "" && env.ExecutorType != info.ValidatedExecutorType {
		return fmt.Errorf("%w: task environment executor changed", ErrSessionWorkspaceNotReady)
	}
	if info.ValidatedTaskEnvironmentGeneration == 0 {
		info.ValidatedTaskEnvironmentGeneration = env.OwnershipGeneration
	}
	if info.ValidatedTaskEnvironmentGeneration != 0 &&
		env.OwnershipGeneration != info.ValidatedTaskEnvironmentGeneration {
		return fmt.Errorf("%w: task environment ownership changed", ErrSessionWorkspaceNotReady)
	}
	switch env.Status {
	case "", models.TaskEnvironmentStatusReady, models.TaskEnvironmentStatusStopped:
		return nil
	default:
		return fmt.Errorf("%w: task environment is not attachable", ErrSessionWorkspaceNotReady)
	}
}

func validateWorkspaceEnvironmentBinding(
	info *WorkspaceInfo,
	session *models.TaskSession,
	env *models.TaskEnvironment,
) error {
	if session.TaskEnvironmentID != "" && session.TaskEnvironmentID != info.TaskEnvironmentID {
		return fmt.Errorf("%w: verify workspace admission: session environment binding changed", ErrSessionWorkspaceNotReady)
	}
	if env == nil || env.ID != info.TaskEnvironmentID || env.TaskID == "" {
		return fmt.Errorf("%w: task environment ownership is unavailable", ErrSessionWorkspaceNotReady)
	}
	return nil
}

func appendWorkspaceCleanupTaskID(taskIDs []string, candidate string) []string {
	if candidate == "" {
		return taskIDs
	}
	for _, existing := range taskIDs {
		if existing == candidate {
			return taskIDs
		}
	}
	return append(taskIDs, candidate)
}

func workspaceCleanupTaskIDs(taskID string, snapshot *workspaceAdmissionSnapshot) []string {
	taskIDs := make([]string, 0, 3)
	taskIDs = appendWorkspaceCleanupTaskID(taskIDs, taskID)
	taskIDs = appendWorkspaceCleanupTaskID(taskIDs, snapshot.session.TaskID)
	if snapshot.env != nil && snapshot.env.TaskID != snapshot.session.TaskID {
		taskIDs = appendWorkspaceCleanupTaskID(taskIDs, snapshot.env.TaskID)
	}
	return taskIDs
}

func (m *Manager) ensureWorkspaceCleanupInactive(ctx context.Context, taskID string, snapshot *workspaceAdmissionSnapshot) error {
	if snapshot == nil || snapshot.session == nil {
		return fmt.Errorf("verify workspace cleanup: session is unavailable")
	}
	for _, cleanupTaskID := range workspaceCleanupTaskIDs(taskID, snapshot) {
		active, err := m.executorProfileReader.HasActiveTaskResourceCleanupJob(ctx, cleanupTaskID)
		if err != nil {
			return fmt.Errorf("verify workspace cleanup: %w", err)
		}
		if active {
			return fmt.Errorf("verify workspace cleanup: %w for task %q", errTaskCleanupActive, cleanupTaskID)
		}
	}
	return nil
}

func (m *Manager) ensureCachedWorkspaceExecutionAdmitted(ctx context.Context, execution *AgentExecution) error {
	if execution == nil || m.executorProfileReader == nil || execution.SessionID == "" {
		return nil
	}
	info := &WorkspaceInfo{
		TaskID:            execution.TaskID,
		SessionID:         execution.SessionID,
		TaskEnvironmentID: execution.TaskEnvironmentID,
	}
	if m.workspaceInfoProvider != nil {
		resolved, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, execution.TaskID, execution.SessionID)
		if err != nil {
			return fmt.Errorf("verify cached workspace: %w", err)
		}
		if resolved == nil {
			return fmt.Errorf("verify cached workspace: session %q not found", execution.SessionID)
		}
		if execution.TaskEnvironmentID != "" && resolved.TaskEnvironmentID != execution.TaskEnvironmentID {
			return fmt.Errorf("%w: cached execution environment changed", ErrSessionWorkspaceNotReady)
		}
		info = resolved
	}
	return m.ensureWorkspaceSessionAdmitted(ctx, execution.TaskID, info)
}
