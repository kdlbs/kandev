package changes

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTurnChangeExportBatchTimeoutResume(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	client.exportErrors = []error{context.DeadlineExceeded}
	client.exportPageBuilder = func(request turnchanges.ExportRequest) *turnchanges.CheckpointExport {
		if request.Cursor == 0 {
			return exportPageFor(request, 1, 2, false)
		}
		return exportPageFor(request, 2, 2, true)
	}
	require.NoError(t, coordinator.Finish(context.Background(), exportTerminal(admission), client))

	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.True(t, changeSet.Complete)
	require.True(t, changeSet.ContentComplete)
	require.Len(t, store.files, 2, "acknowledged pages must not be stored twice")
	require.Equal(t, 3, client.exportCalls)
	row := store.repositoryRows[changeSet.ID][0]
	require.Equal(t, models.TurnChangeExportComplete, row.ExportState)
	require.EqualValues(t, 2, row.ExportCursor)
}

func TestTurnChangeExportRetryExhaustion(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	client.exportErrors = []error{context.DeadlineExceeded, context.DeadlineExceeded, context.DeadlineExceeded}
	require.NoError(t, coordinator.Finish(context.Background(), exportTerminal(admission), client))

	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.False(t, changeSet.ContentComplete)
	require.False(t, changeSet.Complete)
	require.Equal(t, 3, client.exportCalls)
	row := store.repositoryRows[changeSet.ID][0]
	require.Equal(t, models.TurnChangeExportUnavailable, row.ExportState)
	require.Equal(t, models.TurnChangeReasonExportTimeout, row.ExportReason)
}

func TestTurnChangeExportCleanupRejectsLatePage(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	store.forceExportClaimLost = true
	require.NoError(t, coordinator.Finish(context.Background(), exportTerminal(admission), client))

	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.Empty(t, store.files, "a late page must not be stored")
	require.False(t, changeSet.ContentComplete)
	row := store.repositoryRows[changeSet.ID][0]
	require.NotEqual(t, models.TurnChangeExportComplete, row.ExportState, "a stale worker must not settle the row")
}

func TestTurnChangeExportLegacyCapability(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	require.NoError(t, coordinator.Finish(context.Background(), exportTerminal(admission), client))
	require.Equal(t, 1, client.exportCalls, "legacy export keeps one bounded request")
	row := store.repositoryRows[store.changeSets[turnChangeSetID(admission.TurnID)].ID][0]
	require.Equal(t, models.TurnChangeExportComplete, row.ExportState)
}

func newExportFixture(t *testing.T) (*coordinatorStore, *coordinatorCheckpointClient, *Coordinator, Admission) {
	t.Helper()
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	return store, client, coordinator, admission
}

func exportTerminal(admission Admission) Terminal {
	return Terminal{Admission: admission, At: time.Now().UTC(), Outcome: "end_turn"}
}

func exportPageFor(
	request turnchanges.ExportRequest,
	next, total int64,
	complete bool,
) *turnchanges.CheckpointExport {
	one, zero := int64(1), int64(0)
	return &turnchanges.CheckpointExport{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID,
		StartTreeOID: request.StartTreeOID, EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID,
		Files: []turnchanges.CheckpointExportFile{{
			File: turnchanges.CheckpointFile{
				PathBytes: []byte("file-" + string(rune('a'+request.Cursor)) + ".txt"), Kind: "modified",
				Added: &one, Deleted: &zero,
			},
			ContentAvailability: turnchanges.Ready,
		}},
		ExportBytes: 4, ExportBatchVersion: turnchanges.ExportBatchVersion1,
		NextCursor: next, TotalFiles: total, Complete: complete,
	}
}

func TestTurnChangeExportCompletesAcrossManyBoundedPages(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	client.exportPageBuilder = func(request turnchanges.ExportRequest) *turnchanges.CheckpointExport {
		next := request.Cursor + 1
		return exportPageFor(request, next, 5, next >= 5)
	}
	require.NoError(t, coordinator.Finish(context.Background(), exportTerminal(admission), client))

	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	row := store.repositoryRows[changeSet.ID][0]
	require.True(t, changeSet.Complete)
	require.Equal(t, 5, client.exportCalls)
	require.Len(t, store.files, 5)
	require.Equal(t, models.TurnChangeExportComplete, row.ExportState)
}

func TestCoordinatorHistoryComparisonAcceptedOnce(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	require.NoError(t, coordinator.Finish(context.Background(), exportTerminal(admission), client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	legacyRow := store.repositoryRows[changeSet.ID][0]
	require.Zero(t, client.comparisonRequests[0].ComparisonVersion)
	require.Equal(t, turnchanges.ComparisonLegacyRaw, legacyRow.ComparisonBasis)

	store = newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	versionedClient := &coordinatorCheckpointClient{provenance: true}
	versioned := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	require.NoError(t, versioned.Admit(context.Background(), admission, versionedClient))
	require.NoError(t, versioned.Finish(context.Background(), exportTerminal(admission), versionedClient))
	changeSet = store.changeSets[turnChangeSetID(admission.TurnID)]
	row := store.repositoryRows[changeSet.ID][0]
	require.NotEmpty(t, row.StartProvenanceJSON)
	require.NotEmpty(t, row.EndProvenanceJSON)
	require.Equal(t, turnchanges.ComparisonVersion2, versionedClient.comparisonRequests[0].ComparisonVersion)
	require.Equal(t, turnchanges.ComparisonDirect, row.ComparisonBasis)
	require.EqualValues(t, 2, row.ComparisonVersion)
}

func TestTurnChangeExportPageTimeoutClassification(t *testing.T) {
	base := turnchanges.CheckpointExport{
		ExportBatchVersion: turnchanges.ExportBatchVersion1, TotalFiles: 2,
		Files: []turnchanges.CheckpointExportFile{{File: turnchanges.CheckpointFile{PathBytes: []byte("a.txt")}}},
	}
	timeout := base
	timeout.Reason = turnchanges.ReasonExportTimeout
	timeout.NextCursor = 1
	page := turnChangeExportPageFor(&timeout, ExportReceipt{Pending: true}, models.TurnChangeExportClaim{Cursor: 0})
	require.True(t, page.Retry)
	require.False(t, page.Failed)
	require.EqualValues(t, 1, page.NextCursor)

	zero := timeout
	zero.NextCursor = 0
	zeroPage := turnChangeExportPageFor(&zero, ExportReceipt{Pending: true}, models.TurnChangeExportClaim{Cursor: 0})
	require.True(t, zeroPage.Retry)
	require.False(t, zeroPage.Failed)

	terminal := base
	terminal.Reason = turnchanges.ReasonSizeLimit
	terminal.NextCursor = 1
	terminalPage := turnChangeExportPageFor(&terminal, ExportReceipt{Reason: models.TurnChangeReasonSizeLimit}, models.TurnChangeExportClaim{Cursor: 0})
	require.True(t, terminalPage.Failed)
}

func TestSkippedExportAttemptSettlesExpiredExhaustedClaim(t *testing.T) {
	store, _, coordinator, admission := newExportFixture(t)
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	row := store.repositoryRows[changeSet.ID][0]
	row.ExportState = models.TurnChangeExportRunning
	row.ExportAttempts = 3
	row.ExportClaimToken = 7
	expired := coordinator.now().UTC().Add(-time.Minute)
	row.ExportClaimExpiresAt = &expired

	resolved, err := coordinator.resolveSkippedExportAttempt(context.Background(), changeSet.ID, *row)
	require.NoError(t, err)
	require.Equal(t, models.TurnChangeExportUnavailable, resolved.ExportState)
	require.Equal(t, models.TurnChangeReasonExportTimeout, resolved.ExportReason)

	row.ExportState = models.TurnChangeExportRunning
	row.ExportClaimExpiresAt = timePtr(coordinator.now().UTC().Add(time.Minute))
	resolved, err = coordinator.resolveSkippedExportAttempt(context.Background(), changeSet.ID, *row)
	require.NoError(t, err)
	require.Equal(t, models.TurnChangeExportRunning, resolved.ExportState,
		"an unexpired live claim must stay deferred")
}
