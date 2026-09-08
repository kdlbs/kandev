package backendapp

import (
	"context"
	"errors"
	"expvar"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeroutines "github.com/kandev/kandev/internal/office/routines"
)

func newTestRoutineWakeupAdapter(t *testing.T) *routineWakeupAdapter {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new office repo: %v", err)
	}
	return &routineWakeupAdapter{repo: repo}
}

// AC-OFFICE-RUN-DEDUP-004.6: a durable idempotency conflict on the wakeup
// queue moves office_run_dedup_total{queue="wakeup",kind="durable"} through
// ReportDurableDedup, not ReportInsertResult - the adapter does its own
// errors.Is on the repository's sentinel and reports the conflict itself.
func TestRoutineWakeupAdapter_CreateWakeupRequest_DurableConflictReportsCounter(t *testing.T) {
	adapter := newTestRoutineWakeupAdapter(t)
	ctx := context.Background()
	reason := "test_routine_wakeup_conflict_" + t.Name()
	key := "routine:r1:t1:tick:1234567890"

	first := &officeroutines.WakeupRequest{
		ID: "wakeup-1", AgentProfileID: "agent-1", Source: "cron",
		Reason: reason, IdempotencyKey: key,
	}
	if err := adapter.CreateWakeupRequest(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}

	second := &officeroutines.WakeupRequest{
		ID: "wakeup-2", AgentProfileID: "agent-1", Source: "cron",
		Reason: reason, IdempotencyKey: key,
	}
	err := adapter.CreateWakeupRequest(ctx, second)
	if !errors.Is(err, officesqlite.ErrWakeupIdempotencyConflict) {
		t.Fatalf("second create error = %v, want ErrWakeupIdempotencyConflict", err)
	}

	if !counterHasLabel(t, "office_run_dedup_total", "reason="+reason, "kind=durable", "queue=wakeup") {
		t.Fatal("expected office_run_dedup_total to carry a durable/wakeup entry for this reason")
	}
}

// counterHasLabel reports whether the named expvar.Map has an entry whose
// key contains every given substring. Counters are process-global expvar
// state (no reset hook), so tests assert presence/growth rather than exact
// values that could collide with other tests in the same binary run.
func counterHasLabel(t *testing.T, mapName string, substrs ...string) bool {
	t.Helper()
	v := expvar.Get(mapName)
	if v == nil {
		t.Fatalf("expvar map %q not registered", mapName)
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not a *expvar.Map", mapName)
	}
	found := false
	m.Do(func(kv expvar.KeyValue) {
		matches := true
		for _, s := range substrs {
			if !strings.Contains(kv.Key, s) {
				matches = false
				break
			}
		}
		if matches {
			found = true
		}
	})
	return found
}
