package orchestrator

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// turnActivity tracks, in memory, whether a session's open turn is being driven
// by the foreground agent (actively generating) or is merely held open while a
// spawned background task runs.
//
// It is the finer-grained signal behind checkSessionPromptable: a session whose
// foreground turn has yielded to background work should accept a new message
// even though its DB state still reads RUNNING. The single-scalar session state
// cannot tell "the foreground agent is generating" from "the foreground turn is
// idle but a spawned background task (a subagent, a run-in-background shell) is
// still running", which is how a long background job used to lock the operator
// out of the conversation.
//
// The absent/zero state means "foreground generating": callers default to the
// pre-existing behaviour (reject a new prompt while RUNNING) unless a background
// task has been explicitly registered for the session, so nothing changes for a
// session that has no background work outstanding.
type turnActivity struct {
	mu         sync.Mutex
	background map[string]struct{} // outstanding background/spawned tool-call IDs
	yielded    bool                // foreground handed off to background work
}

// turnActivityFor returns the per-session activity record, creating it when
// create is true. Returns nil when the record is absent and create is false.
func (s *Service) turnActivityFor(sessionID string, create bool) *turnActivity {
	if v, ok := s.foregroundActivity.Load(sessionID); ok {
		return v.(*turnActivity)
	}
	if !create {
		return nil
	}
	ta := &turnActivity{background: make(map[string]struct{})}
	actual, _ := s.foregroundActivity.LoadOrStore(sessionID, ta)
	return actual.(*turnActivity)
}

// markForegroundGenerating records that the foreground agent produced output
// (streamed a message/thinking chunk, or a fresh foreground prompt was
// dispatched), so the turn is once again driven by the foreground even if a
// background task is still outstanding. It returns true when this call actually
// flipped the session out of the background-idle substate, so the caller can
// publish the operator-facing activity signal only on a real transition.
func (s *Service) markForegroundGenerating(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	ta := s.turnActivityFor(sessionID, true)
	ta.mu.Lock()
	changed := ta.yielded
	ta.yielded = false
	ta.mu.Unlock()
	return changed
}

// registerBackgroundTask records a spawned background task (a subagent Task or a
// run-in-background shell). While at least one is outstanding, the foreground
// turn is treated as "waiting on background" rather than "actively generating".
// It returns true when this call flipped the session into the background-idle
// substate (i.e. it was foreground-generating before), so the caller publishes
// the activity signal only on the first background task, not every one.
func (s *Service) registerBackgroundTask(sessionID, toolCallID string) bool {
	if sessionID == "" || toolCallID == "" {
		return false
	}
	ta := s.turnActivityFor(sessionID, true)
	ta.mu.Lock()
	changed := !ta.yielded
	ta.background[toolCallID] = struct{}{}
	ta.yielded = true
	ta.mu.Unlock()
	return changed
}

// hasBackgroundTask reports whether toolCallID is already tracked as outstanding
// background work for the session. Used to make the tool_call_update
// registration path fire only on the first recognizable frame: re-registering
// on later updates would re-set `yielded` and clobber a foreground stream that
// marked the turn generating again (see markForegroundGenerating).
func (s *Service) hasBackgroundTask(sessionID, toolCallID string) bool {
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return false
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	_, ok := ta.background[toolCallID]
	return ok
}

// completeBackgroundTask clears a previously-registered background task. When no
// background task remains, the foreground turn is no longer "waiting on
// background". It returns true when clearing this task flipped the session back
// out of the background-idle substate (the last outstanding task finished),
// so the caller publishes the activity signal only on that final completion.
func (s *Service) completeBackgroundTask(sessionID, toolCallID string) bool {
	if sessionID == "" || toolCallID == "" {
		return false
	}
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return false
	}
	ta.mu.Lock()
	delete(ta.background, toolCallID)
	changed := false
	if len(ta.background) == 0 && ta.yielded {
		ta.yielded = false
		changed = true
	}
	ta.mu.Unlock()
	return changed
}

// clearTurnActivity drops all tracked activity for a session. Called from every
// turn-close path (turn complete, error, cancel, teardown) so a fresh turn
// starts from the "foreground generating" default.
func (s *Service) clearTurnActivity(sessionID string) {
	if sessionID == "" {
		return
	}
	s.foregroundActivity.Delete(sessionID)
}

// isForegroundTurnGenerating reports whether the session's foreground agent turn
// is actively generating. It returns true (generating) unless the turn has
// yielded to an outstanding background task. An untracked session defaults to
// true, preserving the historical "reject a new prompt while RUNNING" contract.
func (s *Service) isForegroundTurnGenerating(sessionID string) bool {
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return true
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	return !ta.yielded
}

// foregroundActivityValue reports the fine-grained busy substate of a session
// for the operator-facing signal: "generating" when the foreground turn is
// actively producing output (the default), "background" when it has yielded to
// outstanding spawned work. Only meaningful while the session state is RUNNING.
func (s *Service) foregroundActivityValue(sessionID string) v1.ForegroundActivity {
	if s.isForegroundTurnGenerating(sessionID) {
		return v1.ForegroundActivityGenerating
	}
	return v1.ForegroundActivityBackground
}

// ForegroundActivity exposes the in-memory fine-grained busy substate so the
// page-load / list serialization layer can stamp it onto a RUNNING session's
// DTO (ADR-0035). This is the same value that drives the
// live task_session.activity_changed WS event, read straight from the in-memory
// tracker — the single source of truth. There is no persisted copy: an untracked
// session (including every session after a backend restart, which ends the turn)
// reports the safe "generating" default, so a stale "you may type" can never be
// serialized. Callers must only stamp the result on RUNNING sessions; for every
// other state the coarse session state already tells the whole story.
func (s *Service) ForegroundActivity(sessionID string) v1.ForegroundActivity {
	return s.foregroundActivityValue(sessionID)
}

// publishForegroundActivityChanged emits the fine-grained busy signal so the
// web composer and status indicator can distinguish "generating" from "waiting
// on background work" without a coarse session-state transition. Callers invoke
// it only when a flip actually happened (the mark/register/complete helpers
// return that), so it never fires per background frame.
func (s *Service) publishForegroundActivityChanged(ctx context.Context, taskID, sessionID string) {
	if s.eventBus == nil || taskID == "" || sessionID == "" {
		return
	}
	eventData := map[string]interface{}{
		metaKeyTaskID:         taskID,
		metaKeySessionID:      sessionID,
		"foreground_activity": string(s.foregroundActivityValue(sessionID)),
	}
	if err := s.eventBus.Publish(ctx, events.TaskSessionActivityChanged,
		bus.NewEvent(events.TaskSessionActivityChanged, "task-session", eventData)); err != nil {
		s.logger.Warn("publish task_session.activity_changed failed",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// normalizedIsBackgroundTask reports whether a normalized tool payload represents
// spawned background work the foreground turn waits on: a subagent Task, a
// run-in-background shell command, or an active Claude Monitor watch.
//
// A create_task tool is deliberately NOT background — it spawns an independent
// task/session with its own lifecycle and does not hold the spawning turn open.
func normalizedIsBackgroundTask(n *streams.NormalizedPayload) bool {
	if n == nil {
		return false
	}
	if n.Kind() == streams.ToolKindSubagentTask {
		return true
	}
	if se := n.ShellExec(); se != nil && se.Background {
		return true
	}
	// A Monitor is a long-running watch the foreground turn is not actively
	// generating against. It normalizes to a Generic payload, so it is
	// recognized via the shared streams predicate rather than a tool-name match.
	if n.IsActiveMonitor() {
		return true
	}
	return false
}
