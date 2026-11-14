package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeReconcilesJournalUnknownSQLDispatch(t *testing.T) {
	for _, scenario := range []string{"abandoned", "queue origin", "completed without terminal", "failed without terminal", "cancelled without terminal", "completed with terminal sequence", "completed with retained terminal", "hash conflict", "completed hash conflict", "peer still dispatching", "independent work", "unknown missing peer", "unknown peer hash conflict", "dispatching missing peer", "prepared missing peer", "accepted missing peer", "unknown newer generation missing peer"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
			store, err := journal.Open(journal.Config{Path: journalPath})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			peer := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "original-hash", Payload: []byte("original prompt"), State: journal.SubmissionInterruptedUnknown}
			if scenario == "queue origin" {
				peer.ID = "prompt:queue:interrupted"
			}
			if scenario == "peer still dispatching" {
				peer.State = journal.SubmissionDispatching
			}
			switch scenario {
			case "completed without terminal", "completed with terminal sequence", "completed with retained terminal", "completed hash conflict":
				peer.State = journal.SubmissionCompleted
			case "failed without terminal":
				peer.State = journal.SubmissionFailed
			case "cancelled without terminal":
				peer.State = journal.SubmissionCancelled
			}
			if scenario == "completed with terminal sequence" {
				peer.TerminalSequence = 1
			}
			if scenario == "completed with retained terminal" {
				peer.TerminalEventRetained = true
			}
			canRetire := scenario == "abandoned" || scenario == "queue origin" || scenario == "completed without terminal" || scenario == "failed without terminal" || scenario == "cancelled without terminal" || scenario == "unknown missing peer"
			missingPeer := scenario == "unknown missing peer" || scenario == "dispatching missing peer" || scenario == "prepared missing peer" || scenario == "accepted missing peer" || scenario == "unknown newer generation missing peer"
			if !missingPeer {
				_, err = store.PutSubmission(ctx, peer)
				require.NoError(t, err)
			}
			repo, _ := runOwnerTestRepository(t)
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Native recovery"}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
			hash := peer.Hash
			if scenario == "hash conflict" || scenario == "completed hash conflict" || scenario == "unknown peer hash conflict" {
				hash = "different-hash"
			}
			backendState := models.DeliverySubmissionDispatching
			generation, ownerGeneration := int64(2), int64(2)
			switch scenario {
			case "unknown missing peer", "unknown peer hash conflict":
				backendState = models.DeliverySubmissionInterruptedUnknown
			case "unknown newer generation missing peer":
				backendState = models.DeliverySubmissionInterruptedUnknown
				generation, ownerGeneration = 3, 3
			case "prepared missing peer":
				backendState = models.DeliverySubmissionPrepared
			case "accepted missing peer":
				backendState = models.DeliverySubmissionAccepted
			}
			_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: generation, OwnerGeneration: ownerGeneration, PayloadHash: hash, Payload: peer.Payload, State: backendState})
			require.NoError(t, err)
			if scenario == "independent work" {
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "independent", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "independent", Payload: []byte("independent"), State: models.DeliverySubmissionAccepted})
				require.NoError(t, err)
			}
			manager := newTestManager(t)
			manager.SetAgentDeliveryRepository(repo)
			execution := &AgentExecution{ID: "replacement", SessionID: "session", ACPSessionID: "original-native", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream", agentctl: nativeResumeJournalPeer(t, store, false)}
			execution.setSessionInitialized(true)
			require.NoError(t, manager.executionStore.Add(execution))
			err = manager.AcknowledgeNativeResumeDelivery(ctx, "session")
			if canRetire {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			if scenario == "unknown peer hash conflict" {
				var reasoner interface{ DeliveryRecoveryRefusalReason() string }
				require.ErrorAs(t, err, &reasoner)
				require.Equal(t, DeliveryRecoveryRefusalPeerRecordMismatch, reasoner.DeliveryRecoveryRefusalReason())
			}
			if scenario == "unknown newer generation missing peer" {
				var reasoner interface{ DeliveryRecoveryRefusalReason() string }
				require.ErrorAs(t, err, &reasoner)
				require.Equal(t, DeliveryRecoveryRefusalGenerationSuperseded, reasoner.DeliveryRecoveryRefusalReason())
			}
			if scenario == "dispatching missing peer" || scenario == "prepared missing peer" || scenario == "accepted missing peer" {
				var reasoner interface{ DeliveryRecoveryRefusalReason() string }
				require.ErrorAs(t, err, &reasoner)
				require.Equal(t, DeliveryRecoveryRefusalPeerRecordMissing, reasoner.DeliveryRecoveryRefusalReason())
			}
			submission, err := repo.GetAgentDeliverySubmission(ctx, peer.ID)
			require.NoError(t, err)
			expected := backendState
			if canRetire {
				expected = models.DeliverySubmissionInterruptedUnknown
				if backendState == models.DeliverySubmissionDispatching {
					require.Equal(t, "unknown", submission.Outcome)
				}
			}
			require.Equal(t, expected, submission.State)
			require.Zero(t, submission.TerminalSequence)
			require.Equal(t, peer.Payload, submission.Payload)
			stored, err := store.GetSubmission(ctx, peer.ID)
			if missingPeer && !canRetire {
				require.ErrorIs(t, err, journal.ErrSubmissionNotFound)
			} else {
				require.NoError(t, err)
				require.Equal(t, canRetire, stored.Retired)
			}
			if scenario == "unknown missing peer" {
				require.Equal(t, journal.SubmissionInterruptedUnknown, stored.State)
				require.Equal(t, peer.Hash, stored.Hash)
				require.Empty(t, stored.Payload, "the synthetic retirement record must not copy prompt content")
				require.NoError(t, store.Close())
				store, err = journal.Open(journal.Config{Path: journalPath})
				require.NoError(t, err)
				execution.agentctl = nativeResumeJournalPeer(t, store, false)
				reopened, err := store.GetSubmission(ctx, peer.ID)
				require.NoError(t, err)
				require.True(t, reopened.Retired)
				require.Equal(t, journal.SubmissionInterruptedUnknown, reopened.State)
				status := &agentctl.DeliveryStatus{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream"}
				active, err := acknowledgedNativeResumeHistory(ctx, execution, status, execution.agentctl, []*models.AgentDeliverySubmission{submission}, repo)
				require.NoError(t, err)
				require.Empty(t, active, "retired unknown history must not remain active work after journal reopen")
				_, err = store.PutSubmission(ctx, journal.Submission{
					ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID,
					HarnessGeneration: peer.HarnessGeneration, StreamID: peer.StreamID,
					Hash: peer.Hash, State: journal.SubmissionPrepared,
				})
				require.ErrorIs(t, err, journal.ErrSubmissionState, "the retired ID must not be replayable")
			} else if !missingPeer {
				require.Equal(t, peer.State, stored.State)
			}
			if !missingPeer {
				require.Equal(t, peer.TerminalSequence, stored.TerminalSequence)
				require.Equal(t, peer.TerminalEventRetained, stored.TerminalEventRetained)
			}
			require.Equal(t, "original-native", execution.ACPSessionID)
		})
	}
}

func TestNativeResumeMissingPeerRetirementUsesClosedRefusalReasons(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "owner mismatch", err: journal.ErrOwnerMismatch, want: DeliveryRecoveryRefusalNativeIdentityMismatch},
		{name: "busy dispatcher", err: journal.ErrSubmissionState, want: DeliveryRecoveryRefusalLiveWork},
		{name: "hash conflict", err: journal.ErrSubmissionConflict, want: DeliveryRecoveryRefusalPeerRecordMismatch},
		{name: "http owner mismatch", err: &agentctl.DeliveryHTTPError{Body: `{"code":"OWNER_MISMATCH"}`}, want: DeliveryRecoveryRefusalNativeIdentityMismatch},
		{name: "http live work", err: &agentctl.DeliveryHTTPError{Body: `{"code":"SUBMISSION_STATE"}`}, want: DeliveryRecoveryRefusalLiveWork},
		{name: "http conflict", err: &agentctl.DeliveryHTTPError{Body: `{"code":"SUBMISSION_CONFLICT"}`}, want: DeliveryRecoveryRefusalPeerRecordMismatch},
		{name: "unavailable", err: errors.New("peer unavailable"), want: DeliveryRecoveryRefusalPeerRecordMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, nativeResumeMissingPeerRefusalReason(tc.err))
		})
	}
}
