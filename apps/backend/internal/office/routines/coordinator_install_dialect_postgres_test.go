package routines_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/testutil"
)

// TestCreateDefaultCoordinatorRoutine_IdenticalOnSQLiteAndPostgres covers
// AC-OFFICE-COORDINATOR-INSTALL-001.11: a fresh install and its idempotent
// repeat produce the same routine/trigger shape on SQLite and PostgreSQL.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestCreateDefaultCoordinatorRoutine_IdenticalOnSQLiteAndPostgres(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)

	sqliteDB, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqliteDB.Close() })
	sqliteDB.SetMaxOpenConns(1)
	sqliteRepo, err := sqlite.NewWithDB(sqliteDB, sqliteDB, nil)
	if err != nil {
		t.Fatalf("sqlite repo: %v", err)
	}

	pgDB := testutil.OpenIsolatedPostgres(t, dsn)
	pgRepo, err := sqlite.NewWithDB(pgDB, pgDB, nil)
	if err != nil {
		t.Fatalf("postgres repo: %v", err)
	}

	sqliteSvc := routines.NewRoutineService(sqliteRepo, logger.Default(), nil)
	pgSvc := routines.NewRoutineService(pgRepo, logger.Default(), nil)

	ctx := context.Background()
	for _, svc := range []*routines.RoutineService{sqliteSvc, pgSvc} {
		first, err := svc.CreateDefaultCoordinatorRoutine(ctx, "ws-dialect", "agent-dialect")
		if err != nil {
			t.Fatalf("first install: %v", err)
		}
		if first.Status != "active" {
			t.Errorf("status = %q, want active", first.Status)
		}
		second, err := svc.CreateDefaultCoordinatorRoutine(ctx, "ws-dialect", "agent-dialect")
		if err != nil {
			t.Fatalf("second install: %v", err)
		}
		if second.ID != first.ID {
			t.Errorf("second install returned a different routine: %q vs %q", second.ID, first.ID)
		}
	}

	sqliteTriggers, err := sqliteRepo.ListTriggersByRoutineID(ctx, mustSoleRoutineID(t, ctx, sqliteRepo, "ws-dialect"))
	if err != nil {
		t.Fatalf("list sqlite triggers: %v", err)
	}
	pgTriggers, err := pgRepo.ListTriggersByRoutineID(ctx, mustSoleRoutineID(t, ctx, pgRepo, "ws-dialect"))
	if err != nil {
		t.Fatalf("list postgres triggers: %v", err)
	}
	if len(sqliteTriggers) != 1 || len(pgTriggers) != 1 {
		t.Fatalf("trigger counts = sqlite:%d postgres:%d, want 1 each", len(sqliteTriggers), len(pgTriggers))
	}
	if sqliteTriggers[0].CronExpression != pgTriggers[0].CronExpression || sqliteTriggers[0].Timezone != pgTriggers[0].Timezone {
		t.Errorf("trigger schedule differs: sqlite=%+v postgres=%+v", sqliteTriggers[0], pgTriggers[0])
	}
	if !sqliteTriggers[0].Enabled || !pgTriggers[0].Enabled {
		t.Errorf("expected both canonical triggers enabled: sqlite=%v postgres=%v", sqliteTriggers[0].Enabled, pgTriggers[0].Enabled)
	}
}

// mustSoleRoutineID returns the single routine in workspaceID, failing the
// test if there is not exactly one.
func mustSoleRoutineID(t *testing.T, ctx context.Context, repo *sqlite.Repository, workspaceID string) string {
	t.Helper()
	list, err := repo.ListRoutines(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("routines in %s = %d, want 1", workspaceID, len(list))
	}
	return list[0].ID
}

// TestCreateDefaultCoordinatorRoutine_ConcurrentOnPostgresCreatesAtMostOne
// covers AC-OFFICE-COORDINATOR-INSTALL-001.8/.9/.13 on the dialect where
// the advisory-lock poll actually runs: two concurrent installs for the
// same identity, against two separate connections to the same PostgreSQL
// database, create at most one routine and at most one canonical trigger.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestCreateDefaultCoordinatorRoutine_ConcurrentOnPostgresCreatesAtMostOne(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	pgDB := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := sqlite.NewWithDB(pgDB, pgDB, nil)
	if err != nil {
		t.Fatalf("postgres repo: %v", err)
	}
	svc := routines.NewRoutineService(repo, logger.Default(), nil)

	type result struct {
		id  string
		err error
	}
	resultsCh := make(chan result, 2)
	run := func() {
		routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-pg-concurrent", "agent-pg-concurrent")
		if routine != nil {
			resultsCh <- result{routine.ID, err}
			return
		}
		resultsCh <- result{"", err}
	}
	go run()
	go run()

	var results [2]result
	for i := range results {
		results[i] = <-resultsCh
	}
	for i, res := range results {
		if res.err != nil {
			t.Fatalf("call %d unexpected error: %v", i, res.err)
		}
	}
	if results[0].id != results[1].id {
		t.Errorf("concurrent installs returned different routines: %q vs %q", results[0].id, results[1].id)
	}

	ctx := context.Background()
	routineList, err := repo.ListRoutines(ctx, "ws-pg-concurrent")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(routineList) != 1 {
		t.Fatalf("routines created = %d, want 1", len(routineList))
	}
	triggers, err := repo.ListTriggersByRoutineID(ctx, routineList[0].ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("triggers created = %d, want 1", len(triggers))
	}
}
