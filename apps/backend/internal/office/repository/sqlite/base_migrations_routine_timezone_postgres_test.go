package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresBackfillRoutineTriggerTimezones_LegacyEmptyTimezoneBecomesUTC
// is the PostgreSQL replay twin of the SQLite backfill test. It skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresBackfillRoutineTriggerTimezones_LegacyEmptyTimezoneBecomesUTC(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}

	routine := &models.Routine{
		WorkspaceID:       "ws-postgres",
		Name:              "Legacy Cron PostgreSQL",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	trigger := &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "0 9 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE office_routine_triggers SET timezone = '' WHERE id = $1`, trigger.ID,
	); err != nil {
		t.Fatalf("simulate legacy empty timezone: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	var gotTimezone string
	if err := db.GetContext(ctx, &gotTimezone,
		`SELECT timezone FROM office_routine_triggers WHERE id = $1`, trigger.ID,
	); err != nil {
		t.Fatalf("read post-migration timezone: %v", err)
	}
	if gotTimezone != "UTC" {
		t.Errorf("timezone after backfill = %q, want %q", gotTimezone, "UTC")
	}
}
