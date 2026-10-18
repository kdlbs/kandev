package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	taskPRStateMerged = "merged"
	taskPRStateClosed = "closed"
)

// processTaskPRWorkflowEvent runs the current step's on_pr_merged or
// on_pr_closed actions after the PR poller reports a state change for a pull
// request linked to the task. It never needs an agent turn: the transition
// goes through the same lifecycle as on_children_completed, and the
// operation id makes each PR state change apply at most once.
//
// on_pr_merged runs when every linked PR is merged. on_pr_closed runs when the
// reported PR is closed without merging. Steps without the event ignore it.
func (s *Service) processTaskPRWorkflowEvent(ctx context.Context, pr *github.TaskPR) bool {
	trigger, ok := taskPRWorkflowTrigger(pr)
	if !ok || s.workflowEngine == nil || s.workflowStore == nil ||
		s.workflowStepGetter == nil || s.githubService == nil {
		return false
	}
	task, ok := s.taskPRWorkflowTask(ctx, pr.TaskID, trigger)
	if !ok {
		return false
	}
	linked, ok := s.linkedTaskPRs(ctx, pr)
	if !ok {
		return false
	}
	operationID, payload, ok := taskPRWorkflowOperation(trigger, task.ID, pr, linked)
	if !ok {
		return false
	}
	session, ok := s.taskPRWorkflowSession(ctx, task.ID)
	if !ok {
		return false
	}

	unlock := s.lockChildCompletionOperation(operationID)
	defer unlock()
	if s.operationAlreadyApplied(ctx, task.ID, operationID, trigger) {
		return false
	}

	state := s.buildMachineState(ctx, task, session)
	result, err := s.workflowEngine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:             task.ID,
		SessionID:          session.ID,
		Trigger:            trigger,
		OperationID:        operationID,
		EvaluateOnly:       true,
		DeferOperationMark: true,
		PreloadedState:     &state,
		Payload:            payload,
	})
	if err != nil {
		s.logger.Warn("PR workflow event: workflow engine error",
			zap.String("task_id", task.ID),
			zap.String("trigger", string(trigger)),
			zap.Error(err))
		return false
	}
	if !result.Transitioned {
		s.markOperationApplied(ctx, task.ID, operationID, trigger)
		return false
	}
	if !s.applyEngineTransitionWithMode(ctx, task.ID, session, result, trigger,
		task.Description, transitionLifecycleWithOnEnter) {
		return false
	}
	s.markOperationApplied(ctx, task.ID, operationID, trigger)
	return true
}

// taskPRWorkflowTrigger maps a PR state to its workflow trigger and applies
// the same payload gates as the github_pr_merged automation subscriber.
func taskPRWorkflowTrigger(pr *github.TaskPR) (engine.Trigger, bool) {
	if pr == nil || pr.TaskID == "" || pr.Owner == "" || pr.Repo == "" || pr.PRNumber <= 0 {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(pr.State)) {
	case taskPRStateMerged:
		return engine.TriggerOnPRMerged, true
	case taskPRStateClosed:
		return engine.TriggerOnPRClosed, true
	}
	return "", false
}

// taskPRWorkflowTask loads the task and reports whether its current step
// declares actions for trigger.
func (s *Service) taskPRWorkflowTask(ctx context.Context, taskID string, trigger engine.Trigger) (*models.Task, bool) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, false
	}
	if task.WorkflowStepID == "" || task.IsEphemeral || task.ArchivedAt != nil {
		return nil, false
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil {
		return nil, false
	}
	return task, stepHasPRWorkflowActions(step, trigger)
}

func stepHasPRWorkflowActions(step *wfmodels.WorkflowStep, trigger engine.Trigger) bool {
	switch trigger {
	case engine.TriggerOnPRMerged:
		return len(step.Events.OnPRMerged) > 0
	case engine.TriggerOnPRClosed:
		return len(step.Events.OnPRClosed) > 0
	}
	return false
}

// linkedTaskPRs returns the task's attached PRs with the reported PR's state
// taken from the event. A PR that is no longer attached to the task (for
// example, detached) yields false.
func (s *Service) linkedTaskPRs(ctx context.Context, pr *github.TaskPR) ([]*github.TaskPR, bool) {
	prsByTask, err := s.githubService.ListTaskPRs(ctx, []string{pr.TaskID})
	if err != nil {
		s.logger.Warn("PR workflow event: failed to list task PRs",
			zap.String("task_id", pr.TaskID),
			zap.Error(err))
		return nil, false
	}
	found := false
	linked := make([]*github.TaskPR, 0, len(prsByTask[pr.TaskID]))
	for _, candidate := range prsByTask[pr.TaskID] {
		if candidate == nil {
			continue
		}
		if sameTaskPR(candidate, pr) {
			candidate = pr
			found = true
		}
		linked = append(linked, candidate)
	}
	return linked, found
}

func sameTaskPR(a, b *github.TaskPR) bool {
	return a.PRNumber == b.PRNumber &&
		strings.EqualFold(a.Owner, b.Owner) &&
		strings.EqualFold(a.Repo, b.Repo)
}

// taskPRWorkflowOperation returns the idempotency key and engine payload for
// a PR workflow event, or false when the event does not fire.
func taskPRWorkflowOperation(
	trigger engine.Trigger, taskID string, pr *github.TaskPR, linked []*github.TaskPR,
) (string, any, bool) {
	if trigger == engine.TriggerOnPRClosed {
		closedAt := ""
		if pr.ClosedAt != nil {
			closedAt = pr.ClosedAt.UTC().Format(time.RFC3339Nano)
		}
		operationID := fmt.Sprintf("%s:%s:%s:%s", trigger, taskID, taskPRKey(pr), closedAt)
		return operationID, engine.OnPRClosedPayload{PR: linkedPR(pr)}, true
	}
	keys := make([]string, 0, len(linked))
	prs := make([]engine.LinkedPR, 0, len(linked))
	for _, candidate := range linked {
		if !strings.EqualFold(strings.TrimSpace(candidate.State), taskPRStateMerged) {
			return "", nil, false
		}
		keys = append(keys, taskPRKey(candidate))
		prs = append(prs, linkedPR(candidate))
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "|")))
	operationID := fmt.Sprintf("%s:%s:%s", trigger, taskID, hex.EncodeToString(sum[:]))
	return operationID, engine.OnPRMergedPayload{PRs: prs}, true
}

func taskPRKey(pr *github.TaskPR) string {
	return fmt.Sprintf("%s/%s#%d", strings.ToLower(pr.Owner), strings.ToLower(pr.Repo), pr.PRNumber)
}

func linkedPR(pr *github.TaskPR) engine.LinkedPR {
	return engine.LinkedPR{
		Owner:    pr.Owner,
		Repo:     pr.Repo,
		PRNumber: pr.PRNumber,
		PRURL:    pr.PRURL,
		State:    pr.State,
	}
}

// taskPRWorkflowSession picks the session the transition is recorded
// against: the active session when there is one, otherwise the primary or
// most recent session, so an idle or completed agent does not block the move.
func (s *Service) taskPRWorkflowSession(ctx context.Context, taskID string) (*models.TaskSession, bool) {
	if session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, taskID); err == nil && session != nil {
		return session, true
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil || len(sessions) == 0 {
		s.logger.Debug("PR workflow event: task has no session",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, false
	}
	for _, session := range sessions {
		if session != nil && session.IsPrimary {
			return session, true
		}
	}
	return sessions[0], sessions[0] != nil
}

func (s *Service) operationAlreadyApplied(ctx context.Context, taskID, operationID string, trigger engine.Trigger) bool {
	applied, err := s.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil {
		s.logger.Warn("PR workflow event: failed to check operation idempotency",
			zap.String("task_id", taskID),
			zap.String("trigger", string(trigger)),
			zap.String("operation_id", operationID),
			zap.Error(err))
		return true
	}
	return applied
}

func (s *Service) markOperationApplied(ctx context.Context, taskID, operationID string, trigger engine.Trigger) {
	if err := s.workflowStore.MarkOperationApplied(ctx, operationID); err != nil {
		s.logger.Warn("PR workflow event: failed to mark operation",
			zap.String("task_id", taskID),
			zap.String("trigger", string(trigger)),
			zap.String("operation_id", operationID),
			zap.Error(err))
	}
}
