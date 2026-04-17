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

// workspacePollAggregator tracks the per-session mode contributions for each
// workspace and pushes the effective workspace mode to agentctl when it changes.
type workspacePollAggregator struct {
	mgr *Manager
	mu  sync.Mutex
	// sessionModes maps sessionID -> latest mode reported by the gateway.
	sessionModes map[string]WorkspacePollMode
	// lastPushed maps workspacePath -> last mode we sent to agentctl. Used to
	// suppress duplicate pushes when workspace-level mode is unchanged.
	lastPushed map[string]WorkspacePollMode
}

// newWorkspacePollAggregator wires an aggregator to the lifecycle manager.
func newWorkspacePollAggregator(mgr *Manager) *workspacePollAggregator {
	return &workspacePollAggregator{
		mgr:          mgr,
		sessionModes: make(map[string]WorkspacePollMode),
		lastPushed:   make(map[string]WorkspacePollMode),
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
// has created its execution, we cache the mode in sessionModes but cannot push
// to agentctl yet (no client). The session stays at agentctl's default (slow)
// until the next mode change actually reaches the running agentctl. To avoid
// drifting permanently in that case, callers that create executions should
// invoke RefreshWorkspacePollMode once the execution is registered (TODO).
// In practice the user re-focusing or any subscribe/unsubscribe re-triggers
// the push, so the window self-heals quickly.
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

// recordAndCompute updates the per-session mode for the given workspace and
// returns the new workspace-effective mode, plus whether it changed since the
// last push. We compute this under a single lock so concurrent transitions in
// the same workspace can't observe inconsistent intermediate state.
//
// When a session goes to paused we drop its sessionModes entry so the map
// doesn't grow unbounded over a long-running gateway. Same for lastPushed
// when the workspace itself becomes paused.
func (a *workspacePollAggregator) recordAndCompute(sessionID string, mode WorkspacePollMode, workspacePath string) (string, WorkspacePollMode, bool) {
	// Lock order: a.mu -> executionStore.mu (via GetExecutionBySessionID).
	// Do not acquire a.mu while holding executionStore.mu.
	a.mu.Lock()
	defer a.mu.Unlock()

	if mode == WorkspacePollModePaused {
		delete(a.sessionModes, sessionID)
	} else {
		a.sessionModes[sessionID] = mode
	}

	effective := WorkspacePollModePaused
	for sid, m := range a.sessionModes {
		// Filter by workspace: only sessions in this workspace count toward
		// its effective mode. Sessions in other workspaces are tracked too
		// (in case the gateway races ahead of execution creation), but they
		// don't influence this workspace's polling rate.
		exec, ok := a.mgr.GetExecutionBySessionID(sid)
		if !ok || exec.WorkspacePath != workspacePath {
			continue
		}
		if m.rank() > effective.rank() {
			effective = m
		}
	}

	prev, hadPrev := a.lastPushed[workspacePath]
	if hadPrev && prev == effective {
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
