package journal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestCancelSubmissionRollsBackWhenTerminalCannotBeRetained(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "journal.bbolt")
	store, err := Open(Config{Path: path, MaxStreamBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	submission := Submission{ID: "submission", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Hash: "hash", State: SubmissionInterruptedUnknown}
	if _, err := store.PutSubmission(ctx, submission); err != nil {
		t.Fatal(err)
	}
	event := Event{SessionID: submission.SessionID, IncarnationID: submission.IncarnationID, HarnessGeneration: 1, StreamID: submission.StreamID, SubmissionID: submission.ID, Type: "complete", Terminal: true, Payload: []byte(`{"data":{"stop_reason":"cancelled"}}`)}
	if err := store.CancelSubmission(ctx, submission.ID, event); !errors.Is(err, ErrStreamFull) {
		t.Fatalf("cancel error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	got, err := store.GetSubmission(ctx, submission.ID)
	if err != nil || got.State != SubmissionInterruptedUnknown || got.TerminalEventRetained {
		t.Fatalf("rollback after reopen = %+v, %v", got, err)
	}
	if err := store.CancelSubmission(ctx, submission.ID, event); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelSubmission(ctx, submission.ID, event); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSubmission(ctx, submission.ID)
	if err != nil || got.State != SubmissionCancelled || got.TerminalSequence != 1 {
		t.Fatalf("retry = %+v, %v", got, err)
	}
}
