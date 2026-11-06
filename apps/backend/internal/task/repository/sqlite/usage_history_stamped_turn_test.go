package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-COSTS-USAGE-HISTORY-002.1, AC-COSTS-USAGE-HISTORY-002.2, AC-COSTS-USAGE-HISTORY-002.3
func TestUsageHistoryStampedTurnTransaction(t *testing.T) {
	runUsageHistoryStampedTurn(t, false)
}

// @covers AC-COSTS-USAGE-HISTORY-002.1, AC-COSTS-USAGE-HISTORY-002.2, AC-COSTS-USAGE-HISTORY-002.3
func TestUsageHistoryPostgresStampedTurnTransaction(t *testing.T) {
	runUsageHistoryStampedTurn(t, true)
}

func runUsageHistoryStampedTurn(t *testing.T, postgres bool) {
	t.Helper()
	for _, kind := range []string{"active", "completed", "rollback_retry"} {
		t.Run(kind, func(t *testing.T) {
			repo := newUsageHistoryStampedTurnRepo(t, postgres)
			ctx := t.Context()
			actor := ""
			at := time.Now().UTC().Truncate(time.Microsecond)
			turn := &models.Turn{
				ID: "retained-stamped-turn", TaskID: "stamped-task", TaskSessionID: "stamped-session",
				StartedAt: at, Metadata: map[string]any{"caller_marker": "preserved"},
			}
			if kind == "completed" {
				turn.CompletedAt = &at
				turn.Metadata[models.TurnMetaKeyLifecycleOnly] = true
			} else {
				actor = "stamped-actor"
				ctx = authn.WithIdentity(ctx, authn.Identity{UserID: actor, Role: authn.RoleMember})
			}
			if kind == "rollback_retry" {
				before, err := repo.GetTaskSession(ctx, turn.TaskSessionID)
				require.NoError(t, err)
				drop := failStampedTurnInsert(t, repo)
				_, err = repo.CreateTurnWithStepStamp(ctx, turn)
				require.ErrorContains(t, err, "stamped turn insert fault")
				var turns, bindings int
				require.NoError(t, repo.db.Get(&turns, repo.db.Rebind(`SELECT COUNT(*) FROM task_session_turns WHERE id = ?`), turn.ID))
				require.NoError(t, repo.db.Get(&bindings, repo.db.Rebind(`SELECT COUNT(*) FROM usage_execution_bindings WHERE turn_id = ?`), turn.ID))
				require.Zero(t, turns)
				require.Zero(t, bindings, "native binding admission must roll back with the failed turn insert")
				after, err := repo.GetTaskSession(ctx, turn.TaskSessionID)
				require.NoError(t, err)
				require.Equal(t, before.UpdatedAt, after.UpdatedAt)
				drop()
			}
			stamped, err := repo.CreateTurnWithStepStamp(ctx, turn)
			require.NoError(t, err)
			require.True(t, stamped)
			assertUsageHistoryStampedTurn(t, repo, turn, actor)
			if kind != "completed" {
				event := newTestUsageEvent("stamped-native-event", turn.TaskID, turn.TaskSessionID)
				event.TurnID, event.UsageBindingID = turn.ID, turn.UsageBindingID
				event.CostSource = usageCostSourceProviderReported
				require.NoError(t, repo.CreateTaskUsageEvent(ctx, event))
				var copied int
				require.NoError(t, repo.db.Get(&copied, repo.db.Rebind(`SELECT COUNT(*) FROM session_usage_buckets
					WHERE source = 'native' AND source_record_id = ? AND usage_binding_id = ?
					AND actor_id = ? AND actor_status = 'attributed' AND input_tokens = ? AND total_tokens = ? AND cost_subcents = ?`),
					event.UsageEventID, turn.UsageBindingID, actor, event.TokensIn, event.TokensTotal, event.CostSubcents))
				require.Equal(t, 1, copied, "the production native writer must consume the stamped turn's durable actor binding")
			}
		})
	}
}

func newUsageHistoryStampedTurnRepo(t *testing.T, postgres bool) *Repository {
	t.Helper()
	var repo *Repository
	if postgres {
		db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		var version string
		require.NoError(t, db.Get(&version, `SHOW server_version`))
		t.Logf("PostgreSQL %s", version)
		var err error
		repo, err = NewWithDB(db, db, nil)
		require.NoError(t, err)
	} else {
		repo = newTurnStepStampTestRepo(t)
	}
	history, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	require.NoError(t, err)
	repo.SetUsageHistoryRetainer(history)
	ctx := t.Context()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "stamped-workspace", Name: "Stamped", OrgID: "stamped-org"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "stamped-workflow", WorkspaceID: "stamped-workspace", Name: "Stamped"}))
	_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`),
		"stamped-step", "stamped-workflow", "Work")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "stamped-task", WorkspaceID: "stamped-workspace", WorkflowID: "stamped-workflow", WorkflowStepID: "stamped-step", Title: "Stamped",
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "stamped-session", TaskID: "stamped-task", State: models.TaskSessionStateRunning}))
	return repo
}

func assertUsageHistoryStampedTurn(t *testing.T, repo *Repository, turn *models.Turn, actor string) {
	t.Helper()
	stored, err := repo.GetTurn(t.Context(), turn.ID)
	require.NoError(t, err)
	require.NotEmpty(t, turn.UsageBindingID)
	require.Equal(t, turn.UsageBindingID, stored.UsageBindingID)
	require.Equal(t, "stamped-step", stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
	require.Equal(t, "preserved", stored.Metadata["caller_marker"])
	_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), turn.TaskID)
	require.NoError(t, err)
	require.Positive(t, entry)
	require.Equal(t, float64(entry), stored.Metadata[models.TurnMetaKeyWorkflowStepTransitionIDAtStart])
	if turn.CompletedAt != nil {
		require.Equal(t, *turn.CompletedAt, *stored.CompletedAt)
		require.Equal(t, true, stored.Metadata[models.TurnMetaKeyLifecycleOnly])
	} else {
		require.Nil(t, stored.CompletedAt)
	}
	task, err := repo.GetTask(t.Context(), turn.TaskID)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(t.Context(), turn.TaskSessionID)
	require.NoError(t, err)
	var binding struct {
		Kind         string `db:"binding_kind"`
		WorkspaceID  string `db:"workspace_id"`
		OrgID        string `db:"organization_id"`
		TaskID       string `db:"task_id"`
		TaskBirth    string `db:"task_incarnation_id"`
		SessionID    string `db:"session_id"`
		SessionBirth string `db:"session_incarnation_id"`
		TurnID       string `db:"turn_id"`
		ActorID      string `db:"actor_id"`
		ActorStatus  string `db:"actor_status"`
	}
	require.NoError(t, repo.db.Get(&binding, repo.db.Rebind(`SELECT b.binding_kind, b.workspace_id, s.organization_id,
		b.task_id, b.task_incarnation_id, b.session_id, b.session_incarnation_id, b.turn_id, b.actor_id, b.actor_status
		FROM usage_execution_bindings b JOIN usage_scopes s ON s.scope_id = b.scope_id
		JOIN usage_subjects u ON u.subject_id = b.subject_id AND u.scope_id = b.scope_id
		WHERE b.binding_id = ?`), stored.UsageBindingID))
	require.Equal(t, usagehistory.BindingKindTurn, binding.Kind)
	require.Equal(t, task.WorkspaceID, binding.WorkspaceID)
	require.Equal(t, "stamped-org", binding.OrgID)
	require.Equal(t, turn.TaskID, binding.TaskID)
	require.Equal(t, task.CreatedAt.UTC().Format(time.RFC3339Nano), binding.TaskBirth)
	require.Equal(t, turn.TaskSessionID, binding.SessionID)
	require.Equal(t, session.QueueIncarnationID, binding.SessionBirth)
	require.Equal(t, turn.ID, binding.TurnID)
	require.Equal(t, actor, binding.ActorID)
	status := usagehistory.ActorStatusUnknown
	if actor != "" {
		status = usagehistory.ActorStatusAttributed
	}
	require.Equal(t, status, binding.ActorStatus)
	require.Equal(t, stored.UpdatedAt, session.UpdatedAt)
}

func failStampedTurnInsert(t *testing.T, repo *Repository) func() {
	t.Helper()
	const name = "stamped_turn_insert_fault"
	if repo.db.DriverName() == "pgx" {
		_, err := repo.db.Exec(`CREATE FUNCTION stamped_turn_insert_fault_fn() RETURNS trigger LANGUAGE plpgsql
			AS $$ BEGIN RAISE EXCEPTION 'stamped turn insert fault'; END $$`)
		require.NoError(t, err)
		_, err = repo.db.Exec(`CREATE TRIGGER stamped_turn_insert_fault BEFORE INSERT ON task_session_turns
			FOR EACH ROW EXECUTE FUNCTION stamped_turn_insert_fault_fn()`)
		require.NoError(t, err)
	} else {
		_, err := repo.db.Exec(`CREATE TRIGGER stamped_turn_insert_fault BEFORE INSERT ON task_session_turns
			BEGIN SELECT RAISE(ABORT, 'stamped turn insert fault'); END`)
		require.NoError(t, err)
	}
	drop := func() {
		query := `DROP TRIGGER IF EXISTS ` + name
		if repo.db.DriverName() == "pgx" {
			query += ` ON task_session_turns`
		}
		_, err := repo.db.Exec(query)
		require.NoError(t, err)
		if repo.db.DriverName() == "pgx" {
			_, err = repo.db.Exec(`DROP FUNCTION IF EXISTS stamped_turn_insert_fault_fn()`)
			require.NoError(t, err)
		}
	}
	t.Cleanup(drop)
	return drop
}
