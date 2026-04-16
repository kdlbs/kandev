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

// sessionMode holds the focus map and pending debounce timers. The hub embeds
// this and exposes Focus/Unfocus methods.
type sessionModeTracker struct {
	// One client set per session that has currently focused it. Separate from
	// sessionSubscribers so a client can be "subscribed but not focused" (the
	// sidebar case) — the sets evolve independently.
	focusByClient map[string]map[*Client]bool

	// Last known mode per session, used to suppress redundant listener calls.
	lastMode map[string]SessionMode

	// Pending debounced down-transitions (sessionID → timer that will fire
	// the listener if not cancelled by a re-up).
	pendingDownTransitions map[string]*time.Timer

	// Listeners to invoke on transition. Multiple listeners are supported but
	// in practice there's one (lifecycle manager).
	listeners []SessionModeListener

	mu sync.Mutex
}

func newSessionModeTracker() *sessionModeTracker {
	return &sessionModeTracker{
		focusByClient:          make(map[string]map[*Client]bool),
		lastMode:               make(map[string]SessionMode),
		pendingDownTransitions: make(map[string]*time.Timer),
	}
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

// recomputeSessionMode is called after any state change that could affect a
// session's mode (subscribe, unsubscribe, focus, unfocus, client disconnect).
// Up-transitions notify immediately; down-transitions are debounced.
func (h *Hub) recomputeSessionMode(sessionID string) {
	h.mu.RLock()
	current := h.computeSessionModeLocked(sessionID)
	h.mu.RUnlock()

	h.sessionMode.mu.Lock()
	prev, hadPrev := h.sessionMode.lastMode[sessionID]
	if hadPrev && prev == current {
		// Even if the mode didn't change, cancel any pending down-transition.
		// This handles: slow → fast → slow → fast within the debounce window
		// (we want the latest state to stick, not the first transition).
		if t, ok := h.sessionMode.pendingDownTransitions[sessionID]; ok {
			t.Stop()
			delete(h.sessionMode.pendingDownTransitions, sessionID)
		}
		h.sessionMode.mu.Unlock()
		return
	}

	// Cancel any pending debounced down-transition: it's stale now.
	if t, ok := h.sessionMode.pendingDownTransitions[sessionID]; ok {
		t.Stop()
		delete(h.sessionMode.pendingDownTransitions, sessionID)
	}

	if isUpTransition(prev, current) {
		// Update state and fire immediately.
		h.sessionMode.lastMode[sessionID] = current
		listeners := h.snapshotListenersLocked()
		h.sessionMode.mu.Unlock()
		h.fireListeners(listeners, sessionID, current)
		return
	}

	// Down-transition (or unknown→paused etc): debounce.
	h.sessionMode.pendingDownTransitions[sessionID] = time.AfterFunc(downTransitionDebounce, func() {
		// Re-check on fire — state may have changed since we scheduled.
		h.mu.RLock()
		latest := h.computeSessionModeLocked(sessionID)
		h.mu.RUnlock()

		h.sessionMode.mu.Lock()
		// Drop the timer pointer so a future scheduling can replace it cleanly.
		delete(h.sessionMode.pendingDownTransitions, sessionID)
		prevAtFire := h.sessionMode.lastMode[sessionID]
		if prevAtFire == latest {
			h.sessionMode.mu.Unlock()
			return
		}
		h.sessionMode.lastMode[sessionID] = latest
		listeners := h.snapshotListenersLocked()
		h.sessionMode.mu.Unlock()
		h.fireListeners(listeners, sessionID, latest)
	})
	h.sessionMode.mu.Unlock()
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
