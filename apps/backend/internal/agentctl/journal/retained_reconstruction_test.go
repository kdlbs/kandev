package journal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestReadRetainedReconstructionEvidenceAcceptsLegacyEventAccountingWithoutMutation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	capability := CheckStorage(root, "session")
	j, err := Open(Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"text":"recover without rewriting the retained file"}`)
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "prompt:legacy", SessionID: "session", IncarnationID: "incarnation-current",
		HarnessGeneration: 7, StreamID: "stream-current", Hash: SubmissionHash(payload),
		Payload: payload, State: SubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
		StreamID: "stream-current", SubmissionID: "prompt:legacy", Type: "message",
		Payload: []byte(`{"text":"retained output"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	legacyCounter := int64(0)
	db, err := bolt.Open(capability.Path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		events := tx.Bucket(bucketEvents)
		return events.ForEach(func(streamID, value []byte) error {
			if value != nil {
				return nil
			}
			return events.Bucket(streamID).ForEach(func(_, eventValue []byte) error {
				legacyCounter += int64(len(eventValue))
				return nil
			})
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(legacyCounter))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(capability.Path)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := ReadRetainedReconstructionEvidence(ctx, RetainedReconstructionEvidenceRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
	})
	if err != nil {
		t.Fatalf("inspect supported legacy accounting: %v", err)
	}
	if evidence.Descriptor.SubmissionCount != 1 || len(evidence.Descriptor.Submissions) != 1 {
		t.Fatalf("descriptor = %+v", evidence.Descriptor)
	}
	after, err := os.ReadFile(capability.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only inspection changed legacy journal bytes")
	}
}

func TestReadRetainedReconstructionEvidenceAndSelectedSubmission(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	capability := CheckStorage(root, "session")
	j, err := Open(Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"text":"restore the saved instruction"}`)
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation-current",
		HarnessGeneration: 7, StreamID: "stream-current", Hash: SubmissionHash(payload),
		Payload: payload, State: SubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "prompt:done", SessionID: "session", IncarnationID: "incarnation-old",
		HarnessGeneration: 6, StreamID: "stream-old", Hash: SubmissionHash([]byte(`{"text":"done"}`)),
		Payload: []byte(`{"text":"done"}`), State: SubmissionCompleted, TerminalEventRetained: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
		StreamID: "stream-current", SubmissionID: "prompt:one", Type: "message",
		Payload: []byte(`{"text":"retained output"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(capability.Path)
	if err != nil {
		t.Fatal(err)
	}

	request := RetainedReconstructionEvidenceRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
	}
	evidence, err := ReadRetainedReconstructionEvidence(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.ProcessTerminated != nil {
		t.Fatalf("missing process identity became proof: %v", *evidence.ProcessTerminated)
	}
	if evidence.Descriptor.SubmissionCount != 1 || evidence.Descriptor.SubmissionsTruncated || len(evidence.Descriptor.Submissions) != 1 {
		t.Fatalf("descriptor candidate set = %+v", evidence.Descriptor)
	}
	candidate := evidence.Descriptor.Submissions[0]
	if candidate.ID != "prompt:one" || candidate.StreamID != "stream-current" || candidate.Hash != SubmissionHash(payload) {
		t.Fatalf("candidate = %+v", candidate)
	}
	if evidence.Descriptor.Stream == nil || evidence.Descriptor.Stream.StreamID != candidate.StreamID {
		t.Fatalf("descriptor stream = %+v, want the unique candidate stream", evidence.Descriptor.Stream)
	}

	selected, err := ReadRetainedReconstructionSubmission(ctx, RetainedReconstructionSubmissionRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7, Candidate: candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != candidate.ID || !bytes.Equal(selected.Payload, payload) || selected.Hash != candidate.Hash {
		t.Fatalf("selected submission = %+v", selected)
	}

	candidate.Hash = "changed"
	if _, err := ReadRetainedReconstructionSubmission(ctx, RetainedReconstructionSubmissionRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7, Candidate: candidate,
	}); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatalf("changed candidate error = %v, want conflict", err)
	}
	after, err := os.ReadFile(capability.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("retained evidence and payload inspection mutated the journal file")
	}
}

func TestReadRetainedReconstructionSubmissionRejectsAmbiguityAndInvalidPayload(t *testing.T) {
	ctx := context.Background()
	t.Run("multiple unresolved candidates", func(t *testing.T) {
		root, candidate := retainedReconstructionFixture(t, []byte(`{"text":"first"}`))
		capability := CheckStorage(root, "session")
		j, err := Open(Config{Path: capability.Path})
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte(`{"text":"second"}`)
		if _, err := j.PutSubmission(ctx, Submission{
			ID: "prompt:two", SessionID: "session", IncarnationID: "incarnation-old",
			HarnessGeneration: 6, StreamID: "stream-old", Hash: SubmissionHash(payload), Payload: payload,
			State: SubmissionAccepted,
		}); err != nil {
			t.Fatal(err)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		_, err = ReadRetainedReconstructionSubmission(ctx, RetainedReconstructionSubmissionRequest{
			Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7, Candidate: candidate,
		})
		if !errors.Is(err, ErrSubmissionConflict) {
			t.Fatalf("ambiguous candidates error = %v, want conflict", err)
		}
	})
	t.Run("invalid immutable prompt envelope", func(t *testing.T) {
		for _, payload := range [][]byte{
			[]byte(`{"prompt":"not the prompt envelope"}`),
			[]byte(`{"text":"valid","attachments":null}`),
			[]byte(`{"text":"valid","attachments":[{"type":"resource","unexpected":true}]}`),
			[]byte(`{"text":"valid","steer":null}`),
		} {
			root, candidate := retainedReconstructionFixture(t, payload)
			_, err := ReadRetainedReconstructionSubmission(ctx, RetainedReconstructionSubmissionRequest{
				Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7, Candidate: candidate,
			})
			if !errors.Is(err, ErrSubmissionConflict) {
				t.Fatalf("invalid envelope %s error = %v, want conflict", payload, err)
			}
		}
	})
	t.Run("payload hash differs from its journal summary", func(t *testing.T) {
		root := t.TempDir()
		capability := CheckStorage(root, "session")
		j, err := Open(Config{Path: capability.Path})
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte(`{"text":"changed bytes"}`)
		submission, err := j.PutSubmission(ctx, Submission{
			ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation-current",
			HarnessGeneration: 7, StreamID: "stream-current", Hash: "retained-hash-does-not-match",
			Payload: payload, State: SubmissionDispatching,
		})
		if err != nil {
			t.Fatal(err)
		}
		candidate := submissionSummary(submission)
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		_, err = ReadRetainedReconstructionSubmission(ctx, RetainedReconstructionSubmissionRequest{
			Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7, Candidate: candidate,
		})
		if !errors.Is(err, ErrSubmissionConflict) {
			t.Fatalf("changed payload hash error = %v, want conflict", err)
		}
	})
	t.Run("truncated candidate set", func(t *testing.T) {
		root := t.TempDir()
		capability := CheckStorage(root, "session")
		j, err := Open(Config{Path: capability.Path})
		if err != nil {
			t.Fatal(err)
		}
		var candidate SubmissionSummary
		for i := 0; i < MaxRecoverySubmissionSummaries+1; i++ {
			payload := []byte(`{"text":"candidate"}`)
			submission, putErr := j.PutSubmission(ctx, Submission{
				ID: fmt.Sprintf("prompt:%02d", i), SessionID: "session", IncarnationID: "incarnation-current",
				HarnessGeneration: 7, StreamID: "stream-current", Hash: SubmissionHash(payload),
				Payload: payload, State: SubmissionDispatching,
			})
			if putErr != nil {
				t.Fatal(putErr)
			}
			if i == 0 {
				candidate = submissionSummary(submission)
			}
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		evidence, err := ReadRetainedReconstructionEvidence(ctx, RetainedReconstructionEvidenceRequest{
			Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !evidence.Descriptor.SubmissionsTruncated || evidence.Descriptor.SubmissionCount != MaxRecoverySubmissionSummaries+1 || len(evidence.Descriptor.Submissions) != MaxRecoverySubmissionSummaries {
			t.Fatalf("bounded descriptor = count %d truncated %t summaries %d", evidence.Descriptor.SubmissionCount, evidence.Descriptor.SubmissionsTruncated, len(evidence.Descriptor.Submissions))
		}
		_, err = ReadRetainedReconstructionSubmission(ctx, RetainedReconstructionSubmissionRequest{
			Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7, Candidate: candidate,
		})
		if !errors.Is(err, ErrSubmissionConflict) {
			t.Fatalf("truncated candidate selection error = %v, want conflict", err)
		}
	})
}

func TestReadRetainedReconstructionEvidenceRequiresExistingExclusiveOwner(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	capability := CheckStorage(root, "session")
	j, err := Open(Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	request := RetainedReconstructionEvidenceRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
	}
	if _, err := ReadRetainedReconstructionEvidence(ctx, request); err == nil {
		t.Fatal("retained inspection opened a second writer while the owner held the journal lock")
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(capability.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRetainedReconstructionEvidence(ctx, request); err == nil {
		t.Fatal("retained inspection accepted a missing journal")
	}
	if _, err := os.Stat(capability.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing retained journal was recreated: %v", err)
	}
}

func retainedReconstructionFixture(t *testing.T, payload []byte) (string, SubmissionSummary) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	capability := CheckStorage(root, "session")
	j, err := Open(Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := j.PutSubmission(ctx, Submission{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation-current",
		HarnessGeneration: 7, StreamID: "stream-current", Hash: SubmissionHash(payload),
		Payload: payload, State: SubmissionDispatching,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := submissionSummary(submission)
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	return root, candidate
}
