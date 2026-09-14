// Package backgroundlaunch is the extensibility seam (spec
// docs/specs/disambiguate-waiting/spec.md, D7) for recognising a detached
// background-shell launch. It knows nothing about the liveness probe, the
// parked projection, or rendering — only how to answer "did this agent's
// tool call report a launch that keeps running after the turn ends?" for
// whichever agents register a recogniser. Adding a second agent is a new
// file that calls Register in an init(); nothing here, nor any caller of
// RecognizesDetachedLaunch, changes.
package backgroundlaunch

import (
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Recognizer classifies a normalized tool-call payload as a detached
// background-shell launch for one agent, identified by AgentID().
type Recognizer interface {
	AgentID() string
	RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool
}

var (
	mu       sync.RWMutex
	registry = make(map[string]Recognizer)
)

// Register adds recognizer to the registry, keyed by its AgentID(). It
// panics if recognizer is nil, if its AgentID() is empty, or if an agent ID
// is already registered — these are programmer errors caught once at
// registration time (an init(), in practice), never a runtime condition a
// caller of RecognizesDetachedLaunch needs to handle.
func Register(recognizer Recognizer) {
	if recognizer == nil {
		panic("backgroundlaunch: Register called with a nil recognizer")
	}
	agentID := recognizer.AgentID()
	if agentID == "" {
		panic("backgroundlaunch: Register called with an empty AgentID()")
	}

	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[agentID]; exists {
		panic(fmt.Sprintf("backgroundlaunch: a recognizer is already registered for agent ID %q", agentID))
	}
	registry[agentID] = recognizer
}

// Lookup returns the recognizer registered for agentID, if any.
func Lookup(agentID string) (Recognizer, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := registry[agentID]
	return r, ok
}

// Unregister removes the recognizer registered for agentID, if any. It is a
// no-op for an agent ID with nothing registered. Production recognisers
// register once via init() and are never unregistered; this exists so tests
// that register a throwaway recognizer can undo it via t.Cleanup instead of
// leaking a package-global registration across the test binary.
func Unregister(agentID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, agentID)
}

// RecognizesDetachedLaunch reports whether payload is a detached
// background-shell launch for agentID. An unregistered agent ID reports
// false, and a recognizer whose RecognizesDetachedLaunch panics is treated
// as "did not recognise" — both fail closed to today's behaviour (D7).
func RecognizesDetachedLaunch(agentID string, payload *streams.NormalizedPayload) (recognized bool) {
	recognizer, ok := Lookup(agentID)
	if !ok {
		return false
	}

	defer func() {
		if recover() != nil {
			recognized = false
		}
	}()
	return recognizer.RecognizesDetachedLaunch(payload)
}
