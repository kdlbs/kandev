package routines_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/testutil"
)

// dialectFixtureRepo is the subset of *sqlite.Repository this test needs:
// enough to seed identical rows and then run Task 01's batch classifier
// against them.
type dialectFixtureRepo interface {
	routines.RoutineTriggerReader
	CreateRoutine(ctx context.Context, r *models.Routine) error
	CreateRoutineTrigger(ctx context.Context, t *models.RoutineTrigger) error
}

// seedArmingDialectFixture creates one routine per schedule state this test
// cares about, using fixed IDs so the two dialects can be compared directly
// by routine ID. now anchors the grace-window and past/future trigger times.
func seedArmingDialectFixture(t *testing.T, repo dialectFixtureRepo, now time.Time) []string {
	t.Helper()
	ctx := context.Background()

	ids := []string{"armed-past-due", "trigger-invalid", "armed-in-grace", "trigger-disabled", "no-trigger"}
	for _, id := range ids {
		r := &models.Routine{
			ID: id, WorkspaceID: "ws-dialect", Name: id, TaskTemplate: "{}",
			Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
		}
		if err := repo.CreateRoutine(ctx, r); err != nil {
			t.Fatalf("create routine %s: %v", id, err)
		}
	}

	past := now.Add(-time.Hour)
	firedRecently := now.Add(-30 * time.Second)
	triggers := []*models.RoutineTrigger{
		{
			ID: "t-armed-past-due", RoutineID: "armed-past-due", Kind: "cron",
			CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: true, NextRunAt: &past,
		},
		{
			ID: "t-invalid", RoutineID: "trigger-invalid", Kind: "cron",
			CronExpression: "not a cron", Timezone: "UTC", Enabled: true,
		},
		{
			ID: "t-armed-in-grace", RoutineID: "armed-in-grace", Kind: "cron",
			CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: true, LastFiredAt: &firedRecently,
		},
		{
			ID: "t-disabled", RoutineID: "trigger-disabled", Kind: "cron",
			CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: false,
		},
	}
	for _, tr := range triggers {
		if err := repo.CreateRoutineTrigger(ctx, tr); err != nil {
			t.Fatalf("create trigger %s: %v", tr.ID, err)
		}
	}
	return ids
}

// TestClassifyRoutines_IdenticalOnSQLiteAndPostgres covers
// AC-OFFICE-ROUTINE-ARMING-001.6: the same trigger rows, read and
// classified through Task 01's batch entry point, produce the same
// schedule state and unarmed cron trigger list on SQLite and PostgreSQL.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestClassifyRoutines_IdenticalOnSQLiteAndPostgres(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	sqliteDB, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqliteDB.Close() })
	if _, _, err := settingsstore.Provide(sqliteDB, sqliteDB, nil); err != nil {
		t.Fatalf("sqlite settings store: %v", err)
	}
	sqliteRepo, err := sqlite.NewWithDB(sqliteDB, sqliteDB, nil)
	if err != nil {
		t.Fatalf("sqlite repo: %v", err)
	}

	pgDB := testutil.OpenIsolatedPostgres(t, dsn)
	if _, _, err := settingsstore.Provide(pgDB, pgDB, nil); err != nil {
		t.Fatalf("postgres settings store: %v", err)
	}
	pgRepo, err := sqlite.NewWithDB(pgDB, pgDB, nil)
	if err != nil {
		t.Fatalf("postgres repo: %v", err)
	}

	ctx := context.Background()
	ids := seedArmingDialectFixture(t, sqliteRepo, now)
	seedArmingDialectFixture(t, pgRepo, now)

	sqliteResults, err := routines.ClassifyRoutines(ctx, sqliteRepo, ids, now)
	if err != nil {
		t.Fatalf("classify sqlite: %v", err)
	}
	pgResults, err := routines.ClassifyRoutines(ctx, pgRepo, ids, now)
	if err != nil {
		t.Fatalf("classify postgres: %v", err)
	}

	for _, id := range ids {
		sqliteResult, pgResult := sqliteResults[id], pgResults[id]
		if sqliteResult.State != pgResult.State {
			t.Errorf("routine %s: sqlite state = %q, postgres state = %q", id, sqliteResult.State, pgResult.State)
		}
		if !reflect.DeepEqual(sqliteResult.Unarmed, pgResult.Unarmed) {
			t.Errorf("routine %s: sqlite unarmed = %+v, postgres unarmed = %+v", id, sqliteResult.Unarmed, pgResult.Unarmed)
		}
	}
}
