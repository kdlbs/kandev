package acp

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// modeSettleWindow bounds how long SetMode waits for the agent to report the
// mode it accepted. A provider that clamps a requested mode publishes
// current_mode_update asynchronously, after answering session/set_mode, so a
// reply alone does not tell us which mode is in force.
const modeSettleWindow = 750 * time.Millisecond

const modeChangePollInterval = 10 * time.Millisecond

func (a *Adapter) lockModeChange(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(modeChangePollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if a.modeChangeMu.TryLock() {
			if err := ctx.Err(); err != nil {
				a.modeChangeMu.Unlock()
				return err
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// noteCurrentMode records a mode report from the active provider session and
// wakes any request waiting for a new observation.
func (a *Adapter) noteCurrentMode(sessionID, mode string) bool {
	if sessionID == "" || mode == "" {
		return false
	}
	a.mu.Lock()
	if sessionID != a.sessionID {
		a.mu.Unlock()
		return false
	}
	a.currentModeID = mode
	a.modeSessionID = sessionID
	a.modeObservationGeneration++
	if a.modeObserved != nil {
		close(a.modeObserved)
	}
	a.modeObserved = make(chan struct{})
	a.mu.Unlock()
	return true
}

type currentModeObservation struct {
	sessionID  string
	mode       string
	generation uint64
	observed   chan struct{}
}

// currentModeSnapshot returns the latest observation and a channel that
// closes on the next report.
func (a *Adapter) currentModeSnapshot() currentModeObservation {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.modeObserved == nil {
		a.modeObserved = make(chan struct{})
	}
	return currentModeObservation{
		sessionID:  a.modeSessionID,
		mode:       a.currentModeID,
		generation: a.modeObservationGeneration,
		observed:   a.modeObserved,
	}
}

// awaitModeSettle waits for the agent to report the requested mode, or for the
// settle window to expire.
//
// On expiry it leaves Effective empty rather than treating a cached, pre-request
// mode as evidence for the current request. An agent that never publishes a
// mode update may still have applied it, but Kandev must not claim an observed
// result it did not receive.
func (a *Adapter) awaitModeSettle(
	ctx context.Context,
	sessionID, requested string,
	afterGeneration uint64,
) streams.ModeResult {
	deadline := time.NewTimer(modeSettleWindow)
	defer deadline.Stop()

	for {
		observation := a.currentModeSnapshot()
		if observation.sessionID == sessionID && observation.generation > afterGeneration {
			return streams.ModeResult{Requested: requested, Effective: observation.mode, Confirmed: true}
		}
		select {
		case <-observation.observed:
			// Inspect the generation again. A report from another session must
			// never confirm this request.
		case <-deadline.C:
			return streams.ModeResult{Requested: requested, Confirmed: false}
		case <-ctx.Done():
			return streams.ModeResult{Requested: requested, Confirmed: false}
		}
	}
}
