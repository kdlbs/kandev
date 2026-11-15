package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
func TestRepeatedNativeResumeAfterSilentRestore(t *testing.T) {
	for _, independent := range []bool{false, true} {
		name := "matching duplicate fences"
		if independent {
			name = "independent restriction"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newSilentRestoreErrorIdentityFixture(t)
			commitSilentRestoreErrorIdentityFixture(t, fixture)
			repo := fixture.repo
			var err error
			fixture.session, err = repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			currentGeneration, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.session.QueueIncarnationID)
			require.NoError(t, err)
			require.EqualValues(t, 2, currentGeneration.Generation)
			running, err := repo.GetExecutorRunningBySessionID(ctx, fixture.session.ID)
			require.NoError(t, err)
			running.ResumeToken = currentGeneration.NativeSessionID
			require.NoError(t, repo.UpsertExecutorRunning(ctx, running))

			const currentSubmissionID = "repeated-resume-current-submission"
			const currentStreamID = "repeated-resume-current-stream"
			currentSubmission := &models.AgentDeliverySubmission{
				ID: currentSubmissionID, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
				HarnessGeneration: currentGeneration.Generation, OwnerGeneration: currentGeneration.Generation,
				PayloadHash: "repeated-resume-current-hash", Payload: []byte("distinct interrupted prompt"),
				State: models.DeliverySubmissionInterruptedUnknown, Outcome: "prompt_dispatch_failed",
			}
			inserted, err := repo.PrepareAgentDeliverySubmission(ctx, currentSubmission)
			require.NoError(t, err)
			require.True(t, inserted)
			boundBlock := &models.SessionRecoveryBlock{
				ID: "repeated-resume-current-bound", SessionID: fixture.session.ID,
				IncarnationID: fixture.session.QueueIncarnationID, ExpectedGeneration: currentGeneration.Generation,
				Reason: unknownPromptOutcome, State: models.RecoveryBlockOpen, ConsumerReference: agentDeliveryConsumer,
				DeliverySubmissionID: currentSubmission.ID, DeliveryStreamID: currentStreamID,
			}
			require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, boundBlock))
			genericBlock := &models.SessionRecoveryBlock{
				ID: "repeated-resume-current-generic", SessionID: fixture.session.ID,
				IncarnationID: fixture.session.QueueIncarnationID, ExpectedGeneration: currentGeneration.Generation,
				Reason: durableDeliveryUnresolvedReason, State: models.RecoveryBlockOpen,
				ConsumerReference: agentDeliveryConsumer,
			}
			require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, genericBlock))

			var launched *executor.LaunchAgentRequest
			started := false
			fixture.manager.launchAgentFunc = func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched = request
				return &executor.LaunchAgentResponse{AgentExecutionID: "repeated-resume-execution", Status: v1.AgentStatusStarting}, nil
			}
			baseManager := &sessionUpdatingAgentManager{
				mockAgentManager: fixture.manager, repo: repo, sessionID: fixture.session.ID,
				taskID: fixture.session.TaskID, onStartCalled: &started,
			}
			resumeManager := &nativeResumeTestManager{
				sessionUpdatingAgentManager: baseManager,
				acknowledge: func(ackCtx context.Context, sessionID string) error {
					require.Equal(t, fixture.session.ID, sessionID)
					open, err := repo.ListOpenSessionRecoveryBlocks(ackCtx, sessionID, fixture.session.QueueIncarnationID, currentGeneration.Generation)
					require.NoError(t, err)
					wantOpen := 2
					if independent {
						wantOpen++
					}
					require.Len(t, open, wantOpen, "journal acknowledgment precedes SQL resolution of the complete incident")
					return nil
				},
			}
			proofManager := &nativeProofTestManager{
				nativeResumeTestManager: resumeManager,
				proof: func(proofCtx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
					running, err := repo.GetExecutorRunningBySessionID(proofCtx, sessionID)
					if err != nil {
						return nil, err
					}
					old, err := repo.GetAgentDeliverySubmission(proofCtx, fixture.inputs.recovery.SubmissionID)
					if err != nil {
						return nil, err
					}
					current, err := repo.GetAgentDeliverySubmission(proofCtx, currentSubmission.ID)
					if err != nil {
						return nil, err
					}
					return &models.NativeResumeDeliveryEvidence{
						SessionID: sessionID, IncarnationID: fixture.session.QueueIncarnationID,
						HarnessGeneration: currentGeneration.Generation, NativeSessionID: currentGeneration.NativeSessionID,
						ExecutionID: running.AgentExecutionID, StreamID: currentStreamID,
						Submissions: []models.NativeResumeSubmissionEvidence{
							{ID: old.ID, State: old.State, HarnessGeneration: old.HarnessGeneration,
								PayloadHash: old.PayloadHash, StreamID: fixture.inputs.recovery.StreamID},
							{ID: current.ID, State: current.State, Outcome: current.Outcome,
								HarnessGeneration: current.HarnessGeneration, PayloadHash: current.PayloadHash, StreamID: currentStreamID},
						},
					}, nil
				},
			}
			fixture.service.agentManager = proofManager
			fixture.service.executor = withTestCapabilities(executor.NewExecutor(proofManager, repo, testLogger(), executor.ExecutorConfig{}))

			var independentBlock *models.SessionRecoveryBlock
			if independent {
				independentBlock = &models.SessionRecoveryBlock{
					ID: "repeated-resume-independent", SessionID: fixture.session.ID,
					IncarnationID: fixture.session.QueueIncarnationID, ExpectedGeneration: currentGeneration.Generation,
					Reason: "office_restriction", State: models.RecoveryBlockOpen, ConsumerReference: "office",
				}
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, independentBlock))
			}

			historicalBefore, exists := models.LoadAgentDeliveryRecovery(fixture.session.Metadata)
			require.True(t, exists)
			response, err := fixture.service.RecoverSession(ctx, fixture.session.TaskID, fixture.session.ID, "resume")
			if independent {
				require.Error(t, err)
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.NotNil(t, response)
				require.True(t, response.Success)
			}
			require.True(t, started)
			require.NotNil(t, launched)
			require.Equal(t, currentGeneration.NativeSessionID, launched.ACPSessionID)
			require.Empty(t, fixture.manager.capturedPromptCalls, "native Resume must not replay either prompt")

			oldAfter, err := repo.GetAgentDeliverySubmission(ctx, fixture.inputs.recovery.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, oldAfter.State)
			require.Equal(t, fixture.inputs.recovery.SubmissionID, oldAfter.ID)
			currentAfter, err := repo.GetAgentDeliverySubmission(ctx, currentSubmission.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, currentAfter.State)
			require.Equal(t, currentSubmission.Outcome, currentAfter.Outcome)
			require.Equal(t, currentSubmission.Payload, currentAfter.Payload)

			storedSession, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			historicalAfter, exists := models.LoadAgentDeliveryRecovery(storedSession.Metadata)
			require.True(t, exists)
			require.Equal(t, historicalBefore, historicalAfter, "current recovery must preserve the historical restored notice")

			currentBlocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, currentGeneration.Generation)
			require.NoError(t, err)
			if independent {
				require.Len(t, currentBlocks, 3, "an independent restriction keeps the matching incident blocked atomically")
				stored, err := repo.GetSessionRecoveryBlock(ctx, independentBlock.ID)
				require.NoError(t, err)
				require.Equal(t, models.RecoveryBlockOpen, stored.State)
			} else {
				require.Empty(t, currentBlocks)
				for _, id := range []string{boundBlock.ID, genericBlock.ID} {
					stored, err := repo.GetSessionRecoveryBlock(ctx, id)
					require.NoError(t, err)
					require.Equal(t, models.RecoveryBlockResolved, stored.State)
					require.Equal(t, "resume", stored.AuthorizedAction)
					require.NotNil(t, stored.ResolvedAt)
				}
			}
			oldBlock, err := repo.GetSessionRecoveryBlock(ctx, fixture.inputs.block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockResolved, oldBlock.State)
			require.Equal(t, models.SilentRestoreAction, oldBlock.AuthorizedAction)
		})
	}
}
