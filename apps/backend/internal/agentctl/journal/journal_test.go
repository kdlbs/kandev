package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestJournalCommittedRecordsSurviveKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	ctx := context.Background()

	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	first, err := j.Append(ctx, Event{
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 3,
		StreamID:          "stream-1",
		Type:              "message",
		Payload:           []byte("committed before publication"),
	})
	if err != nil {
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
	events, stream, err := j.Replay(ctx, "stream-1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Sequence != first.Sequence || string(events[0].Payload) != string(first.Payload) {
		t.Fatalf("replayed events = %#v", events)
	}
	if stream.HighWater != 1 || stream.FirstRetained != 1 {
		t.Fatalf("stream = %#v", stream)
	}
}

func TestJournalRecoveryDescriptorIsBoundedAndOmitsPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()

	for i := 0; i < MaxRecoverySubmissionSummaries+2; i++ {
		if _, err := j.PutSubmission(ctx, Submission{
			ID:                fmt.Sprintf("submission-%02d", i),
			SessionID:         "session-1",
			IncarnationID:     "incarnation-1",
			HarnessGeneration: 2,
			Hash:              fmt.Sprintf("hash-%02d", i),
			Payload:           []byte("private prompt payload"),
			State:             SubmissionAccepted,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := j.Append(ctx, Event{
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		StreamID:          "stream-1",
		Type:              "message",
		Payload:           []byte("event"),
	}); err != nil {
		t.Fatal(err)
	}

	descriptor, err := j.RecoveryDescriptor(ctx, "session-1", "incarnation-1", 2, "stream-1")
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Stream == nil || descriptor.Stream.HighWater != 1 || descriptor.Stream.FirstRetained != 1 {
		t.Fatalf("stream = %#v", descriptor.Stream)
	}
	if len(descriptor.Submissions) != MaxRecoverySubmissionSummaries {
		t.Fatalf("submission summaries = %d, want %d", len(descriptor.Submissions), MaxRecoverySubmissionSummaries)
	}
	if descriptor.SubmissionCount != MaxRecoverySubmissionSummaries+2 || !descriptor.SubmissionsTruncated {
		t.Fatalf("submission bounds = count %d truncated %v", descriptor.SubmissionCount, descriptor.SubmissionsTruncated)
	}
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private prompt payload") {
		t.Fatalf("descriptor exposed submission payload: %s", encoded)
	}
}

func TestJournalRecoveryDescriptorOmitsAcknowledgedCompletedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()

	const completedHistory = MaxRecoverySubmissionSummaries + 2
	for i := 0; i < completedHistory; i++ {
		id := fmt.Sprintf("completed-%02d", i)
		if _, err := j.PutSubmission(ctx, Submission{
			ID: id, SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			Hash: id, Payload: []byte("private prompt payload"),
		}); err != nil {
			t.Fatal(err)
		}
		for _, state := range []SubmissionState{SubmissionAccepted, SubmissionDispatching} {
			if _, err := j.TransitionSubmission(ctx, id, state, time.Time{}); err != nil {
				t.Fatalf("transition %s to %s: %v", id, state, err)
			}
		}
		if _, err := j.Append(ctx, Event{
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			StreamID: "stream-1", SubmissionID: id, Type: "complete", Terminal: true,
			Payload: []byte("done"),
		}); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
		if _, err := j.TransitionSubmission(ctx, id, SubmissionCompleted, time.Time{}); err != nil {
			t.Fatalf("complete %s: %v", id, err)
		}
	}
	if err := j.Acknowledge(ctx, "stream-1", completedHistory); err != nil {
		t.Fatalf("acknowledge completed history: %v", err)
	}

	descriptor, err := j.RecoveryDescriptor(ctx, "session-1", "incarnation-1", 2, "stream-1")
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Unresolved || descriptor.SubmissionCount != 0 || len(descriptor.Submissions) != 0 || descriptor.SubmissionsTruncated {
		t.Fatalf("completed history remained recovery evidence: unresolved=%t count=%d summaries=%d truncated=%t", descriptor.Unresolved, descriptor.SubmissionCount, len(descriptor.Submissions), descriptor.SubmissionsTruncated)
	}
}

func TestJournalRecoveryDescriptorKeepsActiveSubmissionAlongsideCompletedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()

	const completedHistory = MaxRecoverySubmissionSummaries + 2
	for i := 0; i < completedHistory; i++ {
		id := fmt.Sprintf("completed-%02d", i)
		if _, err := j.PutSubmission(ctx, Submission{
			ID: id, SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			Hash: id, Payload: []byte("done"),
		}); err != nil {
			t.Fatal(err)
		}
		for _, state := range []SubmissionState{SubmissionAccepted, SubmissionDispatching} {
			if _, err := j.TransitionSubmission(ctx, id, state, time.Time{}); err != nil {
				t.Fatalf("transition %s to %s: %v", id, state, err)
			}
		}
		if _, err := j.Append(ctx, Event{
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			StreamID: "stream-1", SubmissionID: id, Type: "complete", Terminal: true,
			Payload: []byte("done"),
		}); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
		if _, err := j.TransitionSubmission(ctx, id, SubmissionCompleted, time.Time{}); err != nil {
			t.Fatalf("complete %s: %v", id, err)
		}
	}
	if err := j.Acknowledge(ctx, "stream-1", completedHistory); err != nil {
		t.Fatalf("acknowledge completed history: %v", err)
	}
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "active", SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
		Hash: "active", Payload: []byte("active"), State: SubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}

	descriptor, err := j.RecoveryDescriptor(ctx, "session-1", "incarnation-1", 2, "stream-1")
	if err != nil {
		t.Fatal(err)
	}
	if !descriptor.Unresolved || descriptor.SubmissionCount != 1 || len(descriptor.Submissions) != 1 || descriptor.Submissions[0].ID != "active" || descriptor.SubmissionsTruncated {
		t.Fatalf("active evidence was not isolated from history: unresolved=%t count=%d summaries=%+v truncated=%t", descriptor.Unresolved, descriptor.SubmissionCount, descriptor.Submissions, descriptor.SubmissionsTruncated)
	}
}

func TestJournalRecoveryDescriptorDoesNotBoundCompletedTerminalHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()

	const completedHistory = MaxRecoverySubmissionSummaries + 2
	for i := 0; i < completedHistory; i++ {
		id := fmt.Sprintf("completed-%02d", i)
		if _, err := j.PutSubmission(ctx, Submission{
			ID: id, SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			Hash: id, Payload: []byte("done"),
		}); err != nil {
			t.Fatal(err)
		}
		for _, state := range []SubmissionState{SubmissionAccepted, SubmissionDispatching} {
			if _, err := j.TransitionSubmission(ctx, id, state, time.Time{}); err != nil {
				t.Fatalf("transition %s to %s: %v", id, state, err)
			}
		}
		if _, err := j.Append(ctx, Event{
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			StreamID: "stream-1", SubmissionID: id, Type: "complete", Terminal: true,
			Payload: []byte("done"),
		}); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
		if _, err := j.TransitionSubmission(ctx, id, SubmissionCompleted, time.Time{}); err != nil {
			t.Fatalf("complete %s: %v", id, err)
		}
	}

	descriptor, err := j.RecoveryDescriptor(ctx, "session-1", "incarnation-1", 2, "stream-1")
	if err != nil {
		t.Fatal(err)
	}
	if !descriptor.Unresolved || descriptor.SubmissionCount != 0 || len(descriptor.Submissions) != 0 || descriptor.SubmissionsTruncated {
		t.Fatalf("completed terminal history was treated as ambiguous submissions: unresolved=%t count=%d summaries=%d truncated=%t", descriptor.Unresolved, descriptor.SubmissionCount, len(descriptor.Submissions), descriptor.SubmissionsTruncated)
	}
}

func TestJournalStorageFailuresFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{
		Path:            path,
		MaxEventBytes:   64,
		MaxStreamBytes:  1 << 20,
		MaxJournalBytes: 1024,
		ReserveBytes:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = j.Append(ctx, Event{SessionID: "s", IncarnationID: "i", StreamID: "stream", Type: "message", Payload: make([]byte, 65)})
	if !errors.Is(err, ErrStreamFull) {
		t.Fatalf("oversized event error = %v", err)
	}
	for sequence := uint64(1); ; sequence++ {
		_, err = j.Append(ctx, Event{
			SessionID:     "s",
			IncarnationID: "i",
			StreamID:      "stream",
			Sequence:      sequence,
			Type:          "message",
			Payload:       []byte("bounded payload"),
		})
		if errors.Is(err, ErrJournalFull) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if sequence > 100 {
			t.Fatal("journal did not enforce its byte quota")
		}
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	corruptPath := filepath.Join(t.TempDir(), "corrupt.bbolt")
	if err := os.WriteFile(corruptPath, []byte("not a bbolt database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(Config{Path: corruptPath}); err == nil {
		t.Fatal("corrupt journal opened successfully")
	}
	if data, err := os.ReadFile(corruptPath); err != nil || string(data) != "not a bbolt database" {
		t.Fatalf("corrupt journal was replaced: err=%v data=%q", err, data)
	}
}

func TestJournalMalformedByteCounterFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keyJournalBytes, []byte{1})
	})
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Open(Config{Path: path}); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("malformed byte counter open error = %v, want ErrJournalCorrupt", err)
	}
}

func TestJournalReplayCursorAndAcknowledgement(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	for i := uint64(0); i < 3; i++ {
		if _, err := j.Append(ctx, Event{SessionID: "s", IncarnationID: "i", StreamID: "stream", Type: "message", Payload: []byte{byte(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	events, _, err := j.Replay(ctx, "stream", 1, 10)
	if err != nil || len(events) != 2 || events[0].Sequence != 2 {
		t.Fatalf("replay after cursor = %#v, err=%v", events, err)
	}
	if err := j.Acknowledge(ctx, "stream", 2); err != nil {
		t.Fatal(err)
	}
	events, stream, err := j.Replay(ctx, "stream", 2, 10)
	if err != nil || len(events) != 1 || events[0].Sequence != 3 {
		t.Fatalf("replay after acknowledgement = %#v, stream=%#v, err=%v", events, stream, err)
	}
	if _, _, err := j.Replay(ctx, "stream", 0, 10); !errors.Is(err, ErrCursorExpired) {
		t.Fatalf("expired cursor error = %v", err)
	}
}

func TestJournalSubmissionIdentityIsIdempotent(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	first, err := j.PutSubmission(ctx, Submission{ID: "submission-1", Hash: "hash-1", Payload: []byte("prompt")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := j.PutSubmission(ctx, Submission{ID: "submission-1", Hash: "hash-1", Payload: []byte("different bytes")})
	if err != nil {
		t.Fatal(err)
	}
	if string(second.Payload) != string(first.Payload) || second.State != SubmissionPrepared {
		t.Fatalf("duplicate submission changed identity: %#v", second)
	}
	if _, err := j.PutSubmission(ctx, Submission{ID: "submission-1", Hash: "other-hash", Payload: []byte("prompt")}); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatalf("conflicting submission error = %v", err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission-1", SubmissionAccepted, first.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission-1", SubmissionCompleted, first.UpdatedAt); !errors.Is(err, ErrSubmissionState) {
		t.Fatalf("invalid transition error = %v", err)
	}
}

func TestJournalExplicitCancelSettlesUncertainSubmission(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	submission, err := j.PutSubmission(ctx, Submission{
		ID: "submission-cancel", Hash: "hash-cancel", Payload: []byte("prompt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []SubmissionState{SubmissionAccepted, SubmissionDispatching, SubmissionInterruptedUnknown} {
		if _, err := j.TransitionSubmission(ctx, submission.ID, state, time.Time{}); err != nil {
			t.Fatalf("transition to %s: %v", state, err)
		}
	}
	if _, err := j.TransitionSubmission(ctx, submission.ID, SubmissionCancelled, time.Time{}); err != nil {
		t.Fatalf("explicit cancel: %v", err)
	}
	stored, err := j.GetSubmission(ctx, submission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != SubmissionCancelled {
		t.Fatalf("cancelled state = %q, want %q", stored.State, SubmissionCancelled)
	}
	unresolved, err := j.HasUnresolvedSubmissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if unresolved {
		t.Fatal("explicitly cancelled submission remained unresolved")
	}
}

func TestJournalTerminalEventMarksSubmissionAtomically(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "submission-terminal", Hash: "hash-terminal", Payload: []byte("prompt"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission-terminal", SubmissionAccepted, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission-terminal", SubmissionDispatching, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, Event{
		SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
		StreamID: "stream-1", SubmissionID: "submission-terminal", Type: "complete",
		Terminal: true, Payload: []byte("done"),
	}); err != nil {
		t.Fatal(err)
	}
	submission, err := j.GetSubmission(ctx, "submission-terminal")
	if err != nil {
		t.Fatal(err)
	}
	if !submission.TerminalEventRetained || submission.TerminalSequence != 1 {
		t.Fatalf("terminal marker = %#v", submission)
	}
	if err := j.Acknowledge(ctx, "stream-1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := j.TransitionSubmission(ctx, "submission-terminal", SubmissionCompleted, time.Time{}); err != nil {
		t.Fatal(err)
	}
	unresolved, err := j.HasUnresolvedWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if unresolved {
		t.Fatal("terminally retained submission remained unresolved")
	}
}

func TestJournalUnacknowledgedEventsDoNotBlockPromptAdmission(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	if _, err := j.Append(ctx, Event{
		SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 1,
		StreamID: "stream-1", Type: "message", Payload: []byte("boot metadata"),
	}); err != nil {
		t.Fatal(err)
	}

	unresolved, err := j.HasUnresolvedSubmissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if unresolved {
		t.Fatal("an unacknowledged replayable event blocked prompt admission")
	}
	work, err := j.HasUnresolvedWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !work {
		t.Fatal("expected unacknowledged event to remain unresolved work")
	}

	if _, err := j.PutSubmission(ctx, Submission{
		ID: "submission-1", Hash: "hash-1", Payload: []byte("prompt"),
	}); err != nil {
		t.Fatal(err)
	}
	unresolved, err = j.HasUnresolvedSubmissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !unresolved {
		t.Fatal("an unsettled prompt submission did not block prompt admission")
	}
}

func TestJournalMissingStreamHasDistinctError(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if _, err := j.GetStream(context.Background(), "missing"); !errors.Is(err, ErrStreamNotFound) {
		t.Fatalf("missing stream error = %v, want ErrStreamNotFound", err)
	}
}

func TestJournalCompactionPreservesCommittedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 32; i++ {
		if _, err := j.Append(ctx, Event{
			SessionID: "session-compact", IncarnationID: "incarnation-compact",
			HarnessGeneration: 1, StreamID: "stream-compact", Type: "message",
			Payload: []byte(strings.Repeat("x", 1024)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Acknowledge(ctx, "stream-compact", 31); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() >= before.Size() {
		t.Fatalf("compacted journal size = %d, before = %d", after.Size(), before.Size())
	}
	events, _, err := j.Replay(ctx, "stream-compact", 31, 10)
	if err != nil || len(events) != 1 || events[0].Sequence != 32 {
		t.Fatalf("records after compaction = %#v, err=%v", events, err)
	}
	_ = j.Close()
}

func TestJournalRejectsOversizedSubmissionPayload(t *testing.T) {
	j, err := Open(Config{
		Path:          filepath.Join(t.TempDir(), "delivery.bbolt"),
		MaxEventBytes: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	_, err = j.PutSubmission(context.Background(), Submission{
		ID:      "submission-large",
		Hash:    "hash-large",
		Payload: make([]byte, 9),
	})
	if !errors.Is(err, ErrStreamFull) {
		t.Fatalf("oversized submission error = %v, want ErrStreamFull", err)
	}
	if _, err := j.GetSubmission(context.Background(), "submission-large"); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("oversized submission lookup error = %v, want ErrSubmissionNotFound", err)
	}
}
