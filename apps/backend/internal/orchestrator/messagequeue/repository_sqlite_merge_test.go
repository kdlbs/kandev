package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/entityrefs"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

// insertTestEntry inserts a queued message through the repository under test
// and fails the test on error, returning the inserted entry.
func insertTestEntry(t *testing.T, repo Repository, session, task, content, queuedBy string, attachments []MessageAttachment, metadata map[string]interface{}) *QueuedMessage {
	t.Helper()
	msg := &QueuedMessage{
		SessionID:   session,
		TaskID:      task,
		Content:     content,
		QueuedBy:    queuedBy,
		Attachments: attachments,
		Metadata:    metadata,
	}
	if err := repo.Insert(context.Background(), msg, 0); err != nil {
		t.Fatalf("insert %q: %v", content, err)
	}
	return msg
}

// entityRefs builds the metadata reference list for the given issue ids,
// using canonical github/acme-repo references.
func entityRefs(ids ...string) []interface{} {
	out := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]interface{}{
			"version":  apiv1.EntityReferenceVersion,
			"ref":      entityrefs.CanonicalRef("github", "issue", "acme/repo", id),
			"provider": "github",
			"kind":     "issue",
			"id":       id,
			"title":    "Issue " + id,
			"url":      "https://github.com/acme/repo/issues/" + id,
			"scope":    "acme/repo",
		})
	}
	return out
}

// manyEntityRefs returns count references starting at id 1.
func manyEntityRefs(count int) []interface{} {
	return manyEntityRefsFrom(1, count)
}

// manyEntityRefsFrom returns count references starting at the given id,
// used to build unions that straddle the per-message reference cap.
func manyEntityRefsFrom(start, count int) []interface{} {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("%d", start+i)
	}
	return entityRefs(ids...)
}

// TestSQLiteRepository_MergeIntoAbove_ReferenceOverflow asserts a merge whose
// deduplicated reference union would exceed the per-message cap is rejected
// atomically: both rows keep their persisted references and the queue count is
// unchanged.
func TestSQLiteRepository_MergeIntoAbove_ReferenceOverflow(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil,
		map[string]interface{}{MetadataEntityReferences: manyEntityRefs(entityrefs.MaxReferencesPerMessage)})
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil,
		map[string]interface{}{MetadataEntityReferences: entityRefs(fmt.Sprintf("%d", entityrefs.MaxReferencesPerMessage+1))})

	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrMergeReferenceOverflow) {
		t.Fatalf("merge error = %v, want ErrMergeReferenceOverflow", err)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 2 {
		t.Errorf("count after rejected overflow merge = %d, want 2", count)
	}
	entries, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries after rejected overflow merge = %d, want 2", len(entries))
	}
	byID := make(map[string]*QueuedMessage, len(entries))
	for i := range entries {
		byID[entries[i].ID] = &entries[i]
	}
	targetRow, sourceRow := byID[target.ID], byID[source.ID]
	if targetRow == nil || sourceRow == nil {
		t.Fatalf("missing rows after rejected overflow merge: target=%v source=%v", byID[target.ID] != nil, byID[source.ID] != nil)
	}
	if refs := entityrefs.NormalizePersisted(targetRow.Metadata[MetadataEntityReferences]); len(refs) != entityrefs.MaxReferencesPerMessage {
		t.Errorf("target refs len = %d, want %d (untouched)", len(refs), entityrefs.MaxReferencesPerMessage)
	}
	if refs := entityrefs.NormalizePersisted(sourceRow.Metadata[MetadataEntityReferences]); len(refs) != 1 {
		t.Errorf("source refs len = %d, want 1 (untouched)", len(refs))
	}
	if sourceRow.Content != "second" {
		t.Errorf("source content changed after rejected merge = %q", sourceRow.Content)
	}
}

// TestSQLiteRepository_MergeIntoAbove_ReferenceOverflow_DedupeWithinLimit
// asserts overlapping references stay within the cap after deduplication, so
// merging two cap-sized lists of identical references succeeds.
func TestSQLiteRepository_MergeIntoAbove_ReferenceOverflow_DedupeWithinLimit(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	refs := manyEntityRefs(entityrefs.MaxReferencesPerMessage)
	insertTestEntry(t, repo, "s1", "t1", "first", "user", nil,
		map[string]interface{}{MetadataEntityReferences: refs})
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil,
		map[string]interface{}{MetadataEntityReferences: refs})

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	union := entityrefs.NormalizePersisted(merged.Metadata[MetadataEntityReferences])
	if len(union) != entityrefs.MaxReferencesPerMessage {
		t.Errorf("merged refs len = %d, want %d (deduped)", len(union), entityrefs.MaxReferencesPerMessage)
	}
}

// TestSQLiteRepository_MergeDrainOrdering_DrainWins drains the source before the
// merge runs, so the merge must report ErrEntryNotFound without touching the
// target or losing content — the drain-wins ordering of the merge/drain race.
func TestSQLiteRepository_MergeDrainOrdering_DrainWins(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil, nil)

	if _, err := repo.TakeByID(ctx, "s1", source.ID); err != nil {
		t.Fatalf("drain source: %v", err)
	}
	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("merge after drain error = %v, want ErrEntryNotFound", err)
	}
	kept, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept entries = %d, want 1", len(kept))
	}
	if kept[0].ID != target.ID || kept[0].Content != "first" {
		t.Errorf("kept entry = %s %q, want target %q", kept[0].ID, kept[0].Content, "first")
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 1 {
		t.Errorf("count after drain-wins = %d, want 1", count)
	}
}

// TestSQLiteRepository_MergeDrainOrdering_MergeWins merges before the drain runs,
// so the merged entry — not the source — is what later drains, preserving the
// combined content.
func TestSQLiteRepository_MergeDrainOrdering_MergeWins(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil, nil)

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	drained, err := repo.TakeHead(ctx, "s1")
	if err != nil {
		t.Fatalf("drain after merge: %v", err)
	}
	if drained.ID != target.ID || drained.ID != merged.ID {
		t.Errorf("drained id = %s, want merged target %s", drained.ID, merged.ID)
	}
	if drained.Content != "first\n\nsecond" {
		t.Errorf("drained content = %q, want combined content", drained.Content)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 0 {
		t.Errorf("count after merge-wins = %d, want 0", count)
	}
}

// TestSQLiteRepository_MergeIntoAbove_UserMergesIntoUser covers the user↔user
// happy path: combined content joined with "\n\n", attachments concatenated,
// entity references unioned and deduplicated, target identity preserved, source
// row gone, and the queue count drops by one.
func TestSQLiteRepository_MergeIntoAbove_UserMergesIntoUser(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user",
		[]MessageAttachment{{Type: "image", Data: "a", MimeType: "image/png"}},
		map[string]interface{}{
			MetadataEntityReferences: entityRefs("1"),
		})
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user",
		[]MessageAttachment{{Type: "file", Data: "b", MimeType: "text/plain"}},
		map[string]interface{}{
			MetadataEntityReferences: entityRefs("1", "2"),
		})

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged == nil {
		t.Fatal("expected merged entry, got nil")
	}
	if merged.ID != target.ID {
		t.Errorf("merged id = %s, want target id %s", merged.ID, target.ID)
	}
	if merged.Content != "first\n\nsecond" {
		t.Errorf("merged content = %q, want %q", merged.Content, "first\n\nsecond")
	}
	if len(merged.Attachments) != 2 {
		t.Errorf("merged attachments len = %d, want 2", len(merged.Attachments))
	}
	refs := entityrefs.NormalizePersisted(merged.Metadata[MetadataEntityReferences])
	if len(refs) != 2 {
		t.Errorf("merged refs len = %d, want 2 (deduped)", len(refs))
	}
	if merged.QueuedBy != "user" {
		t.Errorf("merged queued_by = %q, want user", merged.QueuedBy)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 1 {
		t.Errorf("count after merge = %d, want 1", count)
	}
}

// TestSQLiteRepository_MergeIntoAbove_EmptyTargetContent ensures an empty target
// content yields the source content verbatim (no leading blank line).
func TestSQLiteRepository_MergeIntoAbove_EmptyTargetContent(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	insertTestEntry(t, repo, "s1", "t1", "", "user", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "only source", "user", nil, nil)

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged.Content != "only source" {
		t.Errorf("merged content = %q, want %q", merged.Content, "only source")
	}
}

// TestSQLiteRepository_MergeIntoAbove_HeadRejected asserts the head entry has no
// target above it and the queue is untouched.
func TestSQLiteRepository_MergeIntoAbove_HeadRejected(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	head := insertTestEntry(t, repo, "s1", "t1", "head", "user", nil, nil)
	if _, err := repo.MergeIntoAbove(ctx, "s1", head.ID, "user"); !errors.Is(err, ErrNoMergeTarget) {
		t.Fatalf("head merge error = %v, want ErrNoMergeTarget", err)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 1 {
		t.Errorf("count after rejected head merge = %d, want 1", count)
	}
}

// TestSQLiteRepository_MergeIntoAbove_MismatchedKinds rejects merges across
// sender kinds in every direction.
func TestSQLiteRepository_MergeIntoAbove_MismatchedKinds(t *testing.T) {
	cases := []struct {
		name           string
		aboveQueuedBy  string
		sourceQueuedBy string
		aboveMetadata  map[string]interface{}
		sourceMetadata map[string]interface{}
	}{
		{name: "agent source above user target", aboveQueuedBy: "user", sourceQueuedBy: QueuedByAgent},
		{name: "user source above agent target", aboveQueuedBy: QueuedByAgent, sourceQueuedBy: "user"},
		{name: "workflow source", aboveQueuedBy: "user", sourceQueuedBy: QueuedByWorkflow},
		{name: "server source", aboveQueuedBy: "user", sourceQueuedBy: QueuedByServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestSQLiteRepo(t)
			ctx := context.Background()
			insertTestEntry(t, repo, "s1", "t1", "above", tc.aboveQueuedBy, nil, tc.aboveMetadata)
			source := insertTestEntry(t, repo, "s1", "t1", "source", tc.sourceQueuedBy, nil, tc.sourceMetadata)
			if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrNoMergeTarget) {
				t.Fatalf("merge error = %v, want ErrNoMergeTarget", err)
			}
			count, _ := repo.CountBySession(ctx, "s1")
			if count != 2 {
				t.Errorf("count after rejected merge = %d, want 2", count)
			}
		})
	}
}

// TestSQLiteRepository_MergeIntoAbove_AgentSameSender allows an agent entry to
// merge into the agent entry above it when both come from the same sender task.
func TestSQLiteRepository_MergeIntoAbove_AgentSameSender(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "agent first", QueuedByAgent, nil,
		map[string]interface{}{MetadataSenderTaskID: "task-7"})
	source := insertTestEntry(t, repo, "s1", "t1", "agent second", QueuedByAgent, nil,
		map[string]interface{}{MetadataSenderTaskID: "task-7"})

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, QueuedByAgent)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged.ID != target.ID {
		t.Errorf("merged id = %s, want target id %s", merged.ID, target.ID)
	}
	if merged.Content != "agent first\n\nagent second" {
		t.Errorf("merged content = %q, want %q", merged.Content, "agent first\n\nagent second")
	}
	if merged.QueuedBy != QueuedByAgent {
		t.Errorf("merged queued_by = %q, want agent", merged.QueuedBy)
	}
}

// TestSQLiteRepository_MergeIntoAbove_AgentDifferentSender rejects agent merges
// across different sender tasks.
func TestSQLiteRepository_MergeIntoAbove_AgentDifferentSender(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	insertTestEntry(t, repo, "s1", "t1", "agent first", QueuedByAgent, nil,
		map[string]interface{}{MetadataSenderTaskID: "task-7"})
	source := insertTestEntry(t, repo, "s1", "t1", "agent second", QueuedByAgent, nil,
		map[string]interface{}{MetadataSenderTaskID: "task-8"})

	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, QueuedByAgent); !errors.Is(err, ErrNoMergeTarget) {
		t.Fatalf("merge error = %v, want ErrNoMergeTarget", err)
	}
}

// TestSQLiteRepository_MergeIntoAbove_WrongCallerRejected asserts a user merge
// requires the caller identity to own both rows.
func TestSQLiteRepository_MergeIntoAbove_WrongCallerRejected(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	insertTestEntry(t, repo, "s1", "t1", "above", "alice", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "source", "alice", nil, nil)

	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "bob"); !errors.Is(err, ErrNoMergeTarget) {
		t.Fatalf("merge error = %v, want ErrNoMergeTarget", err)
	}
}

// TestSQLiteRepository_MergeIntoAbove_SourceOwnerMismatch asserts the source
// entry must be owned by the caller even when the target above is: a caller
// must not be able to fold and delete another user's queued message into one of
// their own rows.
func TestSQLiteRepository_MergeIntoAbove_SourceOwnerMismatch(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	insertTestEntry(t, repo, "s1", "t1", "above", "bob", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "someone else's message", "alice", nil, nil)

	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "bob"); !errors.Is(err, ErrNoMergeTarget) {
		t.Fatalf("merge error = %v, want ErrNoMergeTarget", err)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 2 {
		t.Errorf("count after rejected merge = %d, want 2", count)
	}
}

// TestSQLiteRepository_MergeIntoAbove_SourceMissing returns ErrEntryNotFound for
// a drained or unknown source id.
func TestSQLiteRepository_MergeIntoAbove_SourceMissing(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	if _, err := repo.MergeIntoAbove(ctx, "s1", "missing", "user"); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("missing source error = %v, want ErrEntryNotFound", err)
	}
}

// TestSQLiteRepository_MergeIntoAbove_ReservedTargetRejected refuses to merge a
// user entry into a reserved in-flight lifecycle target.
func TestSQLiteRepository_MergeIntoAbove_ReservedTargetRejected(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	insertTestEntry(t, repo, "s1", "t1", "inflight", "user", nil, map[string]interface{}{
		MetadataLifecycleDurable:  true,
		MetadataLifecycleReserved: true,
	})
	source := insertTestEntry(t, repo, "s1", "t1", "source", "user", nil, nil)

	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrNoMergeTarget) {
		t.Fatalf("merge error = %v, want ErrNoMergeTarget", err)
	}
}

// TestSQLiteRepository_MergeIntoAbove_Chain merges C into B then B into A,
// producing a single entry with content "A\n\nB\n\nC" in order.
func TestSQLiteRepository_MergeIntoAbove_Chain(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	a := insertTestEntry(t, repo, "s1", "t1", "A", "user", nil, nil)
	b := insertTestEntry(t, repo, "s1", "t1", "B", "user", nil, nil)
	c := insertTestEntry(t, repo, "s1", "t1", "C", "user", nil, nil)

	merged, err := repo.MergeIntoAbove(ctx, "s1", c.ID, "user")
	if err != nil {
		t.Fatalf("merge C into B: %v", err)
	}
	if merged.ID != b.ID {
		t.Errorf("after C merge id = %s, want %s", merged.ID, b.ID)
	}

	merged, err = repo.MergeIntoAbove(ctx, "s1", b.ID, "user")
	if err != nil {
		t.Fatalf("merge B into A: %v", err)
	}
	if merged.ID != a.ID {
		t.Errorf("after B merge id = %s, want %s", merged.ID, a.ID)
	}
	if merged.Content != "A\n\nB\n\nC" {
		t.Errorf("chained content = %q, want %q", merged.Content, "A\n\nB\n\nC")
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 1 {
		t.Errorf("count after chain = %d, want 1", count)
	}
}
