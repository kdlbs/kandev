package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestBackfillRoutineTriggerTimezones_LegacyEmptyTimezoneBecomesUTC reproduces
// a pre-migration row (a cron trigger created before the column default
// changed from empty to 'UTC') and asserts a migration replay backfills it,
// mirroring the table-rebuild replay pattern used elsewhere in this package
// (e.g. TestInitSchema_ExecutionProfileRoutingColumnsReplay).
func TestBackfillRoutineTriggerTimezones_LegacyEmptyTimezoneBecomesUTC(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}

	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Legacy Cron",
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

	// Simulate the legacy state the backfill exists to fix: a cron trigger
	// persisted before the column default changed from '' to 'UTC'.
	if _, err := db.Exec(`UPDATE office_routine_triggers SET timezone = '' WHERE id = ?`, trigger.ID); err != nil {
		t.Fatalf("simulate legacy empty timezone: %v", err)
	}
	var preTimezone string
	if err := db.Get(&preTimezone, `SELECT timezone FROM office_routine_triggers WHERE id = ?`, trigger.ID); err != nil {
		t.Fatalf("read pre-migration timezone: %v", err)
	}
	if preTimezone != "" {
		t.Fatalf("pre-migration timezone = %q, want empty string", preTimezone)
	}

	// Replay migrations on the same DB handle, as a restart would.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	var gotTimezone string
	if err := db.Get(&gotTimezone, `SELECT timezone FROM office_routine_triggers WHERE id = ?`, trigger.ID); err != nil {
		t.Fatalf("read post-migration timezone: %v", err)
	}
	if gotTimezone != "UTC" {
		t.Errorf("timezone after backfill = %q, want %q", gotTimezone, "UTC")
	}
}

// TestBackfillRoutineTriggerTimezones_LeavesNonEmptyAndNonCronAlone verifies
// the backfill only touches cron triggers with an empty timezone, not a
// deliberately-set timezone or a non-cron (webhook/manual) trigger kind.
func TestBackfillRoutineTriggerTimezones_LeavesNonEmptyAndNonCronAlone(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}

	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Mixed Triggers",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	cronExplicit := &models.RoutineTrigger{RoutineID: routine.ID, Kind: "cron", CronExpression: "0 9 * * *", Timezone: "America/New_York", Enabled: true}
	if err := repo.CreateRoutineTrigger(ctx, cronExplicit); err != nil {
		t.Fatalf("create explicit-timezone trigger: %v", err)
	}
	webhook := &models.RoutineTrigger{RoutineID: routine.ID, Kind: "webhook", PublicID: "wh-1", SigningMode: "none", Enabled: true}
	if err := repo.CreateRoutineTrigger(ctx, webhook); err != nil {
		t.Fatalf("create webhook trigger: %v", err)
	}
	if _, err := db.Exec(`UPDATE office_routine_triggers SET timezone = '' WHERE id = ?`, webhook.ID); err != nil {
		t.Fatalf("simulate empty timezone on webhook trigger: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	var cronTZ, webhookTZ string
	if err := db.Get(&cronTZ, `SELECT timezone FROM office_routine_triggers WHERE id = ?`, cronExplicit.ID); err != nil {
		t.Fatalf("read cron timezone: %v", err)
	}
	if cronTZ != "America/New_York" {
		t.Errorf("explicit cron timezone = %q, want unchanged %q", cronTZ, "America/New_York")
	}
	if err := db.Get(&webhookTZ, `SELECT timezone FROM office_routine_triggers WHERE id = ?`, webhook.ID); err != nil {
		t.Fatalf("read webhook timezone: %v", err)
	}
	if webhookTZ != "" {
		t.Errorf("webhook timezone = %q, want unchanged empty string (backfill is cron-only)", webhookTZ)
	}
}
