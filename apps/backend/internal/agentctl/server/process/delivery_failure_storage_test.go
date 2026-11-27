package process

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.7
func TestDeliveryStorageSecondaryErrorPreservesSnapshot(t *testing.T) {
	manager := newAllToolManager(t, 4096)
	manager.beginManagedStartupGeneration()
	manager.TrackDeliverySubmission("submission-1", 7)
	update := adapter.AgentEvent{Type: adapter.EventTypeError, PromptGeneration: 7}
	storageErr := journal.NewStorageError(
		journal.StorageReasonStreamLimitReached,
		journal.ErrJournalFull,
		journal.WithLogicalMeasurements(32768, 4096),
	)

	manager.recordDeliveryWriteFailure(storageErr, update, 4096)
	first := manager.DeliveryHealth().Failure
	require.NotNil(t, first)
	require.Equal(t, "submission-1", first.SubmissionID)

	manager.recordDeliveryWriteFailure(errors.New("secondary write failed"), update, 4096)
	got := manager.DeliveryHealth().Failure
	require.NotNil(t, got)
	require.Equal(t, "submission-1", got.SubmissionID)
	require.Equal(t, first.Revision, got.Revision)
	require.NotNil(t, got.Storage)
	require.Equal(t, streams.DeliveryStorageReasonStreamLimitReached, got.Storage.Reason)
	require.EqualValues(t, 32768, *got.Storage.RetainedBytes)
	require.EqualValues(t, 4096, *got.Storage.LimitBytes)

	manager.recordDeliveryWriteFailure(journal.ErrJournalClosed, update, 4096)
	closed := manager.DeliveryHealth().Failure
	require.NotNil(t, closed)
	require.Equal(t, first.Revision, closed.Revision)
	require.Equal(t, streams.DeliveryStorageReasonStreamLimitReached, closed.Storage.Reason)
	require.EqualValues(t, 32768, *closed.Storage.RetainedBytes)
	require.EqualValues(t, 4096, *closed.Storage.LimitBytes)

	manager.recordDeliveryWriteFailure(journal.ErrJournalCorrupt, update, 4096)
	corrupt := manager.DeliveryHealth().Failure
	require.NotNil(t, corrupt)
	require.Equal(t, agenttypes.DeliveryFailureStorageCorrupt, corrupt.Reason)
	require.Equal(t, first.Revision+1, corrupt.Revision)
	require.Nil(t, corrupt.Storage)

	manager.cfg.DeliveryIncarnationID = "replacement-incarnation"
	manager.recordDeliveryWriteFailure(errors.New("replacement writer failed"), update, 4096)
	replacement := manager.DeliveryHealth().Failure
	require.NotNil(t, replacement)
	require.Equal(t, "replacement-incarnation", replacement.IncarnationID)
	require.NotEqual(t, got.Revision, replacement.Revision)
	require.Nil(t, replacement.Storage)
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.7
func TestDeliveryStorageFailureLogIncludesCauseAndMeasurements(t *testing.T) {
	manager := newAllToolManager(t, 4096)
	observedLogger, logs := newObservedTestLogger(t)
	manager.logger = observedLogger
	err := journal.NewStorageError(
		journal.StorageReasonStreamLimitReached,
		journal.ErrJournalFull,
		journal.WithLogicalMeasurements(32768, 4096),
	)

	manager.recordDeliveryWriteFailure(err, adapter.AgentEvent{Type: adapter.EventTypeMessageChunk}, 4096)
	entries := logs.FilterMessage("durable delivery write failed; process liveness unchanged").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "stream_limit_reached", fields["reason"])
	require.Equal(t, int64(32768), fields["retained_bytes"])
	require.Equal(t, int64(4096), fields["limit_bytes"])
}

func TestDeliveryStorageCauseSurvivesDrainedTerminal(t *testing.T) {
	diagnostic := &streams.DeliveryStorageDiagnostic{
		Version: streams.DeliveryStorageDiagnosticVersion,
		Reason:  streams.DeliveryStorageReasonDiskSpaceLow,
	}
	manager := &Manager{deliveryHealth: DeliveryHealth{
		Failure: &agenttypes.DeliveryFailureSnapshot{
			SubmissionID: "submission-1",
			Storage:      diagnostic,
		},
	}}
	update := manager.attachDeliveryFailureStorage(adapter.AgentEvent{
		Type: streams.EventTypeComplete, DeliverySubmissionID: "submission-1",
	})
	if update.Storage == nil || update.Storage.Reason != streams.DeliveryStorageReasonDiskSpaceLow {
		t.Fatalf("terminal storage = %+v", update.Storage)
	}
	other := manager.attachDeliveryFailureStorage(adapter.AgentEvent{
		Type: streams.EventTypeComplete, DeliverySubmissionID: "submission-2",
	})
	if other.Storage != nil {
		t.Fatalf("unrelated terminal inherited storage: %+v", other.Storage)
	}
}
