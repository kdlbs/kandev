//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

const usageBenchWorkspace = "usage-bench-main"

type usageBenchSize struct{ Native, External int }

func usageBenchSQLiteTraceEnabled() bool {
	return os.Getenv("KANDEV_USAGE_BENCH_TRACE_SQLITE") == "1"
}

type usageBenchSubject struct {
	workspace, scope, subject, task, session, binding, work, generation string
	actor, actorStatus, transcript                                      string
	nativeScope                                                         trustedUsageScope
	nativeBinding                                                       retainedNativeUsageBinding
}

type usageBenchFixture struct {
	database              *sqlx.DB
	repo                  *Repository
	observer              *usageBenchObserver
	engine                string
	path                  string
	schema                string
	size                  usageBenchSize
	subjects              []usageBenchSubject
	cases                 []*usageBenchCase
	phase                 string
	seedTime              time.Duration
	storage               usageBenchStorage
	tasks                 *tasksqlite.Repository
	targets               map[string]retainedTarget
	ordinaryAdmission     bool
	retained              map[string]int
	retentionPages        int
	projectionSteps       int
	expectedFacts         map[string]int64
	expectedContributions map[string]int64
}

func usageBenchMust(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newUsageBenchFixture(t testing.TB, engine string, size usageBenchSize) *usageBenchFixture {
	t.Helper()
	started := time.Now()
	f := &usageBenchFixture{engine: engine, size: size, observer: &usageBenchObserver{}, targets: map[string]retainedTarget{}, retained: map[string]int{}, expectedFacts: map[string]int64{}, expectedContributions: map[string]int64{}}
	if engine == "postgres" {
		dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Fatal("PostgreSQL benchmark requires KANDEV_TEST_POSTGRES_DSN; no skipped parity")
		}
		owner := testutil.OpenIsolatedPostgres(t, dsn)
		usageBenchMust(t, owner.Get(&f.schema, `SELECT current_schema()`))
		var version int
		usageBenchMust(t, owner.Get(&version, `SHOW server_version_num`))
		if version < 180000 {
			t.Fatal("PostgreSQL benchmark requires PostgreSQL 18 or newer")
		}
		f.database = owner
		if usageBenchPostgresTraceEnabled() {
			scopedDSN := testutil.PostgresDSNWithSearchPath(t, dsn, f.schema)
			traced, err := usageBenchPostgresPool(scopedDSN, f.observer)
			usageBenchMust(t, err)
			f.database = traced
			t.Cleanup(func() { _ = traced.Close() })
		}
	} else {
		f.path = filepath.Join(t.TempDir(), "usage-history.db")
		raw, err := db.OpenSQLite(f.path)
		usageBenchMust(t, err)
		f.database = sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = f.database.Close() })
		usageBenchMust(t, setUsageBenchSQLiteCollection(context.Background(), f.database.DB, false))
		if usageBenchSQLiteTraceEnabled() {
			usageBenchMust(t, attachUsageBenchSQLite(context.Background(), f.database.DB, f.observer))
		}
	}
	tasks, err := tasksqlite.NewWithDB(f.database, f.database, nil)
	usageBenchMust(t, err)
	f.repo, err = NewWithDB(f.database, f.database)
	usageBenchMust(t, err)
	f.tasks = tasks
	tasks.SetUsageHistoryRetainer(f.repo)
	f.seedSubjects(t, tasks)
	f.cases = usageBenchCases(f.subjects[2].task)
	f.seedTime = time.Since(started)
	return f
}

func (f *usageBenchFixture) seedSubjects(t testing.TB, tasks *tasksqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, workspace := range []string{usageBenchWorkspace, "usage-bench-other"} {
		usageBenchMust(t, tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: workspace, Name: workspace, OrgID: workspace + "-org"}))
		for index := range 12 {
			id := fmt.Sprintf("%s-%d", workspace, index)
			task := &taskmodels.Task{ID: id, WorkspaceID: workspace, Title: id, CreatedAt: created, UpdatedAt: created}
			usageBenchMust(t, tasks.CreateTask(ctx, task))
			// Binding identities use persisted birth precision on both engines.
			// PostgreSQL rounds timestamps independently of the Go input value.
			task, err := tasks.GetTask(ctx, id)
			usageBenchMust(t, err)
			transcript := id + "-producer-thread"
			usageBenchMust(t, tasks.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: id + "-session", TaskID: id, QueueIncarnationID: id + "-session", StartedAt: created, Metadata: map[string]any{"acp": map[string]any{"session_id": transcript}}}))
			session, err := tasks.GetTaskSession(ctx, id+"-session")
			usageBenchMust(t, err)
			binding := usagehistory.Binding{ID: id + "-binding", Kind: usagehistory.BindingKindExecution, WorkspaceID: workspace, OrganizationID: workspace + "-org", TaskID: id, TaskIncarnationID: usageTaskIncarnationID(task.CreatedAt), SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID, ActorID: fmt.Sprintf("user-%d", index%2), ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: created}
			if index == 3 {
				binding.ActorID, binding.ActorStatus, binding.ActorKind = "", "unknown", "unknown"
			}
			tx, err := f.database.BeginTxx(ctx, nil)
			usageBenchMust(t, err)
			if err := f.repo.AdmitBindingTx(ctx, tx, binding); err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			// Read the accepted native evidence back through the same binding
			// validation as retention, rather than constructing trusted rows.
			probe := usagehistory.NativeUsage{BindingID: binding.ID, UsageEventID: "binding-proof", TaskID: id, SessionID: binding.SessionID}
			nativeScope, err := f.repo.resolveNativeBindingScopeTx(ctx, tx, probe)
			usageBenchMust(t, err)
			nativeBinding, err := f.repo.admitNativeUsageBindingTx(ctx, tx, &probe, nativeScope)
			usageBenchMust(t, err)
			usageBenchMust(t, tx.Commit())
			scope := usageScopeID(workspace, binding.OrganizationID)
			subject := usageSubjectID(scope, id, binding.TaskIncarnationID, binding.SessionID, binding.SessionIncarnationID)
			entry := usageBenchSubject{workspace: workspace, scope: scope, subject: subject, task: id, session: binding.SessionID, binding: binding.ID, actor: binding.ActorID, actorStatus: binding.ActorStatus, transcript: transcript}
			entry.nativeScope, entry.nativeBinding = nativeScope, nativeBinding
			entry.work = transcriptUsageWork(scope, subject, transcript)
			f.subjects = append(f.subjects, entry)
			// This point lookup checks the immutable session-birth proof. A fixture
			// must fail rather than manufacture eligibility or enumerate an archive.
			f.collectionTarget(t, entry, "plugin:bench")
		}
	}
}

func (f *usageBenchFixture) seedRecent(t testing.TB) {
	t.Helper()
	start := time.Now()
	t.Logf("usage fixture recent phase start native=%d external=%d", f.size.Native/5, f.size.External/5)
	f.seedRange(t, "recent", 2025, 2026, f.size.Native/5, f.size.External/5)
	t.Log("usage fixture recent range inserts complete")
	f.seedSpecial(t)
	t.Log("usage fixture special records complete")
	f.seedRange(t, "other", 2025, 2026, 8, 8)
	t.Log("usage fixture second scope inserts complete")
	f.publish(t)
	t.Logf("usage fixture recent publication complete pages=%d", f.projectionSteps)
	// Exercise real preservation/deletion after capture and publication.
	usageBenchMust(t, f.tasks.DeleteTask(t.Context(), f.subjects[2].task))
	t.Log("usage fixture deleted-task preservation complete")
	_, err := f.database.Exec(f.database.Rebind(`UPDATE tasks SET archived_at=? WHERE id=?`), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), f.subjects[1].task)
	usageBenchMust(t, err)
	t.Log("usage fixture archived-task preservation complete")
	f.phase = "one_year"
	f.seedTime += time.Since(start)
}

func (f *usageBenchFixture) seedOlder(t testing.TB) {
	t.Helper()
	start := time.Now()
	f.seedRange(t, "older", 2021, 2025, f.size.Native-f.size.Native/5, f.size.External-f.size.External/5)
	f.publish(t)
	f.phase = "five_years"
	f.seedTime += time.Since(start)
}
