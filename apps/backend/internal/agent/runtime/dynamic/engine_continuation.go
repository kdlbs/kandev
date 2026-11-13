package dynamic

import (
	"context"
	"encoding/json"
)

// saveContinuation keeps the cached route and its durable handoff consistent
// with status transitions owned by this engine.
func (e *Engine) saveContinuation(ctx context.Context, persistence ContinuationPersistence, record ContinuationRecord) error {
	payload, err := json.Marshal(record.Continuation)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// Read only the cache. A route not loaded yet has no cached continuation to
	// refresh, and SaveRouteContinuation fences the durable row on its
	// generation, so a stale record is still refused without a DB load here.
	state, exists := e.states[record.SessionID]
	if exists && state.Generation != record.Generation {
		return ErrStaleGeneration
	}
	if err := persistence.SaveRouteContinuation(ctx, record); err != nil {
		return err
	}
	if exists {
		state.ContinuationJSON = string(payload)
		state.UpdatedAt = record.UpdatedAt
		e.states[record.SessionID] = state
	}
	return nil
}
