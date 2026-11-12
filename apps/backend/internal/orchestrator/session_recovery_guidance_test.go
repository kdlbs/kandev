package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type canonicalRecoveryReadFailureRepository struct {
	*sqliterepo.Repository
	reads int
}

type genericResolutionRefusalRepository struct {
	*sqliterepo.Repository
}

func (r *genericResolutionRefusalRepository) ResolveSessionRecoveryBlock(
	context.Context, string, string, time.Time,
) (bool, error) {
	return false, nil
}

func (r *canonicalRecoveryReadFailureRepository) GetOpenSessionRecoveryBlock(
	ctx context.Context, sessionID, incarnationID string, generation int64,
) (*models.SessionRecoveryBlock, error) {
	r.reads++
	if r.reads == 2 {
		return nil, errors.New("canonical recovery block unavailable")
	}
	return r.Repository.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation)
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
func TestRecoveryBlockResolutionGuidance(t *testing.T) {
	for _, reason := range []string{"unresolved_durable_work", "unknown_prompt_outcome"} {
		t.Run(reason, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, submission, _, _ := nativeResumeFixture(t, reason, false, false)
			repo := service.repo.(*sqliterepo.Repository)
			service.agentManager = rejectingNativeResumeGuidanceManager(manager, block, submission, nil)

			err := service.resolveSessionRecoveryBlock(ctx, submission.SessionID, recoveryActionResume)

			var guidance *sessionRecoveryRequiredError
			require.ErrorAs(t, err, &guidance)
			require.Equal(t, reason, guidance.RecoveryReason())
			require.Equal(t, int64(1), guidance.RecoveryGeneration())
			require.Equal(t, block.ID, guidance.Block.ID)
			require.Equal(t, block.IncarnationID, guidance.Block.IncarnationID)
			stored, readErr := repo.GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, readErr)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
			require.Equal(t, submission.Payload, stored.Payload)
		})
	}

	t.Run("generic fallback without native proof retains bounded guidance", func(t *testing.T) {
		for _, reason := range []string{"unresolved_durable_work", "unknown_prompt_outcome"} {
			for _, proofMode := range []string{"capability absent", "reader returned no proof"} {
				t.Run(reason+"/"+proofMode, func(t *testing.T) {
					ctx := context.Background()
					service, manager, block, submission, _, _ := nativeResumeFixture(t, reason, false, false)
					baseRepo := service.repo.(*sqliterepo.Repository)
					repo := &genericResolutionRefusalRepository{Repository: baseRepo}
					service.repo = repo
					service.agentManager = manager
					if proofMode == "reader returned no proof" {
						service.agentManager = &nativeProofTestManager{
							nativeResumeTestManager: manager,
							proof: func(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
								return nil, nil
							},
						}
					}

					err := service.resolveSessionRecoveryBlock(ctx, submission.SessionID, recoveryActionResume)

					var guidance *sessionRecoveryRequiredError
					require.ErrorAs(t, err, &guidance)
					require.Equal(t, reason, guidance.RecoveryReason())
					require.Equal(t, int64(1), guidance.RecoveryGeneration())
					require.Equal(t, block.ID, guidance.RecoveryBlockID())
					require.Equal(t, block.IncarnationID, guidance.RecoveryIncarnationID())
					current, readErr := repo.GetSessionRecoveryBlock(ctx, block.ID)
					require.NoError(t, readErr)
					require.Equal(t, models.RecoveryBlockOpen, current.State)
					stored, readErr := repo.GetAgentDeliverySubmission(ctx, submission.ID)
					require.NoError(t, readErr)
					require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
					require.Equal(t, submission.Outcome, stored.Outcome)
					require.Equal(t, submission.Payload, stored.Payload)
					require.Zero(t, stored.TerminalSequence)
					require.Error(t, service.checkSessionRecoveryBlock(ctx, submission.SessionID), "the open block must continue to deny admission")
				})
			}
		}
	})

	t.Run("replacement incident does not inherit the old hint", func(t *testing.T) {
		ctx := context.Background()
		service, manager, block, submission, _, _ := nativeResumeFixture(t, "unresolved_durable_work", false, false)
		repo := service.repo.(*sqliterepo.Repository)
		replace := func() {
			changed, err := repo.ResolveSessionRecoveryBlock(ctx, block.ID, recoveryActionContinueHistory, time.Now().UTC())
			require.NoError(t, err)
			require.True(t, changed)
			replacement := *block
			replacement.ID = "replacement-recovery-block"
			replacement.Reason = "unknown_prompt_outcome"
			replacement.State = models.RecoveryBlockOpen
			replacement.AuthorizedAction = ""
			replacement.ResolvedAt = nil
			replacement.UpdatedAt = time.Now().UTC().Add(time.Second)
			require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &replacement))
		}
		service.agentManager = rejectingNativeResumeGuidanceManager(manager, block, submission, replace)

		err := service.resolveSessionRecoveryBlock(ctx, submission.SessionID, recoveryActionResume)

		var guidance *sessionRecoveryRequiredError
		require.Error(t, err)
		require.False(t, errors.As(err, &guidance))
	})

	t.Run("canonical reread failure does not return actionable guidance", func(t *testing.T) {
		ctx := context.Background()
		service, manager, block, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
		baseRepo := service.repo.(*sqliterepo.Repository)
		service.agentManager = rejectingNativeResumeGuidanceManager(manager, block, submission, nil)
		repo := &canonicalRecoveryReadFailureRepository{Repository: baseRepo}
		service.repo = repo

		err := service.resolveSessionRecoveryBlock(ctx, submission.SessionID, recoveryActionResume)

		var guidance *sessionRecoveryRequiredError
		require.Error(t, err)
		require.False(t, errors.As(err, &guidance))
	})

	t.Run("unsupported cause does not return actionable guidance", func(t *testing.T) {
		ctx := context.Background()
		service, manager, block, submission, _, _ := nativeResumeFixture(t, "provider_internal_failure", false, false)
		service.agentManager = rejectingNativeResumeGuidanceManager(manager, block, submission, nil)

		err := service.resolveSessionRecoveryBlock(ctx, submission.SessionID, recoveryActionResume)

		var guidance *sessionRecoveryRequiredError
		require.Error(t, err)
		require.False(t, errors.As(err, &guidance))
	})
}

func rejectingNativeResumeGuidanceManager(
	manager *nativeResumeTestManager,
	block *models.SessionRecoveryBlock,
	submission *models.AgentDeliverySubmission,
	beforeRefusal func(),
) *nativeProofTestManager {
	return &nativeProofTestManager{
		nativeResumeTestManager: manager,
		proof: func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
			if beforeRefusal != nil {
				beforeRefusal()
			}
			return &models.NativeResumeDeliveryEvidence{
				SessionID: sessionID, IncarnationID: submission.IncarnationID,
				HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "replacement-native-session",
				ExecutionID: "old-execution", StreamID: block.DeliveryStreamID,
				Submissions: []models.NativeResumeSubmissionEvidence{{
					ID: submission.ID, HarnessGeneration: submission.HarnessGeneration,
					PayloadHash: submission.PayloadHash, StreamID: block.DeliveryStreamID,
				}},
			}, nil
		},
	}
}
