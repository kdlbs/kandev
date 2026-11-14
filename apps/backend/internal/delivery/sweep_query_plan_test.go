package delivery

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/testutil"
)

const deliveryEnvironmentTaskIndex = "idx_task_sessions_environment_task"

// TestDeliveryGitSnapshotObservationUsesEnvironmentTaskIndex exercises the
// production observation query through the real task schema and rejects the
// full session-binding scan that made delivery refresh scale with unrelated
// session history.
func TestDeliveryGitSnapshotObservationUsesEnvironmentTaskIndex(t *testing.T) {
	repo, db := newInternalTestRepo(t)
	fixture := seedGitSnapshotObservationPlanFixture(t, db, 3500)
	if _, err := db.Exec(`ANALYZE`); err != nil {
		t.Fatalf("analyze planner fixture: %v", err)
	}

	details := explainQueryPlan(t, db,
		mostRecentGitSnapshotObservationQuery(db.DriverName()),
		fixture.taskID, fixture.taskID, fixture.repositoryID)
	joined := strings.Join(details, "\n")
	if strings.Contains(joined, "SCAN binding") {
		t.Fatalf("observation plan still scans session bindings:\n%s", joined)
	}
	if !strings.Contains(joined, "SEARCH binding") {
		t.Fatalf("observation plan does not search session bindings:\n%s", joined)
	}
	normalized := strings.Join(strings.Fields(joined), " ")
	if !strings.Contains(normalized, "SEARCH binding USING COVERING INDEX "+deliveryEnvironmentTaskIndex) {
		t.Fatalf("observation plan = %q, want a covering search on %s", normalized, deliveryEnvironmentTaskIndex)
	}
	for _, key := range []string{"task_environment_id=?", "task_id=?"} {
		if !strings.Contains(normalized, key) {
			t.Fatalf("observation plan = %q, want binding equality key %q", normalized, key)
		}
	}

	observed, err := repo.mostRecentInputObservation(context.Background(),
		fixture.taskID, fixture.repositoryID, fixture.repositoryUpdatedAt, fixture.taskUpdatedAt)
	if err != nil {
		t.Fatalf("read observation: %v", err)
	}
	if !observed.Equal(fixture.wantObservation) {
		t.Fatalf("observation = %v, want %v", observed, fixture.wantObservation)
	}
}

// TestDeliveryGitSnapshotEnvironmentEligibility preserves the delivery
// selection semantics while the new index serves the binding predicate. It
// runs the same fixture on SQLite and PostgreSQL; PostgreSQL skips when no DSN
// is configured.
func TestDeliveryGitSnapshotEnvironmentEligibility(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, db := newInternalTestRepoForEngine(t, engine)
			fixture := seedGitSnapshotEligibilityFixture(t, db)

			snapshots, err := repo.SnapshotsForPair(context.Background(), fixture.taskID, fixture.repositoryID)
			if err != nil {
				t.Fatalf("read snapshots for pair: %v", err)
			}
			branches := make(map[string]bool, len(snapshots))
			for _, snapshot := range snapshots {
				branches[snapshot.Branch] = true
			}
			if len(snapshots) != 2 || !branches["own"] || !branches["shared"] {
				t.Fatalf("snapshots = %+v, want own null-provenance and shared mixed-binding environments", snapshots)
			}

			observed, err := repo.mostRecentInputObservation(context.Background(),
				fixture.taskID, fixture.repositoryID, fixture.repositoryUpdatedAt, fixture.taskUpdatedAt)
			if err != nil {
				t.Fatalf("read observation: %v", err)
			}
			if !observed.Equal(fixture.wantObservation) {
				t.Fatalf("observation = %v, want %v (unrelated and deleted bindings must not win)", observed, fixture.wantObservation)
			}
		})
	}
}

func newInternalTestRepoForEngine(t *testing.T, engine string) (*Repository, *sqlx.DB) {
	t.Helper()
	if engine == "postgres" {
		db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		if _, _, err := taskrepo.Provide(db, db, nil); err != nil {
			t.Fatalf("init postgres task schema: %v", err)
		}
		repo, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("init postgres delivery schema: %v", err)
		}
		return repo, db
	}
	return newInternalTestRepo(t)
}

func explainQueryPlan(t *testing.T, db *sqlx.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryxContext(context.Background(), db.Rebind("EXPLAIN QUERY PLAN\n"+query), args...)
	if err != nil {
		t.Fatalf("explain observation query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan observation plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read observation plan: %v", err)
	}
	return details
}

type gitSnapshotObservationPlanFixture struct {
	taskID              string
	repositoryID        string
	taskUpdatedAt       time.Time
	repositoryUpdatedAt time.Time
	wantObservation     time.Time
}

func seedGitSnapshotObservationPlanFixture(t *testing.T, db *sqlx.DB, count int) gitSnapshotObservationPlanFixture {
	t.Helper()
	const (
		workspaceID  = "ws-plan-fixture"
		repositoryID = "repo-plan-fixture"
		taskID       = "task-plan-target"
	)
	base := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	taskUpdatedAt := base.Add(-3 * time.Hour)
	repositoryUpdatedAt := base.Add(-2 * time.Hour)

	tx, err := db.Beginx()
	if err != nil {
		t.Fatalf("begin planner fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), workspaceID, workspaceID, taskUpdatedAt, taskUpdatedAt); err != nil {
		t.Fatalf("seed planner workspace: %v", err)
	}
	if _, err := tx.Exec(db.Rebind(`
		INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), repositoryID, workspaceID, "repo-plan", repositoryUpdatedAt, repositoryUpdatedAt); err != nil {
		t.Fatalf("seed planner repository: %v", err)
	}
	if _, err := tx.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), taskID, workspaceID, taskID, taskUpdatedAt, taskUpdatedAt); err != nil {
		t.Fatalf("seed planner target task: %v", err)
	}

	ownerTask, err := tx.Preparex(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`))
	if err != nil {
		t.Fatalf("prepare owner task insert: %v", err)
	}
	defer func() { _ = ownerTask.Close() }()
	environment, err := tx.Preparex(db.Rebind(`
		INSERT INTO task_environments (id, task_id, status, created_at, updated_at) VALUES (?, ?, 'ready', ?, ?)
	`))
	if err != nil {
		t.Fatalf("prepare environment insert: %v", err)
	}
	defer func() { _ = environment.Close() }()
	environmentRepo, err := tx.Preparex(db.Rebind(`
		INSERT INTO task_environment_repos
			(id, task_environment_id, repository_id, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)
	`))
	if err != nil {
		t.Fatalf("prepare environment repository insert: %v", err)
	}
	defer func() { _ = environmentRepo.Close() }()
	binding, err := tx.Preparex(db.Rebind(`
		INSERT INTO task_sessions
			(id, task_id, repository_id, task_environment_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, 'RUNNING', ?, ?)
	`))
	if err != nil {
		t.Fatalf("prepare binding insert: %v", err)
	}
	defer func() { _ = binding.Close() }()
	snapshot, err := tx.Preparex(db.Rebind(`
		INSERT INTO task_session_git_snapshots
			(id, task_environment_id, session_id, snapshot_type, branch, head_commit, ahead, metadata, created_at)
		VALUES (?, ?, ?, 'test', 'main', ?, 1, '{}', ?)
	`))
	if err != nil {
		t.Fatalf("prepare snapshot insert: %v", err)
	}
	defer func() { _ = snapshot.Close() }()

	seededAt := base.Add(-4 * time.Hour)
	bindingUpdatedAt := base.Add(-3 * time.Hour)
	for index := 0; index < count; index++ {
		suffix := fmt.Sprintf("%04d", index)
		ownerTaskID := "task-plan-owner-" + suffix
		environmentID := "env-plan-" + suffix
		createdAt := base.Add(time.Duration(index) * time.Second)
		if _, err := ownerTask.Exec(ownerTaskID, workspaceID, ownerTaskID, seededAt, seededAt); err != nil {
			t.Fatalf("seed owner task %s: %v", suffix, err)
		}
		if _, err := environment.Exec(environmentID, ownerTaskID, seededAt, seededAt); err != nil {
			t.Fatalf("seed environment %s: %v", suffix, err)
		}
		if _, err := environmentRepo.Exec("env-repo-"+suffix, environmentID, repositoryID, seededAt, seededAt); err != nil {
			t.Fatalf("seed environment repository %s: %v", suffix, err)
		}
		bindingID := "session-plan-" + suffix
		if _, err := binding.Exec(bindingID, taskID, repositoryID, environmentID, bindingUpdatedAt, bindingUpdatedAt); err != nil {
			t.Fatalf("seed target binding %s: %v", suffix, err)
		}
		if _, err := snapshot.Exec("snapshot-plan-"+suffix, environmentID, bindingID, "sha-"+suffix, createdAt); err != nil {
			t.Fatalf("seed snapshot %s: %v", suffix, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit planner fixture: %v", err)
	}

	return gitSnapshotObservationPlanFixture{
		taskID:              taskID,
		repositoryID:        repositoryID,
		taskUpdatedAt:       taskUpdatedAt,
		repositoryUpdatedAt: repositoryUpdatedAt,
		wantObservation:     base.Add(time.Duration(count-1) * time.Second),
	}
}

type gitSnapshotEligibilityFixture struct {
	taskID              string
	repositoryID        string
	taskUpdatedAt       time.Time
	repositoryUpdatedAt time.Time
	wantObservation     time.Time
}

func seedGitSnapshotEligibilityFixture(t *testing.T, db *sqlx.DB) gitSnapshotEligibilityFixture {
	t.Helper()
	const (
		workspaceID  = "ws-eligibility"
		repositoryID = "repo-eligibility"
		targetTaskID = "task-eligibility-target"
		otherTaskID  = "task-eligibility-other"
	)
	base := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	taskUpdatedAt := base.Add(-time.Hour)
	repositoryUpdatedAt := base.Add(-time.Hour)

	tx, err := db.Beginx()
	if err != nil {
		t.Fatalf("begin eligibility fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), workspaceID, workspaceID, taskUpdatedAt, taskUpdatedAt); err != nil {
		t.Fatalf("seed eligibility workspace: %v", err)
	}
	if _, err := tx.Exec(db.Rebind(`
		INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), repositoryID, workspaceID, repositoryID, repositoryUpdatedAt, repositoryUpdatedAt); err != nil {
		t.Fatalf("seed eligibility repository: %v", err)
	}
	for _, task := range []string{targetTaskID, otherTaskID, "task-eligibility-unrelated", "task-eligibility-deleted"} {
		if _, err := tx.Exec(db.Rebind(`
			INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		`), task, workspaceID, task, taskUpdatedAt, taskUpdatedAt); err != nil {
			t.Fatalf("seed eligibility task %s: %v", task, err)
		}
	}
	insertEnvironment := func(id, task string) {
		t.Helper()
		if _, err := tx.Exec(db.Rebind(`
			INSERT INTO task_environments (id, task_id, status, created_at, updated_at) VALUES (?, ?, 'ready', ?, ?)
		`), id, task, base, base); err != nil {
			t.Fatalf("seed eligibility environment %s: %v", id, err)
		}
	}
	insertEnvironmentRepo := func(id, environment string, deletedAt *time.Time) {
		t.Helper()
		if _, err := tx.Exec(db.Rebind(`
			INSERT INTO task_environment_repos
				(id, task_environment_id, repository_id, status, created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, 'active', ?, ?, ?)
		`), id, environment, repositoryID, base, base, deletedAt); err != nil {
			t.Fatalf("seed eligibility environment repository %s: %v", id, err)
		}
	}
	insertBinding := func(id, task, environment, repository string) {
		t.Helper()
		if _, err := tx.Exec(db.Rebind(`
			INSERT INTO task_sessions
				(id, task_id, repository_id, task_environment_id, state, started_at, updated_at)
			VALUES (?, ?, ?, ?, 'RUNNING', ?, ?)
		`), id, task, repository, environment, base.Add(-30*time.Minute), base.Add(-30*time.Minute)); err != nil {
			t.Fatalf("seed eligibility binding %s: %v", id, err)
		}
	}
	insertSnapshot := func(id, environment string, sessionID any, branch string, createdAt time.Time) {
		t.Helper()
		if _, err := tx.Exec(db.Rebind(`
			INSERT INTO task_session_git_snapshots
				(id, task_environment_id, session_id, snapshot_type, branch, head_commit, ahead, metadata, created_at)
			VALUES (?, ?, ?, 'test', ?, 'sha', 1, '{}', ?)
		`), id, environment, sessionID, branch, createdAt); err != nil {
			t.Fatalf("seed eligibility snapshot %s: %v", id, err)
		}
	}

	insertEnvironment("env-eligibility-own", targetTaskID)
	insertEnvironmentRepo("env-repo-eligibility-own", "env-eligibility-own", nil)
	insertSnapshot("snapshot-eligibility-own", "env-eligibility-own", nil, "own", base.Add(time.Minute))

	insertEnvironment("env-eligibility-shared", otherTaskID)
	insertEnvironmentRepo("env-repo-eligibility-shared", "env-eligibility-shared", nil)
	insertBinding("session-eligibility-shared-target", targetTaskID, "env-eligibility-shared", repositoryID)
	insertBinding("session-eligibility-shared-other", otherTaskID, "env-eligibility-shared", "")
	insertSnapshot("snapshot-eligibility-shared", "env-eligibility-shared", "session-eligibility-shared-other", "shared", base.Add(2*time.Minute))

	insertEnvironment("env-eligibility-unrelated", "task-eligibility-unrelated")
	insertEnvironmentRepo("env-repo-eligibility-unrelated", "env-eligibility-unrelated", nil)
	insertBinding("session-eligibility-unrelated", "task-eligibility-unrelated", "env-eligibility-unrelated", repositoryID)
	insertSnapshot("snapshot-eligibility-unrelated", "env-eligibility-unrelated", "session-eligibility-unrelated", "unrelated", base.Add(10*time.Minute))

	deletedAt := base.Add(-time.Minute)
	insertEnvironment("env-eligibility-deleted", "task-eligibility-deleted")
	insertEnvironmentRepo("env-repo-eligibility-deleted", "env-eligibility-deleted", &deletedAt)
	insertBinding("session-eligibility-deleted-target", targetTaskID, "env-eligibility-deleted", repositoryID)
	insertSnapshot("snapshot-eligibility-deleted", "env-eligibility-deleted", nil, "deleted", base.Add(20*time.Minute))

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit eligibility fixture: %v", err)
	}

	return gitSnapshotEligibilityFixture{
		taskID:              targetTaskID,
		repositoryID:        repositoryID,
		taskUpdatedAt:       taskUpdatedAt,
		repositoryUpdatedAt: repositoryUpdatedAt,
		wantObservation:     base.Add(2 * time.Minute),
	}
}
