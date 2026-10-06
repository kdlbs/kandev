package sqlite

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
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

func sqliteAdmissionPair(t *testing.T) (*Repository, *Repository, *sqlx.DB) {
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
	var busyTimeout int
	require.NoError(t, peerDB.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout))
	require.Equal(t, 5000, busyTimeout)
	observer := open()
	_, err = observer.Exec(`PRAGMA busy_timeout=0`)
	require.NoError(t, err)
	return primary, NewWithInitializedDB(peerDB, peerDB, nil), observer
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
		primary, peer, observer := sqliteAdmissionPair(t)
		seedAdmissionRepository(t, primary, "sqlite-admission")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		holder, err := primary.db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = holder.Rollback() })
		_, err = holder.ExecContext(ctx, `INSERT INTO repository_branch_policies (`+repositoryBranchPolicyColumns+`)
		 VALUES ('ordinary', 'sqlite-admission', 'Custom', '', 'release', 'custom/{title}-{suffix}', 'release', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
		require.NoError(t, err)
		work := startAdmissionWork(t, ctx, func(ctx context.Context) error {
			return peer.CreateRepositoryBranchPoliciesIfEmpty(ctx, "sqlite-admission", admissionPolicies("sqlite-admission", "main", "develop"))
		})
		t.Cleanup(func() { cancel(); _ = holder.Rollback(); <-work.done })
		assertAdmissionSQLiteBeginWait(t, ctx, peer, observer, work)
		require.NoError(t, holder.Commit())
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
		primary, _, _ := sqliteAdmissionPair(t)
		checkAdmissionErrors(t, primary)
	})
	t.Run("misleading_constraint_message", checkAdmissionConstraintMessage)
}

func checkAdmissionConstraintMessage(t *testing.T) {
	repo, _, _ := sqliteAdmissionPair(t)
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
	primary, peer, observer := sqliteAdmissionPair(t)
	seedAdmissionRepository(t, primary, "sqlite-order")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ready, resume := gateSQLiteAdmissionWrite(t, ctx, primary)
	winner := admissionPolicies("sqlite-order", "main", "develop")
	first := startAdmissionWork(t, ctx, func(ctx context.Context) error {
		return primary.CreateRepositoryBranchPoliciesIfEmpty(ctx, "sqlite-order", winner)
	})
	awaitAdmissionGate(t, ctx, ready)
	work := startAdmissionWork(t, ctx, func(ctx context.Context) error {
		if ordinary {
			return peer.CreateRepositoryBranchPolicy(ctx, customAdmissionPolicy("sqlite-order"))
		}
		return peer.CreateRepositoryBranchPoliciesIfEmpty(ctx, "sqlite-order", admissionPolicies("sqlite-order", "release", "next"))
	})
	t.Cleanup(func() { cancel(); resume(); <-first.done; <-work.done })
	assertAdmissionSQLiteBeginWait(t, ctx, peer, observer, work)
	resume()
	joinAdmissionWork(t, ctx, first)
	require.NoError(t, first.err)
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

// Observe the same native BEGIN worker across an independent BUSY probe while
// the intended winner owns SQLite's writer; SQL authorizer entry follows BEGIN.
func assertAdmissionSQLiteBeginWait(t *testing.T, ctx context.Context, peer *Repository, observer *sqlx.DB, work *admissionWork) {
	t.Helper()
	var worker string
	for worker == "" {
		worker = admissionSQLiteBeginWorker()
		select {
		case <-work.done:
			t.Fatalf("policy writer returned before physical BEGIN wait: %v", work.err)
		case <-ctx.Done():
			t.Fatal("policy writer BEGIN wait not observed", ctx.Err())
		default:
			runtime.Gosched()
		}
	}
	require.Equal(t, 1, peer.db.Stats().InUse)
	_, err := observer.ExecContext(ctx, `UPDATE tasks SET id=id WHERE 0`)
	var busy sqlite3.Error
	require.ErrorAs(t, err, &busy)
	require.Equal(t, sqlite3.ErrBusy, busy.Code)
	require.Equal(t, worker, admissionSQLiteBeginWorker(), "same physical BEGIN worker across held-writer BUSY probe")
	t.Logf("policy writer %s waits inside driver BEGIN while independent SQL probe returns SQLITE_BUSY", worker)
}

func admissionSQLiteBeginWorker() string {
	buffer := make([]byte, 1<<20)
	sections := strings.Split(string(buffer[:runtime.Stack(buffer, true)]), "\n\n")
	for _, parent := range sections {
		policy := strings.Contains(parent, ".CreateRepositoryBranchPoliciesIfEmpty(") || strings.Contains(parent, ".CreateRepositoryBranchPolicy(")
		if !policy || !strings.Contains(parent, "(*SQLiteConn).begin(") {
			continue
		}
		owner := strings.Fields(parent)[1]
		for _, child := range sections {
			if strings.Contains(child, "_Cfunc__sqlite3_step_row_internal(") && strings.Contains(child, "created by github.com/mattn/go-sqlite3.(*SQLiteStmt).exec in goroutine "+owner+"\n") {
				return owner + "/" + strings.Fields(child)[1]
			}
		}
	}
	return ""
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
