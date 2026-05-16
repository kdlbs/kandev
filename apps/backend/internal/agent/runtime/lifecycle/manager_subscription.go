package lifecycle

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// WorkspacePollMode mirrors process.PollMode for the lifecycle layer. Defined
// here as a string to avoid importing the agentctl process package (and the
// surface area that comes with it). Values must stay aligned with
// process.PollMode{Fast,Slow,Paused}.
type WorkspacePollMode string

const (
	WorkspacePollModeFast   WorkspacePollMode = "fast"
	WorkspacePollModeSlow   WorkspacePollMode = "slow"
	WorkspacePollModePaused WorkspacePollMode = "paused"
)

// rank orders modes from coldest to hottest so we can pick the max across
// sessions sharing a workspace. paused < slow < fast.
func (m WorkspacePollMode) rank() int {
	switch m {
	case WorkspacePollModeFast:
		return 2
	case WorkspacePollModeSlow:
		return 1
	default:
		return 0
	}
}

// pushPollModeTimeout caps how long a single push to agentctl is allowed to
// block before we move on. Keeps the listener responsive even if a single
// agentctl is unreachable.
const pushPollModeTimeout = 5 * time.Second

// SessionModeQuerier is implemented by the gateway hub. It lets the lifecycle
// manager look up the current effective mode for a session, so when an
// execution becomes ready we can proactively push the right mode even when
// the gateway's subscribe/focus events fired before the execution existed.
type SessionModeQuerier interface {
	GetSessionMode(sessionID string) WorkspacePollMode
}

// workspacePollAggregator tracks the per-session mode contributions for each
// workspace and pushes the effective workspace mode to agentctl when it changes.
type workspacePollAggregator struct {
	mgr    *Manager
	hubQry SessionModeQuerier // nil until gateway wires it in
	mu     sync.Mutex
	// sessionModes maps sessionID -> latest mode reported by the gateway.
	sessionModes map[string]WorkspacePollMode
	// lastPushed maps workspacePath -> last mode we sent to agentctl. Used to
	// suppress duplicate pushes when workspace-level mode is unchanged.
	lastPushed map[string]WorkspacePollMode
	// workspaceSessions is a reverse index: workspacePath -> set of sessionIDs
	// known to belong to that workspace. Lets recordAndCompute iterate only
	// sessions in the affected workspace instead of scanning all sessionModes.
	workspaceSessions map[string]map[string]bool
}

// newWorkspacePollAggregator wires an aggregator to the lifecycle manager.
func newWorkspacePollAggregator(mgr *Manager) *workspacePollAggregator {
	return &workspacePollAggregator{
		mgr:               mgr,
		sessionModes:      make(map[string]WorkspacePollMode),
		lastPushed:        make(map[string]WorkspacePollMode),
		workspaceSessions: make(map[string]map[string]bool),
	}
}

// HandleSessionMode is the entry point called by the gateway hub when a
// session's effective UI mode transitions. Resolves the session to its
// workspace, aggregates with sibling sessions in the same workspace, and pushes
// the workspace-level effective mode to agentctl if it changed.
//
// Best-effort: errors are logged, never returned. The hub should not block on
// this (the call is debounced + computed off the hub critical path already).
//
// Pre-execution focus race: if a session.focus arrives before the lifecycle
// has created its execution, we cache the mode in sessionModes and return. The
// lifecycle manager calls FlushSessionMode once agentctl is ready, which
// resolves the race by pushing the cached mode retroactively.
func (a *workspacePollAggregator) HandleSessionMode(sessionID string, mode WorkspacePollMode) {
	execution, exists := a.mgr.GetExecutionBySessionID(sessionID)
	if !exists {
		// No execution yet — cache the mode so the next event can still
		// aggregate correctly. If the mode is paused there's nothing to
		// remember, so drop any prior entry to keep the map bounded.
		a.mu.Lock()
		if mode == WorkspacePollModePaused {
			delete(a.sessionModes, sessionID)
		} else {
			a.sessionModes[sessionID] = mode
		}
		a.mu.Unlock()
		return
	}
	if execution.WorkspacePath == "" {
		return
	}

	workspacePath, effective, changed := a.recordAndCompute(sessionID, mode, execution.WorkspacePath)
	if !changed {
		return
	}

	a.pushAsync(execution, workspacePath, effective)
}

// setSessionModeLocked updates sessionModes and the workspaceSessions reverse
// index for a single session. Caller must hold a.mu.
func (a *workspacePollAggregator) setSessionModeLocked(sessionID string, mode WorkspacePollMode, workspacePath string) {
	if mode == WorkspacePollModePaused {
		delete(a.sessionModes, sessionID)
		a.removeFromWorkspaceIndex(sessionID, workspacePath)
		return
	}
	a.sessionModes[sessionID] = mode
	if a.workspaceSessions[workspacePath] == nil {
		a.workspaceSessions[workspacePath] = make(map[string]bool)
	}
	a.workspaceSessions[workspacePath][sessionID] = true
}

// removeFromWorkspaceIndex drops a session from the reverse index, cleaning up
// the workspace key if empty. Caller must hold a.mu.
func (a *workspacePollAggregator) removeFromWorkspaceIndex(sessionID, workspacePath string) {
	sessions := a.workspaceSessions[workspacePath]
	if sessions == nil {
		return
	}
	delete(sessions, sessionID)
	if len(sessions) == 0 {
		delete(a.workspaceSessions, workspacePath)
	}
}

// recordAndCompute updates the per-session mode for the given workspace and
// returns the new workspace-effective mode, plus whether it changed since the
// last push. We compute this under a single lock so concurrent transitions in
// the same workspace can't observe inconsistent intermediate state.
//
// When a session goes to paused we drop its sessionModes entry so the map
// doesn't grow unbounded over a long-running gateway. Same for lastPushed
// when the workspace itself becomes paused.
func (a *workspacePollAggregator) recordAndCompute(sessionID string, mode WorkspacePollMode, workspacePath string) (string, WorkspacePollMode, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.setSessionModeLocked(sessionID, mode, workspacePath)

	// Compute effective mode using only sessions in this workspace (O(k)
	// where k = sessions in workspace, not O(N) total sessions).
	effective := WorkspacePollModePaused
	for sid := range a.workspaceSessions[workspacePath] {
		if m, ok := a.sessionModes[sid]; ok && m.rank() > effective.rank() {
			effective = m
		}
	}

	prev, hadPrev := a.lastPushed[workspacePath]
	if hadPrev && prev == effective {
		return workspacePath, effective, false
	}
	// Skip pushing paused when there's no prior entry — agentctl defaults to
	// slow, so sending paused to a workspace we've never pushed to is a no-op
	// RPC (and slightly misleading: agentctl would drop from slow to paused
	// even though the gateway never told it to go slow in the first place).
	if !hadPrev && effective == WorkspacePollModePaused {
		return workspacePath, effective, false
	}
	if effective == WorkspacePollModePaused {
		delete(a.lastPushed, workspacePath)
	} else {
		a.lastPushed[workspacePath] = effective
	}
	return workspacePath, effective, true
}

// pushAsync issues the SetWorkspacePollMode RPC to the agentctl client without
// blocking the caller (typically the hub's session-mode goroutine).
func (a *workspacePollAggregator) pushAsync(execution *AgentExecution, workspacePath string, mode WorkspacePollMode) {
	client := execution.GetAgentCtlClient()
	if client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pushPollModeTimeout)
		defer cancel()
		if err := client.SetWorkspacePollMode(ctx, string(mode)); err != nil {
			a.mgr.logger.Warn("failed to push workspace poll mode",
				zap.String("workspace", workspacePath),
				zap.String("mode", string(mode)),
				zap.Error(err))
		}
	}()
}

// FlushSessionMode resolves two races that leave agentctl in the wrong mode:
//
//  1. Pre-execution focus: the gateway sent focus before the execution was
//     in executionStore. HandleSessionMode cached the mode but short-circuited
//     without pushing.
//  2. Pre-ready push: the gateway sent focus after Add() but before agentctl's
//     HTTP server was accepting connections. pushAsync fired the RPC, it
//     failed (connection refused), but lastPushed was already updated so
//     nothing retries.
//
// This is called from the lifecycle manager's waitForAgentctlReady success
// path — once we know agentctl is accepting RPCs, force-resend whatever
// workspace-effective mode we currently believe is correct. No-op if nothing
// was cached (no gateway events reached us before flush).
func (a *workspacePollAggregator) FlushSessionMode(sessionID string) {
	execution, exists := a.mgr.GetExecutionBySessionID(sessionID)
	if !exists || execution.WorkspacePath == "" {
		return
	}

	// Authoritative source: query the hub for the session's current mode.
	// This closes the race where gateway events fired before the execution
	// was registered (nothing cached) or while agentctl was still starting
	// up (the RPC fired but connection-refused).
	//
	// Read hubQry under a.mu so the race detector doesn't flag the
	// concurrent write in SetSessionModeQuerier. Copy to a local and
	// release a.mu before calling into the hub (which acquires its own
	// locks — holding a.mu across the call could invert lock order).
	a.mu.Lock()
	qry := a.hubQry
	a.mu.Unlock()

	var sessionMode WorkspacePollMode
	if qry != nil {
		sessionMode = qry.GetSessionMode(sessionID)
	} else {
		// Fall back to cached value if no hub wired (e.g., tests).
		a.mu.Lock()
		cached, hasCached := a.sessionModes[sessionID]
		a.mu.Unlock()
		if !hasCached {
			return
		}
		sessionMode = cached
	}

	a.mu.Lock()
	// Merge the queried mode with whatever we had cached, then recompute
	// the workspace-level effective mode across all sessions in this
	// workspace.
	wp := execution.WorkspacePath
	a.setSessionModeLocked(sessionID, sessionMode, wp)
	effective := WorkspacePollModePaused
	for sid := range a.workspaceSessions[wp] {
		if m, ok := a.sessionModes[sid]; ok && m.rank() > effective.rank() {
			effective = m
		}
	}
	if effective == WorkspacePollModePaused {
		delete(a.lastPushed, wp)
	} else {
		a.lastPushed[wp] = effective
	}
	a.mu.Unlock()

	if effective != WorkspacePollModePaused {
		a.pushAsync(execution, execution.WorkspacePath, effective)
	}
}

// SetSessionModeQuerier lets the gateway inject itself so the aggregator can
// query the hub's live session mode state.
func (m *Manager) SetSessionModeQuerier(q SessionModeQuerier) {
	if m.pollAggregator != nil {
		m.pollAggregator.mu.Lock()
		m.pollAggregator.hubQry = q
		m.pollAggregator.mu.Unlock()
	}
}
