package dynamic

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type ResourceScope string

const (
	ScopeProvider   ResourceScope = "provider"
	ScopeCredential ResourceScope = "credential"
	ScopeModel      ResourceScope = "model"
	ScopeProfile    ResourceScope = "profile"
)

func ResourceKey(scope ResourceScope, fingerprint string) string {
	return string(scope) + ":" + fingerprint
}

type circuit struct {
	state      CircuitState
	until      time.Time
	code       routingerr.Code
	probeUntil time.Time
}

// CircuitSnapshot is the durable representation of one resource circuit.
// Keys are opaque fingerprints and must already be safe to persist.
type CircuitSnapshot struct {
	Key        string
	State      CircuitState
	Until      time.Time
	Code       routingerr.Code
	ProbeUntil time.Time
}

// CircuitPersistence stores shared resource health across backend restarts.
type CircuitPersistence interface {
	SaveCircuit(context.Context, CircuitSnapshot) error
	LoadCircuits(context.Context) ([]CircuitSnapshot, error)
}

type CircuitRegistryOption func(*CircuitRegistry)

func WithCircuitClock(now func() time.Time) CircuitRegistryOption {
	return func(registry *CircuitRegistry) {
		if now != nil {
			registry.now = now
		}
	}
}

func WithCircuitPersistence(persistence CircuitPersistence) CircuitRegistryOption {
	return func(registry *CircuitRegistry) { registry.persist = persistence }
}

type CircuitRegistry struct {
	mu       sync.Mutex
	now      func() time.Time
	circuits map[string]circuit
	persist  CircuitPersistence
}

func NewCircuitRegistry(options ...CircuitRegistryOption) *CircuitRegistry {
	registry := &CircuitRegistry{now: time.Now, circuits: make(map[string]circuit)}
	for _, option := range options {
		option(registry)
	}
	return registry
}

// Restore loads durable circuit state before routing workers start.
func (r *CircuitRegistry) Restore(ctx context.Context) error {
	if r.persist == nil {
		return nil
	}
	snapshots, err := r.persist.LoadCircuits(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, snapshot := range snapshots {
		r.circuits[snapshot.Key] = circuit{
			state: snapshot.State, until: snapshot.Until,
			code: snapshot.Code, probeUntil: snapshot.ProbeUntil,
		}
	}
	return nil
}

func (r *CircuitRegistry) Open(key string, until time.Time, code routingerr.Code) {
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.circuits[key] = circuit{state: CircuitOpen, until: until, code: code}
	r.persistSnapshotLocked(key)
}

func (r *CircuitRegistry) IsOpen(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.circuits[key]
	if !ok || entry.state == CircuitClosed {
		return false
	}
	if entry.state == CircuitHalfOpen {
		return true
	}
	// An expired open circuit remains unavailable until one caller acquires
	// the exclusive probe lease. Treating it as closed here would let every
	// selector stampede the provider between expiry and probe acquisition.
	return entry.state == CircuitOpen && !entry.until.IsZero()
}

type ProbeLease struct {
	Key       string
	ExpiresAt time.Time
}

func (r *CircuitRegistry) AcquireProbe(key string, duration time.Duration) (ProbeLease, bool) {
	if key == "" || duration <= 0 {
		return ProbeLease{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	entry, ok := r.circuits[key]
	if !ok || entry.state == CircuitClosed || now.Before(entry.until) || now.Before(entry.probeUntil) {
		return ProbeLease{}, false
	}
	lease := ProbeLease{Key: key, ExpiresAt: now.Add(duration)}
	entry.state = CircuitHalfOpen
	entry.probeUntil = lease.ExpiresAt
	r.circuits[key] = entry
	r.persistSnapshotLocked(key)
	return lease, true
}

func (r *CircuitRegistry) ReleaseProbe(lease ProbeLease, success bool, backoff time.Duration) {
	if lease.Key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.circuits[lease.Key]
	if !ok || entry.state != CircuitHalfOpen || !entry.probeUntil.Equal(lease.ExpiresAt) {
		return
	}
	entry.probeUntil = time.Time{}
	if success {
		entry.state = CircuitClosed
		entry.until = time.Time{}
		entry.code = ""
	} else {
		entry.state = CircuitOpen
		entry.until = r.now().Add(backoff)
	}
	r.circuits[lease.Key] = entry
	r.persistSnapshotLocked(lease.Key)
}

func (r *CircuitRegistry) persistSnapshotLocked(key string) {
	if r.persist == nil {
		return
	}
	entry, ok := r.circuits[key]
	if !ok {
		return
	}
	// Selection remains fail-closed in memory if persistence is temporarily
	// unavailable. Startup Restore and health reconciliation surface durable
	// read failures to their caller.
	_ = r.persist.SaveCircuit(context.Background(), CircuitSnapshot{
		Key: key, State: entry.state, Until: entry.until,
		Code: entry.code, ProbeUntil: entry.probeUntil,
	})
}
