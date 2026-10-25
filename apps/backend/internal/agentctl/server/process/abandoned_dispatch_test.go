package process

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
func TestNewManagerReopenClassifiesAbandonedDispatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	store, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	original := journal.Submission{ID: "abandoned", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "stream", Hash: "immutable-hash", Payload: []byte("interrupted prompt"), State: journal.SubmissionDispatching}
	original, err = store.PutSubmission(ctx, original)
	require.NoError(t, err)
	require.NoError(t, store.Close())
	for range 2 {
		manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), DurableJournalPath: path,
			SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryStreamID: "stream", DeliveryHarnessGeneration: 1}, newTestLogger(t))
		t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(ctx)) })
		retained, err := manager.DeliveryJournal()
		require.NoError(t, err)
		got, err := retained.GetSubmission(ctx, original.ID)
		require.NoError(t, err)
		require.Equal(t, journal.SubmissionInterruptedUnknown, got.State)
		require.False(t, got.Retired)
		require.Equal(t, original.Payload, got.Payload)
		require.Equal(t, original.Hash, got.Hash)
		require.Equal(t, original.CreatedAt, got.CreatedAt)
		calls := 0
		_, err = (&SubmissionDelivery{Journal: retained}).Dispatch(ctx, got.ID, func(context.Context) error { calls++; return nil })
		require.Error(t, err)
		require.Zero(t, calls, "an abandoned instruction must never be resent")
		require.NoError(t, manager.StopForTeardown(ctx))
	}
}

func TestNewManagerRejectsConflictingAbandonedOwnersAtomically(t *testing.T) {
	for _, field := range []string{"session", "incarnation", "generation", "stream"} {
		t.Run(field, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "delivery.bbolt")
			store, err := journal.Open(journal.Config{Path: path})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			valid := journal.Submission{ID: "a-valid", SessionID: "session", IncarnationID: "incarnation",
				HarnessGeneration: 1, StreamID: "stream", Hash: "hash", State: journal.SubmissionDispatching}
			_, err = store.PutSubmission(ctx, valid)
			require.NoError(t, err)
			foreign := valid
			foreign.ID = "z-conflicting"
			switch field {
			case "session":
				foreign.SessionID = "foreign"
			case "incarnation":
				foreign.IncarnationID = "foreign"
			case "generation":
				foreign.HarnessGeneration = 2
			case "stream":
				foreign.StreamID = "foreign"
			}
			_, err = store.PutSubmission(ctx, foreign)
			require.NoError(t, err)
			require.NoError(t, store.Close())
			manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), DurableJournalPath: path,
				SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryStreamID: "stream", DeliveryHarnessGeneration: 1}, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(ctx)) })
			_, err = manager.DeliveryJournal()
			require.True(t, errors.Is(err, journal.ErrOwnerMismatch), "owner conflict must close durable admission: %v", err)
			got, err := manager.deliveryJournal.GetSubmission(ctx, valid.ID)
			require.NoError(t, err)
			require.Equal(t, journal.SubmissionDispatching, got.State, "a later conflict must roll back every earlier match")
		})
	}
}

func TestNewManagerRestorePreservesPriorGenerationUnknown(t *testing.T) {
	for _, state := range []journal.SubmissionState{journal.SubmissionInterruptedUnknown, journal.SubmissionDispatching} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "delivery.bbolt")
			store, err := journal.Open(journal.Config{Path: path})
			require.NoError(t, err)
			original, err := store.PutSubmission(ctx, journal.Submission{ID: "old", Hash: "hash", Payload: []byte("never replay"), SessionID: "session", IncarnationID: "incarnation", StreamID: "incarnation:g1", HarnessGeneration: 1, State: state})
			require.NoError(t, err)
			if state == journal.SubmissionDispatching {
				_, err = store.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1", SubmissionID: "old", Type: "message_chunk", Payload: []byte(`{"text":"retained"}`)})
				require.NoError(t, err)
			}
			require.NoError(t, store.Close())
			manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), DurableJournalPath: path, SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryStreamID: "incarnation:g2", DeliveryHarnessGeneration: 2}, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(ctx)) })
			retained, err := manager.DeliveryJournal()
			require.NoError(t, err, "restore-only generation must be able to read prior unknown history before retiring it")
			got, err := retained.GetSubmission(ctx, original.ID)
			require.NoError(t, err)
			require.Equal(t, journal.SubmissionInterruptedUnknown, got.State)
			require.Equal(t, original.Payload, got.Payload)
			require.Equal(t, original.Hash, got.Hash)
			require.Equal(t, original.CreatedAt, got.CreatedAt)
		})
	}
}
