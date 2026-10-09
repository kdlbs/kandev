package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestBlockedRecoveryNotificationDoesNotBlockOtherEnvironment(t *testing.T) {
	ctx := context.Background()
	svc, eventBus, repo := createTestService(t)
	svc.recoveryOperations = repo
	first := seedWorkspaceRecoveryProgressStart(t, repo, "first")
	second := seedWorkspaceRecoveryProgressStart(t, repo, "second")
	blockingBus := &blockingFirstRecoveryNotification{
		EventBus: eventBus, entered: make(chan struct{}), release: make(chan struct{}),
	}
	defer func() {
		select {
		case <-blockingBus.release:
		default:
			close(blockingBus.release)
		}
	}()
	svc.eventBus = blockingBus

	firstResult := make(chan error, 1)
	go func() {
		_, err := svc.BeginWorkspaceRecovery(ctx, first)
		firstResult <- err
	}()
	select {
	case <-blockingBus.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first recovery notification did not reach the blocking bus")
	}

	secondResult := make(chan error, 1)
	go func() {
		_, err := svc.BeginWorkspaceRecovery(ctx, second)
		secondResult <- err
	}()
	select {
	case err := <-secondResult:
		if err != nil {
			t.Fatalf("second environment recovery: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a blocked notification held the recovery-operation mutex across environments")
	}
	close(blockingBus.release)
	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("first environment recovery: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first recovery did not finish after notification release")
	}
}

func seedWorkspaceRecoveryProgressStart(
	t *testing.T,
	repo *sqlite.Repository,
	suffix string,
) worktree.RecoveryProgressStart {
	t.Helper()
	ctx := context.Background()
	workspaceID, workflowID := "workspace-progress-"+suffix, "workflow-progress-"+suffix
	taskID, environmentID := "task-progress-"+suffix, "environment-progress-"+suffix
	sessionID, operationID := "session-progress-"+suffix, "operation-progress-"+suffix
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID,
		WorkflowStepID: "step-" + suffix, Title: taskID, Priority: "medium", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/synthetic/" + environmentID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateFailed,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: environmentID, OwnerTaskID: taskID, OwnershipGeneration: 1,
		SessionID: sessionID, OperationID: operationID, ExecutorType: string(models.ExecutorTypeWorktree),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(context.Background(), claim) })
	return worktree.RecoveryProgressStart{
		TaskID: taskID, TaskEnvironmentID: environmentID, OwnerTaskID: taskID,
		OwnershipGeneration: 1, SessionID: sessionID, OperationID: operationID,
		Kind: recoveryoperation.KindManagedCloneRelocation,
	}
}

type blockingFirstRecoveryNotification struct {
	bus.EventBus
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (b *blockingFirstRecoveryNotification) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if subject == events.SessionWorkspaceRecoveryChanged {
		blocked := false
		b.once.Do(func() {
			close(b.entered)
			blocked = true
		})
		if blocked {
			<-b.release
		}
	}
	return b.EventBus.Publish(ctx, subject, event)
}
