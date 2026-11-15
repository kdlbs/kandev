package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type sortOverrideRepository interface {
	GetAutoSortMergeOverride(context.Context, QueueSessionIdentity) (*AutoSortMergeOverride, error)
	SetAutoSortMergeOverride(context.Context, QueueSessionIdentity, bool) (AutoSortMergeOverride, error)
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3
func TestAutoSortMergeOverridePersistsAndResets(t *testing.T) {
	for name, makeRepo := range map[string]func(*testing.T) Repository{
		"memory":   func(*testing.T) Repository { return NewMemoryRepository() },
		"sqlite":   newTestSQLiteRepo,
		"postgres": newTestPostgresRepo,
	} {
		t.Run(name, func(t *testing.T) {
			repo := makeRepo(t)
			policies, ok := repo.(sortOverrideRepository)
			require.True(t, ok, "repository must persist sort overrides")
			ctx := context.Background()
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			inherited, err := policies.GetAutoSortMergeOverride(ctx, identity)
			require.NoError(t, err)
			require.Nil(t, inherited)
			for i, value := range []bool{true, false} {
				written, err := policies.SetAutoSortMergeOverride(ctx, identity, value)
				require.NoError(t, err)
				require.Equal(t, AutoSortMergeOverride{Enabled: value, Revision: int64(i + 1)}, written)
				stored, err := policies.GetAutoSortMergeOverride(ctx, identity)
				require.NoError(t, err)
				require.Equal(t, &written, stored)
			}
			snapshot, err := repo.Snapshot(ctx, identity)
			require.NoError(t, err)
			require.Equal(t, &AutoSortMergeOverride{Revision: 2}, snapshot.AutoSortMergeOverride)
			_, err = repo.DeleteAllBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			stored, err := policies.GetAutoSortMergeOverride(ctx, identity)
			require.NoError(t, err)
			require.Equal(t, &AutoSortMergeOverride{Revision: 2}, stored, "empty queue preserves policy")
			replacement := identity
			replacement.SessionIncarnationID = "replacement"
			seedQueueSessionIdentity(t, repo, replacement)
			snapshot, err = repo.Snapshot(ctx, replacement)
			require.NoError(t, err)
			require.Nil(t, snapshot.AutoSortMergeOverride)
			_, err = policies.GetAutoSortMergeOverride(ctx, identity)
			require.ErrorIs(t, err, ErrSessionIdentityMismatch)
		})
	}
}

func TestAutoSortMergeSchemaUpgradeAndReplay(t *testing.T) {
	for name, makeRepo := range map[string]func(*testing.T) Repository{"sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo} {
		t.Run(name, func(t *testing.T) {
			repo := makeRepo(t).(*sqliteRepository)
			for _, column := range []string{"auto_sort_merge_override", "auto_sort_merge_revision"} {
				_, err := repo.db.Exec("ALTER TABLE queue_session_state DROP COLUMN " + column)
				require.NoError(t, err)
			}
			reopened, err := NewSQLiteRepository(repo.db, repo.ro)
			require.NoError(t, err)
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, reopened, identity)
			written, err := reopened.SetAutoSortMergeOverride(context.Background(), identity, true)
			require.NoError(t, err)
			replayed, err := NewSQLiteRepository(repo.db, repo.ro)
			require.NoError(t, err)
			stored, err := replayed.GetAutoSortMergeOverride(context.Background(), identity)
			require.NoError(t, err)
			require.Equal(t, &written, stored)
		})
	}
}

func TestAutoSortMergeTransferKeepsDestinationPreference(t *testing.T) {
	for name, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() },
		"sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		t.Run(name, func(t *testing.T) {
			repo := makeRepo(t)
			ctx := context.Background()
			source := QueueSessionIdentity{TaskID: "task", SessionID: "source", SessionIncarnationID: "source-inc"}
			destination := QueueSessionIdentity{TaskID: "task", SessionID: "destination", SessionIncarnationID: "destination-inc"}
			seedQueueSessionIdentity(t, repo, source)
			seedQueueSessionIdentity(t, repo, destination)
			_, err := repo.SetAutoSortMergeOverride(ctx, source, true)
			require.NoError(t, err)
			saved, err := repo.SetAutoSortMergeOverride(ctx, destination, false)
			require.NoError(t, err)
			require.NoError(t, repo.TransferSessionIdentities(ctx, source, destination))
			stored, err := repo.GetAutoSortMergeOverride(ctx, destination)
			require.NoError(t, err)
			require.Equal(t, &saved, stored)
			seedQueueSessionIdentity(t, repo, source)
			stored, err = repo.GetAutoSortMergeOverride(ctx, source)
			require.NoError(t, err)
			require.Nil(t, stored)
		})
	}
}
