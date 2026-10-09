package process

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeRetirementRequiresIdleCurrentOwner(t *testing.T) {
	for _, condition := range []string{"idle", "busy", "foreign incarnation", "foreign session"} {
		t.Run(condition, func(t *testing.T) {
			ctx := context.Background()
			journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
			store, err := journal.Open(journal.Config{Path: journalPath})
			require.NoError(t, err)
			manager := &Manager{cfg: &config.InstanceConfig{DurableJournalPath: journalPath, SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliveryStreamID: "stream"}, deliveryJournal: store}
			t.Cleanup(func() { _ = manager.closeDeliveryJournal() })
			submission := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Hash: "hash", Payload: []byte("prompt"), State: journal.SubmissionInterruptedUnknown}
			switch condition {
			case "busy":
				manager.deliverySubmissionMu.Lock()
				defer manager.deliverySubmissionMu.Unlock()
			case "foreign incarnation":
				submission.IncarnationID = "other-incarnation"
			case "foreign session":
				submission.SessionID = "other-session"
			}
			_, err = store.PutSubmission(ctx, submission)
			require.NoError(t, err)
			err = manager.RetireDeliverySubmission(ctx, submission.ID, 1)
			if condition == "idle" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			stored, err := store.GetSubmission(ctx, submission.ID)
			require.NoError(t, err)
			require.Equal(t, condition == "idle", stored.Retired)
			require.Equal(t, journal.SubmissionInterruptedUnknown, stored.State)
			if condition == "idle" {
				capability := manager.DeliveryCapability()
				require.False(t, capability.Unresolved)
				calls := 0
				_, err := manager.DispatchDeliverySubmission(ctx, submission.ID, func(context.Context) error { calls++; return nil })
				require.Error(t, err)
				require.Zero(t, calls, "acknowledgement must never replay the old instruction")
				fresh := submission
				fresh.ID, fresh.Hash, fresh.Payload, fresh.State = "new-instruction", "new-hash", []byte("distinct instruction"), journal.SubmissionPrepared
				_, err = manager.AdmitDeliverySubmission(ctx, fresh)
				require.NoError(t, err)
				_, err = manager.DispatchDeliverySubmission(ctx, fresh.ID, func(context.Context) error { calls++; return nil })
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}
