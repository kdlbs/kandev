package dynamic

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

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
	// ScopeAccountBlock holds one execution profile after its provider account
	// was restricted. It never shares a key with a credential or model circuit,
	// so the hold and its release cannot touch another profile.
	ScopeAccountBlock ResourceScope = "account_block"
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

// held reports whether the circuit stays open until an owner release: it has
// no expiry and admits no probe.
func (c circuit) held() bool {
	return c.state == CircuitOpen && routingerr.HoldsUntilRelease(c.code)
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

// WithCircuitLogger sets the logger used to report a failed durable write.
// Defaults to a no-op logger.
func WithCircuitLogger(log *zap.Logger) CircuitRegistryOption {
	return func(registry *CircuitRegistry) {
		if log != nil {
			registry.logger = log
		}
	}
}

type CircuitRegistry struct {
	mu       sync.Mutex
	now      func() time.Time
	circuits map[string]circuit
	persist  CircuitPersistence
	logger   *zap.Logger
	pending  map[string]struct{}
}

func NewCircuitRegistry(options ...CircuitRegistryOption) *CircuitRegistry {
	registry := &CircuitRegistry{
		now:      time.Now,
		circuits: make(map[string]circuit),
		logger:   zap.NewNop(),
		pending:  make(map[string]struct{}),
	}
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
	r.flushPendingLocked(key)
	if r.circuits[key].held() {
		// A timed suspension never replaces an owner-release hold.
		return
	}
	r.circuits[key] = circuit{state: CircuitOpen, until: until, code: code}
	_ = r.persistSnapshotLocked(key)
}

// Hold opens a circuit that has no expiry and admits no probe. Only
// ReleaseHold closes it. The hold applies in memory even when its save fails;
// the error is returned and the save stays pending, so holding the key again
// retries it until the hold is durable. The boolean reports whether this call
// established the hold: it is false for an empty key, a code that does not
// hold until release, a circuit that is already held, and a repeated hold that
// only retries a pending save. Callers report the transition from this result
// so concurrent holders of one key produce exactly one transition report.
func (r *CircuitRegistry) Hold(key string, code routingerr.Code) (bool, error) {
	if key == "" || !routingerr.HoldsUntilRelease(code) {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushPendingLocked(key)
	established := false
	if r.circuits[key].held() {
		if _, unsaved := r.pending[key]; !unsaved {
			return false, nil
		}
	} else {
		r.circuits[key] = circuit{state: CircuitOpen, code: code}
		established = true
	}
	return established, r.persistSnapshotLocked(key)
}

// ReleaseHold closes a held circuit and reports whether one was held. Any
// other circuit is left unchanged. The release is durable: when the closed
// snapshot cannot be saved the hold stays in place and the error is returned,
// so a restart cannot resurrect a hold the caller was told is gone.
func (r *CircuitRegistry) ReleaseHold(key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.circuits[key]
	if !ok || !entry.held() {
		return false, nil
	}
	r.flushPendingLocked(key)
	r.circuits[key] = circuit{state: CircuitClosed}
	if err := r.persistSnapshotLocked(key); err != nil {
		r.circuits[key] = entry
		return false, err
	}
	return true, nil
}

// IsHeld reports whether key is held until an owner release.
func (r *CircuitRegistry) IsHeld(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.circuits[key].held()
}

func (r *CircuitRegistry) IsOpen(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.circuits[key]
	if !ok || entry.state == CircuitClosed {
		return false
	}
	if entry.state == CircuitHalfOpen || entry.held() {
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
	if !ok || entry.state == CircuitClosed || entry.held() || now.Before(entry.until) || now.Before(entry.probeUntil) {
		return ProbeLease{}, false
	}
	r.flushPendingLocked(key)
	lease := ProbeLease{Key: key, ExpiresAt: now.Add(duration)}
	entry.state = CircuitHalfOpen
	entry.probeUntil = lease.ExpiresAt
	r.circuits[key] = entry
	_ = r.persistSnapshotLocked(key)
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
	r.flushPendingLocked(lease.Key)
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
	_ = r.persistSnapshotLocked(lease.Key)
}

// pendingFlushBudget bounds how many other pending keys a single mutation
// retries. r.mu also gates IsOpen on the routing hot path, and each retry is
// a blocking SaveCircuit call, so a mutation during a sustained persistence
// outage must hold the lock for at most one extra write, not one per pending
// key: unbounded fan-out both grows total SaveCircuit calls quadratically
// across mutations and holds the routing mutex for the sum of every pending
// write's duration.
const pendingFlushBudget = 1

// flushPendingLocked retries the durable write for up to pendingFlushBudget
// keys whose previous SaveCircuit failed, using each key's current in-memory
// snapshot. Called from every mutator so a transient persistence failure
// self-heals across subsequent circuit events instead of leaving the durable
// row permanently behind memory.
func (r *CircuitRegistry) flushPendingLocked(skipKey string) {
	flushed := 0
	for key := range r.pending {
		if key == skipKey {
			continue
		}
		if flushed >= pendingFlushBudget {
			return
		}
		_ = r.persistSnapshotLocked(key)
		flushed++
	}
}

// persistSnapshotLocked writes key's current snapshot and returns the
// persistence error, if any. A failure is logged at WARN once per failure
// streak (not once per retry) and the key is recorded in pending so a later
// mutation retries the write; a caller-visible error here would either fail
// routing decisions that must still complete (Open) or make every candidate
// unselectable while the store is down (AcquireProbe), so callers do not act
// on the return value directly.
func (r *CircuitRegistry) persistSnapshotLocked(key string) error {
	if r.persist == nil {
		return nil
	}
	entry, ok := r.circuits[key]
	if !ok {
		return nil
	}
	err := r.persist.SaveCircuit(context.Background(), CircuitSnapshot{
		Key: key, State: entry.state, Until: entry.until,
		Code: entry.code, ProbeUntil: entry.probeUntil,
	})
	if err != nil {
		_, alreadyPending := r.pending[key]
		r.pending[key] = struct{}{}
		if !alreadyPending {
			r.logger.Warn("failed to persist circuit snapshot",
				zap.String("key", key),
				zap.String("state", string(entry.state)),
				zap.Time("until", entry.until),
				zap.Error(err),
			)
		}
		return err
	}
	delete(r.pending, key)
	return nil
}
