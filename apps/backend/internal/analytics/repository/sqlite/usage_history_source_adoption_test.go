package sqlite

import (
	"context"
	"expvar"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type sourceAdoptionFixture struct {
	repo          *Repository
	tasks         *tasksqlite.Repository
	db            *sqlx.DB
	workspaceSeen map[string]bool
	taskSeen      map[string]bool
}

func newSourceAdoptionFixture(t *testing.T, postgres bool) *sourceAdoptionFixture {
	t.Helper()
	var database *sqlx.DB
	if postgres {
		database = testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	} else {
		raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "adoption.db"))
		require.NoError(t, err)
		database = sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = database.Close() })
	}
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	repo, err := NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	return &sourceAdoptionFixture{repo: repo, tasks: tasks, db: database, workspaceSeen: map[string]bool{}, taskSeen: map[string]bool{}}
}

func (f *sourceAdoptionFixture) createSession(t *testing.T, workspace, task, session, transcript string) {
	t.Helper()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !f.workspaceSeen[workspace] {
		require.NoError(t, f.tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: workspace, Name: workspace, OrgID: "org-adopt"}))
		f.workspaceSeen[workspace] = true
	}
	if !f.taskSeen[task] {
		require.NoError(t, f.tasks.CreateTask(t.Context(), &taskmodels.Task{ID: task, WorkspaceID: workspace, Title: task, CreatedAt: at, UpdatedAt: at}))
		f.taskSeen[task] = true
	}
	metadata := "{}"
	if transcript != "" {
		metadata = `{"acp":{"session_id":"` + transcript + `"}}`
	}
	_, err := f.db.Exec(f.db.Rebind(`INSERT INTO task_sessions (id,task_id,agent_profile_id,state,started_at,updated_at,metadata)
		VALUES (?, ?, 'agent', 'RUNNING', ?, ?, ?)`), session, task, at, at, metadata)
	require.NoError(t, err)
}

func (f *sourceAdoptionFixture) adoptionPage(t *testing.T, workspace string) pluginsdk.UsageCollectionTargetPageV1 {
	t.Helper()
	page, err := f.repo.ListUsageCollectionTargets(t.Context(), "collector-adoption", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: workspace})
	require.NoError(t, err)
	return page
}

func finishSourceAdoption(t *testing.T, repo *Repository) {
	t.Helper()
	for range 1000 {
		done, err := repo.BackfillUsageSourceAdoption(context.Background(), usageHistoryBatchLimit)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("source adoption did not finish")
}

func TestSourceAdoptionAdoptsSafeSessionsAndStops(t *testing.T) {
	f := newSourceAdoptionFixture(t, false)
	f.createSession(t, "ws-adopt", "task-adopt", "session-adopt", "transcript-adopt")
	before := f.adoptionPage(t, "ws-adopt")
	require.True(t, before.Preparing, "discovery must wait for the adoption cursor")
	require.Empty(t, before.Targets)
	finishSourceAdoption(t, f.repo)
	page := f.adoptionPage(t, "ws-adopt")
	require.False(t, page.Preparing)
	require.Len(t, page.Targets, 1)
	require.Equal(t, "eligible", page.Targets[0].SourceScopeStatus)
	require.Equal(t, "transcript-adopt", *page.Targets[0].ExternalSessionID)
	require.Equal(t, "unknown", page.Targets[0].ActorAttributionStatus)
	var evidence string
	require.NoError(t, f.db.Get(&evidence, `SELECT evidence FROM usage_collection_contexts WHERE workspace_id='ws-adopt'`))
	require.Equal(t, "migration", evidence)
	var contexts int
	require.NoError(t, f.db.Get(&contexts, `SELECT COUNT(*) FROM usage_collection_contexts WHERE workspace_id='ws-adopt'`))
	require.Equal(t, 1, contexts, "adoption must not duplicate context evidence")
	// A session created after the cursor completed is not adopted again.
	f.createSession(t, "ws-adopt", "task-later", "session-later", "transcript-later")
	later := f.adoptionPage(t, "ws-adopt")
	require.Len(t, later.Targets, 2)
	var status string
	for _, target := range later.Targets {
		if target.SessionID != nil && *target.SessionID == "session-later" {
			status = target.SourceScopeStatus
		}
	}
	require.Equal(t, "unknown", status, "a completed adoption cursor must not walk sessions again")
}

func TestSourceAdoptionSkipsMissingForkedTransferredAndErased(t *testing.T) {
	f := newSourceAdoptionFixture(t, false)
	f.createSession(t, "ws-adopt", "task-missing", "session-missing", "")
	f.createSession(t, "ws-fork", "task-fork", "session-fork", "transcript-fork")
	_, err := f.db.Exec(`UPDATE task_sessions SET metadata = ? WHERE id = 'session-fork'`,
		`{"acp":{"session_id":"transcript-fork"},"codex_fork_source_session_id":"foreign"}`)
	require.NoError(t, err)
	f.createSession(t, "ws-transfer", "task-transfer", "session-transfer", "transcript-transfer")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = f.db.Exec(f.db.Rebind(`INSERT INTO task_transfer_audit
		(id,operation_id,actor_kind,actor_id,actor_session_id,task_id,source_workspace_id,source_workflow_id,source_step_id,
		 destination_workspace_id,destination_workflow_id,destination_step_id,task_generation,session_census_json,
		 preservation_digest,idempotency_key,preservation_policy,result,created_at)
		VALUES ('audit-1','operation-1','user','actor','session','task-transfer','ws-a','wf-a','step-a','ws-b','wf-b','step-b',?,'[]','digest','idem','preserve','committed',?)`),
		at, at)
	require.NoError(t, err)
	f.createSession(t, "ws-erased", "task-erased", "session-erased", "transcript-erased")
	_, err = f.db.Exec(f.db.Rebind(`INSERT INTO usage_scope_fences (fence_kind,fence_id,state,generation,updated_at)
		VALUES ('workspace','ws-erased','erasing',1,?)`), at)
	require.NoError(t, err)

	finishSourceAdoption(t, f.repo)
	for _, workspace := range []string{"ws-adopt", "ws-fork", "ws-transfer"} {
		page := f.adoptionPage(t, workspace)
		require.False(t, page.Preparing)
		require.Len(t, page.Targets, 1)
	}
	missing := f.adoptionPage(t, "ws-adopt")
	require.Equal(t, "unknown", missing.Targets[0].SourceScopeStatus)
	forked := f.adoptionPage(t, "ws-fork")
	require.NotEqual(t, "eligible", forked.Targets[0].SourceScopeStatus)
	require.Nil(t, forked.Targets[0].ExternalSessionID)
	transferred := f.adoptionPage(t, "ws-transfer")
	require.NotEqual(t, "eligible", transferred.Targets[0].SourceScopeStatus)
	require.Nil(t, transferred.Targets[0].ExternalSessionID)

	var subjects, bindings, contexts int
	require.NoError(t, f.db.Get(&subjects, `SELECT COUNT(*) FROM usage_subjects s JOIN usage_scopes sc ON sc.scope_id=s.scope_id WHERE sc.workspace_id='ws-erased'`))
	require.NoError(t, f.db.Get(&bindings, `SELECT COUNT(*) FROM usage_collection_bindings WHERE workspace_id='ws-erased'`))
	require.NoError(t, f.db.Get(&contexts, `SELECT COUNT(*) FROM usage_collection_contexts WHERE workspace_id='ws-erased'`))
	require.Zero(t, subjects)
	require.Zero(t, bindings)
	require.Zero(t, contexts)
}

func TestSourceAdoptionMarksSharedTranscriptUnsafe(t *testing.T) {
	f := newSourceAdoptionFixture(t, false)
	f.createSession(t, "ws-first", "task-first", "session-first", "transcript-shared")
	f.createSession(t, "ws-second", "task-second", "session-second", "transcript-shared")
	finishSourceAdoption(t, f.repo)
	first := f.adoptionPage(t, "ws-first")
	second := f.adoptionPage(t, "ws-second")
	require.NotEqual(t, "eligible", first.Targets[0].SourceScopeStatus)
	require.NotEqual(t, "eligible", second.Targets[0].SourceScopeStatus)
	require.Nil(t, first.Targets[0].ExternalSessionID)
	require.Nil(t, second.Targets[0].ExternalSessionID)
}

func TestSourceAdoptionResumesFromCursor(t *testing.T) {
	f := newSourceAdoptionFixture(t, false)
	for _, suffix := range []string{"a", "b", "c", "d", "e"} {
		f.createSession(t, "ws-resume", "task-"+suffix, "session-"+suffix, "transcript-"+suffix)
	}
	done, err := f.repo.BackfillUsageSourceAdoption(t.Context(), 2)
	require.NoError(t, err)
	require.False(t, done)
	finishSourceAdoption(t, f.repo)
	var contexts, eligible int
	require.NoError(t, f.db.Get(&contexts, `SELECT COUNT(*) FROM usage_collection_contexts WHERE workspace_id='ws-resume'`))
	require.NoError(t, f.db.Get(&eligible, `SELECT COUNT(*) FROM usage_collection_contexts WHERE workspace_id='ws-resume' AND trusted=1 AND evidence='migration'`))
	require.Equal(t, 5, contexts)
	require.Equal(t, 5, eligible)
}

func TestSourceAdoptionPostgres(t *testing.T) {
	f := newSourceAdoptionFixture(t, true)
	f.createSession(t, "ws-postgres", "task-postgres", "session-postgres", "transcript-postgres")
	finishSourceAdoption(t, f.repo)
	page := f.adoptionPage(t, "ws-postgres")
	require.False(t, page.Preparing)
	require.Len(t, page.Targets, 1)
	require.Equal(t, "eligible", page.Targets[0].SourceScopeStatus)
}

func TestSourceAdoptionMetricIsRegistered(t *testing.T) {
	require.NotNil(t, expvar.Get("usage_source_adoption_total"))
	f := newSourceAdoptionFixture(t, false)
	f.createSession(t, "ws-metric", "task-metric", "session-metric", "transcript-metric")
	before := usageSourceAdoptionValue("adopted")
	finishSourceAdoption(t, f.repo)
	require.Equal(t, before+1, usageSourceAdoptionValue("adopted"))
}

func usageSourceAdoptionValue(label string) int64 {
	counter, ok := expvar.Get("usage_source_adoption_total").(*expvar.Map)
	if !ok {
		return 0
	}
	value, ok := counter.Get(label).(*expvar.Int)
	if !ok {
		return 0
	}
	return value.Value()
}

func TestSourceAdoptionMetricCountsOnlyCommittedPages(t *testing.T) {
	f := newSourceAdoptionFixture(t, false)
	f.createSession(t, "ws-metric-rollback", "task-metric-rollback", "session-metric-rollback", "transcript-metric-rollback")
	before := usageSourceAdoptionValue("adopted")
	_, err := f.db.Exec(`CREATE TRIGGER fail_adoption_cursor BEFORE UPDATE ON usage_history_migration_state
		WHEN NEW.source_key = 'source-adoption' BEGIN SELECT RAISE(ABORT, 'forced adoption failure'); END`)
	require.NoError(t, err)
	_, err = f.repo.BackfillUsageSourceAdoption(t.Context(), usageHistoryBatchLimit)
	require.Error(t, err)
	require.Equal(t, before, usageSourceAdoptionValue("adopted"), "a rolled-back page must not count an adoption")
	var contexts int
	require.NoError(t, f.db.Get(&contexts, `SELECT COUNT(*) FROM usage_collection_contexts WHERE workspace_id = 'ws-metric-rollback'`))
	require.Zero(t, contexts)
	_, err = f.db.Exec(`DROP TRIGGER fail_adoption_cursor`)
	require.NoError(t, err)
	finishSourceAdoption(t, f.repo)
	require.Equal(t, before+1, usageSourceAdoptionValue("adopted"))
}
