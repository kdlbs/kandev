package messagequeue

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// Folding N same-sender bodies must copy each body a bounded number of times,
// not re-copy the growing merged body on every fold.
func TestCompactionAllocationIsLinearInBacklogBytes(t *testing.T) {
	const rows, bodyBytes = 512, 4096
	backlog := make([]*QueuedMessage, rows)
	body := strings.Repeat("x", bodyBytes)
	for i := range backlog {
		entry := sortTestEntry(body, "A")
		entry.ID = fmt.Sprintf("row-%d", i)
		backlog[i] = entry
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	result := compactQueueRows(backlog, nil, nil)
	runtime.ReadMemStats(&after)

	require.Len(t, result.kept, 1)
	require.Len(t, result.kept[0].merged.Content, rows*bodyBytes+(rows-1)*2)
	inputBytes := uint64(rows * bodyBytes)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, 8*inputBytes,
		"compaction allocation must stay linear in the backlog size")
}

func TestSortedAdmissionScansPastOneMetadataBatch(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry("A1", "A"), 0))
	others := 3*sortedFoldScanBatch + 5
	for i := range others {
		require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry(fmt.Sprintf("other-%d", i), fmt.Sprintf("sender-%d", i)), 0))
	}
	service := NewService(repo, 0, logger.Default())
	service.SetGlobalQueuePolicy(GlobalQueuePolicy{
		AutoMerge: AutoMergePolicy{Enabled: true}, AutoSortMerge: AutoSortMergePolicy{Enabled: true},
	})

	_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil,
		map[string]any{MetadataSenderTaskID: "A"})
	require.NoError(t, err)
	rows, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, rows, others+1)
	require.Equal(t, "A1\n\nA2", rows[0].Content)
}
