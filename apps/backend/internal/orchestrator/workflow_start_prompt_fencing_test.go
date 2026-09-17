package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestWorkflowAsyncStartFailure_RejectsStaleAndDuplicateCallbacks(t *testing.T) {
	t.Run("move committed between ownership read and queue insertion is fenced", func(t *testing.T) {
		svc, repo, ctx, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
			t, models.TaskSessionStateStarting, "workflow-fence-execution",
		)
		moveBeforeInsert := &workflowStartPromptMoveBeforeInsertRepository{
			Repository: newWorkflowStartPromptSQLiteRepository(t, repo),
			beforeInsert: func() {
				task, err := repo.GetTask(ctx, "workflow-fence-task")
				if err != nil {
					t.Fatalf("get task at admission barrier: %v", err)
				}
				task.WorkflowStepID = "successor-step"
				if err := repo.UpdateTask(ctx, task); err != nil {
					t.Fatalf("commit successor move at admission barrier: %v", err)
				}
			},
		}
		svc.messageQueue = messagequeue.NewService(
			moveBeforeInsert, messagequeue.DefaultMaxPerSession, testLogger(),
		)
		svc.preserveWorkflowStartPromptAfterFailure(
			attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
		)
		if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 0 {
			t.Fatalf("obsolete callback queued %d entries after the committed move", got)
		}
		agentMgr := svc.agentManager.(*mockAgentManager)
		if len(agentMgr.capturedPrompts) != 0 {
			t.Fatalf("obsolete callback dispatched %d prompts after the committed move", len(agentMgr.capturedPrompts))
		}
	})

	t.Run("leave and return to the same step is fenced", func(t *testing.T) {
		svc, repo, ctx, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
			t, models.TaskSessionStateStarting, "workflow-fence-execution",
		)
		svc.messageQueue = messagequeue.NewService(
			newWorkflowStartPromptSQLiteRepository(t, repo), messagequeue.DefaultMaxPerSession, testLogger(),
		)
		task, err := repo.GetTask(ctx, "workflow-fence-task")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		task.WorkflowStepID = "successor-step"
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("leave original step: %v", err)
		}
		task.WorkflowStepID = "workflow-fence-step"
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("return to original step: %v", err)
		}

		svc.preserveWorkflowStartPromptAfterFailure(
			attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
		)
		if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 0 {
			t.Fatalf("re-entered step accepted %d obsolete entries", got)
		}
		agentMgr := svc.agentManager.(*mockAgentManager)
		if len(agentMgr.capturedPrompts) != 0 {
			t.Fatalf("re-entered step dispatched %d obsolete prompts", len(agentMgr.capturedPrompts))
		}
	})

	t.Run("duplicate callback claims once", func(t *testing.T) {
		svc, repo, ctx, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
			t, models.TaskSessionStateStarting, "workflow-fence-execution",
		)
		if handled := svc.handleAgentStartFailed(
			attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
			errors.New("startup failed"), false,
		); handled {
			t.Fatal("generic startup failure should remain owned by executor projection")
		}
		if handled := svc.handleAgentStartFailed(
			attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
			errors.New("duplicate startup failed"), false,
		); handled {
			t.Fatal("duplicate generic startup failure should remain owned by executor projection")
		}
		if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 1 {
			t.Fatalf("queue count = %d, want 1 after duplicate callbacks", got)
		}
		if _, err := repo.GetTaskSession(ctx, "workflow-fence-session"); err != nil {
			t.Fatalf("session was not retained after duplicate callbacks: %v", err)
		}
	})

	t.Run("successor turn is fenced", func(t *testing.T) {
		svc, _, ctx, attemptCtx, turnID := newWorkflowStartPromptAttemptFixture(
			t, models.TaskSessionStateStarting, "workflow-fence-execution",
		)
		svc.completeTurnIfCurrent(ctx, "workflow-fence-session", turnID)
		successorTurnID := svc.startTurnForSession(ctx, "workflow-fence-session")
		if successorTurnID == "" || successorTurnID == turnID {
			t.Fatalf("successor turn = %q, want a new turn after %q", successorTurnID, turnID)
		}
		svc.handleAgentStartFailed(
			attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
			errors.New("stale startup failed"), false,
		)
		if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 0 {
			t.Fatalf("stale turn callback queued %d entries", got)
		}
	})

	t.Run("rotated execution is fenced", func(t *testing.T) {
		svc, repo, ctx, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
			t, models.TaskSessionStateStarting, "workflow-fence-execution",
		)
		seedExecutorRunning(t, repo, "workflow-fence-session", "workflow-fence-task", "workflow-successor-execution")
		svc.handleAgentStartFailed(
			attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
			errors.New("superseded startup failed"), false,
		)
		if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 0 {
			t.Fatalf("rotated execution callback queued %d entries", got)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(*Service, *sqliterepo.Repository, context.Context, *workflowStartPromptAttempt, string)
	}{
		{
			name: "terminal session",
			mutate: func(_ *Service, repo *sqliterepo.Repository, ctx context.Context, _ *workflowStartPromptAttempt, _ string) {
				if err := repo.UpdateTaskSessionState(ctx, "workflow-fence-session", models.TaskSessionStateCancelled, "cancelled"); err != nil {
					t.Fatalf("cancel session: %v", err)
				}
			},
		},
		{
			name: "archived task",
			mutate: func(_ *Service, repo *sqliterepo.Repository, ctx context.Context, _ *workflowStartPromptAttempt, _ string) {
				if err := repo.ArchiveTask(ctx, "workflow-fence-task"); err != nil {
					t.Fatalf("archive task: %v", err)
				}
			},
		},
		{
			name: "superseded workflow entry",
			mutate: func(_ *Service, repo *sqliterepo.Repository, ctx context.Context, _ *workflowStartPromptAttempt, _ string) {
				task, err := repo.GetTask(ctx, "workflow-fence-task")
				if err != nil {
					t.Fatalf("get task: %v", err)
				}
				task.WorkflowStepID = "successor-step"
				if err := repo.UpdateTask(ctx, task); err != nil {
					t.Fatalf("move task to successor step: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, ctx, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
				t, models.TaskSessionStateStarting, "workflow-fence-execution",
			)
			tc.mutate(svc, repo, ctx, workflowStartPromptAttemptFromContext(attemptCtx), "workflow-fence-execution")
			svc.handleAgentStartFailed(
				attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
				errors.New("stale startup failed"), false,
			)
			if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 0 {
				t.Fatalf("%s callback queued %d entries", tc.name, got)
			}
		})
	}

	t.Run("deleted task or session is fenced", func(t *testing.T) {
		svc, repo, ctx, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
			t, models.TaskSessionStateStarting, "workflow-fence-execution",
		)
		if err := repo.DeleteTask(ctx, "workflow-fence-task"); err != nil {
			t.Fatalf("delete task: %v", err)
		}
		svc.handleAgentStartFailed(
			attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
			errors.New("deleted startup failed"), false,
		)
		if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 0 {
			t.Fatalf("deleted task callback queued %d entries", got)
		}
	})
}

type workflowStartPromptMoveBeforeInsertRepository struct {
	messagequeue.Repository
	beforeInsert func()
	once         sync.Once
}

func newWorkflowStartPromptSQLiteRepository(t *testing.T, repo *sqliterepo.Repository) messagequeue.Repository {
	t.Helper()
	db := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("create shared queue repository: %v", err)
	}
	return queueRepo
}

func (r *workflowStartPromptMoveBeforeInsertRepository) InsertForSession(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	msg *messagequeue.QueuedMessage,
	maxPerSession int,
) error {
	r.once.Do(r.beforeInsert)
	return r.Repository.InsertForSession(ctx, identity, msg, maxPerSession)
}

func (r *workflowStartPromptMoveBeforeInsertRepository) InsertForSessionWithWorkflowEntry(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	entry messagequeue.WorkflowEntryIdentity,
	msg *messagequeue.QueuedMessage,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
	policy *messagequeue.AutoMergePolicy,
) error {
	r.once.Do(r.beforeInsert)
	repo, ok := r.Repository.(interface {
		InsertForSessionWithWorkflowEntry(
			context.Context,
			messagequeue.QueueSessionIdentity,
			messagequeue.WorkflowEntryIdentity,
			*messagequeue.QueuedMessage,
			*messagequeue.QueueAttachmentClaim,
			int,
			*messagequeue.AutoMergePolicy,
		) error
	})
	if !ok {
		return r.Repository.InsertForSession(ctx, identity, msg, maxPerSession)
	}
	return repo.InsertForSessionWithWorkflowEntry(ctx, identity, entry, msg, claim, maxPerSession, policy)
}

func (r *workflowStartPromptMoveBeforeInsertRepository) AutoMergeCandidateIntoAboveForSessionWithWorkflowEntry(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	entry messagequeue.WorkflowEntryIdentity,
	candidate *messagequeue.QueuedMessage,
	claim *messagequeue.QueueAttachmentClaim,
	policy *messagequeue.AutoMergePolicy,
) (*messagequeue.QueuedMessage, bool, error) {
	repo, ok := r.Repository.(interface {
		AutoMergeCandidateIntoAboveForSessionWithWorkflowEntry(
			context.Context,
			messagequeue.QueueSessionIdentity,
			messagequeue.WorkflowEntryIdentity,
			*messagequeue.QueuedMessage,
			*messagequeue.QueueAttachmentClaim,
			*messagequeue.AutoMergePolicy,
		) (*messagequeue.QueuedMessage, bool, error)
	})
	if !ok {
		return nil, false, messagequeue.ErrNoMergeTarget
	}
	return repo.AutoMergeCandidateIntoAboveForSessionWithWorkflowEntry(
		ctx, identity, entry, candidate, claim, policy,
	)
}
