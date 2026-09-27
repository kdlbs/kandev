package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactExecutionTaskService interface {
	GetTask(context.Context, string) (*taskmodels.Task, error)
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
}

type exactExecutionClaimGuard interface {
	WithTaskManagementClaimFence(context.Context, string, string, string, int64, func() error) error
}

type exactExecutionOrchestrator interface {
	EnsureSession(context.Context, string, ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error)
	RecoverSession(context.Context, string, string, string) (*orchestrator.LaunchSessionResponse, error)
	StopExecution(context.Context, string, string, bool) error
}

type pluginsExactExecutionController struct {
	tasks        exactExecutionTaskService
	orchestrator exactExecutionOrchestrator
	lifecycle    agentruntime.SessionExecutionControl
	queue        *messagequeue.Service
}

func (a pluginsExactExecutionController) EnsureTaskRun(ctx context.Context, installationID string, in pluginsdk.ExactTaskRunCommand) (pluginsdk.ExactTaskRunResult, error) {
	var result pluginsdk.ExactTaskRunResult
	err := a.withManagementClaimFence(ctx, installationID, in.TaskID, in.ManagementInstanceKey, in.ExpectedClaimGeneration, func() error {
		var callErr error
		result, callErr = a.ensureTaskRun(ctx, in)
		return callErr
	})
	return result, err
}

func (a pluginsExactExecutionController) ensureTaskRun(ctx context.Context, in pluginsdk.ExactTaskRunCommand) (pluginsdk.ExactTaskRunResult, error) {
	task, err := a.taskInWorkspace(ctx, in.WorkspaceID, in.TaskID)
	if err != nil {
		return pluginsdk.ExactTaskRunResult{}, err
	}
	if !exactTimeVersion(task.UpdatedAt, in.ExpectedTaskResourceVersion) {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.Aborted, "task version changed")
	}
	if task.ArchivedAt != nil {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.FailedPrecondition, "archived tasks cannot start")
	}
	start := true
	response, err := a.orchestrator.EnsureSession(ctx, in.TaskID, orchestrator.EnsureSessionOptions{
		EnsureExecution: true, AutoStart: &start, ActivationSource: orchestrator.LaunchActivationSourceUserAction,
	})
	if err != nil {
		return pluginsdk.ExactTaskRunResult{}, normalizeExecutionControlError(err)
	}
	if response == nil || response.SessionID == "" {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.Unavailable, "task run did not return a session")
	}
	var executionID string
	if execution, ok := a.lifecycle.GetExecutionBySessionID(response.SessionID); ok {
		executionID = execution.ID
	}
	return pluginsdk.ExactTaskRunResult{SessionID: response.SessionID, SessionState: response.State, ExecutionID: executionID}, nil
}

func (a pluginsExactExecutionController) StopTaskRun(ctx context.Context, installationID string, in pluginsdk.ExactTaskExecutionCommand) (bool, error) {
	var stopped bool
	err := a.withManagementClaimFence(ctx, installationID, in.TaskID, in.ManagementInstanceKey, in.ExpectedClaimGeneration, func() error {
		var callErr error
		stopped, callErr = a.stopTaskRun(ctx, in)
		return callErr
	})
	return stopped, err
}

func (a pluginsExactExecutionController) stopTaskRun(ctx context.Context, in pluginsdk.ExactTaskExecutionCommand) (bool, error) {
	session, err := a.sessionInWorkspace(ctx, in.WorkspaceID, in.TaskID, in.SessionID)
	if err != nil {
		return false, err
	}
	if !exactTimeVersion(session.UpdatedAt, in.ExpectedSessionResourceVersion) {
		return false, status.Error(codes.Aborted, "session version changed")
	}
	execution, found := a.lifecycle.GetExecutionBySessionID(in.SessionID)
	if !found {
		return false, nil
	}
	if execution.ID != in.ExpectedExecutionID {
		return false, status.Error(codes.Aborted, "execution generation changed")
	}
	if err := a.orchestrator.StopExecution(ctx, in.ExpectedExecutionID, "plugin_exact_stop", false); err != nil {
		return false, normalizeExecutionControlError(err)
	}
	return true, nil
}

func (a pluginsExactExecutionController) RecoverSession(ctx context.Context, installationID string, in pluginsdk.ExactSessionRecoveryCommand) (pluginsdk.ExactTaskRunResult, error) {
	var result pluginsdk.ExactTaskRunResult
	base := in.ExactTaskExecutionCommand
	err := a.withManagementClaimFence(ctx, installationID, base.TaskID, base.ManagementInstanceKey, base.ExpectedClaimGeneration, func() error {
		var callErr error
		result, callErr = a.recoverSession(ctx, in)
		return callErr
	})
	return result, err
}

func (a pluginsExactExecutionController) recoverSession(ctx context.Context, in pluginsdk.ExactSessionRecoveryCommand) (pluginsdk.ExactTaskRunResult, error) {
	base := in.ExactTaskExecutionCommand
	session, err := a.sessionInWorkspace(ctx, base.WorkspaceID, base.TaskID, base.SessionID)
	if err != nil {
		return pluginsdk.ExactTaskRunResult{}, err
	}
	if !exactTimeVersion(session.UpdatedAt, base.ExpectedSessionResourceVersion) {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.Aborted, "session version changed")
	}
	if session.AgentExecutionID == "" || session.AgentExecutionID != base.ExpectedExecutionID {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.Aborted, "session execution generation changed")
	}
	if session.State != taskmodels.TaskSessionStateFailed {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.FailedPrecondition, "session is not an interrupted run")
	}
	if _, found := a.lifecycle.GetExecutionBySessionID(base.SessionID); found {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.FailedPrecondition, "session already has a live execution")
	}
	response, err := a.orchestrator.RecoverSession(ctx, base.TaskID, base.SessionID, "resume")
	if err != nil {
		return pluginsdk.ExactTaskRunResult{}, normalizeExecutionControlError(err)
	}
	if response == nil {
		return pluginsdk.ExactTaskRunResult{}, status.Error(codes.Unavailable, "session recovery returned no result")
	}
	return pluginsdk.ExactTaskRunResult{SessionID: response.SessionID, ExecutionID: response.AgentExecutionID, SessionState: response.State}, nil
}

func (a pluginsExactExecutionController) CancelPendingTaskTransition(ctx context.Context, installationID string, in pluginsdk.ExactPendingTransitionCommand) (bool, error) {
	var removed bool
	err := a.withManagementClaimFence(ctx, installationID, in.TaskID, in.ManagementInstanceKey, in.ExpectedClaimGeneration, func() error {
		var callErr error
		removed, callErr = a.cancelPendingTaskTransition(ctx, in)
		return callErr
	})
	return removed, err
}

func (a pluginsExactExecutionController) cancelPendingTaskTransition(ctx context.Context, in pluginsdk.ExactPendingTransitionCommand) (bool, error) {
	if a.queue == nil {
		return false, status.Error(codes.Unimplemented, "pending transition queue is unavailable")
	}
	task, err := a.taskInWorkspace(ctx, in.WorkspaceID, in.TaskID)
	if err != nil {
		return false, err
	}
	records, err := a.queue.ListPendingMoves(ctx)
	if err != nil {
		return false, status.Error(codes.Unavailable, "pending transition state is unavailable")
	}
	for _, record := range records {
		if record.Move.MoveID != in.TransitionID || record.Move.TaskID != in.TaskID {
			continue
		}
		version := pendingTransitionResourceVersion(record, task.UpdatedAt)
		if version != in.ExpectedResourceVersion {
			return false, status.Error(codes.Aborted, "pending transition changed")
		}
		removed, err := a.queue.DeletePendingMoveIfMatch(ctx, record, "")
		if err != nil {
			return false, status.Error(codes.Unavailable, "pending transition cancellation is unavailable")
		}
		if !removed {
			return false, status.Error(codes.Aborted, "pending transition changed")
		}
		return true, nil
	}
	return false, nil
}

func (a pluginsExactExecutionController) GetSessionModeContext(ctx context.Context, installationID string, in pluginsdk.ExactTaskExecutionCommand) (pluginsdk.SessionModeContext, error) {
	var result pluginsdk.SessionModeContext
	err := a.withManagementClaimFence(ctx, installationID, in.TaskID, in.ManagementInstanceKey, in.ExpectedClaimGeneration, func() error {
		var callErr error
		result, callErr = a.getSessionModeContext(ctx, in)
		return callErr
	})
	return result, err
}

func (a pluginsExactExecutionController) getSessionModeContext(ctx context.Context, in pluginsdk.ExactTaskExecutionCommand) (pluginsdk.SessionModeContext, error) {
	session, err := a.sessionInWorkspace(ctx, in.WorkspaceID, in.TaskID, in.SessionID)
	if err != nil {
		return pluginsdk.SessionModeContext{}, err
	}
	execution, found := a.lifecycle.GetExecutionBySessionID(in.SessionID)
	if !found {
		return pluginsdk.SessionModeContext{}, status.Error(codes.FailedPrecondition, "session has no active execution")
	}
	state := a.lifecycle.GetModeStateForSession(in.SessionID)
	if state == nil {
		return pluginsdk.SessionModeContext{}, status.Error(codes.Unimplemented, "provider session modes are unavailable")
	}
	return pluginsdk.SessionModeContext{
		SessionID: session.ID, ExecutionID: execution.ID, CurrentModeID: state.CurrentModeID,
		AvailableModes: pluginSessionModes(state.AvailableModes), ResourceVersion: session.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (a pluginsExactExecutionController) SetSessionMode(ctx context.Context, installationID string, in pluginsdk.ExactSessionModeCommand) (string, error) {
	var current string
	base := in.ExactTaskExecutionCommand
	err := a.withManagementClaimFence(ctx, installationID, base.TaskID, base.ManagementInstanceKey, base.ExpectedClaimGeneration, func() error {
		var callErr error
		current, callErr = a.setSessionMode(ctx, in)
		return callErr
	})
	return current, err
}

func (a pluginsExactExecutionController) setSessionMode(ctx context.Context, in pluginsdk.ExactSessionModeCommand) (string, error) {
	base := in.ExactTaskExecutionCommand
	session, err := a.sessionInWorkspace(ctx, base.WorkspaceID, base.TaskID, base.SessionID)
	if err != nil {
		return "", err
	}
	if !exactTimeVersion(session.UpdatedAt, base.ExpectedSessionResourceVersion) {
		return "", status.Error(codes.Aborted, "session version changed")
	}
	execution, found := a.lifecycle.GetExecutionBySessionID(base.SessionID)
	if !found || execution.ID != base.ExpectedExecutionID {
		return "", status.Error(codes.Aborted, "execution generation changed")
	}
	state := a.lifecycle.GetModeStateForSession(base.SessionID)
	if state == nil || !providerModeAllowed(state.AvailableModes, in.ModeID) {
		return "", status.Error(codes.FailedPrecondition, "provider does not advertise this safe mode")
	}
	if err := a.lifecycle.SetSessionMode(ctx, execution.ID, "", in.ModeID); err != nil {
		return "", normalizeExecutionControlError(err)
	}
	return in.ModeID, nil
}

func (a pluginsExactExecutionController) withManagementClaimFence(
	ctx context.Context,
	installationID, taskID, instanceKey string,
	generation int64,
	effect func() error,
) error {
	guard, ok := a.tasks.(exactExecutionClaimGuard)
	if !ok {
		return status.Error(codes.Unavailable, "task management ownership guard is unavailable")
	}
	err := guard.WithTaskManagementClaimFence(ctx, taskID, installationID, instanceKey, generation, effect)
	if errors.Is(err, repoerrors.ErrTaskManagementClaimConflict) {
		return status.Error(codes.Aborted, "task management ownership changed")
	}
	if err != nil && status.Code(err) == codes.Unknown {
		return status.Error(codes.Unavailable, "task management ownership is unavailable")
	}
	return err
}

func (a pluginsExactExecutionController) taskInWorkspace(ctx context.Context, workspaceID, taskID string) (*taskmodels.Task, error) {
	if a.tasks == nil || a.orchestrator == nil || a.lifecycle == nil {
		return nil, status.Error(codes.Unavailable, "execution services are unavailable")
	}
	task, err := a.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, executionNotFound(err)
	}
	if task.WorkspaceID != workspaceID {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	return task, nil
}

func (a pluginsExactExecutionController) sessionInWorkspace(ctx context.Context, workspaceID, taskID, sessionID string) (*taskmodels.TaskSession, error) {
	if a.tasks == nil || a.lifecycle == nil {
		return nil, status.Error(codes.Unavailable, "execution services are unavailable")
	}
	task, err := a.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, executionNotFound(err)
	}
	if task.WorkspaceID != workspaceID {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	session, err := a.tasks.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	return session, nil
}

func pluginSessionModes(in []streams.SessionModeInfo) []pluginsdk.SessionModeOption {
	out := make([]pluginsdk.SessionModeOption, 0, len(in))
	for _, mode := range in {
		if safeProviderMode(mode) {
			out = append(out, pluginsdk.SessionModeOption{ID: mode.ID, Name: mode.Name, Description: mode.Description})
		}
	}
	return out
}

func providerModeAllowed(modes []streams.SessionModeInfo, modeID string) bool {
	for _, mode := range modes {
		if mode.ID == modeID {
			return safeProviderMode(mode)
		}
	}
	return false
}

func safeProviderMode(mode streams.SessionModeInfo) bool {
	return mode.ID != "" && safeProviderModeValue(mode.ID) && safeProviderModeValue(mode.Name) && safeProviderModeValue(mode.Description)
}

func safeProviderModeValue(value string) bool {
	value = strings.NewReplacer("_", "", "-", "", " ", "", ".", "", "/", "").Replace(strings.ToLower(value))
	for _, forbidden := range []string{
		"bypass", "dangerouslyskip", "dontask", "yolo", "credential", "autoapprove",
		"skippermission", "nopermission", "noapproval", "acceptall", "fullauto", "unrestricted", "allowall",
	} {
		if strings.Contains(value, forbidden) {
			return false
		}
	}
	return true
}

func exactTimeVersion(updatedAt time.Time, expected string) bool {
	return updatedAt.UTC().Format(time.RFC3339Nano) == expected
}

func pendingTransitionResourceVersion(record messagequeue.PendingMoveRecord, taskUpdatedAt time.Time) string {
	queuedAt := record.Move.QueuedAt.UTC().Format(time.RFC3339Nano)
	canonical := map[string]string{"id": record.Move.MoveID, "task_version": taskUpdatedAt.UTC().Format(time.RFC3339Nano), "workflow_id": record.Move.WorkflowID, "step_id": record.Move.WorkflowStepID, "queued_at": queuedAt}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func executionNotFound(err error) error {
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return status.Error(codes.NotFound, "task not found")
	}
	return status.Error(codes.Unavailable, "task lookup is unavailable")
}

func normalizeExecutionControlError(err error) error {
	if err == nil {
		return nil
	}
	if status.Code(err) != codes.Unknown {
		return err
	}
	return status.Error(codes.FailedPrecondition, "native execution admission rejected the request")
}

var _ plugins.ExactExecutionController = pluginsExactExecutionController{}
