package websocket

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// SessionMode is the desired polling intensity for a session derived from UI state.
// Mirrors the agentctl-side process.PollMode, but defined here so the gateway
// doesn't import agentctl/process types.
type SessionMode string

const (
	// SessionModePaused: no clients subscribed to this session.
	SessionModePaused SessionMode = "paused"
	// SessionModeSlow: clients subscribed (e.g. sidebar diff badge) but none focused.
	SessionModeSlow SessionMode = "slow"
	// SessionModeFast: at least one client has the session focused (task details page, panel).
	SessionModeFast SessionMode = "fast"
)

// SessionModeListener is invoked when a session's effective mode transitions.
// Up-transitions (towards fast) fire immediately; down-transitions fire after
// a debounce window so quick tab churn doesn't tear down + restart polling.
type SessionModeListener func(sessionID string, mode SessionMode)

// downTransitionDebounce is how long the hub waits before notifying listeners
// of a down-transition (fast → slow, slow → paused). Tunable; 5s catches the
// common "open / close / reopen" pattern without leaving CPU on for too long.
const downTransitionDebounce = 5 * time.Second

// sessionModeTracker holds the focus map, debounce timers, and listener list.
//
// Locking: focusByClient is protected by Hub.mu (for consistency with
// sessionSubscribers). All other fields are protected by sessionModeTracker.mu.
type sessionModeTracker struct {
	// One client set per session that has currently focused it. Separate from
	// sessionSubscribers so a client can be "subscribed but not focused" (the
	// sidebar case) — the sets evolve independently.
	// Protected by Hub.mu, NOT by sessionModeTracker.mu.
	focusByClient map[string]map[*Client]bool

	// Last known mode per session, used to suppress redundant listener calls.
	lastMode map[string]SessionMode

	// Pending debounced down-transitions (sessionID → timer that will fire
	// the listener if not cancelled by a re-up).
	pendingDownTransitions map[string]*time.Timer

	// Listeners to invoke on transition. Multiple listeners are supported but
	// in practice there's one (lifecycle manager).
	listeners []SessionModeListener

	// debounce is the delay for down-transitions (fast→slow, slow→paused).
	// Defaults to downTransitionDebounce (5s). Tests can shorten it via
	// setDebounceForTest to avoid 5+ seconds of wall-clock sleep.
	debounce time.Duration

	mu sync.Mutex
}

func newSessionModeTracker() *sessionModeTracker {
	return &sessionModeTracker{
		focusByClient:          make(map[string]map[*Client]bool),
		lastMode:               make(map[string]SessionMode),
		pendingDownTransitions: make(map[string]*time.Timer),
		debounce:               downTransitionDebounce,
	}
}

// setDebounceForTest allows tests to use a short debounce duration instead of
// waiting 5+ real seconds per test.
func (h *Hub) setDebounceForTest(d time.Duration) {
	h.sessionMode.mu.Lock()
	h.sessionMode.debounce = d
	h.sessionMode.mu.Unlock()
}

// AddSessionModeListener registers a callback for session mode transitions.
// Listeners are called from arbitrary goroutines; they should be fast.
func (h *Hub) AddSessionModeListener(l SessionModeListener) {
	h.sessionMode.mu.Lock()
	h.sessionMode.listeners = append(h.sessionMode.listeners, l)
	h.sessionMode.mu.Unlock()
}

// FocusSession marks a session as focused by the given client. Causes the
// session mode to transition to fast (immediately).
func (h *Hub) FocusSession(client *Client, sessionID string) {
	if sessionID == "" {
		return
	}
	h.mu.Lock()
	if _, ok := h.sessionMode.focusByClient[sessionID]; !ok {
		h.sessionMode.focusByClient[sessionID] = make(map[*Client]bool)
	}
	h.sessionMode.focusByClient[sessionID][client] = true
	client.sessionFocus[sessionID] = true
	h.mu.Unlock()

	h.logger.Debug("client focused session",
		zap.String("client_id", client.ID),
		zap.String("session_id", sessionID))

	h.recomputeSessionMode(sessionID)
}

// UnfocusSession removes the focus mark for the given client.
func (h *Hub) UnfocusSession(client *Client, sessionID string) {
	if sessionID == "" {
		return
	}
	h.mu.Lock()
	if clients, ok := h.sessionMode.focusByClient[sessionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.sessionMode.focusByClient, sessionID)
		}
	}
	delete(client.sessionFocus, sessionID)
	h.mu.Unlock()

	h.logger.Debug("client unfocused session",
		zap.String("client_id", client.ID),
		zap.String("session_id", sessionID))

	h.recomputeSessionMode(sessionID)
}

// computeSessionModeLocked returns the effective mode for a session given the
// current subscribers and focus sets. Caller must hold h.mu (read or write).
func (h *Hub) computeSessionModeLocked(sessionID string) SessionMode {
	if len(h.sessionMode.focusByClient[sessionID]) > 0 {
		return SessionModeFast
	}
	if len(h.sessionSubscribers[sessionID]) > 0 {
		return SessionModeSlow
	}
	return SessionModePaused
}

// GetSessionMode returns the current effective mode for a session (fast if any
// client focused, slow if any subscribed, paused otherwise). Used by the
// lifecycle manager when an execution becomes ready — it queries the hub's
// live state and pushes to agentctl, closing the race where the hub's mode
// transitions fired before the execution was registered.
func (h *Hub) GetSessionMode(sessionID string) SessionMode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.computeSessionModeLocked(sessionID)
}

// recomputeSessionMode is called after any state change that could affect a
// session's mode (subscribe, unsubscribe, focus, unfocus, client disconnect).
// Up-transitions notify immediately; down-transitions are debounced.
//
// Lock order: sessionMode.mu before h.mu (RLock). This is also the order used
// by the timer callback in scheduleDownTransition, which lets the callback
// re-read the latest mode atomically with respect to other recomputeSessionMode
// calls (otherwise a TOCTOU window between "read latest" and "compare to
// lastMode" can let a concurrent up-transition be silently overwritten).
func (h *Hub) recomputeSessionMode(sessionID string) {
	h.sessionMode.mu.Lock()
	h.mu.RLock()
	current := h.computeSessionModeLocked(sessionID)
	h.mu.RUnlock()

	prev, hadPrev := h.sessionMode.lastMode[sessionID]

	h.logger.Debug("recomputeSessionMode",
		zap.String("session_id", sessionID),
		zap.String("current", string(current)),
		zap.String("prev", string(prev)),
		zap.Bool("had_prev", hadPrev),
		zap.Int("listener_count", len(h.sessionMode.listeners)))

	// Always cancel any pending debounced down-transition — either we're
	// transitioning to a new mode (it's stale), or we're back at the same mode
	// it was scheduled to drop us to (it's redundant). If we kept the old
	// timer alive, a slow→fast→slow→fast within the debounce window would
	// leave the original slow timer pending and fire after the user had
	// already re-focused.
	//
	// When we cancel a pending timer, also clear lastMode so the cancelled
	// transition doesn't leave stale state. Without this, a page refresh
	// (disconnect → paused debounce → reconnect within 5s) would see
	// prev=fast, current=fast → "no change", and never broadcast the mode
	// to the new client.
	if t, ok := h.sessionMode.pendingDownTransitions[sessionID]; ok {
		t.Stop()
		delete(h.sessionMode.pendingDownTransitions, sessionID)
		delete(h.sessionMode.lastMode, sessionID)
		prev = ""
		hadPrev = false
	}

	if hadPrev && prev == current {
		h.logger.Debug("recomputeSessionMode: no change, skipping",
			zap.String("session_id", sessionID))
		h.sessionMode.mu.Unlock()
		return
	}

	if isUpTransition(prev, current) {
		// Up-transitions fire immediately so the user sees fresh data right away.
		h.sessionMode.lastMode[sessionID] = current
		listeners := h.snapshotListenersLocked()
		h.logger.Debug("recomputeSessionMode: up-transition, firing listeners",
			zap.String("session_id", sessionID),
			zap.String("from", string(prev)),
			zap.String("to", string(current)),
			zap.Int("listener_count", len(listeners)))
		h.sessionMode.mu.Unlock()
		h.fireListeners(listeners, sessionID, current)
		return
	}

	// Down-transition: debounce so quick tab churn doesn't tear down + restart.
	h.logger.Debug("recomputeSessionMode: down-transition, scheduling debounce",
		zap.String("session_id", sessionID),
		zap.String("from", string(prev)),
		zap.String("to", string(current)))
	h.sessionMode.pendingDownTransitions[sessionID] = time.AfterFunc(h.sessionMode.debounce, func() {
		h.fireDebouncedDownTransition(sessionID)
	})
	h.sessionMode.mu.Unlock()
}

// fireDebouncedDownTransition is called by the debounce timer. Acquires
// sessionMode.mu first (matching recomputeSessionMode's lock order) so that
// reading latest and comparing/updating lastMode happens atomically — no
// concurrent recomputeSessionMode can sneak an up-transition between the two
// reads.
func (h *Hub) fireDebouncedDownTransition(sessionID string) {
	h.sessionMode.mu.Lock()
	// Drop the timer pointer so a future scheduling can replace it cleanly.
	delete(h.sessionMode.pendingDownTransitions, sessionID)

	h.mu.RLock()
	latest := h.computeSessionModeLocked(sessionID)
	h.mu.RUnlock()

	prevAtFire, hadPrev := h.sessionMode.lastMode[sessionID]
	if hadPrev && prevAtFire == latest {
		h.sessionMode.mu.Unlock()
		return
	}

	if latest == SessionModePaused {
		// Session is fully idle — drop the entry so the maps don't grow
		// unbounded over a long-running gateway. Next event re-adds.
		delete(h.sessionMode.lastMode, sessionID)
	} else {
		h.sessionMode.lastMode[sessionID] = latest
	}
	listeners := h.snapshotListenersLocked()
	h.sessionMode.mu.Unlock()
	h.fireListeners(listeners, sessionID, latest)
}

// stopAllPendingTransitions cancels every pending debounce timer and clears
// the tracking maps. Called from closeAllClients during shutdown so timers
// don't outlive the hub and fire stale events into listeners.
func (h *Hub) stopAllPendingTransitions() {
	h.sessionMode.mu.Lock()
	defer h.sessionMode.mu.Unlock()
	for id, t := range h.sessionMode.pendingDownTransitions {
		t.Stop()
		delete(h.sessionMode.pendingDownTransitions, id)
	}
	h.sessionMode.lastMode = make(map[string]SessionMode)
}

// snapshotListenersLocked returns a copy of the listener slice. Caller holds h.sessionMode.mu.
func (h *Hub) snapshotListenersLocked() []SessionModeListener {
	out := make([]SessionModeListener, len(h.sessionMode.listeners))
	copy(out, h.sessionMode.listeners)
	return out
}

func (h *Hub) fireListeners(listeners []SessionModeListener, sessionID string, mode SessionMode) {
	for _, l := range listeners {
		l(sessionID, mode)
	}
}

// isUpTransition returns true if newMode represents a higher polling intensity
// than oldMode. Order: paused < slow < fast.
func isUpTransition(oldMode, newMode SessionMode) bool {
	return modeRank(newMode) > modeRank(oldMode)
}

func modeRank(m SessionMode) int {
	switch m {
	case SessionModeFast:
		return 2
	case SessionModeSlow:
		return 1
	case SessionModePaused:
		return 0
	default:
		return 0
	}
}
