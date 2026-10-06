package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

const admissionTestKey = "repository-branch-policy-admission:"

type admissionWork struct {
	done chan struct{}
	err  error
}

func startAdmissionWork(t *testing.T, ctx context.Context, run func(context.Context) error) *admissionWork {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	work := &admissionWork{done: make(chan struct{})}
	t.Cleanup(func() { cancel(); <-work.done })
	go func() {
		defer close(work.done)
		work.err = run(ctx)
	}()
	return work
}

func joinAdmissionWork(t *testing.T, ctx context.Context, work *admissionWork) {
	t.Helper()
	select {
	case <-work.done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func seedAdmissionRepository(t *testing.T, repo *Repository, id string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + id, Name: "Admission"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: id, WorkspaceID: "ws-" + id, Name: "Admission"}))
}

func admissionPolicies(repositoryID, production, development string) []*models.RepositoryBranchPolicy {
	return []*models.RepositoryBranchPolicy{
		{RepositoryID: repositoryID, Name: "Feature", BaseBranch: development, BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: development},
		{RepositoryID: repositoryID, Name: "Bugfix", BaseBranch: development, BranchTemplate: "bugfix/{title}-{suffix}", PullRequestTarget: development},
		{RepositoryID: repositoryID, Name: "Hotfix", BaseBranch: production, BranchTemplate: "hotfix/{title}-{suffix}", PullRequestTarget: production},
		{RepositoryID: repositoryID, Name: "Release", BaseBranch: development, BranchTemplate: "release/{title}-{suffix}", PullRequestTarget: production},
	}
}

func holdAdmission(t *testing.T, ctx context.Context, db *sqlx.DB, repositoryID string) *sqlx.Tx {
	t.Helper()
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, admissionTestKey+repositoryID)
	require.NoError(t, err)
	return tx
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.4, AC-WORKSPACES-BRANCH-POLICIES-002.6
func TestPostgresGitflowAdmissionBeforeRead(t *testing.T) {
	f := newPGPolicyPatchFixture(t)
	seedAdmissionRepository(t, f.primary, "admission")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	holder := holdAdmission(t, ctx, f.primary.db, "admission")
	work := startAdmissionWork(t, ctx, func(ctx context.Context) error {
		return f.worker.CreateRepositoryBranchPoliciesIfEmpty(ctx, "admission", admissionPolicies("admission", "main", "develop"))
	})
	// Keep the row/error assertions reachable when the unguarded worker finishes.
	waitErr := waitForPostgresLock(ctx, f.observer.db, f.workerPID, work.done)
	if waitErr == nil {
		var query, event string
		require.NoError(t, f.observer.db.QueryRowContext(ctx,
			`SELECT query, wait_event FROM pg_stat_activity WHERE pid = $1`, f.workerPID).Scan(&query, &event))
		require.Contains(t, query, "pg_advisory_xact_lock")
		require.Equal(t, "advisory", event)
		t.Logf("worker %d waiting at repository admission before predicate read", f.workerPID)
	} else {
		t.Logf("worker did not wait for admission: %v", waitErr)
	}
	_, err := holder.ExecContext(ctx, `INSERT INTO repository_branch_policies (`+repositoryBranchPolicyColumns+`)
	 VALUES ('ordinary', 'admission', 'Custom', '', 'release', 'custom/{title}-{suffix}', 'release', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	require.NoError(t, holder.Commit())
	joinAdmissionWork(t, ctx, work)
	rows, err := f.observer.ListRepositoryBranchPolicies(ctx, "admission")
	require.NoError(t, err)
	assert.ErrorIs(t, work.err, repoerrors.ErrRepositoryBranchPoliciesExist)
	require.Len(t, rows, 1)
	require.Equal(t, "ordinary", rows[0].ID)
	require.Equal(t, "release", rows[0].BaseBranch)
}

func requireAdmissionWait(t *testing.T, ctx context.Context, observer *sqlx.DB, pid int, work *admissionWork, fragment string) {
	t.Helper()
	require.NoError(t, waitForPostgresLock(ctx, observer, pid, work.done))
	var query, event string
	require.NoError(t, observer.QueryRowContext(ctx,
		`SELECT query, wait_event FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&query, &event))
	require.Contains(t, query, fragment)
	require.Equal(t, "advisory", event)
	t.Logf("physical worker %d observed waiting on %s", pid, fragment)
}

func holdAdmissionInsert(t *testing.T, ctx context.Context, f pgPolicyPatchFixture) *sqlx.Tx {
	t.Helper()
	_, err := f.primary.db.ExecContext(ctx, fmt.Sprintf(`CREATE FUNCTION admission_insert_gate() RETURNS trigger
	 LANGUAGE plpgsql AS $$ BEGIN IF pg_backend_pid() = %d THEN
	 PERFORM pg_advisory_xact_lock(hashtextextended('admission-fixture-insert', 0)); END IF; RETURN NEW; END $$;
	 CREATE TRIGGER admission_insert_gate BEFORE INSERT ON repository_branch_policies
	 FOR EACH ROW EXECUTE FUNCTION admission_insert_gate()`, f.workerPID))
	require.NoError(t, err)
	holder, err := f.primary.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('admission-fixture-insert', 0))`)
	require.NoError(t, err)
	return holder
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.3, AC-WORKSPACES-BRANCH-POLICIES-002.4
func TestPostgresGitflowAdmissionConcurrency(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		firstOrdinary, custom bool
		nameCollision         bool
	}{
		{name: "competing_starters"},
		{name: "ordinary_first", firstOrdinary: true},
		{name: "starter_first", custom: true},
		{name: "ordinary_name_conflict", custom: true, nameCollision: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkPostgresAdmissionOrder(t, tc.firstOrdinary, tc.custom, tc.nameCollision)
		})
	}
}

func checkPostgresAdmissionOrder(t *testing.T, firstOrdinary, custom, nameCollision bool) {
	f := newPGPolicyPatchFixture(t)
	seedAdmissionRepository(t, f.primary, "pg-order")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	monitor := openSecondPostgresConnection(t, testutil.PostgresDSNFromEnv(t), f.primary.db)
	var latePID, monitorPID int
	require.NoError(t, f.observer.db.GetContext(ctx, &latePID, `SELECT pg_backend_pid()`))
	require.NoError(t, monitor.GetContext(ctx, &monitorPID, `SELECT pg_backend_pid()`))
	require.NotContains(t, []int{f.workerPID, latePID}, monitorPID)
	holder := holdAdmissionInsert(t, ctx, f)
	winner := admissionPolicies("pg-order", "main", "develop")
	first := startAdmissionWork(t, ctx, func(ctx context.Context) error {
		if firstOrdinary {
			return f.worker.CreateRepositoryBranchPolicy(ctx, customAdmissionPolicy("pg-order"))
		}
		return f.worker.CreateRepositoryBranchPoliciesIfEmpty(ctx, "pg-order", winner)
	})
	requireAdmissionWait(t, ctx, monitor, f.workerPID, first, "INSERT INTO repository_branch_policies")
	late := startAdmissionWork(t, ctx, func(ctx context.Context) error {
		if custom {
			policy := customAdmissionPolicy("pg-order")
			if nameCollision {
				policy.Name = "feature"
			}
			return f.observer.CreateRepositoryBranchPolicy(ctx, policy)
		}
		return f.observer.CreateRepositoryBranchPoliciesIfEmpty(ctx, "pg-order", admissionPolicies("pg-order", "release", "next"))
	})
	requireAdmissionWait(t, ctx, monitor, latePID, late, "pg_advisory_xact_lock")
	require.NoError(t, holder.Commit())
	joinAdmissionWork(t, ctx, first)
	joinAdmissionWork(t, ctx, late)
	require.NoError(t, first.err)
	rows, err := f.primary.ListRepositoryBranchPolicies(ctx, "pg-order")
	require.NoError(t, err)
	switch {
	case firstOrdinary:
		require.ErrorIs(t, late.err, repoerrors.ErrRepositoryBranchPoliciesExist)
		require.Len(t, rows, 1)
		require.Equal(t, "Custom", rows[0].Name)
	case nameCollision:
		require.ErrorIs(t, late.err, repoerrors.ErrRepositoryBranchPolicyNameConflict)
		assertAdmissionRows(t, rows, winner, false)
	case custom:
		require.NoError(t, late.err)
		assertAdmissionRows(t, rows, winner, true)
	default:
		require.ErrorIs(t, late.err, repoerrors.ErrRepositoryBranchPoliciesExist)
		assertAdmissionRows(t, rows, winner, false)
	}
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.6
func TestPostgresGitflowAdmissionBehavior(t *testing.T) {
	t.Run("rollback_and_errors", func(t *testing.T) { checkAdmissionErrors(t, openPostgresRepo(t)) })
	for _, ordinary := range []bool{false, true} {
		name := "cancel_starter_and_repository_isolation"
		if ordinary {
			name = "cancel_ordinary_and_repository_isolation"
		}
		t.Run(name, func(t *testing.T) { checkPostgresAdmissionCancellation(t, ordinary) })
	}
}

func checkPostgresAdmissionCancellation(t *testing.T, ordinary bool) {
	f := newPGPolicyPatchFixture(t)
	seedAdmissionRepository(t, f.primary, "pg-cancel")
	seedAdmissionRepository(t, f.primary, "pg-independent")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	holder := holdAdmission(t, ctx, f.primary.db, "pg-cancel")
	workerCtx, stop := context.WithCancel(ctx)
	t.Cleanup(stop)
	work := startAdmissionWork(t, workerCtx, func(ctx context.Context) error {
		if ordinary {
			return f.worker.CreateRepositoryBranchPolicy(ctx, customAdmissionPolicy("pg-cancel"))
		}
		return f.worker.CreateRepositoryBranchPoliciesIfEmpty(ctx, "pg-cancel", admissionPolicies("pg-cancel", "main", "develop"))
	})
	requireAdmissionWait(t, ctx, f.observer.db, f.workerPID, work, "pg_advisory_xact_lock")
	independent := admissionPolicies("pg-independent", "release", "next")
	require.NoError(t, f.observer.CreateRepositoryBranchPoliciesIfEmpty(ctx, "pg-independent", independent))
	stop()
	joinAdmissionWork(t, ctx, work)
	require.ErrorIs(t, work.err, context.Canceled)
	require.NoError(t, holder.Commit())
	rows, err := f.observer.ListRepositoryBranchPolicies(ctx, "pg-cancel")
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, f.observer.CreateRepositoryBranchPoliciesIfEmpty(ctx, "pg-cancel", admissionPolicies("pg-cancel", "main", "develop")))
}
