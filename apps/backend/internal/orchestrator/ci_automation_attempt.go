package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"go.uber.org/zap"
)

// githubCIAutoFixAttemptService is deliberately narrower than GitHubService.
// Existing provider fakes and GitLab's shared dispatcher do not need to adopt
// the GitHub-only attempt protocol.
type githubCIAutoFixAttemptService interface {
	BindTaskCIAutoFixAttemptTurn(context.Context, github.TaskCIAutoFixAttemptBinding) error
	ReportTaskCIAutoFixOutcome(context.Context, github.TaskCIAutoFixOutcomeReport) error
	ReconcileTaskCIAutoFixTurnCompletion(context.Context, string, string, string) error
	ReconcileTaskCIAutoFixQueuedDispatchFailure(context.Context, github.TaskCIAutoFixAttemptBinding) error
	ReconcileTaskCIAutoFixProviderProgress(context.Context, github.TaskCIAutoFixProviderProgress) error
}

type githubCIAutoFixAttemptStartupService interface {
	ListTaskCIAutoFixStates(context.Context) ([]*github.TaskCIPRAutomationState, error)
	ReconcileTaskCIAutoFixTurnCompletion(context.Context, string, string, string) error
}

func (s *Service) githubCIAutoFixAttemptService() (githubCIAutoFixAttemptService, bool) {
	if s == nil || s.githubService == nil {
		return nil, false
	}
	service, ok := s.githubService.(githubCIAutoFixAttemptService)
	return service, ok
}

func (s *Service) reconcileCIAutoFixAttemptsOnStartup(ctx context.Context) {
	if s == nil || s.turnService == nil || s.githubService == nil {
		return
	}
	service, ok := s.githubService.(githubCIAutoFixAttemptStartupService)
	if !ok {
		return
	}
	reconciled, err := reconcileCIAutoFixAttemptStates(ctx, s.turnService, service)
	if err != nil {
		s.logger.Warn("CI auto-fix startup reconciliation was incomplete", zap.Error(err))
	}
	if reconciled > 0 {
		s.logger.Info("reconciled terminal CI auto-fix attempts on startup", zap.Int("count", reconciled))
	}
}

func reconcileCIAutoFixAttemptStates(
	ctx context.Context,
	turns TurnService,
	attempts githubCIAutoFixAttemptStartupService,
) (int, error) {
	states, err := attempts.ListTaskCIAutoFixStates(ctx)
	if err != nil {
		return 0, err
	}
	reconciled := 0
	for _, state := range states {
		changed, err := reconcileCIAutoFixAttemptState(ctx, turns, attempts, state)
		if err != nil {
			return reconciled, err
		}
		if changed {
			reconciled++
		}
	}
	return reconciled, nil
}

func reconcileCIAutoFixAttemptState(
	ctx context.Context,
	turns TurnService,
	attempts githubCIAutoFixAttemptStartupService,
	state *github.TaskCIPRAutomationState,
) (bool, error) {
	if !isReconciliableCIAutoFixAttempt(state) {
		return false, nil
	}
	terminal, err := isCIAutoFixTurnTerminal(ctx, turns, state)
	if err != nil || !terminal {
		return false, err
	}
	err = attempts.ReconcileTaskCIAutoFixTurnCompletion(
		ctx, state.TaskID, state.AutoFixAttemptSessionID, state.AutoFixAttemptTurnID,
	)
	if errors.Is(err, github.ErrTaskCIAutoFixAttemptNotFound) {
		return false, nil
	}
	return err == nil, err
}

func isReconciliableCIAutoFixAttempt(state *github.TaskCIPRAutomationState) bool {
	return state != nil && state.AutoFixAttemptState == github.TaskCIAutoFixAttemptRunning &&
		strings.TrimSpace(state.AutoFixAttemptSessionID) != "" &&
		strings.TrimSpace(state.AutoFixAttemptTurnID) != ""
}

func isCIAutoFixTurnTerminal(
	ctx context.Context,
	turns TurnService,
	state *github.TaskCIPRAutomationState,
) (bool, error) {
	turn, err := turns.GetTurn(ctx, state.AutoFixAttemptTurnID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return turn == nil || turn.CompletedAt != nil ||
		turn.TaskSessionID != state.AutoFixAttemptSessionID ||
		(turn.TaskID != "" && turn.TaskID != state.TaskID), nil
}

func (s *Service) bindQueuedCIAutoFixAttempt(ctx context.Context, queuedMsg *messagequeue.QueuedMessage, turnID string) {
	if queuedMsg == nil || strings.TrimSpace(turnID) == "" || !isCIAutoFixMetadata(queuedMsg.Metadata) {
		return
	}
	service, ok := s.githubCIAutoFixAttemptService()
	if !ok {
		return
	}
	binding, ok := ciAutoFixAttemptBindingFromMetadata(
		queuedMsg.Metadata, queuedMsg.TaskID, queuedMsg.SessionID, queuedMsg.ID, turnID,
	)
	if !ok {
		s.logger.Debug("CI auto-fix queue entry has incomplete attempt identity",
			zap.String("task_id", queuedMsg.TaskID), zap.String("queue_id", queuedMsg.ID))
		return
	}
	if err := service.BindTaskCIAutoFixAttemptTurn(context.WithoutCancel(ctx), binding); err != nil &&
		!errors.Is(err, github.ErrTaskCIAutoFixAttemptNotFound) {
		s.logger.Debug("bind queued CI auto-fix attempt turn failed",
			zap.String("task_id", queuedMsg.TaskID), zap.String("queue_id", queuedMsg.ID), zap.Error(err))
	}
}

func (s *Service) reconcileQueuedCIAutoFixDispatchFailure(ctx context.Context, queuedMsg *messagequeue.QueuedMessage) {
	if queuedMsg == nil || !isCIAutoFixMetadata(queuedMsg.Metadata) {
		return
	}
	service, ok := s.githubCIAutoFixAttemptService()
	if !ok {
		return
	}
	binding, ok := ciAutoFixAttemptBindingFromMetadata(
		queuedMsg.Metadata, queuedMsg.TaskID, queuedMsg.SessionID, queuedMsg.ID, "",
	)
	if !ok {
		return
	}
	if err := service.ReconcileTaskCIAutoFixQueuedDispatchFailure(context.WithoutCancel(ctx), binding); err != nil &&
		!errors.Is(err, github.ErrTaskCIAutoFixAttemptNotFound) {
		s.logger.Debug("release queued CI auto-fix attempt failed",
			zap.String("task_id", queuedMsg.TaskID), zap.String("queue_id", queuedMsg.ID), zap.Error(err))
	}
}

func (s *Service) reconcileCompletedCIAutoFixTurn(ctx context.Context, taskID, sessionID, turnID string) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(turnID) == "" {
		return
	}
	service, ok := s.githubCIAutoFixAttemptService()
	if !ok {
		return
	}
	if err := service.ReconcileTaskCIAutoFixTurnCompletion(context.WithoutCancel(ctx), taskID, sessionID, turnID); err != nil &&
		!errors.Is(err, github.ErrTaskCIAutoFixAttemptNotFound) {
		s.logger.Debug("reconcile completed CI auto-fix turn failed",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
	}
}

func (s *Service) reconcileCIAutoFixTurnBeforeCompletion(
	ctx context.Context, taskID, sessionID, turnID string,
) {
	if strings.TrimSpace(turnID) == "" {
		turnID, _ = s.peekActiveTurnID(ctx, sessionID)
	}
	s.reconcileCompletedCIAutoFixTurn(ctx, taskID, sessionID, turnID)
}

func isCIAutoFixMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	kind, _ := metadata["automation_kind"].(string)
	return metadata["origin"] == ciAutomationOrigin && kind == ciAutomationKindAutoFix
}

func ciAutoFixAttemptBindingFromMetadata(
	metadata map[string]interface{}, taskID, sessionID, queueEntryID, turnID string,
) (github.TaskCIAutoFixAttemptBinding, bool) {
	repositoryID, _ := metadata["repository_id"].(string)
	signature, _ := metadata["feedback_signature"].(string)
	prNumber, ok := ciAutomationMetadataInt(metadata["pr_number"])
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(sessionID) == "" ||
		strings.TrimSpace(repositoryID) == "" || strings.TrimSpace(signature) == "" || !ok {
		return github.TaskCIAutoFixAttemptBinding{}, false
	}
	return github.TaskCIAutoFixAttemptBinding{
		TaskID:       taskID,
		RepositoryID: repositoryID,
		PRNumber:     prNumber,
		SessionID:    sessionID,
		QueueEntryID: queueEntryID,
		Signature:    signature,
		TurnID:       turnID,
	}, true
}

func ciAutomationMetadataInt(value interface{}) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int32:
		return int(number), true
	case int64:
		return int(number), true
	case float64:
		return int(number), number == float64(int(number))
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(number))
		return parsed, err == nil
	default:
		return 0, false
	}
}

// ReportTaskPRAutoFixOutcome resolves the active turn on the server and then
// forwards only that trusted identity to the GitHub attempt store.
func (s *Service) ReportTaskPRAutoFixOutcome(
	ctx context.Context, taskID, sessionID, outcome, summary string,
) error {
	service, ok := s.githubCIAutoFixAttemptService()
	if !ok {
		return fmt.Errorf("GitHub PR auto-fix outcome reporting is not available")
	}
	if s.turnService == nil {
		return github.ErrTaskCIAutoFixAttemptNotFound
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		return err
	}
	if turn == nil || turn.ID == "" || (turn.TaskID != "" && turn.TaskID != taskID) {
		return github.ErrTaskCIAutoFixAttemptNotFound
	}
	return service.ReportTaskCIAutoFixOutcome(ctx, github.TaskCIAutoFixOutcomeReport{
		TaskID:    taskID,
		SessionID: sessionID,
		TurnID:    turn.ID,
		Outcome:   github.TaskCIAutoFixOutcome(outcome),
		Summary:   summary,
	})
}
