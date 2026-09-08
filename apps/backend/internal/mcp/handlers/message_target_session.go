package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// terminalSessionDispatchError is what message_task returns when the session it
// was asked to deliver to can never accept another turn.
//
// It names the recovery tool on purpose. A parent that stops a wedged child and
// then tries to restart it with a fresh prompt gets this error, and the bare
// "session is CANCELLED — cannot send message" gave it no way forward:
// stop_task_kandev is deliberately halt-only, and nothing in the failure said
// that spawn_session_kandev is what puts the task back to work.
func terminalSessionDispatchError(session *models.TaskSession) error {
	return fmt.Errorf(
		"session %s is %s — cannot send message; that session is terminal and cannot be resumed. "+
			"Use spawn_session_kandev to start a new session on the task, or list_task_sessions_kandev "+
			"to find a live session and pass its session_id",
		session.ID, session.State)
}

// isTerminalMessageTargetState reports whether a session can no longer receive a
// message, so the resolver never hands back a session the dispatcher is about
// to reject.
//
// COMPLETED belongs here even though dispatchTaskMessage's own switch refuses
// only CANCELLED and FAILED. The idle branch it falls into calls
// ProcessOnTurnStart, and the orchestrator's isTerminalSessionState — the one
// in event_handlers_streaming.go, NOT the executor's same-named function —
// counts COMPLETED as terminal and rejects the turn. Leaving it out let a
// retired COMPLETED sibling shadow an older RUNNING one during fallback: newest
// wins, the dispatcher then refuses it, and the message failed with a live
// session sitting right there. The set here is "states a turn can start on",
// which is the question the resolver is actually asking.
func isTerminalMessageTargetState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateCancelled ||
		state == models.TaskSessionStateFailed ||
		state == models.TaskSessionStateCompleted
}

// resolveMessageTargetSession picks the session a message_task call targets:
// the explicit session_id (validated to belong to taskID) or, when none was
// given, the task's live session. It also reports whether the choice is PINNED,
// meaning later dispatch must not re-resolve it. Returns a ready-to-send WS
// error message on failure.
//
// The default is the primary session, which is the session the task's own work
// runs in. When that primary is terminal (CANCELLED after stop_task_kandev,
// FAILED after a crash) the call used to fail outright even though another
// session on the task was running — a message aimed at "the task" bounced off a
// dead pointer while the agent that would have answered sat idle. The rule is
// therefore: prefer the primary, and fall back to the newest live session when
// the primary cannot take a message. The fallback also covers a task whose only
// sessions are spawned siblings (no primary row at all).
//
// A fallback is pinned, exactly like an explicit session_id. The idle dispatch
// path re-resolves an unpinned target to the task's primary session
// (resolveSessionAfterTaskMessageTurnStart), which for a fallback is the very
// cancelled session we routed around — so an unpinned fallback would work for a
// running target and bounce straight back to the dead primary for an idle one.
//
// It never overrides an explicit session_id: a caller that named a session gets
// that session or an error, never a silent redirect to a different agent.
func (h *Handlers) resolveMessageTargetSession(
	ctx context.Context, msg *ws.Message, taskID, sessionID string,
) (*models.TaskSession, bool, *ws.Message) {
	if sessionID != "" {
		session, errResp := h.resolveExplicitTargetSession(ctx, msg, taskID, sessionID)
		return session, true, errResp
	}
	primary, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil && !errors.Is(err, taskrepo.ErrNoPrimarySession) {
		return nil, false, wsError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to get session for task: "+err.Error())
	}
	if primary != nil && !isTerminalMessageTargetState(primary.State) {
		return primary, false, nil
	}
	live, listErr := h.newestLiveSession(ctx, taskID)
	if listErr != nil {
		return nil, false, wsError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to list sessions for task: "+listErr.Error())
	}
	if live != nil {
		return live, true, nil
	}
	if primary != nil {
		// A terminal primary with nothing live is handed to dispatch rather than
		// rejected here, so the caller still gets the documented
		// INTERNAL_ERROR for "target session FAILED/CANCELLED"
		// (docs/specs/tasks/requirements/parent-child-message-interrupt.md, "Errors").
		// Only the message improves; the code is a contract this change has no
		// business rewriting as a side effect.
		return primary, true, nil
	}
	return nil, false, wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound,
		"target task exists but has no active session — use spawn_session_kandev to start one")
}

// newestLiveSession returns the most recently started session on the task that
// can still receive a message, or nil when every session is terminal.
// ListTaskSessions orders by started_at DESC, so the first match is the newest.
func (h *Handlers) newestLiveSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	sessions, err := h.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if session != nil && !isTerminalMessageTargetState(session.State) {
			return session, nil
		}
	}
	return nil, nil
}

// resolveExplicitTargetSession loads and validates a caller-named target
// session for message_task. Only genuine no-row lookups are NotFound;
// transient DB errors surface as internal so callers keep retrying instead of
// treating a live session as gone.
func (h *Handlers) resolveExplicitTargetSession(ctx context.Context, msg *ws.Message, taskID, sessionID string) (*models.TaskSession, *ws.Message) {
	session, err := h.taskSvc.GetTaskSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target session not found: "+sessionID)
		}
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to look up target session: "+err.Error())
	}
	if session == nil {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target session not found: "+sessionID)
	}
	if session.TaskID != taskID {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id does not belong to task_id")
	}
	return session, nil
}
