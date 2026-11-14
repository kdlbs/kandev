package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
func TestNativeResumeAcceptsConcurrentAuthenticatedResolution(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		resolve  bool
		newBlock bool
		wantErr  bool
	}{
		{name: "background recovery resolves the same block", resolve: true},
		{name: "incomplete evidence remains blocked", wantErr: true},
		{name: "a new incident remains blocked", resolve: true, newBlock: true, wantErr: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
			repo := service.repo.(*sqliterepo.Repository)
			service.agentManager = &nativeProofTestManager{nativeResumeTestManager: manager, proof: func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
				proof := &models.NativeResumeDeliveryEvidence{
					SessionID: sessionID, IncarnationID: submission.IncarnationID,
					HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation",
					ExecutionID: "old-execution", StreamID: block.DeliveryStreamID,
					Submissions: []models.NativeResumeSubmissionEvidence{{ID: submission.ID,
						HarnessGeneration: submission.HarnessGeneration, PayloadHash: submission.PayloadHash, StreamID: block.DeliveryStreamID}},
				}
				if scenario.resolve {
					changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, block.ID, proof, time.Now().UTC())
					require.NoError(t, err)
					require.True(t, changed)
				}
				if scenario.newBlock {
					other := *block
					other.ID = "new-incident"
					other.DeliverySubmissionID = "prompt:another-incident"
					require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &other))
				}
				if !scenario.resolve {
					proof.NativeSessionID = "mismatched-native-session"
				}
				return proof, nil
			}}
			err := service.resolveSessionRecoveryBlock(ctx, submission.SessionID, "resume")
			if scenario.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			stored, readErr := repo.GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, readErr)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
			require.Equal(t, submission.Payload, stored.Payload)
		})
	}
}
