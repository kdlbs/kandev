package journal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestJournalBatchAtomicity(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	_, err = j.AppendBatch(context.Background(), []Event{
		{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Type: "message", Payload: []byte("first")},
		{SessionID: "other-session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Type: "message", Payload: []byte("must rollback")},
	})
	if !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("batch error = %v, want owner mismatch", err)
	}
	if _, _, err := j.Replay(context.Background(), "stream", 0, 10); !errors.Is(err, ErrStreamNotFound) {
		t.Fatalf("rolled-back stream replay error = %v, want stream not found", err)
	}

	events, err := j.AppendBatch(context.Background(), []Event{
		{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Type: "message", Payload: []byte("first")},
		{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Type: "message", Payload: []byte("second")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("committed batch events = %#v", events)
	}
}

func TestJournalBatchTerminalEvidenceCommitsWithEvents(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	if _, err := j.PutSubmission(ctx, Submission{ID: "submission", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, Hash: "hash", Payload: []byte("prompt")}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission", SubmissionAccepted, nowForBatchTest()); err != nil {
		t.Fatal(err)
	}
	if _, err := j.AppendBatch(ctx, []Event{
		{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "submission", Type: "complete", Terminal: true, Payload: []byte("done")},
	}); err != nil {
		t.Fatal(err)
	}
	submission, err := j.GetSubmission(ctx, "submission")
	if err != nil {
		t.Fatal(err)
	}
	if !submission.TerminalEventRetained || submission.TerminalSequence != 1 {
		t.Fatalf("terminal evidence = %+v", submission)
	}
}

func BenchmarkJournalDeliveryBurst(b *testing.B) {
	j, err := Open(Config{Path: filepath.Join(b.TempDir(), "delivery.bbolt"), MaxStreamBytes: 1 << 30, MaxJournalBytes: 1 << 30})
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		events := make([]Event, 0, 32)
		for chunk := 0; chunk < cap(events); chunk++ {
			events = append(events, Event{
				SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
				StreamID: "stream-" + fmt.Sprint(i), Type: "message", Payload: []byte("small chunk"),
			})
		}
		if _, err := j.AppendBatch(ctx, events); err != nil {
			b.Fatal(err)
		}
	}
}

func nowForBatchTest() time.Time {
	return time.Now().UTC()
}
