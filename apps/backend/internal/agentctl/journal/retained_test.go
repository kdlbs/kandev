package journal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedRecoveryRejectsAckAheadOfProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	capability := CheckStorage(root, "session")
	j, err := Open(Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = j.PutSubmission(ctx, Submission{ID: "old", SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", Hash: "hash", State: SubmissionDispatching}); err != nil {
		t.Fatal(err)
	}
	if _, err = j.Append(ctx, Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "old", Type: "message", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	request := RetainedRecoveryRequest{Root: root, SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "old", Limit: 4, Acknowledge: 1}
	if _, err = ReadRetainedRecovery(ctx, request); !errors.Is(err, ErrSequenceConflict) {
		t.Fatalf("ACK ahead of projected cursor: %v", err)
	}
	request.Acknowledge = 0
	if _, err = ReadRetainedRecovery(ctx, request); err != nil {
		t.Fatal(err)
	}
	request.HarnessGeneration++
	if _, err = ReadRetainedRecovery(ctx, request); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("wrong generation: %v", err)
	}
	request.HarnessGeneration--
	if err = os.WriteFile(filepath.Join(filepath.Dir(capability.Path), ".lost"), []byte("lost"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadRetainedRecovery(ctx, request); err == nil {
		t.Fatal("lost journal accepted as retained evidence")
	}
}

func TestOpenExistingJournalDoesNotCreateMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	j, err := Open(Config{Path: path, ExistingOnly: true})
	if j != nil {
		_ = j.Close()
	}
	if err == nil {
		t.Fatal("created missing retained journal")
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file recreated: %v", err)
	}
}
