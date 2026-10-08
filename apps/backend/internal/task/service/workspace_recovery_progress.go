package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

var ErrWorkspaceRecoveryOperationsUnavailable = errors.New("workspace recovery progress storage is unavailable")

type workspaceRecoveryRunner struct {
	binding   worktree.RecoveryProgressBinding
	operation *models.TaskEnvironmentRecoveryOperation
}

// ReconcileWorkspaceRecoveryOperations marks only prior-process running rows
// interrupted. It does not inspect checkouts or change their durable claims.
func (s *Service) ReconcileWorkspaceRecoveryOperations(ctx context.Context) error {
	if s == nil || s.recoveryOperations == nil {
		return nil
	}
	_, err := s.recoveryOperations.InterruptTaskEnvironmentRecoveryOperations(ctx, s.recoveryOperationRunnerID)
	return err
}

// WorkspaceRecoveryProjection reads the path-free latest projection and reports
// liveness only when this process owns the exact registered attempt.
func (s *Service) WorkspaceRecoveryProjection(
	ctx context.Context,
	environmentID string,
) (*models.TaskEnvironmentRecoveryOperation, bool, error) {
	if s == nil || s.recoveryOperations == nil || environmentID == "" {
		return nil, false, nil
	}
	operation, err := s.recoveryOperations.GetTaskEnvironmentRecoveryOperation(ctx, environmentID)
	if err != nil || operation == nil {
		return operation, false, err
	}
	s.recoveryOperationMu.Lock()
	defer s.recoveryOperationMu.Unlock()
	runner := s.recoveryOperationRunners[environmentID]
	live := runner != nil && recoveryBindingMatches(runner.binding, operation) &&
		runner.binding.RunnerInstanceID == s.recoveryOperationRunnerID
	return operation, live, nil
}

// WorkspaceRecoveryIsLive implements worktree.RecoveryProgressLiveReader.
func (s *Service) WorkspaceRecoveryIsLive(ctx context.Context, environmentID string) (bool, error) {
	_, live, err := s.WorkspaceRecoveryProjection(ctx, environmentID)
	return live, err
}

// BeginWorkspaceRecovery implements worktree.RecoveryProgressReporter. The
// current database claim is still required by the repository before a row can
// be created.
func (s *Service) BeginWorkspaceRecovery(
	ctx context.Context,
	start worktree.RecoveryProgressStart,
) (worktree.RecoveryProgressBinding, error) {
	if s == nil || s.recoveryOperations == nil {
		return worktree.RecoveryProgressBinding{}, ErrWorkspaceRecoveryOperationsUnavailable
	}
	if start.TaskEnvironmentID == "" || start.OwnerTaskID == "" || start.SessionID == "" ||
		start.OperationID == "" || start.OwnershipGeneration <= 0 || start.RepositoryTotal != len(start.SelectedRepositoryIDs) {
		return worktree.RecoveryProgressBinding{}, errors.New("workspace recovery progress identity is incomplete")
	}
	s.recoveryOperationMu.Lock()
	if s.recoveryOperationRunners[start.TaskEnvironmentID] != nil {
		s.recoveryOperationMu.Unlock()
		return worktree.RecoveryProgressBinding{}, recoveryoperation.ErrInProgress
	}
	interrupted, err := s.settleUnregisteredSameProcessRecoveryAttempt(ctx, start)
	if err != nil {
		s.recoveryOperationMu.Unlock()
		return worktree.RecoveryProgressBinding{}, err
	}
	operation, err := s.recoveryOperations.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: start.TaskEnvironmentID,
		OwnerTaskID:       start.OwnerTaskID, OwnershipGeneration: start.OwnershipGeneration,
		SessionID: start.SessionID, OperationID: start.OperationID,
		ErrorStamp: start.ErrorStamp, Kind: start.Kind,
		RunnerInstanceID: s.recoveryOperationRunnerID,
		State:            recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseChecking,
		RepositoryTotal:       start.RepositoryTotal,
		SelectedRepositoryIDs: append([]string(nil), start.SelectedRepositoryIDs...),
	})
	if err != nil {
		s.recoveryOperationMu.Unlock()
		return worktree.RecoveryProgressBinding{}, err
	}
	binding := recoveryBindingFromOperation(operation)
	s.recoveryOperationRunners[start.TaskEnvironmentID] = &workspaceRecoveryRunner{
		binding: binding, operation: operation,
	}
	s.recoveryOperationMu.Unlock()
	if interrupted != nil {
		s.publishWorkspaceRecoveryChanged(ctx, interrupted, false)
	}
	s.publishWorkspaceRecoveryChanged(ctx, operation, true)
	return binding, nil
}

// settleUnregisteredSameProcessRecoveryAttempt closes a runner row only after
// this process has lost its liveness proof and the retry holds current durable
// authority for the environment.
func (s *Service) settleUnregisteredSameProcessRecoveryAttempt(
	ctx context.Context,
	start worktree.RecoveryProgressStart,
) (*models.TaskEnvironmentRecoveryOperation, error) {
	previous, err := s.recoveryOperations.GetTaskEnvironmentRecoveryOperation(ctx, start.TaskEnvironmentID)
	if err != nil || previous == nil || previous.State != recoveryoperation.StateRunning {
		return nil, err
	}
	if previous.RunnerInstanceID != s.recoveryOperationRunnerID {
		return nil, recoveryoperation.ErrInProgress
	}
	claimReaderAvailable, claimMatches, claimErr := s.recoveryStartHasCurrentClaim(ctx, start)
	if claimErr != nil {
		return nil, claimErr
	}
	if (claimReaderAvailable && !claimMatches) ||
		(!claimReaderAvailable && !recoveryOperationMatchesStartIdentity(previous, start)) {
		return nil, recoveryoperation.ErrInProgress
	}
	endedAt := time.Now().UTC()
	interrupted, err := s.recoveryOperations.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: previous.TaskEnvironmentID, OperationID: previous.OperationID,
		AttemptID: previous.AttemptID, OwnershipGeneration: previous.OwnershipGeneration,
		RunnerInstanceID: previous.RunnerInstanceID, ExpectedRevision: previous.Revision,
		State: recoveryoperation.StateInterrupted, Phase: previous.Phase,
		RepositoryID: previous.RepositoryID, RepositoryPosition: previous.RepositoryPosition,
		RepositoryTotal: previous.RepositoryTotal, CompletedSlots: previous.CompletedSlots,
		WorkspaceComplete: previous.WorkspaceComplete, AgentReady: previous.AgentReady,
		EndedAt: &endedAt, ReasonCode: "runner_ended_unsettled",
	})
	if err != nil {
		return nil, err
	}
	return interrupted, nil
}

func recoveryOperationMatchesStartIdentity(
	previous *models.TaskEnvironmentRecoveryOperation,
	start worktree.RecoveryProgressStart,
) bool {
	return previous.OwnerTaskID == start.OwnerTaskID && previous.OwnershipGeneration == start.OwnershipGeneration &&
		previous.SessionID == start.SessionID && previous.OperationID == start.OperationID &&
		previous.ErrorStamp == start.ErrorStamp && previous.Kind == start.Kind
}

func (s *Service) recoveryStartHasCurrentClaim(
	ctx context.Context,
	start worktree.RecoveryProgressStart,
) (bool, bool, error) {
	reader, ok := s.taskEnvironments.(repository.TaskEnvironmentRecoveryClaimReader)
	if !ok {
		return false, false, nil
	}
	claim, err := reader.GetTaskEnvironmentRecoveryClaim(ctx, start.TaskEnvironmentID)
	if err != nil || claim == nil {
		return true, false, err
	}
	matches := claim.TaskEnvironmentID == start.TaskEnvironmentID && claim.OwnerTaskID == start.OwnerTaskID &&
		claim.OwnershipGeneration == start.OwnershipGeneration && claim.SessionID == start.SessionID &&
		claim.OperationID == start.OperationID
	return true, matches, nil
}

func (s *Service) UpdateWorkspaceRecovery(
	ctx context.Context,
	binding worktree.RecoveryProgressBinding,
	update worktree.RecoveryProgressUpdate,
) (worktree.RecoveryProgressBinding, error) {
	if s == nil || s.recoveryOperations == nil {
		return worktree.RecoveryProgressBinding{}, ErrWorkspaceRecoveryOperationsUnavailable
	}
	s.recoveryOperationMu.Lock()
	runner := s.recoveryOperationRunners[binding.TaskEnvironmentID]
	if runner == nil || !sameRecoveryBinding(runner.binding, binding) || runner.operation.Revision != binding.Revision {
		s.recoveryOperationMu.Unlock()
		return worktree.RecoveryProgressBinding{}, recoveryoperation.ErrStaleWriter
	}
	operation, err := s.recoveryOperations.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: binding.TaskEnvironmentID, OperationID: binding.OperationID,
		AttemptID: binding.AttemptID, OwnershipGeneration: binding.OwnershipGeneration,
		RunnerInstanceID: binding.RunnerInstanceID, ExpectedRevision: binding.Revision,
		State: update.State, Phase: update.Phase, RepositoryID: update.RepositoryID,
		RepositoryPosition: update.RepositoryPosition, RepositoryTotal: update.RepositoryTotal,
		CompletedSlots: update.CompletedSlots, WorkspaceComplete: update.WorkspaceComplete,
		AgentReady: update.AgentReady, EndedAt: update.EndedAt, ReasonCode: update.ReasonCode,
	})
	if err != nil {
		s.recoveryOperationMu.Unlock()
		return worktree.RecoveryProgressBinding{}, err
	}
	next := recoveryBindingFromOperation(operation)
	if recoveryoperation.IsTerminal(operation.State) {
		delete(s.recoveryOperationRunners, binding.TaskEnvironmentID)
	} else {
		runner.binding = next
		runner.operation = operation
	}
	s.recoveryOperationMu.Unlock()
	s.publishWorkspaceRecoveryChanged(ctx, operation, operation.State == recoveryoperation.StateRunning)
	return next, nil
}

func (s *Service) HeartbeatWorkspaceRecovery(
	ctx context.Context,
	binding worktree.RecoveryProgressBinding,
) (worktree.RecoveryProgressBinding, error) {
	if s == nil || s.recoveryOperations == nil {
		return worktree.RecoveryProgressBinding{}, ErrWorkspaceRecoveryOperationsUnavailable
	}
	s.recoveryOperationMu.Lock()
	runner := s.recoveryOperationRunners[binding.TaskEnvironmentID]
	if runner == nil || !sameRecoveryBinding(runner.binding, binding) || runner.operation.Revision != binding.Revision {
		s.recoveryOperationMu.Unlock()
		return worktree.RecoveryProgressBinding{}, recoveryoperation.ErrStaleWriter
	}
	current := runner.operation
	operation, err := s.recoveryOperations.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: current.TaskEnvironmentID, OperationID: current.OperationID,
		AttemptID: current.AttemptID, OwnershipGeneration: current.OwnershipGeneration,
		RunnerInstanceID: current.RunnerInstanceID, ExpectedRevision: current.Revision,
		State: current.State, Phase: current.Phase, RepositoryID: current.RepositoryID,
		RepositoryPosition: current.RepositoryPosition, RepositoryTotal: current.RepositoryTotal,
		CompletedSlots: current.CompletedSlots, WorkspaceComplete: current.WorkspaceComplete,
		AgentReady: current.AgentReady, ReasonCode: current.ReasonCode,
	})
	if err != nil {
		s.recoveryOperationMu.Unlock()
		return worktree.RecoveryProgressBinding{}, err
	}
	next := recoveryBindingFromOperation(operation)
	runner.binding = next
	runner.operation = operation
	s.recoveryOperationMu.Unlock()
	s.publishWorkspaceRecoveryChanged(ctx, operation, true)
	return next, nil
}

// EndWorkspaceRecoveryRunner removes the in-process liveness proof after the
// synchronous admission path exits. A stale exit cannot unregister a retry.
func (s *Service) EndWorkspaceRecoveryRunner(ctx context.Context, binding worktree.RecoveryProgressBinding) {
	if s == nil {
		return
	}
	s.recoveryOperationMu.Lock()
	runner := s.recoveryOperationRunners[binding.TaskEnvironmentID]
	var operation *models.TaskEnvironmentRecoveryOperation
	if runner != nil && sameRecoveryBinding(runner.binding, binding) {
		delete(s.recoveryOperationRunners, binding.TaskEnvironmentID)
		if runner.operation.State == recoveryoperation.StateRunning {
			operation = runner.operation
		}
	}
	s.recoveryOperationMu.Unlock()
	s.publishWorkspaceRecoveryChanged(ctx, operation, false)
}

func (s *Service) publishWorkspaceRecoveryChanged(
	ctx context.Context,
	operation *models.TaskEnvironmentRecoveryOperation,
	runnerLive bool,
) {
	if s.eventBus == nil || operation == nil {
		return
	}
	sessionIDs := s.workspaceRecoveryNotificationRecipients(ctx, operation)
	payload := map[string]any{
		"task_id":        operation.OwnerTaskID,
		"environment_id": operation.TaskEnvironmentID,
		"session_id":     operation.SessionID,
		"session_ids":    sessionIDs,
		"workspace_recovery": map[string]any{
			"task_id":              operation.OwnerTaskID,
			"environment_id":       operation.TaskEnvironmentID,
			"session_id":           operation.SessionID,
			"operation_id":         operation.OperationID,
			"attempt_id":           operation.AttemptID,
			"error_stamp":          operation.ErrorStamp,
			"ownership_generation": strconv.FormatInt(operation.OwnershipGeneration, 10),
			"revision":             strconv.FormatInt(operation.Revision, 10),
			"kind":                 operation.Kind,
			"state":                operation.State,
			"phase":                operation.Phase,
			"repository_id":        operation.RepositoryID,
			"repository_position":  operation.RepositoryPosition,
			"repository_total":     operation.RepositoryTotal,
			"completed_slots":      operation.CompletedSlots,
			"workspace_complete":   operation.WorkspaceComplete,
			"agent_ready":          operation.AgentReady,
			"runner_live":          runnerLive,
			"started_at":           operation.StartedAt,
			"updated_at":           operation.UpdatedAt,
			"ended_at":             operation.EndedAt,
			"reason_code":          operation.ReasonCode,
		},
	}
	durableCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.eventBus.Publish(durableCtx, events.SessionWorkspaceRecoveryChanged, bus.NewEvent(
		events.SessionWorkspaceRecoveryChanged, "task-service", payload,
	)); err != nil && s.logger != nil {
		s.logger.Warn("failed to publish workspace recovery notification",
			zap.String("task_environment_id", operation.TaskEnvironmentID), zap.Error(err))
	}
}

func (s *Service) workspaceRecoveryNotificationRecipients(
	ctx context.Context,
	operation *models.TaskEnvironmentRecoveryOperation,
) []string {
	recipients := []string{operation.SessionID}
	if s.sessions == nil {
		return recipients
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, operation.OwnerTaskID)
	if err != nil && s.logger != nil {
		s.logger.Warn("failed to list environment sessions for recovery notification",
			zap.String("task_environment_id", operation.TaskEnvironmentID), zap.Error(err))
	}
	if err != nil {
		return recipients
	}
	recipients = recipients[:0]
	for _, session := range sessions {
		if session != nil && session.TaskEnvironmentID == operation.TaskEnvironmentID {
			recipients = append(recipients, session.ID)
		}
	}
	if len(recipients) == 0 {
		return []string{operation.SessionID}
	}
	return recipients
}

func recoveryBindingFromOperation(operation *models.TaskEnvironmentRecoveryOperation) worktree.RecoveryProgressBinding {
	if operation == nil {
		return worktree.RecoveryProgressBinding{}
	}
	return worktree.RecoveryProgressBinding{
		TaskEnvironmentID:   operation.TaskEnvironmentID,
		OwnerTaskID:         operation.OwnerTaskID,
		OwnershipGeneration: operation.OwnershipGeneration,
		SessionID:           operation.SessionID,
		OperationID:         operation.OperationID,
		AttemptID:           operation.AttemptID,
		RunnerInstanceID:    operation.RunnerInstanceID,
		Revision:            operation.Revision,
	}
}

func recoveryBindingMatches(binding worktree.RecoveryProgressBinding, operation *models.TaskEnvironmentRecoveryOperation) bool {
	return operation != nil && binding.TaskEnvironmentID == operation.TaskEnvironmentID &&
		binding.OwnerTaskID == operation.OwnerTaskID && binding.OwnershipGeneration == operation.OwnershipGeneration &&
		binding.SessionID == operation.SessionID && binding.OperationID == operation.OperationID &&
		binding.AttemptID == operation.AttemptID && binding.RunnerInstanceID == operation.RunnerInstanceID &&
		operation.State == recoveryoperation.StateRunning
}

func sameRecoveryBinding(a, b worktree.RecoveryProgressBinding) bool {
	return a.TaskEnvironmentID == b.TaskEnvironmentID && a.OwnerTaskID == b.OwnerTaskID &&
		a.OwnershipGeneration == b.OwnershipGeneration && a.SessionID == b.SessionID &&
		a.OperationID == b.OperationID && a.AttemptID == b.AttemptID &&
		a.RunnerInstanceID == b.RunnerInstanceID && a.Revision == b.Revision
}
