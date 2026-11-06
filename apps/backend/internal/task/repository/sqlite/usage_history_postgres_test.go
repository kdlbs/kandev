package sqlite

import (
	"context"
	"testing"
	"time"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestUsageHistoryPostgresNativeAdmissionAndRetention(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	taskRepo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task schema: %v", err)
	}
	_, err = analyticssqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	for _, statement := range []string{
		`ALTER TABLE session_usage_measurements ADD CONSTRAINT usage_history_legacy_measurements_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE`,
		`ALTER TABLE session_usage_measurements ADD CONSTRAINT usage_history_legacy_measurements_session FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL`,
		`ALTER TABLE session_usage_buckets ADD CONSTRAINT usage_history_legacy_buckets_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE`,
		`ALTER TABLE session_usage_buckets ADD CONSTRAINT usage_history_legacy_buckets_session FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("install legacy retained usage foreign key: %v", err)
		}
	}
	analyticsRepo, err := analyticssqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("migrate legacy analytics schema: %v", err)
	}
	var retainedForeignKeys int
	if err := db.Get(&retainedForeignKeys, `SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE constraint_schema = current_schema() AND table_name IN ('session_usage_measurements', 'session_usage_buckets')
		AND constraint_type = 'FOREIGN KEY' AND constraint_name LIKE 'usage_history_legacy_%'`); err != nil {
		t.Fatalf("inspect migrated PostgreSQL retained foreign keys: %v", err)
	}
	if retainedForeignKeys != 0 {
		t.Fatalf("legacy retained usage foreign keys after migration = %d, want 0", retainedForeignKeys)
	}
	taskRepo.SetUsageHistoryRetainer(analyticsRepo)

	ctx := context.Background()
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "usage-history-pg-ws", Name: "Usage history", OrgID: "usage-history-pg-org"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &models.Task{ID: "usage-history-pg-task", WorkspaceID: "usage-history-pg-ws", Title: "Usage history"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "usage-history-pg-session", TaskID: "usage-history-pg-task", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	turn := &models.Turn{
		ID: "usage-history-pg-turn", TaskID: "usage-history-pg-task", TaskSessionID: "usage-history-pg-session",
		StartedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	turnCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "usage-history-pg-user", Role: authn.RoleMember})
	if err := taskRepo.CreateTurn(turnCtx, turn); err != nil {
		t.Fatalf("create actor-bound turn: %v", err)
	}
	if turn.UsageBindingID == "" {
		t.Fatal("turn admission did not return its immutable usage binding id")
	}
	event := newTestUsageEvent("usage-history-pg-event", "usage-history-pg-task", "usage-history-pg-session")
	event.TurnID, event.UsageBindingID = turn.ID, turn.UsageBindingID
	event.ProviderThreadID, event.ProviderTurnID, event.ProviderResponseID = "thread-pg", "turn-pg", "response-pg"
	event.NativeScope, event.MeasurementSource, event.UsageCompleteness = "direct", "response", "exact"
	event.UsageSchemaVersion = 1
	if err := taskRepo.CreateTaskUsageEvent(ctx, event); err != nil {
		t.Fatalf("create retained native event: %v", err)
	}
	var retained struct {
		ActorID        string `db:"actor_id"`
		ActorStatus    string `db:"actor_status"`
		WorkspaceID    string `db:"workspace_id"`
		OrganizationID string `db:"organization_id"`
		BindingID      string `db:"usage_binding_id"`
	}
	if err := db.Get(&retained, db.Rebind(`
		SELECT b.actor_id, b.actor_status, s.workspace_id, s.organization_id, b.usage_binding_id
		FROM session_usage_buckets b JOIN usage_scopes s ON s.scope_id = b.scope_id
		WHERE b.source = 'native' AND b.source_record_id = ?`), event.UsageEventID); err != nil {
		t.Fatalf("read retained PostgreSQL observation: %v", err)
	}
	if retained.ActorID != "usage-history-pg-user" || retained.ActorStatus != "attributed" ||
		retained.WorkspaceID != "usage-history-pg-ws" || retained.OrganizationID != "usage-history-pg-org" ||
		retained.BindingID != turn.UsageBindingID {
		t.Fatalf("retained PostgreSQL ownership = %+v, want trusted user/workspace/org and turn binding", retained)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM tasks WHERE id = ?`), event.TaskID); err != nil {
		t.Fatalf("delete source task: %v", err)
	}
	var retainedCount int
	if err := db.Get(&retainedCount, db.Rebind(`SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = ?`), event.UsageEventID); err != nil {
		t.Fatalf("read native observation after task deletion: %v", err)
	}
	if retainedCount != 1 {
		t.Fatalf("retained PostgreSQL observations after task deletion = %d, want 1", retainedCount)
	}
}
