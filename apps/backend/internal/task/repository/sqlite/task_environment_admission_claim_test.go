package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func TestCreateTurnWithStepStampRejectsForeignEnvironmentRecoveryClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-foreign-turn-claim"
		environmentID = "environment-foreign-turn-claim"
		requesterID   = "session-foreign-turn-requester"
		foreignID     = "session-foreign-turn-consumer"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{
			ID: foreignID, TaskID: taskID, TaskEnvironmentID: environmentID,
			QueueIncarnationID: "incarnation-foreign-turn-consumer",
			State:              models.TaskSessionStateWaitingForInput,
		},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: foreignID, SessionID: foreignID, TaskID: taskID,
		ExecutorID: "executor-foreign-turn", Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", foreignID, err)
	}

	request := recoveryClaimRequest(environmentID, taskID, requesterID, "operation-foreign-turn", 1)
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
	beforeSession, err := repo.GetTaskSession(ctx, foreignID)
	if err != nil {
		t.Fatalf("GetTaskSession before turn: %v", err)
	}

	turn := &models.Turn{ID: "turn-foreign-recovery-claim", TaskSessionID: foreignID, TaskID: taskID}
	stamped, err := repo.CreateTurnWithStepStamp(ctx, turn)
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Errorf("CreateTurnWithStepStamp error = %v, want ErrBusy", err)
	}
	if stamped {
		t.Error("CreateTurnWithStepStamp stamped a foreign turn while the recovery claim was active")
	}
	if _, ok := turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]; ok {
		t.Error("foreign turn retained a workflow-step stamp")
	}
	if _, getErr := repo.GetTurn(ctx, turn.ID); getErr == nil {
		t.Error("foreign turn was durably inserted")
	}

	afterSession, err := repo.GetTaskSession(ctx, foreignID)
	if err != nil {
		t.Fatalf("GetTaskSession after turn: %v", err)
	}
	if afterSession.ID != beforeSession.ID || afterSession.TaskID != beforeSession.TaskID ||
		afterSession.TaskEnvironmentID != beforeSession.TaskEnvironmentID ||
		afterSession.QueueIncarnationID != beforeSession.QueueIncarnationID ||
		afterSession.State != beforeSession.State || !afterSession.UpdatedAt.Equal(beforeSession.UpdatedAt) {
		t.Errorf("foreign session changed: before=%+v after=%+v", beforeSession, afterSession)
	}

	replayed, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("replay recovery claim: %v", err)
	}
	if replayed.TaskEnvironmentID != claim.TaskEnvironmentID || replayed.OwnerTaskID != claim.OwnerTaskID ||
		replayed.OwnershipGeneration != claim.OwnershipGeneration || replayed.SessionID != claim.SessionID ||
		replayed.OperationID != claim.OperationID || replayed.ExecutorType != claim.ExecutorType ||
		!replayed.CreatedAt.Equal(claim.CreatedAt) || !replayed.UpdatedAt.Equal(claim.UpdatedAt) {
		t.Errorf("recovery claim changed: before=%+v after=%+v", claim, replayed)
	}
}

func TestTaskEnvironmentRecoveryClaimInactivePreserved(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-admission-inactive"
		environmentID = "environment-admission-inactive"
		requesterID   = "session-admission-requester"
		preservedID   = "session-admission-preserved"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{ID: preservedID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateWaitingForInput},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: preservedID, SessionID: preservedID, TaskID: taskID,
		ExecutorID: "executor-admission", Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", preservedID, err)
	}

	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(environmentID, taskID, requesterID, "operation-admission-winner", 1))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })

	_, err = repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(environmentID, taskID, preservedID, "operation-admission-competitor", 1))
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("competing claim error = %v, want ErrBusy", err)
	}
}

func TestTaskEnvironmentRecoveryClaimExecutorRunningBlocks(t *testing.T) {
	repo, ctx, request := seedAdmissionClaimBlocker(t, "executor-running", models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-executor-running", SessionID: "session-executor-running", TaskID: "task-executor-running",
		ExecutorID: "executor-admission", Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	assertAdmissionClaimBusy(t, repo, ctx, request)
}

func TestTaskEnvironmentRecoveryClaimOpenTurnBlocks(t *testing.T) {
	repo, ctx, request := seedAdmissionClaimBlocker(t, "open-turn", models.TaskSessionStateWaitingForInput)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-open", TaskSessionID: "session-open-turn", TaskID: "task-open-turn",
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	assertAdmissionClaimBusy(t, repo, ctx, request)
}

func TestTaskEnvironmentRecoveryClaimSessionStartingRunningBlocks(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateStarting, models.TaskSessionStateRunning} {
		t.Run(string(state), func(t *testing.T) {
			suffix := "session-" + string(state)
			repo, ctx, request := seedAdmissionClaimBlocker(t, suffix, state)
			assertAdmissionClaimBusy(t, repo, ctx, request)
		})
	}
}

func TestTaskEnvironmentRecoveryClaimMaterializationBlocks(t *testing.T) {
	repo, ctx, request := seedAdmissionClaimBlocker(t, "materialization", models.TaskSessionStateWaitingForInput)
	if err := repo.UpdateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-materialization", TaskID: "task-materialization",
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "session-materialization",
	}); err != nil {
		t.Fatalf("UpdateTaskEnvironment: %v", err)
	}
	assertAdmissionClaimBusy(t, repo, ctx, request)
}

func seedAdmissionClaimBlocker(
	t *testing.T,
	suffix string,
	state models.TaskSessionState,
) (*Repository, context.Context, models.TaskEnvironmentRecoveryClaimRequest) {
	t.Helper()
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskID := "task-" + suffix
	environmentID := "environment-" + suffix
	requesterID := "requester-" + suffix
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	for _, session := range []*models.TaskSession{
		{ID: requesterID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCreated},
		{ID: "session-" + suffix, TaskID: taskID, TaskEnvironmentID: environmentID, State: state},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	return repo, ctx, recoveryClaimRequest(environmentID, taskID, requesterID, "operation-"+suffix, 1)
}

func assertAdmissionClaimBusy(
	t *testing.T,
	repo *Repository,
	ctx context.Context,
	request models.TaskEnvironmentRecoveryClaimRequest,
) {
	t.Helper()
	_, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim error = %v, want ErrBusy", err)
	}
}
