package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSilentRestoreCommitClearsSanitizedMatchingDeliveryError(t *testing.T) {
	fixture := newSilentRestoreErrorIdentityFixture(t)
	submissionID := fixture.inputs.recovery.SubmissionID
	require.NoError(t, fixture.service.persistLastAgentError(fixture.ctx, watcher.AgentEventData{
		TaskID: fixture.session.TaskID, SessionID: fixture.session.ID,
		AgentExecutionID: fixture.inputs.recovery.AgentExecutionID,
		FailureCode:      "DURABLE_DELIVERY_UNCERTAIN", FailureDetails: submissionID,
		DeliverySubmissionID: submissionID, ErrorMessage: "delivery outcome is uncertain",
	}))

	stored, err := fixture.repo.GetTaskSession(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	lastError, ok := models.LoadLastAgentError(stored.Metadata)
	require.True(t, ok)
	require.Equal(t, submissionID, lastError.DeliverySubmissionID)
	require.NotEqual(t, submissionID, lastError.Details, "free-form details must remain sanitized")

	commitSilentRestoreErrorIdentityFixture(t, fixture)
	stored, err = fixture.repo.GetTaskSession(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	_, ok = models.LoadLastAgentError(stored.Metadata)
	require.False(t, ok, "matching structured delivery error should clear atomically with restore")
}

func TestSilentRestoreCommitPreservesNewerDeliveryError(t *testing.T) {
	fixture := newSilentRestoreErrorIdentityFixture(t)
	const newerSubmissionID = "newer-prompt-submission"
	require.NoError(t, fixture.service.persistLastAgentError(fixture.ctx, watcher.AgentEventData{
		TaskID: fixture.session.TaskID, SessionID: fixture.session.ID,
		AgentExecutionID: fixture.inputs.recovery.AgentExecutionID,
		FailureCode:      "DURABLE_DELIVERY_UNCERTAIN", FailureDetails: newerSubmissionID,
		DeliverySubmissionID: newerSubmissionID, ErrorMessage: "a later delivery outcome is uncertain",
	}))

	commitSilentRestoreErrorIdentityFixture(t, fixture)
	stored, err := fixture.repo.GetTaskSession(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	lastError, ok := models.LoadLastAgentError(stored.Metadata)
	require.True(t, ok, "restore must preserve a newer error")
	require.Equal(t, newerSubmissionID, lastError.DeliverySubmissionID)
	require.Equal(t, "DURABLE_DELIVERY_UNCERTAIN", lastError.Code)
}

func newSilentRestoreErrorIdentityFixture(t *testing.T) *silentRestoreCheckpointFixture {
	t.Helper()
	ctx := context.Background()
	service, repo, manager, session, store := newSilentRestoreEligibilityFixture(
		t, "provider-session", strings.Repeat("a", 40),
	)
	inputs, err := service.loadSilentRestoreInputs(ctx, models.SilentRestoreCandidate{
		TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: "ws1",
	})
	require.NoError(t, err)
	require.NotNil(t, inputs)
	attemptID := silentRestoreAttemptID(session.ID, inputs.recovery.IncarnationID, 1)
	attempt, err := service.ensureSilentRestoreAttempt(ctx, store, attemptID, inputs)
	require.NoError(t, err)
	require.NotNil(t, attempt)
	allocated := *attempt.Checkpoint
	allocated.CandidateExecutionID = "candidate-error-identity"
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	changed, err := store.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, *attempt.Checkpoint, allocated)
	require.NoError(t, err)
	require.True(t, changed)
	launching := allocated
	launching.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = store.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, allocated, launching)
	require.NoError(t, err)
	require.True(t, changed)
	return &silentRestoreCheckpointFixture{
		ctx: ctx, service: service, repo: repo, manager: manager, store: store,
		inputs: inputs, attemptID: attemptID, session: session, checkpoint: launching,
	}
}

func commitSilentRestoreErrorIdentityFixture(t *testing.T, fixture *silentRestoreCheckpointFixture) {
	t.Helper()
	require.NoError(t, fixture.repo.UpsertExecutorRunning(fixture.ctx, &models.ExecutorRunning{
		ID: fixture.session.ID, SessionID: fixture.session.ID, TaskID: fixture.session.TaskID,
		ExecutorID: "executor-silent-restore", AgentExecutionID: fixture.checkpoint.CandidateExecutionID,
		Status: "running",
	}))
	generation := fixture.checkpoint.SourceGeneration
	generation.PredecessorGeneration = generation.Generation
	generation.Generation++
	generation.CreationReason = silentRestartRestoredCreationReason
	now := time.Now().UTC()
	changed, err := fixture.store.CommitSilentRestore(fixture.ctx, &models.SilentRestoreCommit{
		AttemptID: fixture.attemptID, Checkpoint: fixture.checkpoint,
		Generation: generation, CompletedAt: now,
	})
	require.NoError(t, err)
	require.True(t, changed)
}
