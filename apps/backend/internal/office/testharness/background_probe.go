package testharness

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// ScriptedBackgroundProbe is a test-only BackgroundProbe double, structurally
// satisfying orchestrator.BackgroundProbe (Probe(ctx, sessionID)
// (executor.ProbeResult, error)) without this package importing the
// orchestrator package. It mirrors internal/orchestrator's own
// spyBackgroundProbe test double (parked_projection_test.go), scoped
// per-session so the Playwright suite can script one session's liveness
// sequence without affecting any other session sharing the same backend
// process. Each session's sequence is consumed in order and holds at its
// last value once exhausted; a session with no script probes as
// ProbeResultUnknown — the same zero-value default spyBackgroundProbe uses.
type ScriptedBackgroundProbe struct {
	mu      sync.Mutex
	scripts map[string][]executor.ProbeResult
	calls   map[string]int
}

// NewScriptedBackgroundProbe returns an empty probe: every session reads
// Unknown until scripted.
func NewScriptedBackgroundProbe() *ScriptedBackgroundProbe {
	return &ScriptedBackgroundProbe{
		scripts: make(map[string][]executor.ProbeResult),
		calls:   make(map[string]int),
	}
}

// Script sets (or replaces) sessionID's scripted result sequence and resets
// its call counter, so re-scripting a session mid-test restarts the sequence
// at index 0 rather than continuing a stale counter against the new slice.
func (p *ScriptedBackgroundProbe) Script(sessionID string, results []executor.ProbeResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scripts[sessionID] = results
	p.calls[sessionID] = 0
}

// CallCount returns how many times Probe has been called for sessionID
// since its last Script call (0 if never scripted). Lets the Playwright
// suite assert a minimum number of samples were actually taken (AC-73)
// instead of only checking the affordance's current visibility.
func (p *ScriptedBackgroundProbe) CallCount(sessionID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[sessionID]
}

// Probe returns sessionID's next scripted result, holding at the last
// element once the sequence is exhausted (mirrors spyBackgroundProbe.Probe).
func (p *ScriptedBackgroundProbe) Probe(_ context.Context, sessionID string) (executor.ProbeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	results := p.scripts[sessionID]
	if len(results) == 0 {
		return executor.ProbeResultUnknown, nil
	}
	i := p.calls[sessionID]
	p.calls[sessionID] = i + 1
	if i < len(results) {
		return results[i], nil
	}
	return results[len(results)-1], nil
}
