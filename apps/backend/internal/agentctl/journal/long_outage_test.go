package journal

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
func TestDefaultJournalAccountingCrossesFormerTwoGiBLimit(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	const formerLimit = int64(2 << 30)
	if err := j.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(formerLimit-8))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(context.Background(), Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: []byte("cross the old boundary"),
	}); err != nil {
		t.Fatalf("append past former 2 GiB logical limit: %v", err)
	}
	capacity, err := j.Capacity(context.Background(), "stream")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.JournalBytes <= formerLimit {
		t.Fatalf("journal accounting = %d, want greater than former limit %d", capacity.JournalBytes, formerLimit)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.5
func TestWeekOldUnacknowledgedOutputRetainsSubmissionIdentityAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	weekAgo := time.Now().UTC().Add(-7 * 24 * time.Hour)
	submission, err := j.PutSubmission(ctx, Submission{
		ID: "week-old-submission", StreamID: "week-old-stream", SessionID: "session",
		IncarnationID: "incarnation", HarnessGeneration: 9, Hash: "original-prompt-hash",
		Payload: []byte("original prompt identity"), State: SubmissionDispatching,
		CreatedAt: weekAgo, UpdatedAt: weekAgo,
	})
	if err != nil {
		t.Fatalf("put original submission: %v", err)
	}
	event, err := j.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 9,
		StreamID: "week-old-stream", SubmissionID: submission.ID, Type: "message",
		Payload: []byte(`{"text":"retained through a week offline"}`), CreatedAt: weekAgo,
	})
	if err != nil {
		t.Fatalf("append week-old output: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = Open(Config{Path: path, ExistingOnly: true})
	if err != nil {
		t.Fatalf("reopen journal: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })

	recoveredSubmission, err := j.GetSubmission(ctx, "week-old-submission")
	if err != nil {
		t.Fatal(err)
	}
	if recoveredSubmission.ID != submission.ID || recoveredSubmission.Hash != "original-prompt-hash" ||
		recoveredSubmission.HarnessGeneration != 9 || string(recoveredSubmission.Payload) != "original prompt identity" {
		t.Fatalf("submission identity changed after a week: %+v", recoveredSubmission)
	}
	page, stream, err := j.Replay(ctx, "week-old-stream", 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].Sequence != event.Sequence || page[0].CreatedAt != weekAgo ||
		string(page[0].Payload) != `{"text":"retained through a week offline"}` || stream.HighWater != 1 {
		t.Fatalf("week-old output replay = %+v, stream = %+v", page, stream)
	}
}
