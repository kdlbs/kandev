package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresBranchPolicyPatchBehavior(t *testing.T) {
	testutil.PostgresDSNFromEnv(t)
	for _, tc := range []struct {
		name  string
		check func(*testing.T, *Repository)
	}{
		{"presence and defaults", checkBranchPolicyPatchPresence}, {"rollback", checkBranchPolicyPatchRollback},
		{"uniqueness", checkBranchPolicyPatchUniqueness}, {"scope and deletion", checkBranchPolicyPatchScope},
	} {
		t.Run(tc.name, func(t *testing.T) { tc.check(t, openPostgresRepo(t)) })
	}
}

type pgPolicyPatchFixture struct {
	primary, worker, observer *Repository
	workerPID                 int
}

func newPGPolicyPatchFixture(t *testing.T) pgPolicyPatchFixture {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	primary := openPostgresRepo(t)
	workerDB := openSecondPostgresConnection(t, dsn, primary.db)
	observerDB := openSecondPostgresConnection(t, dsn, primary.db)
	worker := NewWithInitializedDB(workerDB, workerDB, nil)
	observer := NewWithInitializedDB(observerDB, observerDB, nil)
	var primaryPID, workerPID, observerPID int
	require.NoError(t, primary.db.Get(&primaryPID, `SELECT pg_backend_pid()`))
	require.NoError(t, workerDB.Get(&workerPID, `SELECT pg_backend_pid()`))
	require.NoError(t, observerDB.Get(&observerPID, `SELECT pg_backend_pid()`))
	require.NotEqual(t, primaryPID, workerPID)
	require.NotEqual(t, observerPID, workerPID)
	require.NotEqual(t, primaryPID, observerPID)
	t.Logf("independent physical connections: holder=%d worker=%d observer=%d", primaryPID, workerPID, observerPID)
	return pgPolicyPatchFixture{primary, worker, observer, workerPID}
}

type pgPolicyPatchWork struct {
	done   chan struct{}
	policy *models.RepositoryBranchPolicy
	err    error
}

func startPGPolicyPatch(t *testing.T, ctx context.Context, repo *Repository, policy *models.RepositoryBranchPolicy, patch *models.RepositoryBranchPolicyPatch) *pgPolicyPatchWork {
	t.Helper()
	operationCtx, cancel := context.WithCancel(ctx)
	work := &pgPolicyPatchWork{done: make(chan struct{})}
	t.Cleanup(func() { cancel(); <-work.done })
	go func() {
		defer close(work.done)
		work.policy, work.err = repo.PatchRepositoryBranchPolicy(operationCtx, policy.ID, policy.RepositoryID, patch, storePatchNormalizer(patch.PullRequestTarget != nil))
	}()
	return work
}

func joinPGPolicyPatch(t *testing.T, ctx context.Context, work *pgPolicyPatchWork) {
	t.Helper()
	select {
	case <-work.done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func assertPGPolicyPatchWait(t *testing.T, ctx context.Context, fixture pgPolicyPatchFixture, work *pgPolicyPatchWork, fragment string) {
	t.Helper()
	require.NoError(t, waitForPostgresLock(ctx, fixture.observer.db, fixture.workerPID, work.done))
	var query, event string
	require.NoError(t, fixture.observer.db.QueryRowContext(ctx,
		`SELECT query, wait_event FROM pg_stat_activity WHERE pid = $1`, fixture.workerPID).Scan(&query, &event))
	require.Contains(t, query, fragment)
	if strings.Contains(fragment, "pg_advisory") {
		require.Equal(t, "advisory", event)
	}
	t.Logf("worker %d observed waiting on %s before proceeding (%s)", fixture.workerPID, event, fragment)
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-001.8, AC-WORKSPACES-BRANCH-POLICIES-001.10
func TestPostgresBranchPolicyPatchConcurrency(t *testing.T) {
	testutil.PostgresDSNFromEnv(t)
	for _, tc := range []struct {
		name, assignment                      string
		patch                                 models.RepositoryBranchPolicyPatch
		wantBase, wantDescription, wantTarget string
	}{
		{"metadata after workflow", "base_branch = 'main', branch_template = 'hotfix/{title}-{suffix}', pull_request_target = 'release'", models.RepositoryBranchPolicyPatch{Description: patchString("new")}, "main", "new", "release"},
		{"workflow after metadata", "description = 'new'", models.RepositoryBranchPolicyPatch{BaseBranch: patchString("main"), BranchTemplate: patchString("hotfix/{title}-{suffix}"), PullRequestTarget: patchString("release")}, "main", "new", "release"},
		{"blank target after base", "base_branch = 'main'", models.RepositoryBranchPolicyPatch{PullRequestTarget: patchString(" ")}, "main", "original", "main"},
		{"omitted target after base", "base_branch = 'main'", models.RepositoryBranchPolicyPatch{Description: patchString("new")}, "main", "new", "develop"},
		{"same field last commit", "description = 'first'", models.RepositoryBranchPolicyPatch{Description: patchString("last")}, "develop", "last", "develop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newPGPolicyPatchFixture(t)
			policy := policyPatchStoreFixture(t, fixture.primary)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
			holder, err := fixture.primary.db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = holder.Rollback() })
			_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, branchPolicyPatchLockNamespace+policy.ID)
			require.NoError(t, err)
			work := startPGPolicyPatch(t, ctx, fixture.worker, policy, &tc.patch)
			assertPGPolicyPatchWait(t, ctx, fixture, work, "pg_advisory_xact_lock")
			_, err = holder.ExecContext(ctx, `UPDATE repository_branch_policies SET `+tc.assignment+` WHERE id = $1`, policy.ID)
			require.NoError(t, err)
			require.NoError(t, holder.Commit())
			joinPGPolicyPatch(t, ctx, work)
			require.NoError(t, work.err)
			require.Equal(t, tc.wantBase, work.policy.BaseBranch)
			require.Equal(t, tc.wantDescription, work.policy.Description)
			require.Equal(t, tc.wantTarget, work.policy.PullRequestTarget)
			if strings.Contains(tc.name, "workflow") {
				require.Equal(t, "hotfix/{title}-{suffix}", work.policy.BranchTemplate)
			}
			saved, err := fixture.observer.GetRepositoryBranchPolicy(ctx, policy.ID)
			require.NoError(t, err)
			require.Equal(t, *saved, *work.policy)
		})
	}
	t.Run("legacy row update", func(t *testing.T) { checkPGPolicyPatchLegacyLock(t, false) })
	t.Run("concurrent delete", func(t *testing.T) { checkPGPolicyPatchLegacyLock(t, true) })
	t.Run("racing name constraint", checkPGPolicyPatchNameConstraint)
}

func checkPGPolicyPatchLegacyLock(t *testing.T, deleted bool) {
	fixture := newPGPolicyPatchFixture(t)
	policy := policyPatchStoreFixture(t, fixture.primary)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	holder, err := fixture.primary.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	if deleted {
		_, err = holder.ExecContext(ctx, `DELETE FROM repository_branch_policies WHERE id = $1`, policy.ID)
	} else {
		_, err = holder.ExecContext(ctx, `UPDATE repository_branch_policies SET base_branch = 'main' WHERE id = $1`, policy.ID)
	}
	require.NoError(t, err)
	work := startPGPolicyPatch(t, ctx, fixture.worker, policy, &models.RepositoryBranchPolicyPatch{PullRequestTarget: patchString("")})
	assertPGPolicyPatchWait(t, ctx, fixture, work, "FOR UPDATE")
	require.NoError(t, holder.Commit())
	joinPGPolicyPatch(t, ctx, work)
	if deleted {
		require.ErrorIs(t, work.err, repoerrors.ErrRepositoryBranchPolicyNotFound)
		_, err := fixture.observer.GetRepositoryBranchPolicy(ctx, policy.ID)
		require.ErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNotFound)
	} else {
		require.NoError(t, work.err)
		require.Equal(t, "main", work.policy.BaseBranch)
		require.Equal(t, "main", work.policy.PullRequestTarget)
	}
}

func checkPGPolicyPatchNameConstraint(t *testing.T) {
	fixture := newPGPolicyPatchFixture(t)
	policy := policyPatchStoreFixture(t, fixture.primary)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	_, err := fixture.primary.db.ExecContext(ctx, `CREATE FUNCTION policy_patch_name_barrier() RETURNS trigger AS $$
 BEGIN
 IF NEW.name = 'Racing' THEN PERFORM pg_advisory_xact_lock(hashtextextended('policy-patch-name-test', 0)); END IF;
 RETURN NEW;
 END; $$ LANGUAGE plpgsql;
 CREATE TRIGGER policy_patch_name_barrier BEFORE UPDATE ON repository_branch_policies
 FOR EACH ROW EXECUTE FUNCTION policy_patch_name_barrier();`)
	require.NoError(t, err)
	holder, err := fixture.primary.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('policy-patch-name-test', 0))`)
	require.NoError(t, err)
	work := startPGPolicyPatch(t, ctx, fixture.worker, policy, &models.RepositoryBranchPolicyPatch{Name: patchString("Racing"), BaseBranch: patchString("main")})
	assertPGPolicyPatchWait(t, ctx, fixture, work, "UPDATE repository_branch_policies")
	competing := *policy
	competing.ID, competing.Name = "competing-policy", "racing"
	require.NoError(t, fixture.observer.CreateRepositoryBranchPolicy(ctx, &competing))
	require.NoError(t, holder.Commit())
	joinPGPolicyPatch(t, ctx, work)
	require.ErrorIs(t, work.err, repoerrors.ErrRepositoryBranchPolicyNameConflict)
	saved, err := fixture.observer.GetRepositoryBranchPolicy(ctx, policy.ID)
	require.NoError(t, err)
	require.Equal(t, *policy, *saved)
}
