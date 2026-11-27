package process

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestCancelledDeliveryRetainsOneTerminalForBackendSettlement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	store, err := journal.Open(journal.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: &config.InstanceConfig{
		DurableJournalPath: path, SessionID: "session-1", DeliveryStreamID: "stream-1",
		DeliveryIncarnationID: "incarnation-1", DeliveryHarnessGeneration: 1,
	}, logger: newTestLogger(t), deliveryJournal: store}
	t.Cleanup(func() { _ = manager.closeDeliveryJournal() })
	_, err = store.PutSubmission(ctx, journal.Submission{
		ID: "submission-1", SessionID: "session-1", IncarnationID: "incarnation-1",
		HarnessGeneration: 1, StreamID: "stream-1", Hash: "hash", State: journal.SubmissionInterruptedUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := manager.CancelDeliverySubmission(ctx, "submission-1"); err != nil {
			t.Fatal(err)
		}
	}
	submission, err := store.GetSubmission(ctx, "submission-1")
	if err != nil || submission.State != journal.SubmissionCancelled || !submission.TerminalEventRetained {
		t.Fatalf("cancelled submission lacks replayable settlement: %+v, %v", submission, err)
	}
	events, _, err := store.Replay(ctx, "stream-1", 0, 10)
	if err != nil || len(events) != 1 || !events[0].Terminal || events[0].SubmissionID != submission.ID {
		t.Fatalf("cancel replay = %+v, %v; want one exact terminal", events, err)
	}
}
