package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func sqliteAdmissionPair(t *testing.T) (*Repository, *Repository) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "admission.db")
	open := func() *sqlx.DB {
		connection, err := db.OpenSQLite(filename)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, connection.Close()) })
		return sqlx.NewDb(connection, "sqlite3")
	}
	primaryDB := open()
	primary, err := NewWithDB(primaryDB, primaryDB, nil)
	require.NoError(t, err)
	peerDB := open()
	return primary, NewWithInitializedDB(peerDB, peerDB, nil)
}

func gateSQLiteAdmissionWrite(t *testing.T, ctx context.Context, repo *Repository) (<-chan struct{}, func()) {
	t.Helper()
	ready, release := make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	resume := func() { unblock.Do(func() { close(release) }) }
	t.Cleanup(resume)
	conn, err := repo.db.Conn(ctx)
	require.NoError(t, err)
	err = conn.Raw(func(driverConn interface{}) error {
		driverConn.(*sqlite3.SQLiteConn).RegisterAuthorizer(func(op int, table, _, _ string) int {
			if table == "repository_branch_policies" && (op == sqlite3.SQLITE_INSERT || op == sqlite3.SQLITE_UPDATE) {
				once.Do(func() {
					close(ready)
					select {
					case <-release:
					case <-ctx.Done():
					}
				})
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	})
	require.NoError(t, conn.Close())
	require.NoError(t, err)
	return ready, resume
}

func awaitAdmissionGate(t *testing.T, ctx context.Context, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func customAdmissionPolicy(repositoryID string) *models.RepositoryBranchPolicy {
	return &models.RepositoryBranchPolicy{RepositoryID: repositoryID, Name: "Custom", BaseBranch: "release",
		BranchTemplate: "custom/{title}-{suffix}", PullRequestTarget: "release"}
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.3, AC-WORKSPACES-BRANCH-POLICIES-002.4
func TestGitflowAdmissionSQLite(t *testing.T) {
	t.Run("ordinary_first", func(t *testing.T) {
		primary, peer := sqliteAdmissionPair(t)
		seedAdmissionRepository(t, primary, "sqlite-admission")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		holder, err := primary.db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = holder.Rollback() })
		_, err = holder.ExecContext(ctx, `INSERT INTO repository_branch_policies (`+repositoryBranchPolicyColumns+`)
		 VALUES ('ordinary', 'sqlite-admission', 'Custom', '', 'release', 'custom/{title}-{suffix}', 'release', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
		require.NoError(t, err)
		ready, resume := gateSQLiteAdmissionWrite(t, ctx, peer)
		work := startAdmissionWork(t, ctx, func(ctx context.Context) error {
			return peer.CreateRepositoryBranchPoliciesIfEmpty(ctx, "sqlite-admission", admissionPolicies("sqlite-admission", "main", "develop"))
		})
		awaitAdmissionGate(t, ctx, ready)
		require.NoError(t, holder.Commit())
		resume()
		joinAdmissionWork(t, ctx, work)
		require.ErrorIs(t, work.err, repoerrors.ErrRepositoryBranchPoliciesExist)
		rows, err := primary.ListRepositoryBranchPolicies(ctx, "sqlite-admission")
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "ordinary", rows[0].ID)
	})
	for _, ordinary := range []bool{false, true} {
		name := "competing_starters"
		if ordinary {
			name = "starter_first"
		}
		t.Run(name, func(t *testing.T) { checkSQLiteAdmissionOrder(t, ordinary) })
	}
	t.Run("rollback_and_errors", func(t *testing.T) {
		primary, _ := sqliteAdmissionPair(t)
		checkAdmissionErrors(t, primary)
	})
	t.Run("misleading_constraint_message", checkAdmissionConstraintMessage)
}

func checkAdmissionConstraintMessage(t *testing.T) {
	repo, _ := sqliteAdmissionPair(t)
	seedAdmissionRepository(t, repo, "constraint-message")
	ctx := context.Background()
	_, err := repo.db.ExecContext(ctx, `CREATE TRIGGER admission_constraint_message BEFORE INSERT ON repository_branch_policies
	 BEGIN SELECT RAISE(ABORT, 'uniq_repository_branch_policies_repository_lower_name'); END`)
	require.NoError(t, err)
	err = repo.CreateRepositoryBranchPolicy(ctx, customAdmissionPolicy("constraint-message"))
	require.Error(t, err)
	require.NotErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNameConflict)
	require.NotErrorIs(t, err, repoerrors.ErrRepositoryBranchPoliciesExist)
	rows, err := repo.ListRepositoryBranchPolicies(ctx, "constraint-message")
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = repo.db.ExecContext(ctx, `DROP TRIGGER admission_constraint_message`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateRepositoryBranchPolicy(ctx, customAdmissionPolicy("constraint-message")))
}

func checkSQLiteAdmissionOrder(t *testing.T, ordinary bool) {
	primary, peer := sqliteAdmissionPair(t)
	seedAdmissionRepository(t, primary, "sqlite-order")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ready, resume := gateSQLiteAdmissionWrite(t, ctx, peer)
	work := startAdmissionWork(t, ctx, func(ctx context.Context) error {
		if ordinary {
			return peer.CreateRepositoryBranchPolicy(ctx, customAdmissionPolicy("sqlite-order"))
		}
		return peer.CreateRepositoryBranchPoliciesIfEmpty(ctx, "sqlite-order", admissionPolicies("sqlite-order", "release", "next"))
	})
	awaitAdmissionGate(t, ctx, ready)
	winner := admissionPolicies("sqlite-order", "main", "develop")
	require.NoError(t, primary.CreateRepositoryBranchPoliciesIfEmpty(ctx, "sqlite-order", winner))
	resume()
	joinAdmissionWork(t, ctx, work)
	if ordinary {
		require.NoError(t, work.err)
	} else {
		require.ErrorIs(t, work.err, repoerrors.ErrRepositoryBranchPoliciesExist)
	}
	rows, err := primary.ListRepositoryBranchPolicies(ctx, "sqlite-order")
	require.NoError(t, err)
	assertAdmissionRows(t, rows, winner, ordinary)
}

func assertAdmissionRows(t *testing.T, rows, winner []*models.RepositoryBranchPolicy, ordinary bool) {
	t.Helper()
	want := len(winner)
	if ordinary {
		want++
	}
	require.Len(t, rows, want)
	byName := make(map[string]*models.RepositoryBranchPolicy, len(rows))
	for _, row := range rows {
		byName[row.Name] = row
	}
	for _, policy := range winner {
		require.Equal(t, *policy, *byName[policy.Name])
	}
	if ordinary {
		require.Equal(t, "release", byName["Custom"].BaseBranch)
	}
}

func checkAdmissionErrors(t *testing.T, repo *Repository) {
	seedAdmissionRepository(t, repo, "admission-errors")
	ctx := context.Background()
	invalid := admissionPolicies("admission-errors", "main", "develop")
	invalid[0].ID, invalid[2].ID = "duplicate-id", "duplicate-id"
	err := repo.CreateRepositoryBranchPoliciesIfEmpty(ctx, "admission-errors", invalid)
	require.Error(t, err)
	require.NotErrorIs(t, err, repoerrors.ErrRepositoryBranchPoliciesExist)
	require.NotErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNameConflict)
	rows, err := repo.ListRepositoryBranchPolicies(ctx, "admission-errors")
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, repo.CreateRepositoryBranchPoliciesIfEmpty(ctx, "admission-errors", admissionPolicies("admission-errors", "main", "develop")))
	duplicate := customAdmissionPolicy("admission-errors")
	duplicate.Name = "feature"
	require.ErrorIs(t, repo.CreateRepositoryBranchPolicy(ctx, duplicate), repoerrors.ErrRepositoryBranchPolicyNameConflict)
	require.NoError(t, repo.DeleteRepository(ctx, "admission-errors"))
	rows, err = repo.ListRepositoryBranchPolicies(ctx, "admission-errors")
	require.NoError(t, err)
	require.Empty(t, rows)
	err = repo.CreateRepositoryBranchPoliciesIfEmpty(ctx, "missing-admission", admissionPolicies("missing-admission", "main", "develop"))
	require.Error(t, err)
	require.NotErrorIs(t, err, repoerrors.ErrRepositoryBranchPoliciesExist)
	require.NotErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNameConflict)
	rows, err = repo.ListRepositoryBranchPolicies(ctx, "missing-admission")
	require.NoError(t, err)
	require.Empty(t, rows)
}
