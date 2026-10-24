package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	internaldb "github.com/kandev/kandev/internal/db"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestBuiltinStoreConcurrentSQLiteWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.db")
	open := func() *sqliteRepository {
		raw, err := internaldb.OpenSQLite(path)
		require.NoError(t, err)
		db := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		repo, err := newSQLiteRepositoryWithDB(db, db, nil)
		require.NoError(t, err)
		return repo
	}
	first := open()
	seedBuiltinStore(t, first)
	testBuiltinStoreConcurrentWrites(t, first, open())
}

func TestBuiltinStoreConcurrentPostgresWrites(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)
	first, err := newSQLiteRepositoryWithDB(db, db, nil)
	require.NoError(t, err)
	seedBuiltinStore(t, first)
	second, err := newSQLiteRepositoryWithDB(db, db, nil)
	require.NoError(t, err)
	testBuiltinStoreConcurrentWrites(t, first, second)
}

func testBuiltinStoreConcurrentWrites(t *testing.T, first, second *sqliteRepository) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, repo := range []*sqliteRepository{first, second} {
		go func() {
			policy := mcpconfig.DefaultBuiltinWorkspacePolicy("workspace")
			id := []p.PackageID{p.PackageRichOutput, p.PackageDiagnostics}[index]
			policy.Packages[id] = mcpconfig.BuiltinPolicyBlocked
			policy.UpdatedBy, policy.UpdatedAt = "human", time.Now().UTC()
			<-start
			_, err := repo.PutBuiltinWorkspacePolicy(ctx, policy, 0)
			results <- err
		}()
	}
	close(start)
	accepted, conflicted := 0, 0
	for range 2 {
		select {
		case err := <-results:
			switch {
			case err == nil:
				accepted++
			case errors.Is(err, mcpconfig.ErrBuiltinPolicyConflict):
				conflicted++
			default:
				t.Errorf("write failed outside revision control: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent policy writes did not settle")
		}
	}
	require.Equal(t, 1, accepted)
	require.Equal(t, 1, conflicted)
	snapshot, err := first.GetBuiltinPolicySnapshot(ctx, "workspace", "task")
	require.NoError(t, err)
	require.EqualValues(t, 1, snapshot.Policy.Revision)
}
