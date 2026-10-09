package journal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func terminalSubmissionFixture(t *testing.T, state SubmissionState, retired bool) (*Journal, Event) {
	t.Helper()
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if _, err := j.PutSubmission(context.Background(), Submission{
		ID: "submission", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Hash: "hash", Payload: []byte("prompt"), State: state, Retired: retired,
	}); err != nil {
		t.Fatal(err)
	}
	return j, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", SubmissionID: "submission", Type: "complete", Terminal: true, Payload: []byte("done"),
	}
}

func TestTerminalCompletionPreservesOtherSubmissionStates(t *testing.T) {
	cases := []struct {
		name       string
		state      SubmissionState
		eventType  string
		retired    bool
		want       SubmissionState
		unresolved bool
	}{
		{"complete", SubmissionDispatching, "complete", false, SubmissionCompleted, false},
		{"error", SubmissionDispatching, "error", false, SubmissionDispatching, true},
		{"prepared", SubmissionPrepared, "complete", false, SubmissionPrepared, true},
		{"accepted", SubmissionAccepted, "complete", false, SubmissionAccepted, true},
		{"uncertain", SubmissionInterruptedUnknown, "complete", false, SubmissionInterruptedUnknown, true},
		{"failed", SubmissionFailed, "complete", false, SubmissionFailed, false},
		{"cancelled", SubmissionCancelled, "complete", false, SubmissionCancelled, false},
		{"retired", SubmissionDispatching, "complete", true, SubmissionDispatching, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j, event := terminalSubmissionFixture(t, tc.state, tc.retired)
			event.Type = tc.eventType
			if _, err := j.Append(context.Background(), event); err != nil {
				t.Fatal(err)
			}
			stored, err := j.GetSubmission(context.Background(), event.SubmissionID)
			if err != nil || stored.State != tc.want || !stored.TerminalEventRetained {
				t.Fatalf("terminal submission = %+v, err=%v, want state %s", stored, err, tc.want)
			}
			unresolved, err := j.HasUnresolvedSubmissions(context.Background())
			if err != nil || unresolved != tc.unresolved {
				t.Fatalf("unresolved = %v, err=%v, want %v", unresolved, err, tc.unresolved)
			}
		})
	}
}

func TestTerminalCompletionSurvivesReopenBeforeDispatchReturn(t *testing.T) {
	j, event := terminalSubmissionFixture(t, SubmissionDispatching, false)
	if _, err := j.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	config := j.config
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	stored, err := reopened.GetSubmission(context.Background(), event.SubmissionID)
	if err != nil || stored.State != SubmissionCompleted || stored.TerminalSequence != 1 {
		t.Fatalf("reopened submission = %+v, err=%v", stored, err)
	}
	unresolved, err := reopened.HasUnresolvedSubmissions(context.Background())
	if err != nil || unresolved {
		t.Fatalf("reopened completion blocks admission: unresolved=%v, err=%v", unresolved, err)
	}
}

func TestTerminalCompletionRejectsWrongSubmissionOwner(t *testing.T) {
	for _, field := range []string{"session", "incarnation", "generation", "stream"} {
		t.Run(field, func(t *testing.T) {
			j, event := terminalSubmissionFixture(t, SubmissionDispatching, false)
			switch field {
			case "session":
				event.SessionID = "other-session"
			case "incarnation":
				event.IncarnationID = "other-incarnation"
			case "generation":
				event.HarnessGeneration++
			case "stream":
				event.StreamID = "other-stream"
			}
			if _, err := j.Append(context.Background(), event); !errors.Is(err, ErrOwnerMismatch) {
				t.Fatalf("owner mismatch error = %v", err)
			}
			assertTerminalCompletionRollback(t, j, event)
		})
	}
}

func TestTerminalCompletionRollsBackWithBatch(t *testing.T) {
	j, event := terminalSubmissionFixture(t, SubmissionDispatching, false)
	other := event
	other.SessionID = "other-session"
	if _, err := j.AppendBatch(context.Background(), []Event{event, other}); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("batch owner mismatch error = %v", err)
	}
	assertTerminalCompletionRollback(t, j, event)
}

func assertTerminalCompletionRollback(t *testing.T, j *Journal, event Event) {
	t.Helper()
	stored, err := j.GetSubmission(context.Background(), event.SubmissionID)
	if err != nil || stored.State != SubmissionDispatching || stored.TerminalEventRetained {
		t.Fatalf("rejected terminal mutated submission = %+v, err=%v", stored, err)
	}
	if _, _, err := j.Replay(context.Background(), event.StreamID, 0, 10); !errors.Is(err, ErrStreamNotFound) {
		t.Fatalf("rejected terminal committed a stream: %v", err)
	}
}
