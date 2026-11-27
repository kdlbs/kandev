package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestManualCreationPermitIsExactTransactionalAndConsumed(t *testing.T) {
	assertManualCreationPermit(t, newRepoForSessionTests(t))
}

func TestPostgresManualCreationPermitIsExactTransactionalAndConsumed(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	assertManualCreationPermit(t, repo)
}

func assertManualCreationPermit(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.initSessionSchema(), "replaying the additive schema must retain manual creation authority")
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "manual-task", Title: "Manual"}))
	for _, kind := range []string{"session", "turn"} {
		table := "task_sessions"
		if kind == "turn" {
			table = "task_session_turns"
		}
		condition := fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM task_creation_authorizations WHERE kind='%s' AND creation_id=NEW.id AND task_id=NEW.task_id)`, kind)
		query := fmt.Sprintf(`CREATE TRIGGER exact_%s_creation BEFORE INSERT ON %s WHEN %s BEGIN SELECT RAISE(ABORT,'missing exact permit'); END;`, kind, table, condition)
		if dialect.IsPostgres(repo.db.DriverName()) {
			query = fmt.Sprintf(`CREATE FUNCTION exact_%s_creation_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF %s THEN RAISE EXCEPTION 'missing exact permit'; END IF; RETURN NEW; END; $$;
   CREATE TRIGGER exact_%s_creation BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION exact_%s_creation_guard();`, kind, condition, kind, table, kind)
		}
		_, err := repo.db.Exec(query)
		require.NoError(t, err)
	}
	require.Error(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "untrusted", TaskID: "manual-task", Metadata: map[string]interface{}{"explicit_manual_creation": true}}))
	manual := models.WithExplicitManualCreation(ctx, "manual-task")
	session := &models.TaskSession{ID: "explicit", TaskID: "manual-task"}
	require.NoError(t, repo.CreateTaskSession(manual, session))
	var permits int
	require.NoError(t, repo.db.Get(&permits, `SELECT COUNT(*) FROM task_creation_authorizations`))
	require.Zero(t, permits)
	require.Error(t, repo.CreateTurn(ctx, &models.Turn{ID: "untrusted-turn", TaskID: session.TaskID, TaskSessionID: session.ID}))
	require.NoError(t, repo.CreateTurn(manual, &models.Turn{ID: "explicit-turn", TaskID: session.TaskID, TaskSessionID: session.ID}))
	require.Error(t, repo.CreateTaskSession(manual, &models.TaskSession{ID: session.ID, TaskID: session.TaskID}), "duplicate INSERT must roll its permit back")
	require.NoError(t, repo.db.Get(&permits, `SELECT COUNT(*) FROM task_creation_authorizations`))
	require.Zero(t, permits)
	require.Error(t, repo.authorizeManualCreation(manual, repo.db, "session", "outside-transaction", session.TaskID))
	require.Error(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "after-manual", TaskID: session.TaskID}))
}
