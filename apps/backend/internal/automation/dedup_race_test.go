package automation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// TestFireTrigger_ConcurrentInstancesRaceTheSameDedupKey proves
// idx_automation_runs_dedup_unique backstops admitTriggerLocked's
// check-then-insert admission across separate instances, not just within one
// process's in-process mutex (s.runLocks). Two Service values — each with its
// own store, connection, and runLocks map, sharing only the underlying
// database file — simulate two backend instances racing the same resolved
// dedup key at the same instant. Exactly one must be admitted; the loser
// must come back as a duplicate skip, never a hard error.
func TestFireTrigger_ConcurrentInstancesRaceTheSameDedupKey(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "race.db") + "?_busy_timeout=5000"

	svcA := newRaceTestService(t, dsn)
	svcB := newRaceTestService(t, dsn)

	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "race", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svcA.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(2)
	results := make(chan FireResult, 2)
	errs := make(chan error, 2)

	fire := func(svc *Service, triggerID string) {
		ready.Done()
		<-start
		res, err := svc.FireTrigger(ctx, a.ID, triggerID, TriggerTypeWebhook,
			json.RawMessage(`{}`), DedupKey("webhook:alert-1"))
		results <- res
		errs <- err
	}

	go fire(svcA, "t-a")
	go fire(svcB, "t-b")
	ready.Wait()
	close(start)

	var admitted, skipped int
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("expected no hard error from either racer, got %v", err)
		}
		res := <-results
		if res.Skipped {
			skipped++
			if res.Reason != "this trigger has already fired" {
				t.Errorf("expected the losing racer's skip reason to name the duplicate, got %q", res.Reason)
			}
		} else {
			admitted++
		}
	}
	if admitted != 1 || skipped != 1 {
		t.Fatalf("expected exactly one admission and one duplicate skip, got %d admitted, %d skipped", admitted, skipped)
	}

	var runCount int
	if err := svcA.store.db.Get(&runCount,
		`SELECT COUNT(*) FROM automation_runs WHERE automation_id = ? AND dedup_key = 'webhook:alert-1'`,
		a.ID); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("expected exactly one run to hold the dedup key, got %d", runCount)
	}
}

func newRaceTestService(t *testing.T, dsn string) *Service {
	t.Helper()
	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(store, bus.NewMemoryEventBus(log), log)
}
