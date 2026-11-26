package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.7
// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.8
func TestAutoSortMergeAdmissionPreservesNonMergeableOrder(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() }, "sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		for _, scenario := range []string{"latest-incompatible", "barrier", "local-lease", "merge-off"} {
			for _, receipt := range []bool{false, true} {
				t.Run(backend+"/"+scenario+map[bool]string{false: "/two-tx", true: "/receipt"}[receipt], func(t *testing.T) {
					repo := makeRepo(t)
					ctx := context.Background()
					identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
					seedQueueSessionIdentity(t, repo, identity)
					rows := []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("A2", "A"), sortTestEntry("B1", "B")}
					service := NewService(repo, 10, logger.Default())
					global := AutoMergePolicy{Enabled: true}
					switch scenario {
					case "latest-incompatible":
						rows[1].Model = "different"
					case "barrier":
						rows[1].Metadata[MetadataLifecycleReserved] = true
					case "merge-off":
						global.Enabled = false
					}
					for _, row := range rows {
						require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
					}
					if scenario == "local-lease" {
						service.editLeases[service.editLeaseKey("session", rows[1].ID)] = &QueueEditLease{SessionID: "session", EntryID: rows[1].ID, ExpiresAt: time.Now().Add(time.Hour)}
					}
					service.SetGlobalAutoMergePolicy(global)
					var err error
					if receipt {
						_, _, err = service.QueueMessageWithMetadataForSessionWithClientQueueID(ctx, identity, "A3-id", "A3", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"}, nil)
					} else {
						_, err = service.QueueMessageWithMetadataForSession(ctx, identity, "A3", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
					}
					require.NoError(t, err)
					after, err := repo.ListBySession(ctx, "session")
					require.NoError(t, err)
					require.Equal(t, []string{"A1", "A2", "B1", "A3"}, sortTestContents(after))
				})
			}
		}
	}
}

func TestAutoSortMergeDurableLeaseBlocksChosenTarget(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{"sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo} {
		t.Run(backend, func(t *testing.T) {
			repo := makeRepo(t).(*sqliteRepository)
			ctx := context.Background()
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			first := sortTestEntry("A1", "A")
			require.NoError(t, repo.InsertForSession(ctx, identity, first, 0))
			require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry("B1", "B"), 0))
			require.NoError(t, repo.acquireEditLease(ctx, &QueueEditLease{SessionID: "session", EntryID: first.ID, LeaseID: "lease", ExpiresAt: time.Now().Add(time.Hour)}))
			service := NewService(repo, 2, logger.Default())
			_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
			require.ErrorIs(t, err, ErrQueueFull)
			after, err := repo.ListBySession(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, []string{"A1", "B1"}, sortTestContents(after))
		})
	}
}
