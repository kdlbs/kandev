package journal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestJournalOperationsAfterClose(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	for name, operation := range closedJournalOperations(j) {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
				t.Fatalf("operation after close = %v, want database not open", err)
			}
		})
	}
	if err := j.Close(); err != nil {
		t.Fatalf("repeated close: %v", err)
	}
}

func closedJournalOperations(j *Journal) map[string]func() error {
	ctx := context.Background()
	event := Event{SessionID: "session", StreamID: "stream", Type: "message"}
	return map[string]func() error{
		"replay": func() error { _, _, err := j.Replay(ctx, "stream", 0, 10); return err },
		"append": func() error { _, err := j.Append(ctx, event); return err },
		"empty append batch": func() error {
			_, err := j.AppendBatch(ctx, nil)
			return err
		},
		"acknowledge": func() error { return j.Acknowledge(ctx, "stream", 1) },
		"get stream":  func() error { _, err := j.GetStream(ctx, "stream"); return err },
		"recovery": func() error {
			_, err := j.RecoveryDescriptor(ctx, "session", "incarnation", 1, "stream")
			return err
		},
		"rollover":               func() error { return j.RolloverStream(ctx, "stream", Stream{StreamID: "next"}) },
		"unresolved submissions": func() error { _, err := j.HasUnresolvedSubmissions(ctx); return err },
		"list submissions":       func() error { _, err := j.ListSubmissions(ctx, "session"); return err },
		"get submission":         func() error { _, err := j.GetSubmission(ctx, "submission"); return err },
		"unresolved work":        func() error { _, err := j.HasUnresolvedWork(ctx); return err },
		"put submission": func() error {
			_, err := j.PutSubmission(ctx, Submission{ID: "submission", SessionID: "session", Hash: "hash"})
			return err
		},
		"invalid put submission": func() error {
			_, err := j.PutSubmission(ctx, Submission{})
			return err
		},
		"retire submission": func() error { _, err := j.RetireSubmission(ctx, "submission", 1); return err },
		"transition submission": func() error {
			_, err := j.TransitionSubmission(ctx, "submission", SubmissionDispatching, time.Now())
			return err
		},
		"cancel submission": func() error { return j.CancelSubmission(ctx, "submission", event) },
		"invalid cancel submission": func() error {
			return j.CancelSubmission(ctx, "submission", Event{})
		},
	}
}
