package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeEmptyBlockAfterEarlierRestoration(t *testing.T) {
	for _, metadata := range []string{"absent", "resolved history"} {
		t.Run(metadata, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, submission, launched, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
			repo := service.repo.(*sqliterepo.Repository)
			if metadata == "resolved history" {
				changed, err := repo.ResolveSessionRecoveryBlock(ctx, block.ID, "resume", time.Now().UTC())
				require.NoError(t, err)
				require.True(t, changed)
			} else {
				require.NoError(t, repo.SetSessionMetadataKey(ctx, submission.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
				_, err := repo.DB().ExecContext(ctx, `DELETE FROM session_recovery_blocks WHERE id = ?`, block.ID)
				require.NoError(t, err)
			}
			generic := *block
			generic.ID = "current-empty-block"
			generic.DeliverySubmissionID, generic.DeliveryStreamID = "", ""
			generic.ResolvedAt, generic.AuthorizedAction = nil, ""
			generic.State = models.RecoveryBlockOpen
			generic.Reason = durableDeliveryUnresolvedReason
			require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &generic))
			current := *submission
			current.ID, current.State = "prompt:current-crash", models.DeliverySubmissionDispatching
			_, err := repo.PrepareAgentDeliverySubmission(ctx, &current)
			require.NoError(t, err)
			manager.acknowledge = func(ctx context.Context, sessionID string) error {
				require.Equal(t, submission.SessionID, sessionID)
				changed, err := repo.TransitionAgentDeliverySubmission(ctx, current.ID, models.DeliverySubmissionDispatching, models.DeliverySubmissionInterruptedUnknown, "unknown", time.Now().UTC())
				require.NoError(t, err)
				require.True(t, changed)
				return nil
			}
			response, err := service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
			require.NoError(t, err)
			require.True(t, response.Success)
			require.Equal(t, "native-conversation", (*launched).ACPSessionID)
			require.Empty(t, manager.capturedPromptCalls)
			resolved, err := repo.GetSessionRecoveryBlock(ctx, generic.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockResolved, resolved.State)
			stored, err := repo.GetAgentDeliverySubmission(ctx, current.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
			require.Equal(t, "unknown", stored.Outcome)
			require.Zero(t, stored.TerminalSequence)
			require.Equal(t, current.Payload, stored.Payload)
		})
	}
}
