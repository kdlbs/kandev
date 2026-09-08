package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresQueueRun_DedupesOnIdempotencyIndexRace is the PostgreSQL twin
// of TestQueueRun_DedupesOnIdempotencyIndexRace: SQLite and PostgreSQL
// classify unique-violation errors completely differently (typed
// pgconn.PgError with a constraint name vs. SQLite's message-substring
// match — see runssqlite.IsIdempotencyKeyUniqueViolation), so the SQLite
// path passing is not evidence the PostgreSQL path does. Without this test
// a Postgres deployment could silently fail to recognize the
// idx_run_idempotency violation, fall through to insertRun's generic error
// wrap, and reproduce the exact "spurious ERROR on the losing producer"
// bug the fix exists to remove. Skips unless KANDEV_TEST_POSTGRES_DSN is
// set.
func TestPostgresQueueRun_DedupesOnIdempotencyIndexRace(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	// runs is created by the office repo's own schema init, but that init
	// references the tasks table, so the task repository's schema must
	// run first — mirroring production boot order (see failure_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eb := bus.NewMemoryEventBus(log)
	svc := runsservice.New(officeRepo.RunsRepository(), eb, log, nil)
	repo := officeRepo.RunsRepository()
	ctx := context.Background()

	const key = "task_children_completed:parent-1:0541818c598576d27d09bff86195037f87910dbf34f0420f6a99dc468aa1dfc7"

	outcome, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:         "task_children_completed",
		IdempotencyKey: key,
		Payload:        map[string]any{"agent_profile_id": "a1", "task_id": "t1"},
	})
	if err != nil {
		t.Fatalf("queue first run: %v", err)
	}
	if outcome != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want %q", outcome, runsservice.QueueOutcomeQueued)
	}

	var firstRunID string
	if err := repo.Reader().GetContext(ctx, &firstRunID,
		repo.Reader().Rebind(`SELECT id FROM runs WHERE idempotency_key = ?`), key); err != nil {
		t.Fatalf("look up first run id: %v", err)
	}
	// Push the first row outside CheckIdempotencyKey's 24h window so the
	// racing insert reaches idx_run_idempotency instead of being caught
	// by the earlier check — the exact gap a true concurrent race falls
	// into (both producers pass the check before either commits).
	if err := repo.SetRunRequestedAtForTest(ctx, firstRunID, time.Now().UTC().Add(-25*time.Hour)); err != nil {
		t.Fatalf("age first run past the idempotency window: %v", err)
	}

	second, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:         "task_children_completed",
		IdempotencyKey: key,
		Payload:        map[string]any{"agent_profile_id": "a1", "task_id": "t1"},
	})
	if err != nil {
		t.Fatalf("queue racing run: %v", err)
	}
	if second != runsservice.QueueOutcomeDeduped {
		t.Fatalf("racing outcome = %q, want %q", second, runsservice.QueueOutcomeDeduped)
	}

	var count int
	if err := repo.Reader().GetContext(ctx, &count,
		repo.Reader().Rebind(`SELECT COUNT(*) FROM runs WHERE idempotency_key = ?`), key); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("runs with idempotency_key %q = %d, want 1", key, count)
	}
}
