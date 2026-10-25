package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type legacyNativeTestManager struct {
	*nativeProofTestManager
	inspect func(context.Context, agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error)
}

func (m *legacyNativeTestManager) InspectDeliveryRecordEvidence(ctx context.Context, request agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error) {
	return m.inspect(ctx, request)
}
func (m *legacyNativeTestManager) ReadDeliveryRecordSubmission(context.Context, agentruntime.DeliveryRecordSubmissionRequest) (*agentruntime.DeliveryRecordSubmission, error) {
	return nil, errors.New("old prompt must never be read for dispatch")
}

func TestLegacyNativeRestartWithoutRecoveryNotice(t *testing.T) {
	for _, condition := range []string{"dead owner", "backlog", "queue", "retired history", "generic history", "unprojected output", "live owner", "hash mismatch", "retained terminal", "changed owner", "independent work"} {
		t.Run(condition, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, row, launched, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
			repo := service.repo.(*sqliterepo.Repository)
			require.NoError(t, repo.UpdateTaskSessionState(ctx, row.SessionID, models.TaskSessionStateWaitingForInput, ""))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, row.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
			_, err := repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`, block.ID)
			require.NoError(t, err)
			if condition == "queue" {
				_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET id = 'queue:delivery' WHERE id = ?`, row.ID)
				require.NoError(t, err)
				row.ID = "queue:delivery"
				block.DeliverySubmissionID = row.ID
				_, err = repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_submission_id = ? WHERE id = ?`, row.ID, block.ID)
				require.NoError(t, err)
			}
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "journal.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			peer := journal.Submission{ID: row.ID, SessionID: row.SessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1, StreamID: block.DeliveryStreamID, Hash: row.PayloadHash, Payload: row.Payload, State: journal.SubmissionFailed}
			_, err = store.PutSubmission(ctx, peer)
			require.NoError(t, err)
			if condition == "unprojected output" {
				_, err = store.Append(ctx, journal.Event{SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: 1, StreamID: peer.StreamID, SubmissionID: peer.ID, Type: "tool_call", Payload: []byte("unprojected")})
				require.NoError(t, err)
			}
			if condition == "retired history" || condition == "generic history" {
				older := *row
				older.ID = "prior"
				older.PayloadHash = "prior-hash"
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &older)
				require.NoError(t, err)
				retired := peer
				retired.ID = older.ID
				retired.Hash = older.PayloadHash
				retired.Retired = true
				_, err = store.PutSubmission(ctx, retired)
				require.NoError(t, err)
				if condition == "generic history" {
					_, err = repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_submission_id = '', reason = 'unresolved_durable_work' WHERE id = ?`, block.ID)
					require.NoError(t, err)
				}
			}
			calls := 0
			identity := agentruntime.AgentDeliveryRecoveryIdentity{TaskID: "task1", SessionID: row.SessionID, ExecutionID: "old-execution", IncarnationID: row.IncarnationID, HarnessGeneration: 1, StreamID: peer.StreamID, SubmissionID: peer.ID, PromptGeneration: 1, OriginalRuntime: processidentity.Identity{PID: 42, GroupID: 42, SessionID: 42, BirthToken: "linux:boot:1"}}
			wrapper := &legacyNativeTestManager{nativeProofTestManager: &nativeProofTestManager{nativeResumeTestManager: manager}}
			wrapper.inspect = func(ctx context.Context, request agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error) {
				calls++
				d, err := store.RecoveryDescriptor(ctx, row.SessionID, row.IncarnationID, 1, peer.StreamID)
				require.NoError(t, err)
				if request.StoredSubmissionID == "" {
					return &agentruntime.DeliveryRecordEvidence{Descriptor: d}, nil
				}
				p, err := store.GetSubmission(ctx, request.StoredSubmissionID)
				require.NoError(t, err)
				dead := condition != "live owner"
				binding := identity
				switch condition {
				case "hash mismatch":
					p.Hash = "foreign"
				case "retained terminal":
					p.TerminalEventRetained = true
					p.TerminalSequence = 1
				case "changed owner":
					if calls > 1 {
						binding.ExecutionID = "foreign"
					}
				case "independent work":
					d.SubmissionCount = 1
				}
				return &agentruntime.DeliveryRecordEvidence{Descriptor: d, StoredSubmission: &p, ControlIdentity: &binding, ProcessTerminated: &dead}, nil
			}
			originalAck := manager.acknowledge
			manager.acknowledge = func(ctx context.Context, sessionID string) error {
				require.NoError(t, originalAck(ctx, sessionID))
				_, err := store.RetireSubmission(ctx, peer.ID, 1)
				return err
			}
			wrapper.proof = func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
				running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
				require.NoError(t, err)
				retired, err := store.GetSubmission(ctx, peer.ID)
				require.NoError(t, err)
				require.True(t, retired.Retired)
				proof := &models.NativeResumeDeliveryEvidence{SessionID: sessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1, NativeSessionID: "native-conversation", ExecutionID: running.AgentExecutionID, StreamID: peer.StreamID, Submissions: []models.NativeResumeSubmissionEvidence{{ID: peer.ID, HarnessGeneration: 1, PayloadHash: peer.Hash, StreamID: peer.StreamID}}}
				if condition == "retired history" || condition == "generic history" {
					proof.Submissions = append(proof.Submissions, models.NativeResumeSubmissionEvidence{ID: "prior", HarnessGeneration: 1, PayloadHash: "prior-hash", StreamID: peer.StreamID})
				}
				return proof, nil
			}
			service.agentManager = wrapper
			if condition == "backlog" {
				_, err = service.messageQueue.QueueMessage(ctx, row.SessionID, "task1", "queued instruction", "", "user", false, nil)
				require.NoError(t, err)
				require.NoError(t, service.messageQueue.SetAutoRun(ctx, row.SessionID, true))
			}
			session, err := repo.GetTaskSession(ctx, row.SessionID)
			require.NoError(t, err)
			restored, err := service.resumeLegacyNativeRestart(ctx, session)
			if condition == "changed owner" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			want := condition == "dead owner" || condition == "backlog" || condition == "queue" || condition == "retired history" || condition == "generic history"
			require.Equal(t, want, restored)
			if restored {
				require.NotNil(t, *launched)
				saved, err := store.GetSubmission(ctx, peer.ID)
				require.NoError(t, err)
				require.True(t, saved.Retired)
				require.Equal(t, journal.SubmissionFailed, saved.State)
				again, err := service.resumeLegacyNativeRestart(ctx, session)
				require.NoError(t, err)
				require.False(t, again)
			} else {
				require.Nil(t, *launched)
			}
			if condition == "backlog" {
				status := service.messageQueue.GetStatus(ctx, row.SessionID)
				require.False(t, status.AutoRun, "native restoration must not release queued instructions")
				require.Len(t, status.Entries, 1)
			}
			require.Empty(t, manager.capturedPromptCalls)
		})
	}
}
