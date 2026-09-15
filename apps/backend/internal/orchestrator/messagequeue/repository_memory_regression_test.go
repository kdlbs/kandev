package messagequeue

import (
	"context"
	"testing"
	"time"
)

func TestMemoryReplaceSessionOwnsQueueSnapshotData(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	original := QueuedMessage{
		ID:        "entry-1",
		SessionID: "source-session",
		TaskID:    "task-1",
		Position:  1,
		Content:   "original",
		QueuedAt:  time.Unix(1, 0).UTC(),
		QueuedBy:  QueuedByUser,
		Attachments: []MessageAttachment{{
			AttachmentID: "attachment-1",
			Name:         "original.txt",
		}},
		Metadata: map[string]interface{}{"source": "original"},
	}
	move := &PendingMove{MoveID: "move-1", TaskID: "task-1", QueuedAt: time.Unix(2, 0).UTC()}

	if err := repo.ReplaceSession(ctx, "restored-session", []QueuedMessage{original}, move); err != nil {
		t.Fatalf("replace session: %v", err)
	}
	original.Attachments[0].Name = "mutated.txt"
	original.Metadata["source"] = "mutated"
	move.MoveID = "mutated-move"

	entries, err := repo.ListBySession(ctx, "restored-session")
	if err != nil {
		t.Fatalf("list restored session: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("restored entry count = %d, want 1", len(entries))
	}
	if got := entries[0].Attachments[0].Name; got != "original.txt" {
		t.Fatalf("restored attachment name = %q, want original.txt", got)
	}
	if got := entries[0].Metadata["source"]; got != "original" {
		t.Fatalf("restored metadata source = %q, want original", got)
	}
	storedMove, err := repo.GetPendingMove(ctx, "restored-session")
	if err != nil {
		t.Fatalf("get restored move: %v", err)
	}
	if storedMove == nil || storedMove.MoveID != "move-1" {
		t.Fatalf("restored move = %#v, want move-1", storedMove)
	}
}

func TestMemoryReserveHeadUsesFIFOPositionAfterReplacement(t *testing.T) {
	svc := setupService(t)
	repo := svc.repo
	ctx := context.Background()
	if err := repo.ReplaceSession(ctx, "session-1", []QueuedMessage{
		{ID: "late", SessionID: "session-1", TaskID: "task-1", Position: 2, Content: "late", QueuedBy: QueuedByUser},
		{ID: "head", SessionID: "session-1", TaskID: "task-1", Position: 1, Content: "head", QueuedBy: QueuedByUser},
	}, nil); err != nil {
		t.Fatalf("replace session: %v", err)
	}

	reserved, ok := svc.ReserveQueued(ctx, "session-1")
	if !ok || reserved == nil {
		t.Fatalf("reserve head = %#v, %v", reserved, ok)
	}
	if reserved.ID != "head" {
		t.Fatalf("reserved entry = %q, want head", reserved.ID)
	}
}

func TestMemoryInsertAndListOwnQueueSnapshotData(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	original := &QueuedMessage{
		ID:        "entry-1",
		SessionID: "session-1",
		TaskID:    "task-1",
		Content:   "original",
		QueuedBy:  QueuedByUser,
		Attachments: []MessageAttachment{{
			AttachmentID: "attachment-1",
			Name:         "original.txt",
		}},
		Metadata: map[string]interface{}{
			"nested": map[string]interface{}{"value": "original"},
		},
	}
	if err := repo.Insert(ctx, original, 0); err != nil {
		t.Fatalf("insert: %v", err)
	}

	original.Attachments[0].Name = "caller-mutated.txt"
	original.Metadata["nested"].(map[string]interface{})["value"] = "caller-mutated"
	entries, err := repo.ListBySession(ctx, original.SessionID)
	if err != nil {
		t.Fatalf("list after caller mutation: %v", err)
	}
	if got := entries[0].Attachments[0].Name; got != "original.txt" {
		t.Fatalf("stored attachment after caller mutation = %q, want original.txt", got)
	}
	if got := entries[0].Metadata["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("stored metadata after caller mutation = %q, want original", got)
	}

	entries[0].Attachments[0].Name = "result-mutated.txt"
	entries[0].Metadata["nested"].(map[string]interface{})["value"] = "result-mutated"
	fresh, err := repo.ListBySession(ctx, original.SessionID)
	if err != nil {
		t.Fatalf("list after result mutation: %v", err)
	}
	if got := fresh[0].Attachments[0].Name; got != "original.txt" {
		t.Fatalf("stored attachment after result mutation = %q, want original.txt", got)
	}
	if got := fresh[0].Metadata["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("stored metadata after result mutation = %q, want original", got)
	}
	lifecycle := &QueuedMessage{
		ID:        "lifecycle-1",
		SessionID: "lifecycle-session",
		TaskID:    "task-1",
		Content:   "lifecycle",
		QueuedBy:  QueuedByWorkflow,
		Attachments: []MessageAttachment{{
			AttachmentID: "lifecycle-attachment",
			Name:         "lifecycle.txt",
		}},
		Metadata: map[string]interface{}{
			MetadataLifecycleDurable: true,
			"nested":                 map[string]interface{}{"value": "original"},
		},
	}
	if err := repo.Insert(ctx, lifecycle, 0); err != nil {
		t.Fatalf("insert lifecycle: %v", err)
	}
	reserved, err := repo.ReserveHead(ctx, lifecycle.SessionID)
	if err != nil {
		t.Fatalf("reserve lifecycle: %v", err)
	}
	reserved.Attachments[0].Name = "result-mutated.txt"
	reserved.Metadata["nested"].(map[string]interface{})["value"] = "result-mutated"
	fresh, err = repo.ListBySession(ctx, lifecycle.SessionID)
	if err != nil {
		t.Fatalf("list after reserve result mutation: %v", err)
	}
	if got := fresh[0].Attachments[0].Name; got != "lifecycle.txt" {
		t.Fatalf("reserved attachment after result mutation = %q, want lifecycle.txt", got)
	}
	if got := fresh[0].Metadata["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("reserved metadata after result mutation = %q, want original", got)
	}
}

func TestMemoryAppendInsertReturnsOwnedSnapshot(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	attachments := []MessageAttachment{{AttachmentID: "attachment-1", Name: "original.txt"}}
	metadata := map[string]interface{}{
		"nested": map[string]interface{}{"value": "original"},
	}

	out, appended, err := repo.AppendOrInsertTail(
		ctx, "session-append-snapshot", "task-1", "content", "", QueuedByUser,
		false, attachments, metadata, 0,
	)
	if err != nil {
		t.Fatalf("append or insert: %v", err)
	}
	if appended {
		t.Fatal("initial append or insert unexpectedly appended")
	}
	out.Attachments[0].Name = "mutated.txt"
	out.Metadata["nested"].(map[string]interface{})["value"] = "mutated"
	if got := attachments[0].Name; got != "original.txt" {
		t.Fatalf("input attachment after returned snapshot mutation = %q, want original.txt", got)
	}
	if got := metadata["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("input metadata after returned snapshot mutation = %q, want original", got)
	}

	entries, err := repo.ListBySession(ctx, "session-append-snapshot")
	if err != nil {
		t.Fatalf("list after returned snapshot mutation: %v", err)
	}
	if got := entries[0].Attachments[0].Name; got != "original.txt" {
		t.Fatalf("stored attachment after returned snapshot mutation = %q, want original.txt", got)
	}
	if got := entries[0].Metadata["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("stored metadata after returned snapshot mutation = %q, want original", got)
	}
}

func TestMemoryCoalesceReplacementPreservesTimestampAndPlanMode(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	coalesceKey := "coalesce-key"
	original := &QueuedMessage{
		SessionID: "session-coalesce-snapshot",
		TaskID:    "task-1",
		Content:   "original",
		PlanMode:  false,
		QueuedBy:  QueuedByWorkflow,
		QueuedAt:  time.Unix(10, 0).UTC(),
		Metadata:  map[string]interface{}{MetadataCoalesceKey: coalesceKey},
	}
	if err := repo.Insert(ctx, original, 0); err != nil {
		t.Fatalf("insert original: %v", err)
	}
	replacement := &QueuedMessage{
		SessionID: "session-coalesce-snapshot",
		TaskID:    "task-2",
		Content:   "replacement",
		PlanMode:  true,
		QueuedBy:  QueuedByWorkflow,
		QueuedAt:  time.Unix(20, 0).UTC(),
		Metadata:  map[string]interface{}{MetadataCoalesceKey: coalesceKey},
	}
	updated, replaced, err := repo.InsertOrReplaceByCoalesceKey(
		ctx, replacement, coalesceKey, 0, true,
	)
	if err != nil {
		t.Fatalf("replace ordinary coalesced entry: %v", err)
	}
	if !replaced {
		t.Fatal("ordinary coalesce unexpectedly inserted")
	}
	if !updated.PlanMode || !updated.QueuedAt.Equal(replacement.QueuedAt) {
		t.Fatalf("ordinary replacement = plan_mode:%t queued_at:%s, want plan_mode:true queued_at:%s",
			updated.PlanMode, updated.QueuedAt, replacement.QueuedAt)
	}

	lifecycle := &QueuedMessage{
		SessionID: "session-lifecycle-coalesce-snapshot",
		TaskID:    "task-1",
		Content:   "original lifecycle",
		PlanMode:  false,
		QueuedBy:  QueuedByWorkflow,
		QueuedAt:  time.Unix(30, 0).UTC(),
		Metadata: map[string]interface{}{
			MetadataCoalesceKey:         coalesceKey,
			MetadataLifecycleDurable:    true,
			MetadataLifecycleGeneration: int64(0),
		},
	}
	if err := repo.Insert(ctx, lifecycle, 0); err != nil {
		t.Fatalf("insert lifecycle original: %v", err)
	}
	lifecycleReplacement := &QueuedMessage{
		SessionID: "session-lifecycle-coalesce-snapshot",
		TaskID:    "task-2",
		Content:   "replacement lifecycle",
		PlanMode:  true,
		QueuedBy:  QueuedByWorkflow,
		QueuedAt:  time.Unix(40, 0).UTC(),
		Metadata: map[string]interface{}{
			MetadataCoalesceKey:         coalesceKey,
			MetadataLifecycleDurable:    true,
			MetadataLifecycleGeneration: int64(0),
		},
	}
	updated, replaced, err = repo.InsertOrReplaceLifecycleByCoalesceKey(
		ctx, lifecycleReplacement, coalesceKey, 0, true,
	)
	if err != nil {
		t.Fatalf("replace lifecycle coalesced entry: %v", err)
	}
	if !replaced {
		t.Fatal("lifecycle coalesce unexpectedly inserted")
	}
	if !updated.PlanMode || !updated.QueuedAt.Equal(lifecycleReplacement.QueuedAt) {
		t.Fatalf("lifecycle replacement = plan_mode:%t queued_at:%s, want plan_mode:true queued_at:%s",
			updated.PlanMode, updated.QueuedAt, lifecycleReplacement.QueuedAt)
	}
}
