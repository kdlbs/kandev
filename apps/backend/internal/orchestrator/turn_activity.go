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

// turnActivity tracks foreground ownership and spawned background-work liveness
// independently. Foreground activity always has precedence; background work is
// visible only after an explicit foreground-idle boundary.
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

	// promptInFlight marks an admitted prompt that has claimed the foreground turn
	// but has not yet been handed to the agent. It is deliberately independent of
	// `yielded`: during that window the session must stay un-promptable no matter
	// what happens to the background set. Otherwise a background tool_call landing
	// mid-admission (registerBackgroundTask re-sets `yielded`) would reopen the gate
	// under the in-flight prompt and let a second one through — two prompts reaching
	// one ACP session, which is the exact overlap the claim exists to prevent.
	promptInFlight  bool
	claimGeneration uint64 // identifies the current admission owner
	foregroundEpoch uint64 // increments on genuine foreground output

	// foregroundGeneration identifies the prompt cycle that currently owns the
	// foreground. Provider-idle records that immutable generation so a delayed
	// completion from the old cycle cannot yield a successor cycle that has since
	// claimed the same session.
	foregroundGeneration           uint64
	pendingCompletionGeneration    uint64
	hasPendingCompletionGeneration bool
	completedGeneration            uint64
	hasCompletedGeneration         bool
}

type foregroundYieldSource uint8

const (
	foregroundYieldProviderIdle foregroundYieldSource = iota
	foregroundYieldTurnCompletion
)

// foregroundClaim binds admission to the exact activity record and generation
// it claimed. The foreground epoch separately detects output that makes a failed
// prompt's background-idle restoration stale.
type foregroundClaim struct {
	activity        *turnActivity
	claimGeneration uint64
	foregroundEpoch uint64
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
	if changed {
		ta.foregroundGeneration++
	}
	// Real foreground output redefines ownership of the turn: it invalidates any
	// outstanding claim's epoch, so a prompt that later fails cannot release the
	// gate back open on top of a foreground that is now genuinely generating.
	ta.foregroundEpoch++
	ta.mu.Unlock()
	return changed
}

// markForegroundIdle records an explicit provider boundary proving the current
// foreground model cycle ended. Outstanding background work becomes the visible
// activity only when no prompt admission claim is still in flight.
func (s *Service) markForegroundIdle(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return false
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	return ta.markForegroundIdleLocked()
}

func (ta *turnActivity) markForegroundIdleLocked() bool {
	if ta.promptInFlight || len(ta.background) == 0 || ta.yielded {
		return false
	}
	ta.yielded = true
	return true
}

// yieldForegroundAndPublish records the foreground-to-background transition and
// publishes both operator-facing activity signals exactly once. taskID may be
// omitted by lifecycle callers that only have a session ID. Resolve identity
// before touching the tracker so a transient lookup failure cannot consume an
// otherwise publishable transition. The source-specific generation check and
// mutation happen atomically; publication happens after the lock is released.
func (s *Service) yieldForegroundAndPublish(
	ctx context.Context,
	taskID, sessionID string,
	source foregroundYieldSource,
) {
	if taskID == "" {
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil {
			s.logger.Warn("resolve task for foreground activity publish failed",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}
		taskID = session.TaskID
	}
	if !s.transitionForegroundToBackground(sessionID, source) {
		return
	}
	s.publishForegroundActivityChanged(ctx, taskID, sessionID)
}

func (s *Service) transitionForegroundToBackground(sessionID string, source foregroundYieldSource) bool {
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return false
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if source == foregroundYieldProviderIdle {
		if !ta.hasCompletedGeneration || ta.completedGeneration != ta.foregroundGeneration {
			ta.pendingCompletionGeneration = ta.foregroundGeneration
			ta.hasPendingCompletionGeneration = true
		}
		return ta.markForegroundIdleLocked()
	}

	completionGeneration := ta.foregroundGeneration
	if ta.hasPendingCompletionGeneration {
		completionGeneration = ta.pendingCompletionGeneration
		ta.hasPendingCompletionGeneration = false
	}
	if ta.hasCompletedGeneration && ta.completedGeneration == completionGeneration {
		return false
	}
	ta.completedGeneration = completionGeneration
	ta.hasCompletedGeneration = true
	if completionGeneration != ta.foregroundGeneration {
		return false
	}
	return ta.markForegroundIdleLocked()
}

// registerBackgroundTask records a spawned background task (a subagent Task or a
// run-in-background shell). Registration alone is not evidence that the
// foreground yielded: launch frames can arrive while the top-level agent is
// still generating, and foreground activity must retain precedence. A later
// foreground-idle boundary exposes the outstanding work.
func (s *Service) registerBackgroundTask(sessionID, toolCallID string) {
	if sessionID == "" || toolCallID == "" {
		return
	}
	ta := s.turnActivityFor(sessionID, true)
	ta.mu.Lock()
	ta.background[toolCallID] = struct{}{}
	ta.mu.Unlock()
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

// completeOneBackgroundTask retires one outstanding workload when the provider
// reports a task-notification completion without exposing its task ID. The
// indicator only depends on whether any work remains, so arbitrary retirement
// is safe; one completion can never clear more than one registration.
func (s *Service) completeOneBackgroundTask(sessionID string) bool {
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return false
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	for toolCallID := range ta.background {
		delete(ta.background, toolCallID)
		if len(ta.background) == 0 && ta.yielded {
			ta.yielded = false
			return true
		}
		return false
	}
	return false
}

// clearTurnActivity drops all tracked activity for a session. Foreground turn
// completion deliberately does not call it because detached work may outlive
// that turn; execution teardown and session removal do.
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
//
// This is a pure predicate: checkSessionPromptable and the DTO/WS serializers
// call it to *report* promptability. A caller that is about to actually drive a
// turn on the strength of the answer must use claimForegroundTurn instead, or it
// races every other prompt reading the same window.
func (s *Service) isForegroundTurnGenerating(sessionID string) bool {
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return true
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	return ta.generatingLocked()
}

// generatingLocked is the single definition of "the foreground turn is busy".
// An admitted-but-not-yet-dispatched prompt counts as busy in its own right: until
// it reaches the agent the session must not admit another, regardless of what the
// background set does underneath it.
func (ta *turnActivity) generatingLocked() bool {
	if ta.promptInFlight {
		return true
	}
	return !ta.yielded
}

// claimForegroundTurn is the check-and-claim half of the background-idle gate.
// It atomically verifies the session's foreground turn has yielded to background
// work and, if so, takes it for the caller by flipping it back to generating —
// under the same lock, so exactly one caller can win.
//
// checkSessionPromptable only *reads* the substate, which leaves a wide
// check-then-act window in PromptTask: between the gate and the point the turn is
// finally marked generating sit a session reload, ensureSessionRunning, and an
// optional (network-bound) model switch. Two prompts arriving in that window —
// a double-send, or two browser tabs onto the same background-idle session —
// would both pass the read-only gate and both reach executor.Prompt, starting
// overlapping turns on one ACP session. Claiming closes that window: the first
// prompt in wins, and every prompt behind it sees a generating foreground and is
// rejected with ErrAgentPromptInProgress exactly as it would have been before
// ADR-0049.
//
// The claim is held until agentctl accepts the prompt (completeForegroundClaim)
// or it is handed back (releaseForegroundClaim). The returned token binds both
// operations to this activity record and admission generation.
//
// Returns nil for an untracked session — no background work is outstanding, so
// there is nothing to claim and the historical reject-while-RUNNING default
// stands — and for a session that already has a prompt in flight.
func (s *Service) claimForegroundTurn(sessionID string) *foregroundClaim {
	if sessionID == "" {
		return nil
	}
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return nil
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	if ta.generatingLocked() {
		return nil
	}
	ta.yielded = false
	ta.promptInFlight = true
	ta.claimGeneration++
	return &foregroundClaim{
		activity:        ta,
		claimGeneration: ta.claimGeneration,
		foregroundEpoch: ta.foregroundEpoch,
	}
}

// isForegroundClaimCurrent reports whether claim still owns sessionID's prompt
// admission window. The guarded session-state recheck uses this to distinguish
// its own claim from a competing prompt that has already taken the foreground.
func (s *Service) isForegroundClaimCurrent(sessionID string, claim *foregroundClaim) bool {
	if sessionID == "" || claim == nil || claim.activity == nil {
		return false
	}
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil || ta != claim.activity {
		return false
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	return ta.promptInFlight && ta.claimGeneration == claim.claimGeneration
}

// completeForegroundClaim ends the admission window: the prompt has been handed to
// the agent, so this is now an ordinary foreground turn and the background set
// governs promptability again (a subagent spawned by *this* turn may legitimately
// yield it back to background-idle). It reports whether background work hidden by
// the active claim became visible, so the caller can publish that transition.
func (s *Service) completeForegroundClaim(claim *foregroundClaim) bool {
	if claim == nil || claim.activity == nil {
		return false
	}
	ta := claim.activity
	ta.mu.Lock()
	defer ta.mu.Unlock()
	if !ta.promptInFlight || ta.claimGeneration != claim.claimGeneration {
		return false
	}
	ta.promptInFlight = false
	ta.foregroundGeneration++
	return ta.yielded
}

// releaseForegroundClaim hands a claimForegroundTurn claim back when the prompt
// it was taken for never made it to the agent (ensureSessionRunning failed, the
// model switch failed). Without it the session would sit in RUNNING advertising a
// generating foreground it does not have, locking the operator out for the rest
// of the turn — the exact lockout ADR-0049 exists to remove.
//
// It reports whether the turn was actually handed back to background-idle, so the
// caller can broadcast the restored substate. Two things stop a release from
// opening the gate when it shouldn't:
//
//   - The epoch. If the agent's foreground streamed real output while this prompt
//     was in preflight, markForegroundGenerating bumped the epoch: the turn is
//     genuinely generating now, and handing it back to background-idle would let a
//     second prompt overlap a live turn.
//   - The background set. If the last background task finished while the failing
//     prompt was in flight, nothing is outstanding and the generating default is
//     correct.
func (s *Service) releaseForegroundClaim(claim *foregroundClaim) bool {
	if claim == nil || claim.activity == nil {
		return false
	}
	ta := claim.activity
	ta.mu.Lock()
	defer ta.mu.Unlock()
	if !ta.promptInFlight || ta.claimGeneration != claim.claimGeneration {
		return false
	}
	ta.promptInFlight = false
	if ta.foregroundEpoch != claim.foregroundEpoch || len(ta.background) == 0 {
		return false
	}
	ta.yielded = true
	return true
}

// foregroundActivityValue reports the fine-grained busy substate of a session
// for the operator-facing signal: "generating" when the foreground turn is
// actively producing output (the default), "background" when it has yielded to
// outstanding spawned work. Background activity can remain meaningful after the
// coarse session state settles because detached work can outlive a turn.
func (s *Service) foregroundActivityValue(sessionID string) v1.ForegroundActivity {
	if s.isForegroundTurnGenerating(sessionID) {
		return v1.ForegroundActivityGenerating
	}
	return v1.ForegroundActivityBackground
}

// ForegroundActivity exposes the in-memory fine-grained busy substate so the
// page-load / list serialization layer can stamp it onto a session DTO
// (ADR-0049). This is the same value that drives the
// live task_session.activity_changed WS event, read straight from the in-memory
// tracker — the single source of truth. There is no persisted copy. Callers emit
// generating only for RUNNING sessions, but may emit background for a settled
// session while its connected execution still has detached work.
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
	// Propagate the flip to the task-level aggregate so at-a-glance task surfaces
	// (board card, task list) update live; emits task.updated only when the
	// task-level three-state value actually changes (§spec:task-level-indicator).
	s.publishTaskActivityIfChanged(ctx, taskID)
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

// normalizedIsDetachedLaunch distinguishes a launch tool completing from its
// asynchronously-running workload completing.
func normalizedIsDetachedLaunch(n *streams.NormalizedPayload) bool {
	if n == nil {
		return false
	}
	if subagent := n.SubagentTask(); subagent != nil {
		return subagent.IsAsync
	}
	if shell := n.ShellExec(); shell != nil {
		return shell.Background
	}
	return false
}
