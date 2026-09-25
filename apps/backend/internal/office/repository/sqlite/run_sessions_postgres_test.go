package sqlite_test

import (
	"context"
	"testing"
	"time"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// AC-OFFICE-TASKLESS-001.1: the reservation CAS uses the same portable
// transaction on the supported PostgreSQL backend, not a SQLite-only insert.
func TestPostgresRunSessionReservationCAS(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at)
		VALUES ('pg-run-session-agent-type', 'Agent', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed agent type: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, created_at, updated_at)
		VALUES ('pg-run-session-agent', 'pg-run-session-agent-type', 'Agent', 'Agent', 'pg-run-session-workspace', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES ('pg-run-session-run', 'pg-run-session-agent', 'routine_dispatch_cron', '{}', 'claimed', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	session := &models.RunSession{
		ID: "pg-run-session-1", WorkspaceID: "pg-run-session-workspace",
		AgentProfileID: "pg-run-session-agent", RunID: "pg-run-session-run",
		Attempt: 1, State: models.RunSessionStatePreparing, CreatedAt: time.Now().UTC(), Version: 1,
	}
	reserved, err := repo.ReserveRunSession(ctx, session)
	if err != nil || !reserved {
		t.Fatalf("reservation = %v, %v; want true, nil", reserved, err)
	}
	duplicate := *session
	duplicate.ID = "pg-run-session-duplicate"
	reserved, err = repo.ReserveRunSession(ctx, &duplicate)
	if err != nil || reserved {
		t.Fatalf("duplicate reservation = %v, %v; want false, nil", reserved, err)
	}
}
