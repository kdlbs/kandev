package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// Sorted selection must stop at the latest same-sender row and never decode
// the payload of older backlog rows: an undecodable payload above that point
// would otherwise fail every admission and costs O(backlog) per message.
func TestSortedAdmissionDoesNotDecodeBacklogPayloads(t *testing.T) {
	for _, full := range []bool{false, true} {
		name := map[bool]string{false: "space", true: "full"}[full]
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t).(*sqliteRepository)
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			backlog := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "backlog"}
			for _, row := range []*QueuedMessage{backlog, sortTestEntry("A1", "A"), sortTestEntry("B1", "B")} {
				require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
			}
			_, err := repo.db.Exec(`UPDATE queued_messages SET attachments_json = 'undecodable' WHERE id = ?`, backlog.ID)
			require.NoError(t, err)
			capacity := 10
			if full {
				capacity = 3
			}
			service := NewService(repo, capacity, logger.Default())

			for _, sender := range []string{"A", "B"} {
				_, err = service.QueueMessageWithMetadataForSession(ctx, identity, sender+"2", "", QueuedByAgent, false, nil,
					map[string]any{MetadataSenderTaskID: sender})
				require.NoError(t, err)
			}
			var contents []string
			require.NoError(t, repo.db.Select(&contents, `SELECT content FROM queued_messages WHERE session_id = ? ORDER BY position`, identity.SessionID))
			require.Equal(t, []string{"backlog", "A1\n\nA2", "B1\n\nB2"}, contents)
		})
	}
}
