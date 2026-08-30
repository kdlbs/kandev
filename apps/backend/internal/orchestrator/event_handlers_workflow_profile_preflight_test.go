package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeSwitchSessionCredentialIssuer struct{}

func (fakeSwitchSessionCredentialIssuer) Issue(
	context.Context, gitcredentials.Scope,
) (gitcredentials.Lease, error) {
	return gitcredentials.Lease{Token: "opaque-lease"}, nil
}

// TestSwitchSessionForStep_RejectsIrreparableManagedCredentialRepository
// covers AC-8/AC-9 at the profile-switch/re-entry seam: a task bound to a
// repository whose managed Git credential identity is irreparable (a custom
// host with no trustworthy provider host metadata) must reject the switch
// before mutating either session's ownership - not stop the still-working
// current session only to hand the task to a session that can never launch.
func TestSwitchSessionForStep_RejectsIrreparableManagedCredentialRepository(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	task := &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step2",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	repository := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "widgets", SourceType: "local",
		Provider: "acme-forge", RemoteURL: "https://forge.example/acme/widgets.git",
	}
	if err := repo.CreateRepository(ctx, repository); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "taskrepo1", TaskID: "t1", RepositoryID: "repo1",
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}

	session := &models.TaskSession{
		ID: "s1", TaskID: "t1", AgentProfileID: "profile-a",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", AgentExecutionID: "ae1",
		State: models.TaskSessionStateRunning, IsPrimary: true,
		StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create task session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	exec.SetGitHubCredentialBroker(fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve")
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
	svc := &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: newMockStepGetter(),
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewServiceMemory(log),
		executor:           exec,
		scheduler:          sched,
	}

	_, err := svc.switchSessionForStep(ctx, "t1", session, "profile-b")
	if err == nil {
		t.Fatal("switchSessionForStep() error = nil, want rejection of the irreparable repository binding")
	}
	if !strings.Contains(err.Error(), "repo1") {
		t.Fatalf("error %q does not name the safe repository id repo1", err.Error())
	}

	// The current session must be untouched: still RUNNING and still primary.
	oldSession, getErr := repo.GetTaskSession(ctx, "s1")
	if getErr != nil {
		t.Fatalf("get old session: %v", getErr)
	}
	if oldSession.State != models.TaskSessionStateRunning {
		t.Errorf("old session state = %s, want RUNNING (must not be touched by a rejected switch)", oldSession.State)
	}
	if !oldSession.IsPrimary {
		t.Error("old session must still be primary after a rejected switch")
	}

	// No replacement session for profile-b was created.
	sessions, listErr := repo.ListTaskSessions(ctx, "t1")
	if listErr != nil {
		t.Fatalf("list task sessions: %v", listErr)
	}
	if len(sessions) != 1 {
		t.Fatalf("task session count = %d, want 1 (no doomed replacement session)", len(sessions))
	}
}

func TestApplyEngineTransitionRejectsTargetProfileBeforePersistingStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "widgets", SourceType: "local",
		Provider: "acme-forge", RemoteURL: "https://forge.example/acme/widgets.git",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "taskrepo1", TaskID: "t1", RepositoryID: "repo1",
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session := &models.TaskSession{
		ID: "s1", TaskID: "t1", AgentProfileID: "profile-a", ExecutorID: "exec-local",
		State: models.TaskSessionStateRunning, IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create task session: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Position: 1}
	steps.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Position: 2, AgentProfileID: "profile-b",
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Test", State: v1.TaskStateInProgress,
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	exec.SetGitHubCredentialBroker(fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve")
	svc := &Service{
		logger: log, repo: repo, workflowStepGetter: steps, taskRepo: taskRepo, agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log), executor: exec,
		workflowStore: newWorkflowStore(repo, steps, agentMgr, noopPublisher, log),
	}

	applied := svc.applyEngineTransition(ctx, "t1", session, engine.HandleResult{
		Transitioned: true, FromStepID: "step1", ToStepID: "step2",
	}, engine.TriggerOnTurnStart, "", false)
	if applied {
		t.Fatal("applyEngineTransition() = true, want credential admission rejection")
	}
	storedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if storedTask.WorkflowStepID != "step1" {
		t.Fatalf("workflow step = %q, want source step1", storedTask.WorkflowStepID)
	}
	storedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.State != models.TaskSessionStateRunning || !storedSession.IsPrimary {
		t.Fatalf("session after rejection = %#v, want running primary source session", storedSession)
	}
}

func TestSwitchSessionForStepUsesReusableSessionExecutorProfileForCredentialAdmission(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Test", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "widgets", Provider: "acme-forge",
		RemoteURL: "https://forge.example/acme/widgets.git",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "taskrepo1", TaskID: "t1", RepositoryID: "repo1",
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: "exec-ssh", Name: "SSH", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive,
	}); err != nil {
		t.Fatalf("create executor: %v", err)
	}
	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "ep-token", ExecutorID: "exec-ssh", Name: "Token profile",
		Config: map[string]string{"remote_auth_secrets": `{"gh_cli_env":"secret-gh"}`},
	}); err != nil {
		t.Fatalf("create executor profile: %v", err)
	}
	current := &models.TaskSession{
		ID: "s-current", TaskID: "t1", AgentProfileID: "profile-a", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}
	target := &models.TaskSession{
		ID: "s-target", TaskID: "t1", AgentProfileID: "profile-b", ExecutorID: "exec-ssh",
		ExecutorProfileID: "ep-token", State: models.TaskSessionStateWaitingForInput,
		StartedAt: now.Add(-time.Minute), UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, current); err != nil {
		t.Fatalf("create current session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, target); err != nil {
		t.Fatalf("create target session: %v", err)
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	exec.SetGitHubCredentialBroker(fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve")
	svc := &Service{
		logger: log, repo: repo, taskRepo: taskRepo, agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log), executor: exec,
	}

	got, err := svc.switchSessionForStep(ctx, "t1", current, "profile-b")
	if err != nil {
		t.Fatalf("switchSessionForStep() error = %v, want reusable profile token override", err)
	}
	if got.ID != target.ID {
		t.Fatalf("session = %q, want reusable target %q", got.ID, target.ID)
	}
}
