package dynamic

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// stubCircuitPersistence is an in-memory CircuitPersistence whose SaveCircuit
// can be made to fail on demand, to exercise the persistence-failure paths.
type stubCircuitPersistence struct {
	mu      sync.Mutex
	rows    map[string]CircuitSnapshot
	failing bool
	saves   int
}

func newStubCircuitPersistence() *stubCircuitPersistence {
	return &stubCircuitPersistence{rows: make(map[string]CircuitSnapshot)}
}

func (s *stubCircuitPersistence) SaveCircuit(_ context.Context, snapshot CircuitSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.failing {
		return errors.New("db is locked")
	}
	s.rows[snapshot.Key] = snapshot
	return nil
}

func (s *stubCircuitPersistence) LoadCircuits(_ context.Context) ([]CircuitSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshots := make([]CircuitSnapshot, 0, len(s.rows))
	for _, row := range s.rows {
		if row.State == CircuitClosed {
			continue
		}
		snapshots = append(snapshots, row)
	}
	return snapshots, nil
}

func (s *stubCircuitPersistence) setFailing(failing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failing = failing
}

func (s *stubCircuitPersistence) row(key string) (CircuitSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[key]
	return row, ok
}

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, observed := observer.New(zapcore.WarnLevel)
	return zap.New(core), observed
}

func TestPersistSnapshotFailureIsSurfacedOnOpen(t *testing.T) {
	persist := newStubCircuitPersistence()
	persist.setFailing(true)
	log, observed := newObservedLogger()
	registry := NewCircuitRegistry(WithCircuitPersistence(persist), WithCircuitLogger(log))

	registry.Open("credential:acme", time.Now().Add(time.Hour), routingerr.CodeProviderUnavailable)

	pending := registry.PendingPersists()
	if len(pending) != 1 || pending[0] != "credential:acme" {
		t.Fatalf("PendingPersists = %v, want [credential:acme]", pending)
	}
	entries := observed.FilterMessageSnippet("persist circuit").All()
	if len(entries) != 1 {
		t.Fatalf("warn log entries = %d, want 1", len(entries))
	}
}

func TestPersistSnapshotFailureIsSurfacedOnAcquireProbe(t *testing.T) {
	persist := newStubCircuitPersistence()
	log, observed := newObservedLogger()
	registry := NewCircuitRegistry(
		WithCircuitPersistence(persist),
		WithCircuitLogger(log),
		WithCircuitClock(func() time.Time { return time.Unix(1000, 0) }),
	)
	registry.Open("credential:acme", time.Unix(900, 0), routingerr.CodeProviderUnavailable)
	observed.TakeAll()

	persist.setFailing(true)
	lease, ok := registry.AcquireProbe("credential:acme", time.Minute)
	if !ok {
		t.Fatalf("AcquireProbe: expected success, lease=%#v", lease)
	}

	pending := registry.PendingPersists()
	if len(pending) != 1 || pending[0] != "credential:acme" {
		t.Fatalf("PendingPersists = %v, want [credential:acme]", pending)
	}
	entries := observed.FilterMessageSnippet("persist circuit").All()
	if len(entries) != 1 {
		t.Fatalf("warn log entries = %d, want 1", len(entries))
	}
}

func TestPersistSnapshotFailureIsSurfacedOnReleaseProbe(t *testing.T) {
	persist := newStubCircuitPersistence()
	log, observed := newObservedLogger()
	registry := NewCircuitRegistry(
		WithCircuitPersistence(persist),
		WithCircuitLogger(log),
		WithCircuitClock(func() time.Time { return time.Unix(1000, 0) }),
	)
	registry.Open("credential:acme", time.Unix(900, 0), routingerr.CodeProviderUnavailable)
	lease, ok := registry.AcquireProbe("credential:acme", time.Minute)
	if !ok {
		t.Fatalf("AcquireProbe: expected success")
	}
	observed.TakeAll()

	persist.setFailing(true)
	registry.ReleaseProbe(lease, true, time.Minute)

	pending := registry.PendingPersists()
	if len(pending) != 1 || pending[0] != "credential:acme" {
		t.Fatalf("PendingPersists = %v, want [credential:acme]", pending)
	}
	entries := observed.FilterMessageSnippet("persist circuit").All()
	if len(entries) != 1 {
		t.Fatalf("warn log entries = %d, want 1", len(entries))
	}
}

// TestFailedPersistDoesNotSurviveRestart is the regression test: an Open
// whose SaveCircuit fails must not leave the durable row permanently absent.
// A later mutation that succeeds should flush the pending write so a fresh
// registry's Restore sees the binding as open, not closed.
func TestFailedPersistDoesNotSurviveRestart(t *testing.T) {
	persist := newStubCircuitPersistence()
	persist.setFailing(true)
	registry := NewCircuitRegistry(WithCircuitPersistence(persist))

	until := time.Now().Add(time.Hour)
	registry.Open("credential:acme", until, routingerr.CodeProviderUnavailable)

	if _, ok := persist.row("credential:acme"); ok {
		t.Fatalf("durable row should be absent while persistence is failing")
	}

	// Restart now: a fresh registry restores from the (still-empty) store and
	// reads the binding as closed. This is the bug before the fix.
	restored := NewCircuitRegistry(WithCircuitPersistence(persist))
	if err := restored.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.IsOpen("credential:acme", time.Now()) {
		t.Fatalf("restored registry should not see the lost write yet")
	}

	// The store recovers; a further mutation on the original registry should
	// flush the pending write.
	persist.setFailing(false)
	registry.Open("credential:other", time.Now().Add(time.Hour), routingerr.CodeProviderUnavailable)

	if len(registry.PendingPersists()) != 0 {
		t.Fatalf("PendingPersists should be empty after a successful flush, got %v", registry.PendingPersists())
	}
	if _, ok := persist.row("credential:acme"); !ok {
		t.Fatalf("durable row for credential:acme should have been backfilled")
	}

	restoredAfterRecovery := NewCircuitRegistry(WithCircuitPersistence(persist))
	if err := restoredAfterRecovery.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !restoredAfterRecovery.IsOpen("credential:acme", time.Now()) {
		t.Fatalf("restored registry should see credential:acme as open after recovery")
	}
}

// TestFlushWritesCurrentSnapshotNotStale ensures the pending-flush re-reads
// the circuit's current in-memory state rather than replaying the stale
// snapshot captured at the time of the original failed write.
func TestFlushWritesCurrentSnapshotNotStale(t *testing.T) {
	persist := newStubCircuitPersistence()
	persist.setFailing(true)
	registry := NewCircuitRegistry(
		WithCircuitPersistence(persist),
		WithCircuitClock(func() time.Time { return time.Unix(1000, 0) }),
	)

	registry.Open("credential:acme", time.Unix(900, 0), routingerr.CodeProviderUnavailable)
	lease, ok := registry.AcquireProbe("credential:acme", time.Minute)
	if !ok {
		t.Fatalf("AcquireProbe: expected success")
	}

	persist.setFailing(false)
	registry.ReleaseProbe(lease, true, time.Minute)

	if len(registry.PendingPersists()) != 0 {
		t.Fatalf("PendingPersists should be empty, got %v", registry.PendingPersists())
	}
	row, ok := persist.row("credential:acme")
	if !ok {
		t.Fatalf("durable row for credential:acme should exist")
	}
	if row.State != CircuitClosed {
		t.Fatalf("durable row state = %v, want %v (current snapshot, not stale open)", row.State, CircuitClosed)
	}
}

func TestCleanPathLeavesNoPendingAndLogsNothing(t *testing.T) {
	persist := newStubCircuitPersistence()
	log, observed := newObservedLogger()
	registry := NewCircuitRegistry(WithCircuitPersistence(persist), WithCircuitLogger(log))

	registry.Open("credential:acme", time.Now().Add(time.Hour), routingerr.CodeProviderUnavailable)

	if pending := registry.PendingPersists(); len(pending) != 0 {
		t.Fatalf("PendingPersists = %v, want empty", pending)
	}
	if observed.Len() != 0 {
		t.Fatalf("expected no log entries on the clean path, got %d", observed.Len())
	}
}
