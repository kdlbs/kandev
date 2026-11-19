package messagequeue

import (
	"context"
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/messageconstraints"
	"github.com/stretchr/testify/require"
)

func measureCompaction(backlog []*QueuedMessage) (queueCompaction, uint64) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	result := compactQueueRows(backlog, nil, nil)
	runtime.ReadMemStats(&after)
	return result, after.TotalAlloc - before.TotalAlloc
}

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
	result, allocated := measureCompaction(backlog)

	require.Len(t, result.kept, 1)
	require.Len(t, result.kept[0].merged.Content, rows*bodyBytes+(rows-1)*2)
	require.Less(t, allocated, uint64(8*rows*bodyBytes), "compaction allocation must stay linear in the backlog size")
}

// Provenance IDs accumulate on the survivor; each fold must not re-copy and
// re-encode the whole accumulated list.
func TestCompactionAdmissionProvenanceIsLinear(t *testing.T) {
	allocations := map[int]uint64{}
	for _, rows := range []int{256, 1024} {
		backlog := make([]*QueuedMessage, rows)
		for i := range backlog {
			backlog[i] = &QueuedMessage{
				ID: fmt.Sprintf("row-%d", i), TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "x",
				Metadata: map[string]interface{}{MetadataQueueAdmissionIDs: []interface{}{fmt.Sprintf("client-%d", i)}},
			}
		}
		result, allocated := measureCompaction(backlog)
		require.Len(t, result.kept, 1)
		ids, ok := normalizeQueueAdmissionIDs(result.kept[0].merged.Metadata[MetadataQueueAdmissionIDs])
		require.True(t, ok)
		require.Len(t, ids, rows)
		require.Equal(t, "client-0", ids[0])
		require.Equal(t, fmt.Sprintf("client-%d", rows-1), ids[rows-1])
		allocations[rows] = allocated
	}
	require.Less(t, allocations[1024], 8*allocations[256], "4x rows must not cost ~16x allocation")
}

// A large attachment on the survivor must be validated once, not decoded
// again for every later text-only fold.
func TestCompactionValidatesKeptAttachmentOnce(t *testing.T) {
	const rows, attachmentBytes = 256, 256 << 10
	backlog := make([]*QueuedMessage, rows)
	for i := range backlog {
		backlog[i] = sortTestEntry("text", "A")
		backlog[i].ID = fmt.Sprintf("row-%d", i)
	}
	backlog[0].Attachments = []MessageAttachment{{
		Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(make([]byte, attachmentBytes)),
	}}
	result, allocated := measureCompaction(backlog)
	require.Len(t, result.kept, 1)
	require.Len(t, result.kept[0].merged.Attachments, 1)
	require.Less(t, allocated, uint64(16*attachmentBytes), "the kept attachment must not be decoded on every fold")
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

	_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil,
		map[string]any{MetadataSenderTaskID: "A"})
	require.NoError(t, err)
	rows, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, rows, others+1)
	require.Equal(t, "A1\n\nA2", rows[0].Content)
}

// Restoring a reservation keeps its old slot, so positions can tie. The
// metadata walk must not skip a tied row at a batch boundary.
func TestSortedAdmissionFindsTargetAcrossTiedBatchBoundary(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	entries := make([]*QueuedMessage, sortedFoldScanBatch+1)
	for i := range entries {
		entries[i] = sortTestEntry(fmt.Sprintf("row-%d", i), fmt.Sprintf("sender-%d", i))
		require.NoError(t, repo.InsertForSession(ctx, identity, entries[i], 0))
	}
	reserved, err := repo.ReserveHead(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, entries[0].ID, reserved.ID)
	remaining := make([]string, 0, len(entries)-1)
	for _, entry := range entries[1:] {
		remaining = append(remaining, entry.ID)
	}
	require.NoError(t, repo.ReorderEntriesForSession(ctx, identity, remaining))
	require.NoError(t, repo.RestoreForSession(ctx, identity, reserved, 0))
	var tied int
	require.NoError(t, repo.db.Get(&tied, `SELECT COUNT(*) FROM queued_messages WHERE session_id = ? AND position = 1`, identity.SessionID))
	require.Equal(t, 2, tied, "precondition: restore and reorder leave two rows at position 1")

	service := NewService(repo, len(entries), logger.Default())
	for _, sender := range []string{"sender-0", "sender-1"} {
		_, err = service.QueueMessageWithMetadataForSession(ctx, identity, sender+"-again", "", QueuedByAgent, false, nil,
			map[string]any{MetadataSenderTaskID: sender})
		require.NoError(t, err, "a full queue must fold %s into its tied row", sender)
	}
	var contents []string
	require.NoError(t, repo.db.Select(&contents, `SELECT content FROM queued_messages WHERE session_id = ? AND position = 1 ORDER BY content`, identity.SessionID))
	require.Equal(t, []string{"row-0\n\nsender-0-again", "row-1\n\nsender-1-again"}, contents)
}

// An incompatible pair must be rejected before the target's attachment
// payload is decoded, for single admissions and for compaction alike.
func TestIncompatibleAutoMergeDoesNotDecodeTargetAttachment(t *testing.T) {
	const attachmentBytes = 4 << 20
	target := sortTestEntry("A1", "A")
	target.Attachments = []MessageAttachment{{
		Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(make([]byte, attachmentBytes)),
	}}
	source := sortTestEntry("B1", "B")

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range 4 {
		_, merged := buildAutoMergedEntry(target, source)
		require.False(t, merged)
	}
	result := compactQueueRows([]*QueuedMessage{target, source}, nil, nil)
	runtime.ReadMemStats(&after)

	require.Len(t, result.kept, 2)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(attachmentBytes/2),
		"rejecting an incompatible pair must not decode the target attachment")
}

// A compatible pair that exceeds the attachment count is rejected before the
// target's payload is decoded, in compaction as in a single admission.
func TestCompactionAttachmentCountOverflowDoesNotDecodeTarget(t *testing.T) {
	const attachmentBytes = 4 << 20
	target := sortTestEntry("A1", "A")
	target.Attachments = []MessageAttachment{{
		Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(make([]byte, attachmentBytes)),
	}}
	for i := 1; i < messageconstraints.MaxAttachmentCount; i++ {
		target.Attachments = append(target.Attachments, MessageAttachment{AttachmentID: fmt.Sprintf("durable-%d", i), SizeBytes: 1})
	}
	source := sortTestEntry("A2", "A")
	source.Attachments = []MessageAttachment{{AttachmentID: "overflow", SizeBytes: 1}}

	result, allocated := measureCompaction([]*QueuedMessage{target, source})
	require.Len(t, result.kept, 2)
	require.Less(t, allocated, uint64(attachmentBytes/2), "a count overflow must not decode the target attachment")
}

// Compaction folds through the chain accumulator while single admissions use
// buildAutoMergedEntry; both must produce the same merged entry.
func TestCompactionFoldMatchesSingleAutoMerge(t *testing.T) {
	inline := MessageAttachment{Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("png"))}
	ref := func(id string) map[string]interface{} {
		return map[string]interface{}{"type": "task", "id": id, "ref": "task:" + id}
	}
	cases := map[string][2]*QueuedMessage{
		"text":           {{Content: "one"}, {Content: "two"}},
		"empty target":   {{Content: ""}, {Content: "two"}},
		"empty source":   {{Content: "one"}, {Content: ""}},
		"attachments":    {{Content: "one", Attachments: []MessageAttachment{inline}}, {Content: "two", Attachments: []MessageAttachment{inline}}},
		"admission ids":  {{Metadata: map[string]interface{}{MetadataQueueAdmissionIDs: []interface{}{"a", "b"}}}, {Metadata: map[string]interface{}{MetadataQueueAdmissionIDs: []interface{}{"b", "c", "c"}}}},
		"context files":  {{Metadata: map[string]interface{}{MetadataContextFiles: []interface{}{map[string]interface{}{"path": "a.go"}}}}, {Metadata: map[string]interface{}{MetadataContextFiles: []interface{}{map[string]interface{}{"path": "a.go"}, map[string]interface{}{"path": "b.go"}}}}},
		"entity refs":    {{Metadata: map[string]interface{}{MetadataEntityReferences: []interface{}{ref("1")}}}, {Metadata: map[string]interface{}{MetadataEntityReferences: []interface{}{ref("1"), ref("2")}}}},
		"bad admission":  {{Content: "one"}, {Content: "two", Metadata: map[string]interface{}{MetadataQueueAdmissionIDs: []interface{}{""}}}},
		"different text": {{Content: "one", Model: "a"}, {Content: "two", Model: "b"}},
	}
	for name, pair := range cases {
		t.Run(name, func(t *testing.T) {
			rows := make([]*QueuedMessage, 2)
			for i, row := range pair {
				clone := *row
				clone.ID, clone.TaskID, clone.SessionID, clone.QueuedBy = fmt.Sprintf("row-%d", i), "task", "session", QueuedByUser
				clone.Position = int64(i + 1)
				rows[i] = &clone
			}
			values, merged := buildAutoMergedEntry(rows[0], rows[1])
			require.Equal(t, name != "bad admission" && name != "different text", merged)
			result := compactQueueRows(rows, nil, nil)
			if !merged {
				require.Len(t, result.kept, 2)
				return
			}
			require.Len(t, result.kept, 1)
			survivor := result.kept[0].merged
			require.Equal(t, values.content, survivor.Content)
			require.Equal(t, values.attachments, survivor.Attachments)
			require.Equal(t, values.metadata, survivor.Metadata)
			require.Equal(t, values.queuedAt, survivor.QueuedAt)
		})
	}
}

// Each sorted-walk batch must seek the (session_id, position, id) index from
// the previous boundary rather than rescan and re-sort the session from its
// tail, or a distant or missing target costs quadratic work per admission.
func TestSortedWalkBatchSeeksSessionOrderIndex(t *testing.T) {
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	var plan []struct {
		ID      int    `db:"id"`
		Parent  int    `db:"parent"`
		NotUsed int    `db:"notused"`
		Detail  string `db:"detail"`
	}
	require.NoError(t, repo.db.Select(&plan, "EXPLAIN QUERY PLAN "+sortedFoldScanQuery, "session", 10, "id", sortedFoldScanBatch))
	details := make([]string, len(plan))
	for i, step := range plan {
		details[i] = step.Detail
	}
	joined := strings.Join(details, "; ")
	require.Contains(t, joined, "idx_queued_messages_session_order")
	require.Contains(t, joined, "(position,id)<")
	require.NotContains(t, joined, "TEMP B-TREE")
}
