package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/task/models"
)

const taskHostRuntimeSessionPrefix = "task-host-"

// createTaskHostExecution creates the internal agentctl execution used by
// task-owned services. It deliberately omits session identity, agent profile,
// traces, lifecycle events, and session-owned persistence.
func (m *Manager) createTaskHostExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
) (*AgentExecution, error) {
	if info == nil {
		return nil, fmt.Errorf("workspace info is required")
	}
	if err := m.reconcileExecutionWorkspace(ctx, taskID, info); err != nil {
		return nil, err
	}
	activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStarting)
	if err != nil {
		return nil, err
	}
	defer activityLease.Release()
	activityLease.SetKind(activity.KindExecutionPreparing)

	rt, err := m.getExecutorBackend(info.ExecutorType)
	if err != nil {
		return nil, fmt.Errorf("no runtime configured: %w", err)
	}
	executionID := taskHostExecutionID(info.TaskEnvironmentID)
	request, err := m.prepareTaskHostCreateRequest(ctx, taskID, info, executionID)
	if err != nil {
		return nil, err
	}
	if err := resumeRemoteInstancePreflight(ctx, rt, request); err != nil {
		return nil, err
	}
	runtimeInstance, err := rt.CreateInstance(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}
	execution := runtimeInstance.ToAgentExecution(request)
	execution.RuntimeName = rt.Name()

	if addErr := m.executionStore.Add(execution); addErr != nil {
		if errors.Is(addErr, ErrExecutionAlreadyExistsForTaskHost) {
			m.rollbackRacedExecution(ctx, rt, runtimeInstance, execution)
			if existing, ok := m.executionStore.GetTaskHostByEnvironmentID(info.TaskEnvironmentID); ok {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("failed to register execution: %w", addErr)
	}
	return m.finishTaskHostExecution(ctx, taskID, info, rt, runtimeInstance, execution)
}

func (m *Manager) prepareTaskHostCreateRequest(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	executionID string,
) (*ExecutorCreateRequest, error) {
	runtimeSessionID := taskHostRuntimeSessionPrefix + info.TaskEnvironmentID
	hostInfo := *info
	hostInfo.SessionID = runtimeSessionID
	hostInfo.AgentProfileID = ""
	hostInfo.ExecutionProfileID = ""
	environment, err := m.prepareExecutionEnvironment(ctx, taskID, &hostInfo, executionID, "", nil, nil)
	if err != nil {
		return nil, err
	}
	metadata := executionMetadata(info.Metadata, true)
	metadata["task_host"] = true
	if environment.managedGoCachePath != "" {
		metadata[managedGoCacheMetadataKey] = environment.managedGoCachePath
	}
	remoteContributions, err := remoteContributionsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	previousExecutionID, authToken, bootstrapNonce := m.taskHostReconnectDetails(ctx, info, executionID)
	return &ExecutorCreateRequest{
		InstanceID:            executionID,
		TaskID:                taskID,
		SessionID:             runtimeSessionID,
		TaskEnvironmentID:     info.TaskEnvironmentID,
		IsTaskHost:            true,
		WorkspacePath:         info.WorkspacePath,
		WorkspaceSourceRoots:  workspaceSourceRoots(info.WorkspaceFolders, info.WorkspaceRepositories),
		Env:                   environment.env,
		Metadata:              metadata,
		ApprovedSecretEnvKeys: append([]string(nil), environment.approvedSecretEnvKeys...),
		RemoteContributions:   remoteContributions,
		PreviousExecutionID:   previousExecutionID,
		AuthToken:             authToken,
		BootstrapNonce:        bootstrapNonce,
	}, nil
}

func (m *Manager) taskHostReconnectDetails(
	ctx context.Context,
	info *WorkspaceInfo,
	executionID string,
) (string, string, string) {
	if info.ExecutorType != string(models.ExecutorTypeLocalDocker) {
		return "", "", ""
	}
	authToken := m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyAuthTokenSecret)
	bootstrapNonce := m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyBootstrapNonceSecret)
	if authToken != "" && bootstrapNonce != "" {
		return executionID, authToken, bootstrapNonce
	}

	// The environment-ready callback can run before the session execution's
	// control-secret references have reached executors_running. The live
	// execution already owns those transport credentials at that point. Use it
	// only to reattach the task-owned host to the shared container; session
	// identity does not become part of task-host ownership.
	if live, exists := m.executionStore.GetByTaskEnvironmentID(info.TaskEnvironmentID); exists {
		metadata := live.MetadataSnapshot()
		if authToken == "" {
			authToken = m.revealRuntimeSecret(ctx, metadata, MetadataKeyAuthTokenSecret)
		}
		if bootstrapNonce == "" {
			bootstrapNonce = m.revealRuntimeSecret(ctx, metadata, MetadataKeyBootstrapNonceSecret)
		}
	}
	return executionID, authToken, bootstrapNonce
}

func (m *Manager) finishTaskHostExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
) (*AgentExecution, error) {
	if err := m.ensureTaskHostTaskActive(ctx, taskID); err != nil {
		m.rollbackTaskHostExecution(rt, runtimeInstance, execution, "task cleanup won task-host registration")
		return nil, err
	}
	if execution.agentctl == nil {
		m.rollbackTaskHostExecution(rt, runtimeInstance, execution, "task host has no control client")
		return nil, fmt.Errorf("task-host execution has no agentctl client")
	}
	if err := execution.agentctl.WaitForReady(ctx, coalescedExecutionCreationTimeout); err != nil {
		m.rollbackTaskHostExecution(rt, runtimeInstance, execution, "task host did not become ready")
		return nil, fmt.Errorf("task-host agentctl not ready: %w", err)
	}
	execution.MarkAgentctlReady()
	m.logger.Info("task-host execution created",
		zap.String("execution_id", execution.ID),
		zap.String("task_id", taskID),
		zap.String("task_environment_id", info.TaskEnvironmentID),
		zap.Stringer("runtime", execution.RuntimeName))
	return execution, nil
}

func taskHostExecutionID(taskEnvironmentID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("kandev/task-host/"+taskEnvironmentID)).String()
}

func executionMetadata(source map[string]interface{}, taskHost bool) map[string]interface{} {
	metadata := make(map[string]interface{}, len(source)+1)
	for key, value := range source {
		if taskHost && (strings.HasPrefix(key, "env_secret_id_") || IsSessionScopedMetadataKey(key)) {
			continue
		}
		metadata[key] = value
	}
	return metadata
}

func (m *Manager) rollbackTaskHostExecution(
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
	reason string,
) {
	m.logger.Warn("rolling back task-host execution",
		zap.String("execution_id", execution.ID),
		zap.String("task_environment_id", execution.TaskEnvironmentID),
		zap.String("reason", reason))
	m.executionStore.Remove(execution.ID)
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rt != nil && runtimeInstance != nil {
		if err := rt.StopInstance(cleanupCtx, runtimeInstance, true); err != nil {
			m.logger.Warn("failed to stop task-host runtime during rollback",
				zap.String("execution_id", execution.ID), zap.Error(err))
		}
	}
	if execution.agentctl != nil {
		execution.agentctl.Close()
	}
}
