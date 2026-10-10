package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestRetainedReconstructionEvidence(t *testing.T) {
	ctx := context.Background()
	capability := CheckStorage(t.TempDir(), "session")
	j, err := Open(Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	payload := []byte(`{"prompt":"continue the existing conversation"}`)
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "unresolved", StreamID: "stream-current", SessionID: "session",
		IncarnationID: "incarnation-current", HarnessGeneration: 7,
		Hash: SubmissionHash(payload), Payload: payload, State: SubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.PutSubmission(ctx, Submission{
		ID: "completed-history", StreamID: "stream-old", SessionID: "session",
		IncarnationID: "incarnation-old", HarnessGeneration: 6,
		Hash: SubmissionHash([]byte("completed")), Payload: []byte("completed"),
		State: SubmissionCompleted, TerminalEventRetained: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
		StreamID: "stream-current", SubmissionID: "unresolved", Type: "message",
		Payload: []byte(`{"text":"retained output"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.Acknowledge(ctx, "stream-current", 1); err != nil {
		t.Fatal(err)
	}

	descriptor, err := j.RecoveryDescriptor(ctx, "session", "incarnation-current", 7, "")
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.SubmissionCount != 1 || descriptor.SubmissionsTruncated || len(descriptor.Submissions) != 1 {
		t.Fatalf("candidate set = count %d, truncated %t, summaries %+v; want the complete unresolved set only", descriptor.SubmissionCount, descriptor.SubmissionsTruncated, descriptor.Submissions)
	}
	summary := descriptor.Submissions[0]
	if summary.ID != "unresolved" || summary.SessionID != "session" || summary.IncarnationID != "incarnation-current" || summary.HarnessGeneration != 7 || summary.Hash != SubmissionHash(payload) || summary.State != SubmissionDispatching {
		t.Fatalf("candidate summary = %+v", summary)
	}
	if summary.StreamID != "stream-current" {
		t.Fatalf("candidate stream identity = %q, want stream-current", summary.StreamID)
	}
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 {
		t.Fatal("empty recovery descriptor")
	}
	if bytes.Contains(encoded, []byte(`"payload"`)) {
		t.Fatalf("recovery descriptor exposed a submission payload: %s", encoded)
	}
	stream, err := j.GetStream(ctx, "stream-current")
	if err != nil || stream.HighWater != 1 || stream.Acknowledged != 1 {
		t.Fatalf("stream after inspection = %+v, err %v; want fully acknowledged retained output", stream, err)
	}
	stored, err := j.GetSubmission(ctx, "unresolved")
	if err != nil || string(stored.Payload) != string(payload) || stored.State != SubmissionDispatching {
		t.Fatalf("submission after inspection = %+v, err %v; inspection must not mutate retained work", stored, err)
	}
}
