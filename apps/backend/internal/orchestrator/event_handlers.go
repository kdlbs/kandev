// Package orchestrator provides event handler methods for the orchestrator service.
package orchestrator

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// Agent event type string constants.
const (
	agentEventComplete   = "complete"
	agentEventCompleted  = "completed"
	agentEventError      = "error"
	agentEventToolCall   = "tool_call"
	agentEventToolUpdate = "tool_update"
	agentEventFailed     = "failed"
)

// toolKindToMessageType maps the normalized tool kind to a frontend message type.
func toolKindToMessageType(normalized *streams.NormalizedPayload) string {
	if normalized == nil {
		return "tool_call"
	}
	return normalized.Kind().ToMessageType()
}

// Event handlers

func (s *Service) handleTaskDeleted(ctx context.Context, data watcher.TaskEventData) {
	s.scheduler.RemoveTask(data.TaskID)
	s.clearParkedProjectionOnTaskDeleted(data.TaskID)
	if closer, ok := s.automationService.(deferredAutomationRunCloser); ok {
		if err := closer.MarkDeferredRunFailedByTaskID(
			ctx, data.TaskID, "task deleted before deferred automation start",
		); err != nil {
			s.logger.Warn("failed to close deferred automation run after task deletion",
				zap.String("task_id", data.TaskID), zap.Error(err))
		}
	}
}

func (s *Service) handleACPSessionCreated(ctx context.Context, data watcher.ACPSessionEventData) {
	if data.SessionID == "" || data.ACPSessionID == "" {
		return
	}
	guard := s.lockCancelInFlightGuard(data.SessionID)
	defer guard.release()
	// A cancellation operation has already claimed the session. Let its owner
	// invalidate the startup identity before accepting any late provider event.
	if s.currentCancellation(data.SessionID) != nil || !s.resumeAttemptAllowsExecution(data.SessionID, data.AgentExecutionID, data.AttemptID) {
		return
	}
	if data.DeliveryHarnessGeneration > 0 {
		session, err := s.repo.GetTaskSession(ctx, data.SessionID)
		if err != nil || !harnessEventMatchesSession(data, session) {
			return
		}
	}
	tokenAccepted := s.storeResumeToken(ctx, data.TaskID, data.SessionID, data.AgentExecutionID, data.ACPSessionID, "", data.AttemptID)
	s.persistInitialHarnessGeneration(ctx, data, tokenAccepted)
}

// persistInitialHarnessGeneration records the first native conversation for a
// session incarnation, or the successor created by an explicit fresh start.
// SQL revalidates execution ownership atomically with the predecessor CAS.
// Explicit context continuation owns its generation CAS separately.
func (s *Service) persistInitialHarnessGeneration(ctx context.Context, data watcher.ACPSessionEventData, tokenAccepted bool) {
	if !tokenAccepted || data.DeliveryHarnessGeneration == 0 || s.repo == nil {
		return
	}
	store, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		if err != nil {
			s.logger.Warn("failed to load session for harness generation",
				zap.String("session_id", data.SessionID), zap.Error(err))
		}
		return
	}
	incarnationID := harnessIncarnationID(data, session)
	if !harnessEventMatchesSession(data, session) {
		return
	}
	if current, currentErr := store.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnationID); currentErr == nil && current != nil {
		if isAdjacentReplacement(current, data) {
			s.commitReplacementHarnessGeneration(ctx, session, current, data)
		}
		return
	} else if currentErr != nil && !errors.Is(currentErr, models.ErrTaskSessionNotFound) {
		s.logger.Warn("failed to inspect current harness generation",
			zap.String("session_id", data.SessionID), zap.Error(currentErr))
		return
	}
	s.persistFirstHarnessGeneration(ctx, session, incarnationID, data)
}

func (s *Service) persistFirstHarnessGeneration(
	ctx context.Context, session *models.TaskSession, incarnationID string, data watcher.ACPSessionEventData,
) {
	now := time.Now().UTC()
	committed, err := s.commitOwnedHarnessGeneration(ctx, data, &models.HarnessSessionGeneration{
		SessionID:         session.ID,
		IncarnationID:     incarnationID,
		Generation:        int64(data.DeliveryHarnessGeneration),
		NativeSessionID:   data.ACPSessionID,
		AgentType:         session.AgentProfileID,
		OriginalWorkspace: session.WorkspacePath,
		CurrentWorkspace:  session.WorkspacePath,
		CreationReason:    "native_started",
		CreatedAt:         now,
		CommittedAt:       now,
	}, 0)
	if err != nil || !committed {
		s.logger.Warn("failed to persist initial harness generation",
			zap.String("session_id", data.SessionID),
			zap.Int64("generation", int64(data.DeliveryHarnessGeneration)),
			zap.Error(err))
	}
}

// harnessIncarnationID resolves the incarnation that owns the event's harness
// generation, preferring the delivery identity carried by the event.
func harnessIncarnationID(data watcher.ACPSessionEventData, session *models.TaskSession) string {
	if data.DeliveryIncarnationID != "" {
		return data.DeliveryIncarnationID
	}
	if session.QueueIncarnationID != "" {
		return session.QueueIncarnationID
	}
	return session.ID
}

func harnessEventMatchesSession(data watcher.ACPSessionEventData, session *models.TaskSession) bool {
	if session == nil || data.TaskID != session.TaskID || data.AgentExecutionID == "" {
		return false
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	return harnessIncarnationID(data, session) == incarnationID
}

// isAdjacentReplacement reports whether the event is a fresh-start replacement
// for the generation directly after the committed one.
func isAdjacentReplacement(current *models.HarnessSessionGeneration, data watcher.ACPSessionEventData) bool {
	return data.DeliveryReplacesNativeConversation && uint64(current.Generation)+1 == data.DeliveryHarnessGeneration
}

// commitReplacementHarnessGeneration records a fresh-start native conversation
// as the direct successor of the committed generation. The compare-and-set
// keeps a concurrent replacement or continuation authoritative.
func (s *Service) commitReplacementHarnessGeneration(
	ctx context.Context,
	session *models.TaskSession,
	current *models.HarnessSessionGeneration,
	data watcher.ACPSessionEventData,
) {
	now := time.Now().UTC()
	committed, err := s.commitOwnedHarnessGeneration(ctx, data, &models.HarnessSessionGeneration{
		SessionID:             session.ID,
		IncarnationID:         current.IncarnationID,
		Generation:            current.Generation + 1,
		PredecessorGeneration: current.Generation,
		NativeSessionID:       data.ACPSessionID,
		AgentType:             session.AgentProfileID,
		OriginalWorkspace:     session.WorkspacePath,
		CurrentWorkspace:      session.WorkspacePath,
		CreationReason:        "native_replaced",
		CreatedAt:             now,
		CommittedAt:           now,
	}, current.Generation)
	if err != nil || !committed {
		s.logger.Warn("failed to persist replacement harness generation",
			zap.String("session_id", data.SessionID),
			zap.Int64("generation", current.Generation+1),
			zap.Error(err))
	}
}

type harnessGenerationExecutionStore interface {
	CommitHarnessSessionGenerationForExecution(context.Context, *models.HarnessSessionGeneration, int64, string, string) (bool, error)
}

func (s *Service) commitOwnedHarnessGeneration(
	ctx context.Context, data watcher.ACPSessionEventData, generation *models.HarnessSessionGeneration, expectedGeneration int64,
) (bool, error) {
	store, ok := s.repo.(harnessGenerationExecutionStore)
	if !ok {
		return false, errors.New("execution-owned harness generation store is unavailable")
	}
	return store.CommitHarnessSessionGenerationForExecution(ctx, generation, expectedGeneration, data.TaskID, data.AgentExecutionID)
}

// storeResumeToken stores an agent's session ID as the resume token for session recovery.
// Called from handleACPSessionCreated, handleSessionStatusEvent, and handleCompleteStreamEvent.
//
// expectedExecID identifies the lifecycle execution that emitted the event. The repo
// performs a CAS update keyed on agent_execution_id: if the row has rotated to a
// different execution since this event was queued, the update is rejected with
// models.ErrExecutionRotated and the stale token is silently dropped. This prevents
// a previous execution's tail-end events (post model-switch / context-reset) from
// overwriting the live execution's resume_token with a defunct ACP session ID.
//
// Pass expectedExecID="" only when the caller genuinely doesn't care which execution
// the event came from (no callers should today; the parameter exists for forward
// compatibility with paths that don't have an execution context).
//
// The token is always stored when CAS succeeds. NativeSessionResume only gates ACP
// session/load vs session/new in session.go — agents without native resume (e.g.,
// Claude Code) use the token for their own --resume CLI flag instead.
//
// It reports whether this token write was accepted. Later generation writes
// must revalidate ownership in their own transaction.
func (s *Service) storeResumeToken(ctx context.Context, taskID, sessionID, expectedExecID, acpSessionID, lastMessageUUID string, origin ...string) bool {
	if !s.resumeAttemptAllowsExecution(sessionID, expectedExecID, origin...) {
		s.logger.Info("dropping resume token from cancelled or superseded resume attempt",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID),
			zap.String("resume_token", acpSessionID))
		return false
	}
	// The lifecycle manager updates its in-memory ACP session ID before it
	// publishes reset/start events. Events from the previous ACP session can
	// still be queued after that point, so reject those events before the
	// execution-level CAS. The execution ID alone does not identify an ACP
	// session generation because context resets keep the same execution.
	if currentACPSessionID := s.currentACPSessionID(sessionID); currentACPSessionID != "" &&
		acpSessionID != "" && acpSessionID != currentACPSessionID {
		s.logger.Info("dropping resume token from stale ACP session generation",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID),
			zap.String("resume_token", acpSessionID),
			zap.String("current_resume_token", currentACPSessionID))
		return false
	}

	err := s.repo.UpdateResumeToken(ctx, sessionID, expectedExecID, acpSessionID, lastMessageUUID)
	switch {
	case err == nil:
		s.logger.Debug("stored resume token for session",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID),
			zap.String("resume_token", acpSessionID),
			zap.String("last_message_uuid", lastMessageUUID))
		// The CAS above proved the token belongs to the session's current
		// execution — mirror it into the session's durable "acp" metadata
		// too. executors_running rows are operational state and get pruned;
		// without this, the ACP session id only survives for agents that
		// happen to emit a session_info frame (handleSessionInfoEvent).
		s.persistACPSessionID(ctx, sessionID, acpSessionID)
		return true
	case errors.Is(err, models.ErrExecutionRotated):
		// The row's agent_execution_id has rotated since the event was emitted —
		// the token belongs to a defunct execution and must be dropped. The new
		// execution will emit its own ACP session created event and a fresh
		// storeResumeToken will succeed.
		s.logger.Info("dropping resume token from rotated execution",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID))
	case errors.Is(err, models.ErrExecutorRunningNotFound):
		// No executors_running row for this session. With the lifecycle-manager-
		// owned persistence model this should never happen for a live execution
		// (the row is created in lockstep with the in-memory Add). Most likely
		// causes: a session was torn down between event emission and handling,
		// or a code path that emits ACP events without a registered execution.
		// Logged loud; nothing to retry.
		s.logger.Warn("no executors_running row when storing resume token",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID))
	default:
		s.logger.Warn("failed to persist resume token for session",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	return false
}

// currentACPSessionID returns the lifecycle manager's current ACP session ID
// when the concrete manager exposes it. The optional seam keeps the generic
// AgentManagerClient contract unchanged for remote clients and test doubles.
func (s *Service) currentACPSessionID(sessionID string) string {
	provider, ok := s.agentManager.(interface {
		GetACPSessionIDForSession(string) (string, bool)
	})
	if !ok {
		return ""
	}
	acpSessionID, ok := provider.GetACPSessionIDForSession(sessionID)
	if !ok {
		return ""
	}
	return acpSessionID
}

// persistACPSessionID mirrors the agent's ACP session id into the session's
// "acp" metadata map. Best-effort: resume correctness never depends on this
// copy — it exists so the id survives executors_running cleanup for consumers
// that join sessions to agent-CLI artifacts (e.g. transcript-based usage
// stats). The repo performs the mirror as a single guarded UPDATE: it merges
// into the existing acp map (preserving session_info keys), skips when the
// stored id is already current, and only writes while executors_running still
// holds acpSessionID as the resume token — so a stale event from a rotated
// execution can never overwrite the live execution's id.
func (s *Service) persistACPSessionID(ctx context.Context, sessionID, acpSessionID string) {
	if sessionID == "" || acpSessionID == "" || s.repo == nil {
		return
	}
	if _, err := s.repo.SetSessionACPSessionID(ctx, sessionID, acpSessionID); err != nil {
		s.logger.Warn("failed to persist ACP session id to session metadata",
			zap.String("session_id", sessionID),
			zap.String("acp_session_id", acpSessionID),
			zap.Error(err))
	}
}
