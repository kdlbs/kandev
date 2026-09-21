package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

type turnCreateSurface struct {
	name        string
	wantStamped bool
	wantReceipt bool
	create      func(context.Context, *Repository, *models.Turn) (bool, *models.ConversationMutationReceipt, error)
}

func turnCreateSurfaces() []turnCreateSurface {
	return []turnCreateSurface{
		{
			name: "plain",
			create: func(ctx context.Context, repo *Repository, turn *models.Turn) (bool, *models.ConversationMutationReceipt, error) {
				return false, nil, repo.CreateTurn(ctx, turn)
			},
		},
		{
			name: "step-stamp", wantStamped: true,
			create: func(ctx context.Context, repo *Repository, turn *models.Turn) (bool, *models.ConversationMutationReceipt, error) {
				stamped, err := repo.CreateTurnWithStepStamp(ctx, turn)
				return stamped, nil, err
			},
		},
		{
			name: "conversation-receipt", wantReceipt: true,
			create: func(ctx context.Context, repo *Repository, turn *models.Turn) (bool, *models.ConversationMutationReceipt, error) {
				receipt, err := repo.CreateTurnWithConversationReceipt(ctx, turn)
				return false, receipt, err
			},
		},
		{
			name: "step-stamp-and-conversation-receipt", wantStamped: true, wantReceipt: true,
			create: func(ctx context.Context, repo *Repository, turn *models.Turn) (bool, *models.ConversationMutationReceipt, error) {
				return repo.CreateTurnWithStepStampConversationReceipt(ctx, turn)
			},
		},
	}
}

func TestCreateTurnSurfacesSucceedWithoutRecoveryClaim(t *testing.T) {
	for _, test := range turnCreateSurfaces() {
		t.Run(test.name, func(t *testing.T) {
			repo := newTurnStepStampTestRepo(t)
			taskID := "task-turn-surface-success-" + test.name
			sessionID := "session-turn-surface-success-" + test.name
			seedTurnStepStampSession(t, repo, taskID, "step-turn-surface-success", sessionID)
			turn := &models.Turn{
				ID: "turn-surface-success-" + test.name, TaskID: taskID, TaskSessionID: sessionID,
			}

			stamped, receipt, err := test.create(t.Context(), repo, turn)
			if err != nil {
				t.Fatalf("create turn: %v", err)
			}
			if stamped != test.wantStamped {
				t.Errorf("stamped = %t, want %t", stamped, test.wantStamped)
			}
			if (receipt != nil) != test.wantReceipt {
				t.Errorf("receipt present = %t, want %t", receipt != nil, test.wantReceipt)
			}
			if _, err := repo.GetTurn(t.Context(), turn.ID); err != nil {
				t.Fatalf("GetTurn: %v", err)
			}
		})
	}
}

func TestCreateTurnWithStepStampConversationReceiptPersistsUnstampedWhenPayloadTaskNotFound(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	const (
		taskID    = "task-stamped-receipt-authority"
		sessionID = "session-stamped-receipt-authority"
		turnID    = "turn-stamped-receipt-missing-payload-task"
	)
	seedTurnStepStampSession(t, repo, taskID, "step-stamped-receipt-authority", sessionID)
	turn := &models.Turn{ID: turnID, TaskID: "task-stamped-receipt-missing", TaskSessionID: sessionID}

	stamped, receipt, err := repo.CreateTurnWithStepStampConversationReceipt(t.Context(), turn)
	if err != nil {
		t.Fatalf("CreateTurnWithStepStampConversationReceipt: %v", err)
	}
	if stamped {
		t.Fatal("stamped = true, want false for missing payload task")
	}
	if receipt == nil || !receipt.Complete || len(receipt.Operations) != 1 {
		t.Fatalf("receipt = %+v, want one complete operation", receipt)
	}
	operation := receipt.Operations[0]
	if operation.Kind != models.ConversationMutationUpsert || operation.Entity != models.ConversationEntityTurn ||
		operation.ID != turnID || operation.Turn == nil {
		t.Fatalf("receipt operation = %+v", operation)
	}
	stored, err := repo.GetTurn(t.Context(), turnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if _, ok := stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]; ok {
		t.Fatalf("persisted turn carries a workflow-step stamp: %v", stored.Metadata)
	}
}

func TestCreateTurnSurfacesRejectForeignEnvironmentRecoveryClaim(t *testing.T) {
	for _, test := range turnCreateSurfaces() {
		t.Run(test.name, func(t *testing.T) {
			repo, ctx, claim, foreignSessionID := seedForeignTurnClaim(t, test.name)
			turn := &models.Turn{
				ID:            "turn-foreign-claim-" + test.name,
				TaskID:        claim.OwnerTaskID,
				TaskSessionID: foreignSessionID,
			}

			stamped, receipt, err := test.create(ctx, repo, turn)
			if !errors.Is(err, recoveryclaim.ErrBusy) {
				t.Fatalf("create turn error = %v, want ErrBusy", err)
			}
			if stamped {
				t.Error("foreign turn was stamped")
			}
			if receipt != nil {
				t.Errorf("foreign turn receipt = %+v, want nil", receipt)
			}
			if _, err := repo.GetTurn(ctx, turn.ID); err == nil {
				t.Error("foreign turn was durably inserted")
			}
		})
	}
}

func TestRecoveryClaimContextRejectsTurnForDifferentSession(t *testing.T) {
	repo, ctx, claim, foreignSessionID := seedForeignTurnClaim(t, "claim-context-session-mismatch")
	turn := &models.Turn{
		ID: "turn-claim-context-session-mismatch", TaskID: claim.OwnerTaskID, TaskSessionID: foreignSessionID,
	}

	err := repo.CreateTurn(recoveryclaim.WithClaim(ctx, claim), turn)
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("CreateTurn error = %v, want ErrBusy", err)
	}
	if _, err := repo.GetTurn(ctx, turn.ID); err == nil {
		t.Error("claim context authorized a turn for a different session")
	}
}

func seedForeignTurnClaim(
	t *testing.T,
	suffix string,
) (*Repository, context.Context, *models.TaskEnvironmentRecoveryClaim, string) {
	t.Helper()
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskID := "task-turn-surface-" + suffix
	environmentID := "environment-turn-surface-" + suffix
	requesterID := "session-turn-requester-" + suffix
	foreignID := "session-turn-foreign-" + suffix
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{
			ID: foreignID, TaskID: taskID, TaskEnvironmentID: environmentID,
			QueueIncarnationID: "incarnation-turn-foreign-" + suffix,
			State:              models.TaskSessionStateWaitingForInput,
		},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: foreignID, SessionID: foreignID, TaskID: taskID,
		ExecutorID: "executor-turn-foreign-" + suffix, Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", foreignID, err)
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(t, repo, environmentID, taskID, requesterID, "operation-turn-surface-"+suffix, 1))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
	return repo, ctx, claim, foreignID
}
