package process

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestSubmissionRetryDispatchesOnce(t *testing.T) {
	deliveryJournal, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = deliveryJournal.Close() })
	delivery := &SubmissionDelivery{Journal: deliveryJournal}
	submission, err := delivery.Admit(context.Background(), journal.Submission{ID: "submission-1", Hash: "hash-1", Payload: []byte("prompt")})
	if err != nil || submission.State != journal.SubmissionAccepted {
		t.Fatalf("admission = %#v, err=%v", submission, err)
	}
	calls := 0
	completed, err := delivery.Dispatch(context.Background(), submission.ID, func(context.Context) error {
		calls++
		return nil
	})
	if err != nil || completed.State != journal.SubmissionCompleted || calls != 1 {
		t.Fatalf("first dispatch = %#v, calls=%d, err=%v", completed, calls, err)
	}
	completed, err = delivery.Dispatch(context.Background(), submission.ID, func(context.Context) error {
		calls++
		return nil
	})
	if err != nil || completed.State != journal.SubmissionCompleted || calls != 1 {
		t.Fatalf("retry dispatch = %#v, calls=%d, err=%v", completed, calls, err)
	}
}

func TestSubmissionCrashWindowAndHashConflict(t *testing.T) {
	deliveryJournal, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = deliveryJournal.Close() })
	delivery := &SubmissionDelivery{Journal: deliveryJournal}
	submission, err := delivery.Admit(context.Background(), journal.Submission{ID: "submission-2", Hash: "hash-2", Payload: []byte("prompt")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deliveryJournal.TransitionSubmission(context.Background(), submission.ID, journal.SubmissionDispatching, delivery.now()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if _, err := delivery.Dispatch(context.Background(), submission.ID, func(context.Context) error {
		calls++
		return nil
	}); !errors.Is(err, ErrSubmissionUncertain) {
		t.Fatalf("crash-window error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("uncertain submission was redispatched: %d calls", calls)
	}
	if _, err := delivery.Admit(context.Background(), journal.Submission{ID: submission.ID, Hash: "different-hash"}); !errors.Is(err, journal.ErrSubmissionConflict) {
		t.Fatalf("hash conflict error = %v", err)
	}
}

func TestSubmissionKnownFailureSettlesWithoutReconciliation(t *testing.T) {
	deliveryJournal, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = deliveryJournal.Close() })
	delivery := &SubmissionDelivery{Journal: deliveryJournal}
	submission, err := delivery.Admit(context.Background(), journal.Submission{
		ID: "submission-known-failure", Hash: "hash-known-failure", Payload: []byte("prompt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("provider rejected prompt")
	requestErr := &acp.RequestError{Code: -32603, Message: wantErr.Error()}
	settled, err := delivery.Dispatch(context.Background(), submission.ID, func(context.Context) error {
		return requestErr
	})
	if !errors.Is(err, requestErr) {
		t.Fatalf("known failure = %v, want %v", err, requestErr)
	}
	if settled.State != journal.SubmissionFailed {
		t.Fatalf("settled state = %q, want %q", settled.State, journal.SubmissionFailed)
	}
	unresolved, err := deliveryJournal.HasUnresolvedSubmissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if unresolved {
		t.Fatal("known failure left an unresolved submission")
	}
}
