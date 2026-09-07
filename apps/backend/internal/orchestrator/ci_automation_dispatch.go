package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// This file holds the provider-agnostic CI-automation prompt-dispatch
// mechanics shared by GitHub PR auto-fix (event_handlers_github_ci_automation.go)
// and GitLab MR auto-fix (event_handlers_gitlab_mr_ci_automation.go). It is
// named ci_automation_dispatch.go, not automation_dispatch.go, so it is not
// confused with the unrelated internal/automation package (the user-facing
// Automations/triggers feature). Mirrors #2125's own extraction shape,
// event_handlers_lifecycle_dispatch.go.

type ciAutomationDispatchKind string

const (
	ciAutomationDispatchDirect        ciAutomationDispatchKind = "direct"
	ciAutomationDispatchQueuedInsert  ciAutomationDispatchKind = "queued_insert"
	ciAutomationDispatchQueuedReplace ciAutomationDispatchKind = "queued_replace"
)

type ciAutomationDispatchResult struct {
	kind         ciAutomationDispatchKind
	queueEntryID string
	turnID       string
}

func (r ciAutomationDispatchResult) consumesRound() bool {
	return r.kind == ciAutomationDispatchDirect || r.kind == ciAutomationDispatchQueuedInsert
}

// errCIAutoFixRoundCapReached signals that a new round would exceed the
// provider's fix-round cap; callers translate this into their own
// "mark exhausted" bookkeeping.
var errCIAutoFixRoundCapReached = errors.New("CI auto-fix round cap reached")

// ciAutomationDispatchParams carries everything dispatchCIAutomationPrompt
// needs that is NOT provider-specific. The caller resolves the mention
// token, feedback rendering, and PR/MR identity into these plain values
// before calling in.
type ciAutomationDispatchParams struct {
	ChatPrompt    string
	CoalesceKey   string
	Metadata      map[string]interface{}
	AllowNewRound bool
	// The callbacks keep provider-specific attempt persistence out of this
	// shared GitHub/GitLab dispatcher. They run at the queue admission and
	// agentctl acceptance boundaries, before PromptTask returns at turn end.
	OnDirectAdmission func(turnID string) error
	OnQueued          func(queueEntryID string, replaced bool) error
	OnAccepted        func(turnID, queueEntryID string)
	OnRejected        func(turnID, queueEntryID string)
}

// dispatchCIAutomationPrompt decides how to deliver an auto-fix prompt to a
// task's active session based on session state — queue/replace for a
// running session, replace-or-insert-direct for one idle/waiting for input —
// and reports whether the delivery consumed one of the round cap's rounds.
func (s *Service) dispatchCIAutomationPrompt(
	ctx context.Context, session *models.TaskSession, params ciAutomationDispatchParams,
) (ciAutomationDispatchResult, error) {
	switch session.State {
	case models.TaskSessionStateCreated, models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		return s.queueOrReplaceCIAutomationPrompt(ctx, session, params, params.AllowNewRound)
	case models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle:
		return s.dispatchCIAutomationPromptToIdleSession(ctx, session, params)
	default:
		return ciAutomationDispatchResult{}, fmt.Errorf("session is not promptable: %s", session.State)
	}
}

func (s *Service) dispatchCIAutomationPromptToIdleSession(
	ctx context.Context, session *models.TaskSession, params ciAutomationDispatchParams,
) (ciAutomationDispatchResult, error) {
	result, replaced, err := s.replacePendingCIAutomationPrompt(ctx, session, params)
	if err != nil {
		return ciAutomationDispatchResult{}, err
	}
	if replaced {
		outcome := s.drainQueuedMessageForPromptableSessionOutcome(ctx, session.ID)
		if outcome == queueDrainPaused {
			return result, nil
		}
		if outcome != queueDrainDispatched {
			return ciAutomationDispatchResult{}, fmt.Errorf("failed to dispatch replaced CI automation prompt")
		}
		return result, nil
	}
	if !params.AllowNewRound {
		return ciAutomationDispatchResult{}, errCIAutoFixRoundCapReached
	}
	turnID, recorded := s.recordCIAutomationUserMessage(ctx, session.TaskID, session.ID, params.ChatPrompt, params.Metadata)
	if !recorded {
		return ciAutomationDispatchResult{}, fmt.Errorf("failed to record CI automation user message")
	}
	if params.OnDirectAdmission != nil {
		if err := params.OnDirectAdmission(turnID); err != nil {
			return ciAutomationDispatchResult{}, err
		}
	}
	promptResult, promptErr := s.promptTask(ctx, session.TaskID, session.ID, params.ChatPrompt, "", false, nil, true, promptTaskOptions{
		onAccepted: func(acceptedTurnID string) {
			if params.OnAccepted != nil {
				params.OnAccepted(acceptedTurnID, "")
			}
		},
	})
	if promptErr != nil {
		if params.OnRejected != nil {
			params.OnRejected(turnID, "")
		}
		return ciAutomationDispatchResult{}, promptErr
	}
	if promptResult != nil && promptResult.TurnID != "" {
		turnID = promptResult.TurnID
	}
	return ciAutomationDispatchResult{kind: ciAutomationDispatchDirect, turnID: turnID}, nil
}

func (s *Service) replacePendingCIAutomationPrompt(
	ctx context.Context, session *models.TaskSession, params ciAutomationDispatchParams,
) (ciAutomationDispatchResult, bool, error) {
	if s.messageQueue == nil {
		return ciAutomationDispatchResult{}, false, nil
	}
	result, err := s.queueOrReplaceCIAutomationPrompt(ctx, session, params, false)
	if err == nil {
		return result, true, nil
	}
	if errors.Is(err, errCIAutoFixRoundCapReached) {
		return ciAutomationDispatchResult{}, false, nil
	}
	return ciAutomationDispatchResult{}, false, err
}

func (s *Service) queueOrReplaceCIAutomationPrompt(
	ctx context.Context, session *models.TaskSession, params ciAutomationDispatchParams, allowInsert bool,
) (ciAutomationDispatchResult, error) {
	if s.messageQueue == nil {
		return ciAutomationDispatchResult{}, fmt.Errorf("message queue is not configured")
	}
	var queued *messagequeue.QueuedMessage
	var replaced, accepted bool
	var err error
	if params.OnQueued == nil {
		queued, replaced, err = s.messageQueue.QueueMessageWithCoalesceKey(
			ctx, session.ID, session.TaskID, params.ChatPrompt, "", messagequeue.QueuedByWorkflow,
			false, nil, params.Metadata, params.CoalesceKey, allowInsert,
		)
		accepted = err == nil
	} else {
		queued, replaced, accepted, err = s.messageQueue.QueueLifecycleMessageWithCoalesceKeyAfterInsert(
			ctx, session.ID, session.TaskID, params.ChatPrompt, "", messagequeue.QueuedByWorkflow,
			false, nil, params.Metadata, params.CoalesceKey, allowInsert,
			func(_ context.Context, queued *messagequeue.QueuedMessage, replaced bool) error {
				return params.OnQueued(queued.ID, replaced)
			},
		)
	}
	if err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) && !allowInsert {
			return ciAutomationDispatchResult{}, errCIAutoFixRoundCapReached
		}
		return ciAutomationDispatchResult{}, err
	}
	if !accepted {
		return ciAutomationDispatchResult{}, messagequeue.ErrLifecycleCancelled
	}
	if queued == nil {
		return ciAutomationDispatchResult{}, fmt.Errorf("CI automation queue returned no entry")
	}
	s.publishQueueStatusEvent(ctx, session.ID)
	if replaced {
		return ciAutomationDispatchResult{kind: ciAutomationDispatchQueuedReplace, queueEntryID: queued.ID}, nil
	}
	return ciAutomationDispatchResult{kind: ciAutomationDispatchQueuedInsert, queueEntryID: queued.ID}, nil
}

// resolveAutoFixSession picks the session to receive the next auto-fix
// prompt: the previously used session (lastFixSessionID) when it can still
// receive one, otherwise the task's primary (or first) promptable active
// session. Shared between GitHub PR auto-fix and GitLab MR auto-fix (C5) —
// provider-specific checkpoint state stays in the caller, which passes in
// just the one field this needs.
func (s *Service) resolveAutoFixSession(ctx context.Context, taskID string, lastFixSessionID *string) (*models.TaskSession, error) {
	if lastFixSessionID != nil && strings.TrimSpace(*lastFixSessionID) != "" {
		session, err := s.repo.GetTaskSession(ctx, *lastFixSessionID)
		if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
			return nil, err
		}
		if session != nil && session.TaskID != taskID {
			return nil, fmt.Errorf("previous auto-fix session belongs to task %s", session.TaskID)
		}
		if ciAutomationSessionCanReceivePrompt(session) {
			return session, nil
		}
	}
	sessions, err := s.repo.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if ciAutomationSessionCanReceivePrompt(session) && session.IsPrimary {
			return session, nil
		}
	}
	for _, session := range sessions {
		if ciAutomationSessionCanReceivePrompt(session) {
			return session, nil
		}
	}
	return nil, fmt.Errorf("no active agent session for task: %s", taskID)
}

func ciAutomationSessionCanReceivePrompt(session *models.TaskSession) bool {
	if session == nil {
		return false
	}
	switch session.State {
	case models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateIdle:
		return true
	default:
		return false
	}
}

func (s *Service) recordCIAutomationUserMessage(ctx context.Context, taskID, sessionID, prompt string, meta map[string]interface{}) (string, bool) {
	if s.messageCreator == nil || prompt == "" {
		return "", false
	}
	turnID := s.getActiveTurnID(sessionID)
	if turnID == "" {
		s.startTurnForSession(ctx, sessionID)
		turnID = s.getActiveTurnID(sessionID)
	}
	if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, turnID, meta); err != nil {
		s.logger.Error("failed to create CI automation user message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return "", false
	}
	return turnID, true
}
