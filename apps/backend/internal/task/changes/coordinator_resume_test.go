package changes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type staticExportProvider struct {
	client            CheckpointClient
	ok                bool
	restoreInProgress bool
}

func (p staticExportProvider) AcquireTurnChangeExporter(context.Context, string) (CheckpointClient, func(), bool) {
	return p.client, func() {}, p.ok
}

func (p staticExportProvider) TurnChangeRestoreInProgress(string) bool {
	return p.restoreInProgress
}

type countingSummaryPublisher struct{ calls int }

func (p *countingSummaryPublisher) PublishTurnChangeSummary(context.Context, string, string, string) error {
	p.calls++
	return nil
}

func TestResumeUnfinishedReusesAcceptedComparisonWithoutCapture(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	one := int64(1)
	client := &coordinatorCheckpointClient{provenance: true, comparisonFiles: []turnchanges.CheckpointFile{{
		PathBytes: []byte("resumed.txt"), Kind: "modified", Added: &one, Deleted: &one,
	}}}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	publisher := &countingSummaryPublisher{}
	coordinator.SetSummaryPublisher(publisher)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), exportTerminal(admission), client))
	require.NoError(t, coordinator.ProcessTerminalComparison(context.Background(), exportTerminal(admission), client))

	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	resetTerminalForResume(changeSet)
	require.NoError(t, coordinator.ResumeUnfinished(context.Background(), staticExportProvider{client: client, ok: true}))

	changeSet = store.changeSets[changeSet.ID]
	require.NotNil(t, changeSet.TerminalAt)
	require.Equal(t, 1, client.endCalls, "resume must not capture another endpoint")
	require.Equal(t, 1, client.compareCalls, "accepted comparison must be reused")
	require.GreaterOrEqual(t, publisher.calls, 1)
}

func TestResumeUnfinishedAcceptedEndRecomparesWithoutCapture(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	client := &coordinatorCheckpointClient{provenance: true}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), exportTerminal(admission), client))

	require.NoError(t, coordinator.ResumeUnfinished(context.Background(), staticExportProvider{client: client, ok: true}))
	require.Equal(t, 1, client.endCalls)
	require.Equal(t, 1, client.compareCalls)
	require.NotNil(t, store.changeSets[turnChangeSetID(admission.TurnID)].TerminalAt)
}

func TestResumeUnfinishedMixedRepositorySettlesOnlyMissingEnd(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	one := int64(1)
	client := &coordinatorCheckpointClient{provenance: true, comparisonFiles: []turnchanges.CheckpointFile{{
		PathBytes: []byte("resumed.txt"), Kind: "modified", Added: &one, Deleted: &one,
	}}}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	admission.Checkouts[0].RepositorySubpathKnown = true
	admission.Checkouts[0].RepositorySubpath = ""
	secondCheckout := admission.Checkouts[0]
	secondCheckout.ID = "checkout-2"
	secondCheckout.RepositorySubpath = "second"
	secondCheckout.RepositorySubpathKnown = true
	admission.Checkouts = append(admission.Checkouts, secondCheckout)
	client.scopes = []string{"", "second"}
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), exportTerminal(admission), client))

	store.repositoryRows[turnChangeSetID(admission.TurnID)][1].EndCommitOID = ""
	store.repositoryRows[turnChangeSetID(admission.TurnID)][1].EndTreeOID = ""
	require.NoError(t, coordinator.ResumeUnfinished(context.Background(), staticExportProvider{client: client, ok: true}))
	rows := store.repositoryRows[turnChangeSetID(admission.TurnID)]
	require.Equal(t, models.TurnChangeAvailabilityReady, rows[0].Availability)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, rows[1].Availability)
}

func resetTerminalForResume(changeSet *models.TurnChangeSet) {
	changeSet.TerminalAt = nil
	changeSet.Availability = models.TurnChangeAvailabilityPending
	changeSet.Complete, changeSet.SummaryComplete, changeSet.ContentComplete = false, false, false
}

func TestSuccessorAdmissionRetainsRefsForPendingPreservation(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	turns := coordinatorTurnReaderMap{
		"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1"},
		"turn-2": {ID: "turn-2", TaskID: "task-1", TaskSessionID: "session-1"},
	}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	first := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), first, client))
	terminal := exportTerminal(first)
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), terminal, client))
	require.NoError(t, coordinator.ProcessTerminalComparison(context.Background(), terminal, client))
	changeSetID := turnChangeSetID(first.TurnID)
	require.Equal(t, models.TurnChangeExportQueued, store.repositoryRows[changeSetID][0].ExportState)

	successor := first
	successor.TurnID = "turn-2"
	successor.ExecutionID = "execution-2"
	successor.StartupAttemptID = "startup-2"
	successor.PromptGeneration++
	require.NoError(t, coordinator.Admit(context.Background(), successor, client))
	require.Empty(t, client.deleteRequests, "successor admission must retain pending preservation refs")
	require.True(t, store.repositoryRows[changeSetID][0].CleanupPending)

	require.NoError(t, coordinator.ProcessTerminalContent(context.Background(), terminal, client))
	require.NotEmpty(t, client.deleteRequests, "settled content must release refs")
}

func TestWriterOverlapReconciliationKeepsEveryCheckoutRoot(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	turns := coordinatorTurnReaderMap{
		"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1"},
		"turn-2": {ID: "turn-2", TaskID: "task-1", TaskSessionID: "session-1"},
		"turn-3": {ID: "turn-3", TaskID: "task-1", TaskSessionID: "session-1"},
	}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	publisher := &countingSummaryPublisher{}
	coordinator.SetSummaryPublisher(publisher)

	first := coordinatorAdmissionWithCheckouts("turn-1", "checkout-a", "checkout-b")
	require.NoError(t, coordinator.Admit(context.Background(), first, &coordinatorCheckpointClient{scopes: []string{"a", "b"}}))
	second := coordinatorAdmissionWithCheckouts("turn-2", "checkout-a")
	require.NoError(t, coordinator.Admit(context.Background(), second, &coordinatorCheckpointClient{scopes: []string{""}}))
	third := coordinatorAdmissionWithCheckouts("turn-3", "checkout-b")
	require.NoError(t, coordinator.Admit(context.Background(), third, &coordinatorCheckpointClient{scopes: []string{""}}))

	firstSet := store.changeSets[turnChangeSetID("turn-1")]
	require.Len(t, firstSet.OverlapIntervals, 2, "both roots keep their own sibling overlap")
	secondSet := store.changeSets[turnChangeSetID("turn-2")]
	thirdSet := store.changeSets[turnChangeSetID("turn-3")]
	require.Len(t, secondSet.OverlapIntervals, 1)
	require.Len(t, thirdSet.OverlapIntervals, 1)
	require.GreaterOrEqual(t, publisher.calls, 2, "peer overlap corrections are published")
}

func coordinatorAdmissionWithCheckouts(turnID string, checkoutIDs ...string) Admission {
	admission := coordinatorAdmission()
	admission.TurnID = turnID
	admission.ExecutionID = "execution-" + turnID
	admission.StartupAttemptID = "startup-" + turnID
	admission.Checkouts = nil
	for _, checkoutID := range checkoutIDs {
		checkout := Checkout{
			ID: checkoutID, RepositoryID: "repository-" + checkoutID,
			RepositorySubpath: strings.TrimPrefix(checkoutID, "checkout-"), RepositorySubpathKnown: true,
		}
		admission.Checkouts = append(admission.Checkouts, checkout)
	}
	return admission
}

func TestStopPreservationIsBoundedToTwoSeconds(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	client.waitExport = true
	terminal := exportTerminal(admission)
	terminal.Outcome = "stopped"
	started := time.Now()
	require.NoError(t, coordinator.Finish(context.Background(), terminal, client))
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("stop preservation exceeded its bound: %s", elapsed)
	}
	row := store.repositoryRows[store.changeSets[turnChangeSetID(admission.TurnID)].ID][0]
	require.NotEqual(t, models.TurnChangeExportComplete, row.ExportState)
}

func TestStopPreservationBoundsSlowPersistenceAndLeaseRelease(t *testing.T) {
	store, client, coordinator, admission := newExportFixture(t)
	store.exportStoreDelay = 10 * time.Second
	store.leaseReleaseDelay = 10 * time.Second
	terminal := exportTerminal(admission)
	terminal.Outcome = "stopped"
	started := time.Now()
	require.NoError(t, coordinator.Finish(context.Background(), terminal, client))
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("slow persistence exceeded the stop deadline: %s", elapsed)
	}
	if store.leaseReleaseDeadline.IsZero() {
		t.Fatal("stop preservation did not attempt a bounded lease release")
	}
	if store.leaseReleaseDeadline.After(started.Add(2100 * time.Millisecond)) {
		t.Fatalf("lease release received a fresh deadline after the stop budget: %s", store.leaseReleaseDeadline.Sub(started))
	}
}

func TestResumeWithoutProviderPreservesDurableComparison(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	one := int64(1)
	client := &coordinatorCheckpointClient{provenance: true, comparisonFiles: []turnchanges.CheckpointFile{{
		PathBytes: []byte("resumed.txt"), Kind: "modified", Added: &one, Deleted: &one,
	}}}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	publisher := &countingSummaryPublisher{}
	coordinator.SetSummaryPublisher(publisher)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), exportTerminal(admission), client))
	require.NoError(t, coordinator.ProcessTerminalComparison(context.Background(), exportTerminal(admission), client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	resetTerminalForResume(changeSet)

	require.NoError(t, coordinator.ResumeUnfinished(context.Background(), staticExportProvider{ok: false}))
	changeSet = store.changeSets[changeSet.ID]
	require.NotNil(t, changeSet.TerminalAt)
	require.Equal(t, int64(1), changeSet.FileCount)
	require.Equal(t, 1, client.endCalls)
	row := store.repositoryRows[changeSet.ID][0]
	require.Equal(t, models.TurnChangeAvailabilityReady, row.Availability)
	require.Equal(t, models.TurnChangeExportUnavailable, row.ExportState)
	require.Equal(t, models.TurnChangeReasonExportMissingObject, row.Reason)
	require.GreaterOrEqual(t, publisher.calls, 1)
}

func TestResumeWithoutProviderSettlesAcceptedEndOnly(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), exportTerminal(admission), client))

	require.NoError(t, coordinator.ResumeUnfinished(context.Background(), staticExportProvider{ok: false}))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.NotNil(t, changeSet.TerminalAt)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, changeSet.Availability)
	require.Equal(t, models.TurnChangeReasonExportMissingObject, changeSet.Reason)
	require.Zero(t, client.compareCalls, "missing source must not invent a comparison")
	require.Equal(t, 1, client.endCalls)
}

func TestResumeWithoutProviderMixedRepositoryKeepsAvailableCounts(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	one := int64(1)
	client := &coordinatorCheckpointClient{provenance: true, comparisonFiles: []turnchanges.CheckpointFile{{
		PathBytes: []byte("resumed.txt"), Kind: "modified", Added: &one, Deleted: &one,
	}}}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	admission.Checkouts[0].RepositorySubpathKnown = true
	admission.Checkouts[0].RepositorySubpath = ""
	second := admission.Checkouts[0]
	second.ID, second.RepositorySubpath = "checkout-2", "second"
	admission.Checkouts = append(admission.Checkouts, second)
	client.scopes = []string{"", "second"}
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), exportTerminal(admission), client))
	require.NoError(t, coordinator.ProcessTerminalComparison(context.Background(), exportTerminal(admission), client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	resetTerminalForResume(changeSet)
	store.repositoryRows[changeSet.ID][1].ComparisonBasis = ""

	require.NoError(t, coordinator.ResumeUnfinished(context.Background(), staticExportProvider{ok: false}))
	rows := store.repositoryRows[changeSet.ID]
	require.Equal(t, models.TurnChangeAvailabilityReady, rows[0].Availability)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, rows[1].Availability)
	require.Equal(t, int64(1), store.changeSets[changeSet.ID].FileCount)
}

func TestResumeWithoutProviderDefersConcreteRestoreInProgress(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	client := &coordinatorCheckpointClient{provenance: true}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	publisher := &countingSummaryPublisher{}
	coordinator.SetSummaryPublisher(publisher)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), exportTerminal(admission), client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	resetTerminalForResume(changeSet)

	require.NoError(t, coordinator.ResumeUnfinished(context.Background(), staticExportProvider{
		ok: false, restoreInProgress: true,
	}))
	changeSet = store.changeSets[changeSet.ID]
	require.Nil(t, changeSet.TerminalAt, "restore in progress must not settle the turn")
	require.Zero(t, publisher.calls)
}
