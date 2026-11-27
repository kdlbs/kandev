package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkspaceRecoveryProjectionFencesCallbacksAndNotifiesEnvironmentSessions(t *testing.T) {
	ctx := context.Background()
	svc, eventBus, repo := createTestService(t)
	svc.recoveryOperations = repo
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-recovery-progress", Name: "Recovery"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-recovery-progress", WorkspaceID: "workspace-recovery-progress", Name: "Flow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-recovery-progress", WorkspaceID: "workspace-recovery-progress",
		WorkflowID: "workflow-recovery-progress", WorkflowStepID: "step-recovery-progress",
		Title: "Recovery", Priority: "medium", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-recovery-progress", TaskID: "task-recovery-progress",
		OwnershipGeneration: 4, ExecutorType: string(models.ExecutorTypeWorktree),
		ExecutorID: models.ExecutorIDWorktree, Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/synthetic/recovery-progress",
	}); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"session-recovery-progress", "session-recovery-sibling"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-recovery-progress",
			TaskEnvironmentID: "environment-recovery-progress", State: models.TaskSessionStateFailed,
			StartedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-recovery-progress", OwnerTaskID: "task-recovery-progress",
		OwnershipGeneration: 4, SessionID: "session-recovery-progress", OperationID: "operation-recovery-progress",
		ExecutorType: string(models.ExecutorTypeWorktree),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if claim != nil {
			_ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim)
		}
	}()

	binding, err := svc.BeginWorkspaceRecovery(ctx, worktree.RecoveryProgressStart{
		TaskID: "task-recovery-progress", SessionID: claim.SessionID,
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID,
		OwnershipGeneration: claim.OwnershipGeneration, OperationID: claim.OperationID,
		ErrorStamp: "private-error-stamp", Kind: recoveryoperation.KindManagedCloneRelocation,
		SelectedRepositoryIDs: []string{"repository-recovery-progress"}, RepositoryTotal: 1,
	})
	if err != nil {
		t.Fatalf("BeginWorkspaceRecovery: %v", err)
	}
	if binding.AttemptID == "" || binding.Revision < 1 {
		t.Fatalf("operation binding = %+v", binding)
	}
	operationRepo := &readThenAdvanceRecoveryOperationRepository{TaskEnvironmentRecoveryOperationRepository: repo}
	svc.recoveryOperations = operationRepo
	var advanced worktree.RecoveryProgressBinding
	operationRepo.afterRead = func() {
		var updateErr error
		advanced, updateErr = svc.UpdateWorkspaceRecovery(ctx, binding, worktree.RecoveryProgressUpdate{
			State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseSnapshotting,
			RepositoryID: "repository-recovery-progress", RepositoryPosition: 1, RepositoryTotal: 1,
		})
		if updateErr != nil {
			t.Errorf("concurrent phase update: %v", updateErr)
		}
	}
	operation, live, err := svc.WorkspaceRecoveryProjection(ctx, claim.TaskEnvironmentID)
	if err != nil || operation == nil || !live {
		t.Fatalf("WorkspaceRecoveryProjection = %+v, live=%v, err=%v", operation, live, err)
	}
	if advanced.Revision <= operation.Revision {
		t.Fatalf("projection race did not advance revision: projection=%d runner=%d", operation.Revision, advanced.Revision)
	}
	svc.recoveryOperations = repo

	var startEvent map[string]any
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.SessionWorkspaceRecoveryChanged {
			continue
		}
		startEvent, _ = event.Data.(map[string]any)
	}
	if startEvent == nil {
		t.Fatal("workspace recovery change was not published")
	}
	ids, ok := startEvent["session_ids"].([]string)
	if !ok || len(ids) != 2 || ids[0] != "session-recovery-progress" || ids[1] != "session-recovery-sibling" {
		t.Fatalf("environment session recipients = %#v", startEvent["session_ids"])
	}
	encoded, err := json.Marshal(startEvent)
	if err != nil {
		t.Fatal(err)
	}
	if recoveryPayload, ok := startEvent["workspace_recovery"].(map[string]any); !ok || recoveryPayload["error_stamp"] != "private-error-stamp" {
		t.Fatalf("workspace recovery notification omitted its correlation stamp: %#v", startEvent["workspace_recovery"])
	}
	for _, privateValue := range []string{svc.recoveryOperationRunnerID, "/private/checkout"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("notification leaked private value %q: %s", privateValue, encoded)
		}
	}

	latest, err := svc.UpdateWorkspaceRecovery(ctx, advanced, worktree.RecoveryProgressUpdate{
		State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseRestoring,
	})
	if err != nil {
		t.Fatalf("follow-up UpdateWorkspaceRecovery: %v", err)
	}
	_, err = svc.UpdateWorkspaceRecovery(ctx, advanced, worktree.RecoveryProgressUpdate{State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhasePublishing})
	if !errors.Is(err, recoveryoperation.ErrStaleWriter) {
		t.Fatalf("late callback error = %v, want stale writer", err)
	}
	svc.EndWorkspaceRecoveryRunner(ctx, latest)
	operation, live, err = svc.WorkspaceRecoveryProjection(ctx, claim.TaskEnvironmentID)
	if err != nil || operation == nil || live || operation.Phase != recoveryoperation.PhaseRestoring {
		t.Fatalf("projection after runner exit = %+v, live=%v, err=%v", operation, live, err)
	}
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		t.Fatal(err)
	}
	claim = nil
	persisted, err := repo.GetTaskEnvironmentRecoveryOperation(ctx, "environment-recovery-progress")
	if err != nil || persisted == nil || persisted.OperationID != "operation-recovery-progress" {
		t.Fatalf("projection after claim release = %+v, err=%v", persisted, err)
	}
}

type readThenAdvanceRecoveryOperationRepository struct {
	repository.TaskEnvironmentRecoveryOperationRepository
	afterRead func()
}

func (r *readThenAdvanceRecoveryOperationRepository) GetTaskEnvironmentRecoveryOperation(
	ctx context.Context,
	environmentID string,
) (*models.TaskEnvironmentRecoveryOperation, error) {
	operation, err := r.TaskEnvironmentRecoveryOperationRepository.GetTaskEnvironmentRecoveryOperation(ctx, environmentID)
	if err == nil && r.afterRead != nil {
		afterRead := r.afterRead
		r.afterRead = nil
		afterRead()
	}
	return operation, err
}

func TestBeginWorkspaceRecoverySettlesDeadSameProcessAttemptBeforeRetry(t *testing.T) {
	ctx := context.Background()
	svc, eventBus, repo := createTestService(t)
	operationRepo := &failOnceRecoveryOperationUpdateRepository{TaskEnvironmentRecoveryOperationRepository: repo}
	svc.recoveryOperations = operationRepo
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-recovery-retry", Name: "Recovery"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-recovery-retry", WorkspaceID: "workspace-recovery-retry", Name: "Flow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-recovery-retry", WorkspaceID: "workspace-recovery-retry",
		WorkflowID: "workflow-recovery-retry", WorkflowStepID: "step-recovery-retry",
		Title: "Recovery", Priority: "medium", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-recovery-retry", TaskID: "task-recovery-retry",
		OwnershipGeneration: 5, ExecutorType: string(models.ExecutorTypeWorktree),
		ExecutorID: models.ExecutorIDWorktree, Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/synthetic/recovery-retry",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-recovery-retry", TaskID: "task-recovery-retry",
		TaskEnvironmentID: "environment-recovery-retry", State: models.TaskSessionStateFailed,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-recovery-retry", OwnerTaskID: "task-recovery-retry",
		OwnershipGeneration: 5, SessionID: "session-recovery-retry", OperationID: "operation-recovery-retry",
		ExecutorType: string(models.ExecutorTypeWorktree),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()
	start := worktree.RecoveryProgressStart{
		TaskID: "task-recovery-retry", SessionID: claim.SessionID,
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID,
		OwnershipGeneration: claim.OwnershipGeneration, OperationID: claim.OperationID,
		ErrorStamp: "error-recovery-retry", Kind: recoveryoperation.KindManagedCloneRelocation,
		SelectedRepositoryIDs: []string{"repository-recovery-retry"}, RepositoryTotal: 1,
	}
	firstBinding, err := svc.BeginWorkspaceRecovery(ctx, start)
	if err != nil {
		t.Fatalf("first BeginWorkspaceRecovery: %v", err)
	}
	operationRepo.failNextUpdate = true
	if _, err := svc.UpdateWorkspaceRecovery(ctx, firstBinding, worktree.RecoveryProgressUpdate{
		State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseSnapshotting,
	}); err == nil {
		t.Fatal("injected progress persistence failure was ignored")
	}
	svc.EndWorkspaceRecoveryRunner(ctx, firstBinding)

	secondBinding, err := svc.BeginWorkspaceRecovery(ctx, start)
	if err != nil {
		t.Fatalf("same-process retry after dead attempt: %v", err)
	}
	if secondBinding.AttemptID == firstBinding.AttemptID {
		t.Fatalf("retry reused dead attempt ID %q", secondBinding.AttemptID)
	}
	interruptedEvent := false
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.SessionWorkspaceRecoveryChanged {
			continue
		}
		payload, ok := event.Data.(map[string]any)
		if !ok {
			continue
		}
		projection, ok := payload["workspace_recovery"].(map[string]any)
		if ok && projection["state"] == recoveryoperation.StateInterrupted && projection["reason_code"] == "runner_ended_unsettled" {
			interruptedEvent = true
		}
	}
	if !interruptedEvent {
		t.Fatal("dead same-process attempt was not published as interrupted before retry")
	}
	svc.EndWorkspaceRecoveryRunner(ctx, secondBinding)
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		t.Fatal(err)
	}
	claim = nil
	claim, err = repo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-recovery-retry", OwnerTaskID: "task-recovery-retry",
		OwnershipGeneration: 5, SessionID: "session-recovery-retry", OperationID: "operation-recovery-retry-next",
		ExecutorType: string(models.ExecutorTypeWorktree),
	})
	if err != nil {
		t.Fatal(err)
	}
	start.OperationID = claim.OperationID
	nextBinding, err := svc.BeginWorkspaceRecovery(ctx, start)
	if err != nil {
		t.Fatalf("new operation after dead same-process attempt: %v", err)
	}
	if nextBinding.OperationID != "operation-recovery-retry-next" {
		t.Fatalf("new operation binding = %+v", nextBinding)
	}
	svc.EndWorkspaceRecoveryRunner(ctx, nextBinding)
}

type failOnceRecoveryOperationUpdateRepository struct {
	repository.TaskEnvironmentRecoveryOperationRepository
	failNextUpdate bool
}

func (r *failOnceRecoveryOperationUpdateRepository) UpdateTaskEnvironmentRecoveryOperation(
	ctx context.Context,
	update models.TaskEnvironmentRecoveryOperationUpdate,
) (*models.TaskEnvironmentRecoveryOperation, error) {
	if r.failNextUpdate {
		r.failNextUpdate = false
		return nil, errors.New("injected workspace recovery progress write failure")
	}
	return r.TaskEnvironmentRecoveryOperationRepository.UpdateTaskEnvironmentRecoveryOperation(ctx, update)
}
