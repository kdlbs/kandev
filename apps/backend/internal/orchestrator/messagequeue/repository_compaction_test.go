package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.12
func TestCompactionPreservesLeasedSourceAgainstCompatiblePredecessor(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() }, "sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		for _, leaseSource := range []bool{false, true} {
			t.Run(backend+map[bool]string{false: "/control", true: "/leased"}[leaseSource], func(t *testing.T) {
				repo := makeRepo(t)
				ctx := context.Background()
				identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
				seedQueueSessionIdentity(t, repo, identity)
				first := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "U1"}
				second := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "U2"}
				require.NoError(t, repo.InsertForSession(ctx, identity, first, 0))
				require.NoError(t, repo.InsertForSession(ctx, identity, second, 0))
				service := NewService(repo, 10, logger.Default())
				service.SetAutoMergeEnabled(false)
				var lease *QueueEditLease
				if leaseSource {
					var err error
					lease, err = service.BeginEdit(ctx, "session", second.ID, "connection")
					require.NoError(t, err)
				}
				result, err := service.SetSessionAutoMerge(ctx, identity, true)
				require.NoError(t, err)
				after, err := repo.ListBySession(ctx, "session")
				require.NoError(t, err)
				if !leaseSource {
					require.Equal(t, 1, result.CompactedCount)
					require.Equal(t, []string{"U1\n\nU2"}, sortTestContents(after))
					return
				}
				require.Zero(t, result.CompactedCount)
				require.Equal(t, []string{"U1", "U2"}, sortTestContents(after))
				require.Equal(t, first.ID, after[0].ID)
				require.Equal(t, second.ID, after[1].ID)
				current, err := service.GetEditLease(ctx, "session", second.ID)
				require.NoError(t, err)
				require.Equal(t, lease.LeaseID, current.LeaseID)
			})
		}
	}
}

func TestCompactionCrossesLeasedOtherSenderButNotHiddenBarrier(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() }, "sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		t.Run(backend, func(t *testing.T) {
			repo := makeRepo(t)
			ctx := context.Background()
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			user := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "U1"}
			barrier := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByAgent, Content: "hidden", Metadata: map[string]any{MetadataLifecycleReserved: true}}
			for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), user, sortTestEntry("A2", "A"), barrier, sortTestEntry("A3", "A")} {
				require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
			}
			service := NewService(repo, 10, logger.Default())
			service.SetAutoMergeEnabled(false)
			_, err := service.BeginEdit(ctx, "session", user.ID, "connection")
			require.NoError(t, err)
			result, err := service.SetSessionAutoMerge(ctx, identity, true)
			require.NoError(t, err)
			require.Equal(t, 1, result.CompactedCount)
			after, err := repo.ListBySession(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, []string{"A1\n\nA2", "U1", "hidden", "A3"}, sortTestContents(after))
			require.Equal(t, barrier.ID, after[2].ID)
			require.Equal(t, barrier.Metadata, after[2].Metadata)
		})
	}
}

func TestCompactionWriteFailureRollsBackPolicyAndQueue(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{"sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo} {
		t.Run(backend, func(t *testing.T) {
			repo := makeRepo(t).(*sqliteRepository)
			ctx := context.Background()
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B"), sortTestEntry("A2", "A"), sortTestEntry("B2", "B")} {
				require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
			}
			before := autoQueueSnapshot(t, repo, "session")
			trigger := `CREATE TRIGGER fail_compaction_delete BEFORE DELETE ON queued_messages BEGIN SELECT RAISE(ABORT,'forced compaction failure'); END;`
			if backend == "postgres" {
				_, err := repo.db.Exec(`CREATE FUNCTION fail_compaction_delete() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'forced compaction failure'; END; $$ LANGUAGE plpgsql`)
				require.NoError(t, err)
				trigger = `CREATE TRIGGER fail_compaction_delete BEFORE DELETE ON queued_messages FOR EACH ROW EXECUTE FUNCTION fail_compaction_delete()`
			}
			_, err := repo.db.Exec(trigger)
			require.NoError(t, err)
			service := NewService(repo, 10, logger.Default())
			service.SetAutoMergeEnabled(false)
			_, err = service.SetSessionAutoMerge(ctx, identity, true)
			require.Error(t, err)
			require.Equal(t, before, autoQueueSnapshot(t, repo, "session"))
			override, err := repo.GetAutoMergeOverride(ctx, identity)
			require.NoError(t, err)
			require.Nil(t, override)
		})
	}
}
