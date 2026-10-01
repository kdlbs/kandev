package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/outcomes/recorder"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func outcomeCount(t *testing.T, p routeParams) int {
	t.Helper()
	var n int
	if err := p.dbPool.Reader().Get(&n, `SELECT COUNT(*) FROM coordinator_outcomes`); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	return n
}

func TestWireCoordinatorOutcomes_SweepGradesAndCleanupStops(t *testing.T) {
	p, _, _, setter := runLedgerWiringWithSetter(t, true)
	var cleanups []func() error
	p.addCleanup = func(fn func() error) { cleanups = append(cleanups, fn) }
	var coordID, wsID string
	if err := p.dbPool.Writer().QueryRow(`SELECT id, workspace_id FROM coordinators LIMIT 1`).Scan(&coordID, &wsID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := p.dbPool.Writer().Exec(`INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, kind, created_at, updated_at)
		VALUES ('p1', ?, ?, 'rejected', '{}', 'create_task', ?, ?)`, coordID, wsID, now, now); err != nil {
		t.Fatal(err)
	}
	wireCoordinatorOutcomes(p, setter.svc)
	if len(cleanups) != 1 {
		t.Fatalf("cleanups = %d", len(cleanups))
	}
	deadline := time.After(5 * time.Second)
	for outcomeCount(t, p) != 1 {
		select {
		case <-deadline:
			t.Fatal("startup sweep did not grade")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := cleanups[0](); err != nil {
		t.Fatal(err)
	}
}

func TestWireCoordinatorOutcomes_NoBusRecordsNothing(t *testing.T) {
	p, _, _, setter := runLedgerWiringWithSetter(t, true)
	p.eventBus = nil
	var cleanups []func() error
	p.addCleanup = func(fn func() error) { cleanups = append(cleanups, fn) }
	wireCoordinatorOutcomes(p, setter.svc)
	if len(cleanups) != 0 {
		t.Fatal("wired without an event bus")
	}
}

func TestCoordinatorActorChecker_AuthDisabled(t *testing.T) {
	c := coordinatorActorChecker{}
	for actor, want := range map[string]recorder.Verdict{
		"":                      recorder.VerdictManager,
		userstore.DefaultUserID: recorder.VerdictManager,
		"plugin-x":              recorder.VerdictPrincipal,
	} {
		got, err := c.Check(context.Background(), "ws", actor)
		if err != nil || got != want {
			t.Fatalf("actor %q = %v, %v, want %v", actor, got, err, want)
		}
	}
}
