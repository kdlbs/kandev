package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPreflightWorkflowStepMoveRejectsDeletedExecutorProfile(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowExecutorAdmissionFixture(t, v1.TaskStateReview, models.TaskSessionStateWaitingForInput)
	fixture.executor.SetExecutorAdmission(func(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
		fixture.admissionCalls++
		fixture.admissionSessionID = sessionID
		fixture.admissionExecutorID = executorID
		fixture.admissionProfileID = profileID
		return ctx, repoerrors.ErrExecutorProfileNotFound
	})

	err := fixture.service.PreflightWorkflowStepMove(ctx, "task-1", fixture.session, workflowAdmissionTarget("profile-b"))
	if !errors.Is(err, repoerrors.ErrExecutorProfileNotFound) {
		t.Fatalf("PreflightWorkflowStepMove() error = %v, want deleted-profile refusal", err)
	}
	if !errors.Is(err, workflowmove.ErrDestinationExecutorUnavailable) {
		t.Fatalf("PreflightWorkflowStepMove() error = %v, want destination-executor sentinel", err)
	}
	if fixture.admissionCalls != 1 {
		t.Fatalf("executor admission calls = %d, want 1", fixture.admissionCalls)
	}
	if fixture.admissionSessionID != "" {
		t.Fatalf("executor admission session ID = %q, want empty preflight ID", fixture.admissionSessionID)
	}
	if fixture.admissionExecutorID != "exec-1" || fixture.admissionProfileID != "deleted-profile" {
		t.Fatalf("executor admission selection = (%q, %q), want (%q, %q)",
			fixture.admissionExecutorID, fixture.admissionProfileID, "exec-1", "deleted-profile")
	}
}

func TestPreflightWorkflowStepMoveKeepsExecutorLookupFailuresUntyped(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowExecutorAdmissionFixture(t, v1.TaskStateReview, models.TaskSessionStateWaitingForInput)
	lookupErr := errors.New("executor ownership store unavailable")
	fixture.executor.SetExecutorAdmission(func(ctx context.Context, _, _, _ string) (context.Context, error) {
		return ctx, lookupErr
	})

	err := fixture.service.PreflightWorkflowStepMove(ctx, "task-1", fixture.session, workflowAdmissionTarget("profile-b"))
	if !errors.Is(err, lookupErr) {
		t.Fatalf("PreflightWorkflowStepMove() error = %v, want lookup failure", err)
	}
	if errors.Is(err, workflowmove.ErrDestinationExecutorUnavailable) {
		t.Fatalf("lookup failure %v must not classify as executor refusal", err)
	}
}

func TestPreflightWorkflowStepMoveKeepCurrentSkipsExecutorAdmission(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowExecutorAdmissionFixture(t, v1.TaskStateReview, models.TaskSessionStateWaitingForInput)
	fixture.executor.SetExecutorAdmission(func(ctx context.Context, _, _, _ string) (context.Context, error) {
		fixture.admissionCalls++
		return ctx, repoerrors.ErrExecutorProfileNotFound
	})

	step := workflowAdmissionTarget("profile-a")
	step.ProfileSessionStartPolicy = models.WorkflowProfileSessionStartPolicyReuse
	err := fixture.service.PreflightWorkflowStepMove(ctx, "task-1", fixture.session, step)
	if err != nil {
		t.Fatalf("PreflightWorkflowStepMove() error = %v, want keep-current success", err)
	}
	if fixture.admissionCalls != 0 {
		t.Fatalf("executor admission calls = %d, want 0 for keep-current", fixture.admissionCalls)
	}
}

func TestPreflightExplicitWorkflowStepMoveKeepsCurrentSessionWithoutExecutorAdmission(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowExecutorAdmissionFixture(t, v1.TaskStateReview, models.TaskSessionStateWaitingForInput)
	fixture.executor.SetExecutorAdmission(func(ctx context.Context, _, _, _ string) (context.Context, error) {
		fixture.admissionCalls++
		return ctx, repoerrors.ErrExecutorProfileNotFound
	})
	target := &wfmodels.WorkflowStep{
		ID: "step-2", WorkflowID: "wf-1",
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial},
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}

	err := fixture.service.PreflightWorkflowStepMove(ctx, "task-1", fixture.session, target)
	if err != nil {
		t.Fatalf("PreflightWorkflowStepMove() error = %v, want keep-current success", err)
	}
	if fixture.admissionCalls != 0 {
		t.Fatalf("executor admission calls = %d, want 0 when explicit target reuses current session", fixture.admissionCalls)
	}
}

func TestSwitchSessionForStepDeletedExecutorProfileRestoresTaskState(t *testing.T) {
	fixture := newWorkflowExecutorAdmissionFixture(t, v1.TaskStateReview, models.TaskSessionStateWaitingForInput)
	ctx := context.Background()
	if err := fixture.repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeLocal),
		ExecutorID: "exec-1", ExecutorProfileID: "deleted-profile",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create ready environment: %v", err)
	}
	fixture.session.TaskEnvironmentID = "env-1"
	if err := fixture.repo.UpdateTaskSession(ctx, fixture.session); err != nil {
		t.Fatalf("bind source session environment: %v", err)
	}
	fixture.executor.SetExecutorAdmission(func(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
		fixture.admissionCalls++
		fixture.admissionSessionID = sessionID
		fixture.admissionExecutorID = executorID
		fixture.admissionProfileID = profileID
		return ctx, repoerrors.ErrExecutorProfileNotFound
	})
	publisher := &recordingTaskUpdatedPublisher{}
	fixture.service.SetTaskEventPublisher(publisher)

	_, err := fixture.service.switchSessionForStep(ctx, "task-1", fixture.session, "profile-b")
	if err == nil {
		t.Fatal("switchSessionForStep() error = nil, want deleted executor profile refusal")
	}
	if !errors.Is(err, repoerrors.ErrExecutorProfileNotFound) {
		t.Fatalf("switchSessionForStep() error = %v, want deleted profile refusal", err)
	}
	if fixture.admissionCalls != 1 || fixture.admissionSessionID != "" ||
		fixture.admissionExecutorID != "exec-1" || fixture.admissionProfileID != "deleted-profile" {
		t.Fatalf("executor admission = calls:%d session:%q executor:%q profile:%q, want one sessionless deleted-profile refusal",
			fixture.admissionCalls, fixture.admissionSessionID, fixture.admissionExecutorID, fixture.admissionProfileID)
	}
	if got := fixture.taskRepo.tasks["task-1"].State; got != v1.TaskStateReview {
		t.Fatalf("task state after failed switch = %q, want prior REVIEW state", got)
	}
	if got := fixture.taskRepo.stateHistory["task-1"]; len(got) != 2 || got[0] != v1.TaskStateScheduling || got[1] != v1.TaskStateReview {
		t.Fatalf("task state writes = %v, want SCHEDULING followed by REVIEW rollback", got)
	}
	if len(publisher.updatedTaskIDs) != 1 || publisher.updatedTaskIDs[0] != "task-1" {
		t.Fatalf("published task updates = %v, want one update for task-1", publisher.updatedTaskIDs)
	}
	stored, getErr := fixture.repo.GetTaskSession(ctx, fixture.session.ID)
	if getErr != nil {
		t.Fatalf("get source session: %v", getErr)
	}
	if stored.State != models.TaskSessionStateWaitingForInput || !stored.IsPrimary {
		t.Fatalf("source session after failed switch = %#v, want recoverable primary session", stored)
	}
	sessions, listErr := fixture.repo.ListTaskSessions(ctx, fixture.session.TaskID)
	if listErr != nil {
		t.Fatalf("list sessions after failed switch: %v", listErr)
	}
	if len(sessions) != 1 || sessions[0].ID != fixture.session.ID {
		t.Fatalf("sessions after deleted-profile refusal = %#v, want only source session %q", sessions, fixture.session.ID)
	}
}

func TestSwitchSessionForStepRollbackDoesNotOverwriteConcurrentState(t *testing.T) {
	fixture := newWorkflowExecutorAdmissionFixture(t, v1.TaskStateReview, models.TaskSessionStateWaitingForInput)
	fixture.taskRepo.getTaskErr = errors.New("task preparation unavailable")
	concurrentWrite := false
	fixture.taskRepo.updateStateIfCurrentInHook = func(taskID string) {
		if concurrentWrite {
			return
		}
		concurrentWrite = true
		if err := fixture.taskRepo.UpdateTaskState(context.Background(), taskID, v1.TaskStateCompleted); err != nil {
			t.Errorf("concurrent task state write: %v", err)
		}
	}

	_, err := fixture.service.switchSessionForStep(context.Background(), "task-1", fixture.session, "profile-b")
	if err == nil {
		t.Fatal("switchSessionForStep() error = nil, want destination preparation failure")
	}
	if got := fixture.taskRepo.tasks["task-1"].State; got != v1.TaskStateCompleted {
		t.Fatalf("task state after concurrent write = %q, want COMPLETED", got)
	}
}

type workflowExecutorAdmissionFixture struct {
	repo interface {
		CreateTaskEnvironment(context.Context, *models.TaskEnvironment) error
		GetTask(context.Context, string) (*models.Task, error)
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
		ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
		UpdateTaskSession(context.Context, *models.TaskSession) error
	}
	service             *Service
	executor            *executor.Executor
	taskRepo            *mockTaskRepo
	session             *models.TaskSession
	admissionCalls      int
	admissionSessionID  string
	admissionExecutorID string
	admissionProfileID  string
}

func newWorkflowExecutorAdmissionFixture(
	t *testing.T,
	taskState v1.TaskState,
	sessionState models.TaskSessionState,
) *workflowExecutorAdmissionFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "Task", State: taskState, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: "exec-1", Name: "Local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
	}); err != nil {
		t.Fatalf("create executor: %v", err)
	}
	session := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", AgentProfileID: "profile-a",
		ExecutorID: "exec-1", ExecutorProfileID: "deleted-profile",
		State: sessionState, IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create source session: %v", err)
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-1", taskState)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	log := testLogger()
	exec := withTestCapabilities(executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{}))
	service := &Service{
		logger: log, repo: repo, workflowStepGetter: newMockStepGetter(), taskRepo: taskRepo,
		agentManager: agentMgr, messageQueue: messagequeue.NewServiceMemory(log), executor: exec,
	}
	service.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
	return &workflowExecutorAdmissionFixture{
		repo: repo, service: service, executor: exec, taskRepo: taskRepo, session: session,
	}
}

func workflowAdmissionTarget(profileID string) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID: "step-2", WorkflowID: "wf-1", AgentProfileID: profileID,
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}
}
