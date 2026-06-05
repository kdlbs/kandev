package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// subscribeStepCompletionEvents wires the ADR 0015 out-of-band subscriber
// for `step_complete_kandev` signals that arrive after the agent's turn
// already ended. Safe to call when the feature is gated off — the
// subscriber's gating check short-circuits on every event in that case.
func (s *Service) subscribeStepCompletionEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.WorkflowStepCompletionSignaled, s.handleStepCompletionSignaled); err != nil {
		s.logger.Error("failed to subscribe to workflow.step_completion_signaled events", zap.Error(err))
	}
}

// handleStepCompletionSignaled adapts the bus.Subscribe callback signature
// (returns error) to onStepCompletionSignaled, which does its own logging
// and does not surface errors to the bus.
func (s *Service) handleStepCompletionSignaled(ctx context.Context, event *bus.Event) error {
	s.onStepCompletionSignaled(ctx, event)
	return nil
}

// stringFromAny narrows an `interface{}` slot to a string, returning "" when
// the value is absent or of a different type. Used to decode event-payload
// scalars (which always arrive typed as `map[string]interface{}`).
func stringFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// clearPendingStepSignal removes the pending bag entry from the session's
// metadata, both in-memory (so callers operating on the same struct see it
// gone) and in the DB (so a later reload doesn't resurrect a stale entry).
// Best-effort: on DB error the in-memory mutation still wins, since the
// orchestrator's read uses the in-memory copy for the rest of the turn.
func (s *Service) clearPendingStepSignal(ctx context.Context, session *models.TaskSession) {
	if session == nil {
		return
	}
	if session.Metadata != nil {
		delete(session.Metadata, models.SessionMetaKeyPendingStepCompletion)
	}
	s.clearPendingStepSignalByID(ctx, session.ID)
}

// clearPendingStepSignalByID issues only the DB write — skip when the
// caller has no in-memory session struct to update (or already discarded
// it). Same best-effort failure handling as clearPendingStepSignal.
func (s *Service) clearPendingStepSignalByID(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	if err := s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, nil); err != nil {
		s.logger.Debug("clearPendingStepSignal: failed to persist nil bag entry",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

// onStepCompletionSignaled subscribes to events.WorkflowStepCompletionSignaled
// to handle the case where the agent's `step_complete_kandev` call lands
// AFTER the turn already ended — at that point processOnTurnCompleteViaEngine
// has already setSessionWaitingForInput. The subscriber re-triggers the
// transition pipeline so the gated step finally advances.
//
// Happy path (call lands before turn-end): the bag is already populated by
// the time processOnTurnCompleteViaEngine runs, the gating check passes,
// and the transition fires inline — the bus event arrives later and finds
// nothing to do (bag already cleared by the transition). Idempotent.
func (s *Service) onStepCompletionSignaled(ctx context.Context, event *bus.Event) {
	taskID, sessionID, stepID, ok := parseStepCompletionEvent(event)
	if !ok {
		if event != nil && event.Data != nil {
			if _, isMap := event.Data.(map[string]interface{}); !isMap {
				s.logger.Warn("onStepCompletionSignaled: unexpected event payload type")
			}
		}
		return
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("onStepCompletionSignaled: failed to load session",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	// If the session is still running (turn hasn't ended yet) the inline
	// turn-end check will pick the signal up — no out-of-band work needed.
	// Only act on signals that arrive while the session is waiting.
	if session.State != models.TaskSessionStateWaitingForInput {
		return
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("onStepCompletionSignaled: failed to load task",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if task.WorkflowStepID != stepID {
		s.logger.Debug("onStepCompletionSignaled: signal stale (step changed)",
			zap.String("signal_step", stepID), zap.String("current_step", task.WorkflowStepID))
		// Only clear the bag when its current contents are themselves
		// stale (matching THIS subscriber's stepID). Re-load the session
		// before checking: the local `session` snapshot was taken when
		// the subscriber fired and may be older than a concurrent
		// write from the new step's agent. Without the reload the
		// in-memory check could see a stale signal that's already been
		// cleared/replaced and erase a freshly-written valid bag entry.
		latestSession, loadErr := s.repo.GetTaskSession(ctx, sessionID)
		if loadErr != nil {
			return
		}
		if existing, ok := models.LoadPendingStepSignal(latestSession.Metadata); ok && existing.StepID == stepID {
			s.clearPendingStepSignal(ctx, latestSession)
		}
		return
	}

	if !s.stepIsSignalGated(ctx, task.WorkflowStepID) {
		return
	}

	// Drive the transition via the engine path. It will re-read the bag
	// and consume it through the same code path the inline turn-end uses.
	s.processOnTurnCompleteViaEngine(ctx, taskID, session)
}

// parseStepCompletionEvent extracts (task_id, session_id, step_id) from a
// step-completion bus event. Returns ok=false when the event is nil/empty,
// the payload isn't a map, or any required ID is missing.
func parseStepCompletionEvent(event *bus.Event) (taskID, sessionID, stepID string, ok bool) {
	if event == nil || event.Data == nil {
		return "", "", "", false
	}
	data, isMap := event.Data.(map[string]interface{})
	if !isMap {
		return "", "", "", false
	}
	taskID = stringFromAny(data["task_id"])
	sessionID = stringFromAny(data["session_id"])
	stepID = stringFromAny(data["step_id"])
	if taskID == "" || sessionID == "" || stepID == "" {
		return "", "", "", false
	}
	return taskID, sessionID, stepID, true
}

// stepIsSignalGated reports whether the given workflow step opts in to
// the ADR-0015 explicit-signal gate. False on a missing step-getter or
// any lookup error — those land the subscriber on the safe side (don't
// auto-advance a step we can't classify).
func (s *Service) stepIsSignalGated(ctx context.Context, stepID string) bool {
	if s.workflowStepGetter == nil {
		return false
	}
	currentStep, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil || currentStep == nil {
		s.logger.Debug("onStepCompletionSignaled: cannot load current step, skipping",
			zap.String("step_id", stepID), zap.Error(err))
		return false
	}
	if !currentStep.AutoAdvanceRequiresSignal {
		s.logger.Debug("onStepCompletionSignaled: step is not signal-gated, ignoring",
			zap.String("step_id", currentStep.ID))
		return false
	}
	return true
}
