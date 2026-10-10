package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresRepositorySetPatchBehavior(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	for _, tc := range []struct {
		name  string
		check func(*testing.T, *Repository)
	}{
		{"presence", checkSetPatchPresence},
		{"rollback", checkSetPatchRollback},
		{"deleted", checkSetPatchDeleted},
		{"name conflict", checkSetPatchNameConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := testutil.OpenIsolatedPostgres(t, dsn)
			repo, err := NewWithDB(database, database, nil)
			require.NoError(t, err)
			tc.check(t, repo)
		})
	}
}

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.10, AC-WORKSPACES-REPOSITORY-SETS-001.11
func TestPostgresRepositorySetConcurrentPatches(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	name, description := "Concurrent name", "Concurrent description"
	items := []models.RepositorySetItem{
		{RepositoryID: "repo-orders", BaseBranch: "release-orders"},
		{RepositoryID: "repo-web", BaseBranch: "release-web"},
	}
	for _, tc := range []struct {
		name                      string
		holderColumn, holderValue string
		patch                     models.RepositorySetPatch
		wantName, wantDescription string
	}{
		{"description after name", "name", name, models.RepositorySetPatch{Description: &description}, name, description},
		{"name after description", "description", description, models.RepositorySetPatch{Name: &name}, name, description},
		{"members after name", "name", name, models.RepositorySetPatch{Items: &items}, name, "web + gateway + orders"},
		{"same name last commit", "name", "Earlier name", models.RepositorySetPatch{Name: &name}, name, "web + gateway + orders"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newPGSetPatchFixture(t, dsn)
			before := createSetPatchFixture(t, fixture.repo)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			holder, err := fixture.primary.BeginTxx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = holder.Rollback() })
			query := `UPDATE repository_sets SET name = ? WHERE id = ?`
			if tc.holderColumn == "description" {
				query = `UPDATE repository_sets SET description = ? WHERE id = ?`
			}
			_, err = holder.ExecContext(ctx, fixture.primary.Rebind(query), tc.holderValue, before.ID)
			require.NoError(t, err)
			worker := startPGSetPatch(t, ctx, fixture.writer, before.ID, &tc.patch)
			require.NoError(t, waitForPostgresLock(ctx, fixture.observer, fixture.writerPID, worker.done))
			require.NoError(t, holder.Commit())
			require.NoError(t, worker.join(t, ctx))
			stored, err := fixture.repo.GetRepositorySet(ctx, before.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantName, stored.Name)
			require.Equal(t, tc.wantDescription, stored.Description)
			if tc.patch.Items == nil {
				require.Equal(t, before.Items, stored.Items)
			} else {
				assertPGSetPatchMembers(t, stored, []string{"repo-orders", "repo-web"}, []string{"release-orders", "release-web"})
			}
		})
	}
	t.Run("concurrent member replacements", func(t *testing.T) { testPGSetMemberCompetition(t, dsn) })
}

type pgSetPatchFixture struct {
	primary, observer *sqlx.DB
	repo, writer      *Repository
	writerPID         int
}

func newPGSetPatchFixture(t *testing.T, dsn string) pgSetPatchFixture {
	t.Helper()
	primary := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(primary, primary, nil)
	require.NoError(t, err)
	writerDB := openSecondPostgresConnection(t, dsn, primary)
	observer := openSecondPostgresConnection(t, dsn, primary)
	var pid int
	require.NoError(t, writerDB.Get(&pid, `SELECT pg_backend_pid()`))
	return pgSetPatchFixture{primary: primary, observer: observer, repo: repo,
		writer: NewWithInitializedDB(writerDB, writerDB, nil), writerPID: pid}
}

type pgSetPatchWorker struct {
	done chan struct{}
	err  error
}

func startPGSetPatch(
	t *testing.T, ctx context.Context, repo *Repository, id string, patch *models.RepositorySetPatch,
) *pgSetPatchWorker {
	t.Helper()
	workerCtx, cancel := context.WithCancel(ctx)
	worker := &pgSetPatchWorker{done: make(chan struct{})}
	t.Cleanup(func() { cancel(); <-worker.done })
	go func() {
		defer close(worker.done)
		worker.err = repo.PatchRepositorySet(workerCtx, id, patch)
	}()
	return worker
}

func (w *pgSetPatchWorker) join(t *testing.T, ctx context.Context) error {
	t.Helper()
	select {
	case <-w.done:
		return w.err
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return ctx.Err()
	}
}

func testPGSetMemberCompetition(t *testing.T, dsn string) {
	fixture := newPGSetPatchFixture(t, dsn)
	before := createSetPatchFixture(t, fixture.repo)
	secondDB := openSecondPostgresConnection(t, dsn, fixture.primary)
	second := NewWithInitializedDB(secondDB, secondDB, nil)
	var secondPID int
	require.NoError(t, secondDB.Get(&secondPID, `SELECT pg_backend_pid()`))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	holder, err := fixture.primary.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	_, err = holder.ExecContext(ctx, `SELECT id FROM repository_sets WHERE id = $1 FOR UPDATE`, before.ID)
	require.NoError(t, err)
	firstItems := []models.RepositorySetItem{
		{RepositoryID: "repo-orders", BaseBranch: "first-orders"},
		{RepositoryID: "repo-web", BaseBranch: "first-web"},
	}
	secondItems := []models.RepositorySetItem{
		{RepositoryID: "repo-gateway", BaseBranch: "second-gateway"},
		{RepositoryID: "repo-orders", BaseBranch: "second-orders"},
	}
	name, description := "Concurrent member name", "Concurrent member description"
	first := startPGSetPatch(t, ctx, fixture.writer, before.ID, &models.RepositorySetPatch{Name: &name, Items: &firstItems})
	other := startPGSetPatch(t, ctx, second, before.ID, &models.RepositorySetPatch{Description: &description, Items: &secondItems})
	require.NoError(t, waitForPostgresLock(ctx, fixture.observer, fixture.writerPID, first.done))
	require.NoError(t, waitForPostgresLock(ctx, fixture.observer, secondPID, other.done))
	require.NoError(t, holder.Commit())
	require.NoError(t, first.join(t, ctx))
	require.NoError(t, other.join(t, ctx))
	stored, err := fixture.repo.GetRepositorySet(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, name, stored.Name)
	require.Equal(t, description, stored.Description)
	require.Len(t, stored.Items, 2)
	if stored.Items[0].RepositoryID == "repo-orders" {
		assertPGSetPatchMembers(t, stored, []string{"repo-orders", "repo-web"}, []string{"first-orders", "first-web"})
	} else {
		assertPGSetPatchMembers(t, stored, []string{"repo-gateway", "repo-orders"}, []string{"second-gateway", "second-orders"})
	}
}

func assertPGSetPatchMembers(t *testing.T, set *models.RepositorySet, ids, bases []string) {
	t.Helper()
	require.Equal(t, ids, set.RepositoryIDs())
	for i, item := range set.Items {
		require.Equal(t, i, item.Position)
		require.Equal(t, bases[i], item.BaseBranch)
	}
}
