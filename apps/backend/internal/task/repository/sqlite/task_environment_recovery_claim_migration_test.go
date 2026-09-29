package sqlite

import (
	"context"
	"testing"
)

func TestTaskEnvironmentRecoveryClaimFreshSchemaAndMigrationReplay(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	if _, err := repo.db.ExecContext(ctx, `
		SELECT session_incarnation_id
		FROM task_environment_recovery_claims
		LIMIT 0`); err != nil {
		t.Fatalf("fresh recovery claim schema lacks session incarnation: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("replay recovery claim migration: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("replay recovery claim migration twice: %v", err)
	}
}
