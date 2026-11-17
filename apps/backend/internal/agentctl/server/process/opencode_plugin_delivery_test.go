package process

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/opencodeplugin"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func pluginClaim() opencodeplugin.Claim {
	return opencodeplugin.Claim{
		ShellID:          "shell-1",
		NativeSessionID:  "session",
		NativeGeneration: 1,
		WorkID:           "shell-1",
		RunID:            "shell-1",
		SourceCallID:     "call-1",
		Title:            "run tests",
		PromptGeneration: 1,
		SubmissionID:     "submission-1",
	}
}

func pluginObservation() streams.WorkloadRunObservation {
	exit := 3
	return streams.WorkloadRunObservation{
		WorkID:       "shell-1",
		RunID:        "shell-1",
		Kind:         streams.WorkloadKindShell,
		Title:        "run tests",
		State:        streams.RunStateFailed,
		SourceCallID: "call-1",
		ExitCode:     &exit,
		Capabilities: streams.InspectionOnlyWorkloadCapabilities(),
	}
}

func commitAdmission(t *testing.T, manager *Manager, admission opencodeplugin.Admission) error {
	t.Helper()
	publish, err := manager.CommitOpenCodePluginObservation(context.Background(), admission)
	if err == nil && publish != nil {
		publish()
	}
	return err
}

func pluginAdmission(eventID string, claim opencodeplugin.Claim, observation streams.WorkloadRunObservation) opencodeplugin.Admission {
	return opencodeplugin.Admission{
		EventID:          eventID,
		BehaviorID:       opencodeplugin.BehaviorShellOutcomes,
		NativeSessionID:  claim.NativeSessionID,
		NativeGeneration: claim.NativeGeneration,
		Observation:      observation,
		SubmissionID:     claim.SubmissionID,
		PromptGeneration: claim.PromptGeneration,
		ClaimKey:         claim.ShellID,
	}
}

func TestOpenCodePluginDurableAckCommitsOneJournalEvent(t *testing.T) {
	manager := newIncrementalShellManager(t)
	ctx := context.Background()
	claim := pluginClaim()
	require.NoError(t, commitAdmission(t, manager, pluginAdmission("plugin:event-1", claim, pluginObservation())))
	require.NoError(t, commitAdmission(t, manager, pluginAdmission("plugin:event-1", claim, pluginObservation())))
	events, _, err := manager.deliveryJournal.Replay(ctx, "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, streams.EventTypeBackgroundWorkUpdated, events[0].Type)
	require.Equal(t, "submission-1", events[0].SubmissionID)
	var payload adapter.AgentEvent
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	require.NotNil(t, payload.BackgroundWork)
	require.Equal(t, streams.RunStateFailed, payload.BackgroundWork.State)
	require.Equal(t, 3, *payload.BackgroundWork.ExitCode)
	require.Equal(t, "call-1", payload.BackgroundWork.SourceCallID)
	require.Equal(t, "run tests", payload.BackgroundWork.Title)
	require.Equal(t, "plugin:event-1", payload.DeliveryEventID)
}

func TestOpenCodePluginConflictingDuplicateFailsClosed(t *testing.T) {
	manager := newIncrementalShellManager(t)
	claim := pluginClaim()
	require.NoError(t, commitAdmission(t, manager, pluginAdmission("plugin:event-1", claim, pluginObservation())))
	conflicting := pluginObservation()
	conflicting.State = streams.RunStateCompleted
	conflicting.ExitCode = nil
	err := commitAdmission(t, manager, pluginAdmission("plugin:event-1", claim, conflicting))
	require.Error(t, err)
	require.True(t, errors.Is(err, journal.ErrEventIdentityConflict), "error = %v", err)
}

func TestOpenCodePluginOutcomeKeepsOrigin(t *testing.T) {
	manager := newIncrementalShellManager(t)
	manager.TrackDeliverySubmission("submission-origin", 7)
	manager.TrackDeliverySubmission("submission-successor", 8)
	claim := pluginClaim()
	claim.PromptGeneration = 7
	claim.SubmissionID = ""
	ctx := context.Background()
	require.NoError(t, commitAdmission(t, manager, pluginAdmission("plugin:event-origin", claim, pluginObservation())))
	events, _, err := manager.deliveryJournal.Replay(ctx, "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "submission-origin", events[0].SubmissionID, "a late outcome must keep the launch attribution")
}

func TestOpenCodePluginOutcomeDuringSuccessorPrompt(t *testing.T) {
	manager := newIncrementalShellManager(t)
	manager.TrackDeliverySubmission("submission-origin", 7)
	manager.TrackDeliverySubmission("submission-successor", 8)
	claim := pluginClaim()
	claim.PromptGeneration = 7
	claim.SubmissionID = "submission-origin"
	ctx := context.Background()
	require.NoError(t, commitAdmission(t, manager, pluginAdmission("plugin:event-successor", claim, pluginObservation())))
	events, _, err := manager.deliveryJournal.Replay(ctx, "stream", 0, 10)
	require.NoError(t, err)
	require.Equal(t, "submission-origin", events[0].SubmissionID)
	for _, event := range events {
		require.NotEqual(t, adapter.EventTypeComplete, event.Type, "a job outcome must not complete the successor prompt")
	}
}

type blockingAdmissionValidator struct {
	mu      sync.Mutex
	valid   bool
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (v *blockingAdmissionValidator) ValidateOpenCodePluginAdmission(opencodeplugin.Admission) bool {
	v.once.Do(func() {
		if v.started != nil {
			close(v.started)
		}
	})
	if v.release != nil {
		<-v.release
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.valid
}

func (v *blockingAdmissionValidator) setValid(valid bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.valid = valid
}

func TestOpenCodePluginStaleAdmissionAfterRevocationHasNoEffects(t *testing.T) {
	manager := newIncrementalShellManager(t)
	validator := &blockingAdmissionValidator{valid: true, started: make(chan struct{}), release: make(chan struct{})}
	manager.openCodePluginValidator = validator
	claim := pluginClaim()
	admission := pluginAdmission("plugin:event-stale", claim, pluginObservation())

	done := make(chan error, 1)
	go func() {
		_, err := manager.CommitOpenCodePluginObservation(context.Background(), admission)
		done <- err
	}()
	<-validator.started
	validator.setValid(false)
	close(validator.release)
	require.ErrorIs(t, <-done, errOpenCodePluginStaleAdmission)

	events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	if !errors.Is(err, journal.ErrStreamNotFound) {
		require.NoError(t, err)
	}
	require.Empty(t, events, "a revoked native generation must not commit")
}

func TestOpenCodePluginCommitBeforeRevocationIsAccepted(t *testing.T) {
	manager := newIncrementalShellManager(t)
	validator := &blockingAdmissionValidator{valid: true, started: make(chan struct{}), release: make(chan struct{})}
	manager.openCodePluginValidator = validator
	claim := pluginClaim()
	done := make(chan error, 1)
	go func() {
		_, err := manager.CommitOpenCodePluginObservation(context.Background(), pluginAdmission("plugin:event-valid", claim, pluginObservation()))
		done <- err
	}()
	<-validator.started
	close(validator.release)
	require.NoError(t, <-done)
	events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestOpenCodePluginCommitRespectsCancellation(t *testing.T) {
	manager := newIncrementalShellManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := manager.CommitOpenCodePluginObservation(ctx, pluginAdmission("plugin:event-cancelled", pluginClaim(), pluginObservation()))
	require.Error(t, err)
	events, _, replayErr := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	if !errors.Is(replayErr, journal.ErrStreamNotFound) {
		require.NoError(t, replayErr)
	}
	require.Empty(t, events)
}

type recordingClaimRetirer struct {
	mu         sync.Mutex
	admissions []opencodeplugin.Admission
}

func (r *recordingClaimRetirer) RetireOpenCodePluginClaim(admission opencodeplugin.Admission) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.admissions = append(r.admissions, admission)
}

func TestOpenCodePluginRetiresAcceptedClaim(t *testing.T) {
	manager := newIncrementalShellManager(t)
	retirer := &recordingClaimRetirer{}
	manager.openCodePluginRetirer = retirer
	claim := pluginClaim()
	require.NoError(t, commitAdmission(t, manager, pluginAdmission("plugin:event-retire", claim, pluginObservation())))
	retirer.mu.Lock()
	defer retirer.mu.Unlock()
	require.Len(t, retirer.admissions, 1)
	require.Equal(t, "shell-1", retirer.admissions[0].ClaimKey)
}

func TestOpenCodePluginAdmissionCheckRevokesBeforeJournalWrite(t *testing.T) {
	manager := newIncrementalShellManager(t)
	admission := pluginAdmission("plugin:event-fenced", pluginClaim(), pluginObservation())
	admission.Check = func() bool { return false }
	_, err := manager.CommitOpenCodePluginObservation(context.Background(), admission)
	require.ErrorIs(t, err, opencodeplugin.ErrAdmissionRevoked)
	events, _, replayErr := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	if !errors.Is(replayErr, journal.ErrStreamNotFound) {
		require.NoError(t, replayErr)
	}
	require.Empty(t, events)
}
