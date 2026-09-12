package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	stallCancelTurnButtonTestID = "stall-cancel-turn-button"
	metaKeyActionVisibility     = "action_visibility"
	actionVisibilityRunning     = "running"
	neverStartedNoticeContent   = "Agent produced no output since start."
	// neverStartedStopTimeout bounds the detached runtime teardown started by
	// stopNeverStartedExecution. A forced stop skips the graceful agentctl
	// call, so nothing downstream (e.g. the Docker backend's cleanup context)
	// applies its own bound in that path - an unresponsive daemon would
	// otherwise hang the teardown goroutine indefinitely.
	neverStartedStopTimeout = 30 * time.Second
)

// errAgentNeverStarted is the launch-failure error recorded when a stalled
// prompt's agent has emitted zero events since dispatch. Its Error() string
// is persisted as the session's error_message by recordSessionLaunchFailure;
// it is not a missing-branch error, so no legacy branch guidance is created.
var errAgentNeverStarted = errors.New("agent produced no output since start")

// handleAgentStalled persists an advisory recovery affordance for a turn that
// stalled after producing at least one agent event, leaving the prompt,
// session, and task lifecycle unchanged. When the agent has produced no
// output at all since the prompt was dispatched (payload.NeverStarted), the
// condition is terminal and unrecoverable: the notice is posted as an error
// and the session/task transition to FAILED via the same idempotent CAS path
// used for launch failures. The activity epoch prevents a late event from
// turning an old watchdog snapshot into a terminal failure.
func (s *Service) handleAgentStalled(ctx context.Context, payload lifecycle.AgentStalledPayload) {
	if s.messageCreator == nil || s.repo == nil || payload.TaskID == "" || payload.SessionID == "" {
		return
	}
	lock, release := s.acquireCancelInFlightGuard(payload.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if s.isCancelInFlight(payload.SessionID) {
		s.logger.Debug("ignoring agent stall while cancellation is in progress",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}
	session, err := s.repo.GetTaskSession(ctx, payload.SessionID)
	if err != nil || session == nil || session.State != models.TaskSessionStateRunning {
		return
	}
	generationOwner, ok := s.agentManager.(interface {
		OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
	})
	if !ok || payload.PromptGeneration == 0 || !generationOwner.OwnsPromptGeneration(
		payload.SessionID,
		payload.AgentExecutionID,
		payload.PromptGeneration,
	) {
		return
	}
	if payload.ActivityEpoch != 0 {
		activityOwner, ok := s.agentManager.(interface {
			OwnsPromptActivity(sessionID, executionID string, generation, activityEpoch uint64) bool
		})
		if !ok || !activityOwner.OwnsPromptActivity(
			payload.SessionID,
			payload.AgentExecutionID,
			payload.PromptGeneration,
			payload.ActivityEpoch,
		) {
			return
		}
	}
	turnID, err := s.peekActiveTurnID(ctx, payload.SessionID)
	if err != nil {
		return
	}
	if s.turnService != nil && turnID == "" {
		return
	}
	metadata := map[string]interface{}{
		metaKeySessionID: payload.SessionID,
		metaKeyTaskID:    payload.TaskID,
	}
	messageType := string(v1.MessageTypeStatus)
	if payload.NeverStarted {
		messageType = string(v1.MessageTypeError)
	} else {
		metadata[metaKeyActionVisibility] = actionVisibilityRunning
		metadata["actions"] = []map[string]interface{}{{
			actionMetaKeyType:   "ws_request",
			actionMetaKeyLabel:  "Cancel turn",
			actionMetaKeyTestID: stallCancelTurnButtonTestID,
			"params": map[string]interface{}{
				"method":  "agent.cancel",
				"payload": map[string]interface{}{"session_id": payload.SessionID},
			},
		}}
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx,
		payload.TaskID,
		stallNoticeContent(payload),
		payload.SessionID,
		messageType,
		turnID,
		metadata,
		false,
	); err != nil {
		s.logger.Warn("failed to create agent stall notice",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.Error(err))
	}
	if payload.NeverStarted {
		if s.recordSessionLaunchFailure(ctx, payload.TaskID, payload.SessionID, errAgentNeverStarted, session) {
			s.stopNeverStartedExecution(ctx, payload)
		} else {
			s.logger.Warn("skipping never-started teardown: session was not durably recorded FAILED",
				zap.String("task_id", payload.TaskID),
				zap.String("session_id", payload.SessionID))
		}
	}
}

// stopNeverStartedExecution tears down the process behind a never-started
// stall. It runs after the session and task have already been recorded
// FAILED, so a teardown failure never changes that recorded state: FAILED and
// its launch-failure message stay authoritative either way.
//
// Teardown ownership is claimed synchronously, while handleAgentStalled still
// holds the per-session cancelInFlight guard, via claimExecutionTeardown
// directly rather than through Executor's RegisterExecutionStopOwner
// callback: that callback TryLocks the same guard, so it would always fail to
// reentrantly acquire it and silently no-op. claimExecutionTeardown's own
// contract is to be called while already holding that guard.
//
// The actual stop runs detached and bounded (mirroring Executor.scheduleStop)
// rather than inline: handleAgentStalled's caller is the synchronous event
// bus, so a stop that blocked here would hold the cancelInFlight guard for
// its whole duration - blocking the user's own Stop/Cancel/Delete for this
// exact session, plus the event bus's dispatch lock. A forced stop skips the
// graceful agentctl call, so the runtime backend applies no bound of its own;
// neverStartedStopTimeout is this call's only protection against a hung
// daemon. lifecycle.ErrExecutionNotFound means nothing is running and is
// treated as success; any other failure is logged and left for a later stop
// to retry against the still-registered execution.
func (s *Service) stopNeverStartedExecution(ctx context.Context, payload lifecycle.AgentStalledPayload) {
	if payload.AgentExecutionID == "" || s.agentManager == nil {
		return
	}
	s.claimExecutionTeardown(payload.SessionID, payload.AgentExecutionID, executionTeardownIntentForce)
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), neverStartedStopTimeout)
	go func() {
		defer cancel()
		err := s.agentManager.StopAgentWithReason(stopCtx, payload.AgentExecutionID, "agent never started before stall", true)
		if err == nil || errors.Is(err, lifecycle.ErrExecutionNotFound) {
			return
		}
		s.logger.Warn("failed to tear down never-started execution after stall",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.String("execution_id", payload.AgentExecutionID),
			zap.Error(err))
	}()
}

func stallNoticeContent(payload lifecycle.AgentStalledPayload) string {
	if payload.NeverStarted {
		return neverStartedNoticeContent
	}
	tool := strings.TrimSpace(payload.ToolTitle)
	if tool == "" {
		tool = strings.TrimSpace(payload.ToolName)
	}
	if tool == "" {
		return "Still waiting for the agent."
	}
	return fmt.Sprintf("Still waiting on %s.", tool)
}
