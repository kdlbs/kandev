package orchestrator

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCreateNewSessionForStepTransfersQueueStateExactlyOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-queue-state", "session-queue-state", "step-one")
	current, err := repo.GetTaskSession(ctx, "session-queue-state")
	if err != nil {
		t.Fatal(err)
	}
	current.State = models.TaskSessionStateRunning
	current.IsPrimary = true
	current.AgentProfileID = "profile-old"
	current.ExecutorID = models.ExecutorIDWorktree
	current.TaskEnvironmentID = "environment-queue-state"
	if err := repo.UpdateTaskSession(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: current.TaskEnvironmentID, TaskID: current.TaskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[current.TaskID] = &v1.Task{ID: current.TaskID, WorkspaceID: "ws1", Title: "Test Task"}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-execution"}, nil
		},
	})

	rawDB, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	attachmentTransfer := &workflowAttachmentTransferStub{}
	svc.SetSessionAttachmentTransferer(attachmentTransfer)
	queued, err := svc.messageQueue.QueueMessage(ctx, current.ID, current.TaskID, "attachment-backed", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment-one", Name: "one.txt", MimeType: "text/plain"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.messageQueue.SetPendingMove(ctx, current.ID, &messagequeue.PendingMove{
		MoveID: "move-one", TaskID: current.TaskID, WorkflowID: "workflow-one", WorkflowStepID: "step-two",
	}); err != nil {
		t.Fatal(err)
	}

	created, err := svc.createNewSessionForStep(ctx, current.TaskID, current, "profile-new")
	if err != nil {
		t.Fatalf("createNewSessionForStep: %v", err)
	}
	move, _, err := svc.messageQueue.GetPendingMoveWithError(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if move == nil || move.MoveID != "move-one" {
		t.Fatalf("replacement pending move = %+v, want move-one", move)
	}
	transferred, err := svc.messageQueue.GetEntry(ctx, created.ID, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transferred.Attachments) != 1 || transferred.Attachments[0].AttachmentID != "attachment-one" {
		t.Fatalf("replacement attachments = %+v, want attachment-one", transferred.Attachments)
	}
	if len(attachmentTransfer.calls) != 1 {
		t.Fatalf("attachment transfer calls = %d, want 1", len(attachmentTransfer.calls))
	}
}
