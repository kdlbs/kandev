package process

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestInterruptedRetirementRequiresCurrentOwnerGeneration(t *testing.T) {
	m, j := pressureTestManager(t)
	ctx := context.Background()
	for _, input := range []journal.Submission{
		{ID: "foreign", SessionID: "foreign", IncarnationID: "foreign", HarnessGeneration: 1, Hash: "hash", Payload: []byte("old"), State: journal.SubmissionInterruptedUnknown},
		{ID: "owned", SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, Hash: "hash", Payload: []byte("old"), State: journal.SubmissionInterruptedUnknown},
	} {
		if _, err := j.PutSubmission(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"foreign", "owned"} {
		if err := m.RetireDeliverySubmission(ctx, id, 2); !errors.Is(err, journal.ErrOwnerMismatch) {
			t.Fatalf("retired %s without current owner: %v", id, err)
		}
		stored, err := j.GetSubmission(ctx, id)
		if err != nil || stored.Retired {
			t.Fatalf("invalid retirement mutated %s: %+v, %v", id, stored, err)
		}
	}
	m.cfg.DeliveryHarnessGeneration = 2
	if err := m.RetireDeliverySubmission(ctx, "owned", 2); err != nil {
		t.Fatal(err)
	}
	stored, err := j.GetSubmission(ctx, "owned")
	if err != nil || !stored.Retired || stored.State != journal.SubmissionInterruptedUnknown {
		t.Fatalf("owned retirement: %+v, %v", stored, err)
	}
}
