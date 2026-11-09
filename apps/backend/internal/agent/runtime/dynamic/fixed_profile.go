package dynamic

import (
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// Suspension describes why one execution profile's resource circuit refuses a
// new dispatch at one instant. Scope names the protected resource, Code the
// classified reason, and Until the reset instant. A hold has no Until: only an
// owner release lifts it.
type Suspension struct {
	Scope ResourceScope
	Code  routingerr.Code
	Until time.Time
}

// Blocked reports whether the suspension refuses dispatch.
func (s Suspension) Blocked() bool { return s.Code != "" }

// RecordResourceFailure opens one concrete candidate's resource circuit
// without touching any route. It reuses the engine's existing policy: an
// owner-release hold for a restriction code, the fixed backoff (extended by a
// later reset hint) for a qualifying provider error, and nothing otherwise.
func (e *Engine) RecordResourceFailure(profile Profile, candidateID string, failure *routingerr.Error) {
	if e == nil {
		return
	}
	e.openCircuitForFailure(profile, candidateID, failure)
}

// CandidateSuspension reports the circuit refusing a new dispatch for one
// concrete candidate at now. An owner-release account block outranks a timed
// resource circuit. It is expiry-aware: unlike IsOpen it does not keep blocking
// after a timed circuit's reset instant, because a fixed-profile dispatch
// acquires no probe lease.
func (e *Engine) CandidateSuspension(candidate Candidate, now time.Time) (Suspension, bool) {
	if e == nil || e.circuits == nil {
		return Suspension{}, false
	}
	if snapshot, ok := e.circuits.ActiveSuspension(accountBlockKey(candidate.ID), now); ok {
		return Suspension{Scope: ScopeAccountBlock, Code: snapshot.Code, Until: snapshot.Until}, true
	}
	if candidate.BindingKey == "" {
		return Suspension{}, false
	}
	snapshot, ok := e.circuits.ActiveSuspension(candidate.BindingKey, now)
	if !ok {
		return Suspension{}, false
	}
	return Suspension{Scope: suspensionScopeForKey(candidate.BindingKey), Code: snapshot.Code, Until: snapshot.Until}, true
}

// suspensionScopeForKey names the resource a circuit key protects. Keys carry
// their scope prefix, so the prefix is the scope; a key without one stays
// credential-scoped, the shared default for a provider resource.
func suspensionScopeForKey(key string) ResourceScope {
	prefix, _, found := strings.Cut(key, ":")
	if !found || prefix == "" {
		return ScopeCredential
	}
	return ResourceScope(prefix)
}
