package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestAutoMergeUsersKeepOrderAndAdjacentMerge(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() }, "sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(backend+map[bool]string{false: "/off", true: "/on"}[enabled], func(t *testing.T) {
				ctx := context.Background()
				repo := makeRepo(t)
				identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
				seedQueueSessionIdentity(t, repo, identity)
				service := NewService(repo, 10, logger.Default())
				service.SetAutoMergeEnabled(enabled)
				user := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "U1"}
				require.NoError(t, repo.InsertForSession(ctx, identity, user, 0))
				require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry("A1", "A"), 0))
				for _, body := range []string{"U2", "U3"} {
					_, err := service.QueueMessageWithMetadataForSession(ctx, identity, body, "", QueuedByUser, false, nil, nil)
					require.NoError(t, err)
				}
				_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
				require.NoError(t, err)
				rows, err := repo.ListBySession(ctx, identity.SessionID)
				require.NoError(t, err)
				expected := []string{"U1", "A1", "U2", "U3", "A2"}
				if enabled {
					expected = []string{"U1", "A1\n\nA2", "U2\n\nU3"}
				}
				require.Equal(t, expected, sortTestContents(rows))
				require.Equal(t, user.ID, rows[0].ID)
			})
		}
	}
}

func TestAutoMergeGlobalChangeDoesNotCompact(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() }, "sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			repo := makeRepo(t)
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			service := NewService(repo, 10, logger.Default())
			service.SetAutoMergeEnabled(false)
			for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B"), sortTestEntry("A2", "A")} {
				require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
			}
			service.SetAutoMergeEnabled(true)
			rows, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, []string{"A1", "B1", "A2"}, sortTestContents(rows))
			_, err = service.QueueMessageWithMetadataForSession(ctx, identity, "B2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "B"})
			require.NoError(t, err)
			rows, err = repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, []string{"A1", "B1\n\nB2", "A2"}, sortTestContents(rows))
		})
	}
}
