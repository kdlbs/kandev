package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.6
// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.9
func TestAutoSortMergeInterleavedAdmissions(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() },
		"sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		for _, clientID := range []bool{false, true} {
			for _, full := range []bool{false, true} {
				name := backend + map[bool]string{false: "/two-tx", true: "/receipt"}[clientID] + map[bool]string{false: "/space", true: "/full"}[full]
				t.Run(name, func(t *testing.T) {
					repo := makeRepo(t)
					ctx := context.Background()
					identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
					seedQueueSessionIdentity(t, repo, identity)
					rows := []*QueuedMessage{
						sortTestEntry("A1", "A"), sortTestEntry("B1", "B"), sortTestEntry("A2", "A"),
					}
					for _, row := range rows {
						require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
					}
					capacity := 10
					if full {
						capacity = 3
					}
					service := NewService(repo, capacity, logger.Default())
					var accepted *QueuedMessage
					var err error
					if clientID {
						accepted, _, err = service.QueueMessageWithMetadataForSessionWithClientQueueID(ctx, identity, "B2-id", "B2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "B"}, nil)
					} else {
						accepted, err = service.QueueMessageWithMetadataForSession(ctx, identity, "B2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "B"})
					}
					require.NoError(t, err)
					require.Equal(t, rows[1].ID, accepted.ID)
					after, err := repo.ListBySession(ctx, identity.SessionID)
					require.NoError(t, err)
					require.Equal(t, []string{"A1", "B1\n\nB2", "A2"}, sortTestContents(after))
					require.Equal(t, rows[1].Position, after[1].Position)
					require.True(t, after[1].QueuedAt.After(rows[1].QueuedAt))
				})
			}
		}
	}
}

func sortTestEntry(content, sender string) *QueuedMessage {
	return &QueuedMessage{TaskID: "task", SessionID: "session", Content: content, QueuedBy: QueuedByAgent, Metadata: map[string]any{MetadataSenderTaskID: sender}, QueuedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}
func sortTestContents(rows []QueuedMessage) []string {
	contents := make([]string, len(rows))
	for i := range rows {
		contents[i] = rows[i].Content
	}
	return contents
}

func TestAutoSortMergeLegacyAdmissionUsesChosenTarget(t *testing.T) {
	for name, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() },
		"sqlite": newTestSQLiteRepo,
	} {
		t.Run(name, func(t *testing.T) {
			repo := makeRepo(t)
			ctx := context.Background()
			for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B"), sortTestEntry("A2", "A")} {
				require.NoError(t, repo.Insert(ctx, row, 0))
			}
			service := NewService(repo, 3, logger.Default())
			_, err := service.QueueMessageWithMetadata(ctx, "session", "task", "B2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "B"})
			require.NoError(t, err)
			rows, err := repo.ListBySession(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, []string{"A1", "B1\n\nB2", "A2"}, sortTestContents(rows))
		})
	}
}
