package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestRecoverSessionPermissionRetryRetiresMatchingError(t *testing.T) {
	const (
		taskID        = "task-permission-recovery-error"
		sessionID     = "session-permission-recovery-error"
		environmentID = "environment-permission-recovery-error"
		repositoryID  = "repository-permission-recovery-error"
		errorStamp    = "managed-clone-permission-error-stamp"
	)
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step1")
	seedPermissionRecoverySession(t, repo, taskID, sessionID, environmentID, repositoryID, errorStamp)

	type providerStart struct {
		executionID string
		attemptID   string
		sessionID   string
		resumeToken string
	}
	starts := make(chan providerStart, 1)
	agentManager := &mockAgentManager{
		isAgentReadyFn: func(context.Context, string) bool { return true },
		launchAgentFunc: func(callCtx context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			starts <- providerStart{
				executionID: "execution-permission-recovery-error",
				attemptID:   executor.ResumeAttemptIDFromContext(callCtx),
				sessionID:   request.SessionID,
				resumeToken: request.ACPSessionID,
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-permission-recovery-error"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})
	admissionCalls := 0
	svc.executor.SetSelectedWorktreeRecoveryAdmission(func(callCtx context.Context, request worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		if !worktree.DirtyCloneRelocationAllowed(callCtx) {
			return nil, &worktree.ManagedCloneRelocationRequiredError{TaskID: taskID}
		}
		return nil, nil
	})

	if _, err := svc.RecoverSessionWithOptions(ctx, taskID, sessionID, "resume", RecoverSessionOptions{}); err == nil {
		t.Fatal("ordinary resume bypassed managed-clone relocation admission")
	}
	if admissionCalls != 1 || len(starts) != 0 {
		t.Fatalf("ordinary resume admissions/starts = %d/%d, want 1/0", admissionCalls, len(starts))
	}
	if _, err := svc.RecoverSessionWithOptions(ctx, taskID, sessionID, models.RecoveryActionRelocateAndResume, RecoverSessionOptions{
		ErrorStamp: "stale-permission-error-stamp",
	}); err == nil {
		t.Fatal("explicit relocation accepted a stale session error stamp")
	}
	if admissionCalls != 1 || len(starts) != 0 {
		t.Fatalf("stale action admissions/starts = %d/%d, want unchanged 1/0", admissionCalls, len(starts))
	}

	result := make(chan error, 1)
	go func() {
		_, err := svc.RecoverSessionWithOptions(ctx, taskID, sessionID, models.RecoveryActionRelocateAndResume, RecoverSessionOptions{
			ErrorStamp: errorStamp,
		})
		result <- err
	}()
	var started providerStart
	select {
	case started = <-starts:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for provider resume startup")
	}
	if started.sessionID != sessionID || started.resumeToken != "provider-conversation-kept" || started.attemptID == "" {
		t.Fatalf("provider resume identity = %+v", started)
	}
	waitForPermissionRecoveryAttempt(t, svc, repo, sessionID, started)
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: started.executionID, AttemptID: started.attemptID,
	})
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("RecoverSessionWithOptions explicit retry: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RecoverSessionWithOptions did not finish after provider boot")
	}
	if admissionCalls != 4 {
		t.Fatalf("total recovery admissions = %d, want ordinary preflight, explicit preflight, and both resume inspections", admissionCalls)
	}
	stored, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload recovered session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(stored.Metadata)
	if !ok || !lastError.MatchesStamp(errorStamp) || !lastError.IsDismissed() {
		t.Fatalf("matching relocation error after provider boot = %+v, found=%v, want dismissed history", lastError, ok)
	}
}

func seedPermissionRecoverySession(
	t *testing.T,
	repo interface {
		CreateRepository(context.Context, *models.Repository) error
		CreateTaskRepository(context.Context, *models.TaskRepository) error
		CreateTaskEnvironment(context.Context, *models.TaskEnvironment) error
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
		UpdateTaskSession(context.Context, *models.TaskSession) error
		SetSessionMetadataKey(context.Context, string, string, interface{}) error
		UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
	},
	taskID, sessionID, environmentID, repositoryID, errorStamp string,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: "ws1", Name: "recovery", SourceType: "local", LocalPath: "/repos/recovery",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create recovery repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-permission-recovery-error", TaskID: taskID,
		RepositoryID: repositoryID, BaseBranch: "main", Position: 0, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	worktreePath := "/tasks/permission-recovery/recovery"
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/tasks/permission-recovery", TaskDirName: "permission-recovery",
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-permission-recovery-error", TaskEnvironmentID: environmentID,
			RepositoryID: repositoryID, BranchSlug: "main", WorktreeID: "worktree-permission-recovery-error",
			WorktreePath: worktreePath, WorktreeBranch: "feature/permission-recovery", Status: "active", Position: 0,
			CreatedAt: now, UpdatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("create recovery environment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		t.Fatalf("load seeded session: %v", err)
	}
	session.State, session.TaskEnvironmentID = models.TaskSessionStateCancelled, environmentID
	session.ExecutorID, session.AgentProfileID = models.ExecutorIDWorktree, "profile-recovery"
	session.RepositoryID, session.BaseBranch = repositoryID, "main"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update recovery session: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "Move files and resume is available.", OccurredAt: now, Scope: models.ErrorScopeSession,
		Code:            models.LaunchErrorCategoryManagedCloneRelocationRequired,
		RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: errorStamp,
	}); err != nil {
		t.Fatalf("seed managed-clone recovery error: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, ResumeToken: "provider-conversation-kept",
		Resumable: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed provider conversation: %v", err)
	}
}

func waitForPermissionRecoveryAttempt(
	t *testing.T,
	svc *Service,
	repo interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	},
	sessionID string,
	started struct {
		executionID string
		attemptID   string
		sessionID   string
		resumeToken string
	},
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		session, err := repo.GetTaskSession(context.Background(), sessionID)
		if err == nil && session != nil && session.State == models.TaskSessionStateStarting &&
			svc.resumeAttemptAllowsExecution(sessionID, started.executionID, started.attemptID) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("resume attempt did not reach the provider startup boundary")
}
