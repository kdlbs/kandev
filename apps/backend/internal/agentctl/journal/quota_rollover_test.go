package journal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestJournalSubmissionQuotaCountsPayloadAndMetadata(t *testing.T) {
	j, err := Open(Config{
		Path:            filepath.Join(t.TempDir(), "delivery.bbolt"),
		MaxJournalBytes: 4096,
		ReserveBytes:    128,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	var accepted Submission
	for i := 0; i < 100; i++ {
		submission, err := j.PutSubmission(ctx, Submission{
			ID: fmt.Sprintf("submission-%03d", i), StreamID: "stream", SessionID: "session",
			IncarnationID: "incarnation", HarnessGeneration: 1, Hash: fmt.Sprintf("hash-%03d", i),
			Payload: []byte("payload that counts against the logical journal quota"),
		})
		if errors.Is(err, ErrJournalFull) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		accepted = submission
	}
	if accepted.ID == "" {
		t.Fatal("submission quota did not admit any record")
	}
	duplicate, err := j.PutSubmission(ctx, accepted)
	if err != nil || duplicate.ID != accepted.ID {
		t.Fatalf("duplicate submission after quota = %+v, err=%v", duplicate, err)
	}
}

func TestJournalPruneMaterializedSubmissionPages(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	for i := 0; i < 32; i++ {
		id := fmt.Sprintf("submission-%02d", i)
		if _, err := j.PutSubmission(ctx, Submission{
			ID: id, StreamID: "stream", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			Hash: id, Payload: []byte("payload"),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := j.TransitionSubmission(ctx, id, SubmissionAccepted, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if _, err := j.TransitionSubmission(ctx, id, SubmissionDispatching, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if _, err := j.Append(ctx, Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream",
			SubmissionID: id, Type: "complete", Terminal: true, Payload: []byte("done"),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := j.TransitionSubmission(ctx, id, SubmissionCompleted, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Acknowledge(ctx, "stream", 32); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		submission, err := j.GetSubmission(ctx, fmt.Sprintf("submission-%02d", i))
		if err != nil {
			t.Fatal(err)
		}
		if len(submission.Payload) != 0 {
			t.Fatalf("submission %s payload was not pruned", submission.ID)
		}
	}
}

func TestJournalAccountingUpgradeReconcilesSubmissionBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.PutSubmission(context.Background(), Submission{ID: "submission", Hash: "hash", Payload: []byte("payload")}); err != nil {
		t.Fatal(err)
	}
	if err := j.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(1)) }); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	var logicalBytes int64
	if err := j.db.View(func(tx *bolt.Tx) error {
		var err error
		logicalBytes, err = decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if logicalBytes <= 1 {
		t.Fatalf("reconciled logical bytes = %d, want submission bytes", logicalBytes)
	}
}

func TestJournalRolloverSealsIdleStream(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "submission", StreamID: "old-stream", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		Hash: "hash", Payload: []byte("prompt"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission", SubmissionAccepted, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission", SubmissionDispatching, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "old-stream",
		SubmissionID: "submission", Type: "complete", Terminal: true, Payload: []byte("done"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission", SubmissionCompleted, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := j.Acknowledge(ctx, "old-stream", 1); err != nil {
		t.Fatal(err)
	}
	if err := j.RolloverStream(ctx, "old-stream", Stream{StreamID: "new-stream"}); err != nil {
		t.Fatal(err)
	}
	old, err := j.GetStream(ctx, "old-stream")
	if err != nil || !old.Sealed {
		t.Fatalf("old stream = %+v, err=%v", old, err)
	}
	if _, err := j.Append(ctx, Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "old-stream", Type: "message", Payload: []byte("old")}); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("append to sealed stream error = %v, want owner mismatch", err)
	}
	if _, err := j.Append(ctx, Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "new-stream", Type: "message", Payload: []byte("new")}); err != nil {
		t.Fatal(err)
	}
	retired, err := j.GetSubmission(ctx, "submission")
	if err != nil || retired.ID != "submission" {
		t.Fatalf("old submission tombstone = %+v, err=%v", retired, err)
	}
}
