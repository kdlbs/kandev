package orchestrator

import (
	"context"
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

// sessionScopedPassthroughAgentManager keeps the source transport distinct
// from the destination profile in routing tests. A global passthrough flag can
// otherwise make the newly created destination session look like a TUI session.
type sessionScopedPassthroughAgentManager struct {
	*mockAgentManager
	passthroughSessionID string
}

func (m *sessionScopedPassthroughAgentManager) IsPassthroughSession(_ context.Context, sessionID string) bool {
	return sessionID == m.passthroughSessionID
}

func TestPrepareWorkflowStepSessionSwitchesPassthroughProfile(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step2")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	now := time.Now().UTC()
	session.AgentProfileID = "profile-a"
	session.ExecutorID = "exec-local"
	session.ExecutorProfileID = "executor-profile"
	session.IsPrimary = true
	session.TaskEnvironmentID = "env-1"
	session.State = models.TaskSessionStateRunning
	session.StartedAt = now
	session.UpdatedAt = now
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "t1", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}

	stepGetter := newMockStepGetter()
	sourceStep := &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", AgentProfileID: "profile-a",
	}
	step := &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", AgentProfileID: "profile-b",
	}
	stepGetter.steps[sourceStep.ID] = sourceStep
	stepGetter.steps[step.ID] = step
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress,
	}
	baseAgentManager := &mockAgentManager{}
	agentManager := &sessionScopedPassthroughAgentManager{
		mockAgentManager:     baseAgentManager,
		passthroughSessionID: session.ID,
	}
	log := testLogger()
	exec := executor.NewExecutor(agentManager, repo, log, executor.ExecutorConfig{})
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentManager)
	svc.logger = log
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(10), exec, taskRepo, log, scheduler.SchedulerConfig{})

	effective, switched, err := svc.prepareWorkflowStepSession(ctx, "t1", session, step, sourceStep)
	if err != nil {
		t.Fatalf("prepareWorkflowStepSession returned error: %v", err)
	}
	if !switched {
		t.Fatal("passthrough source session should switch to the fixed step profile")
	}
	if effective.AgentProfileID != step.AgentProfileID {
		t.Fatalf("effective profile = %q, want %q", effective.AgentProfileID, step.AgentProfileID)
	}
	if effective.TaskEnvironmentID != session.TaskEnvironmentID {
		t.Fatalf("destination environment = %q, want %q", effective.TaskEnvironmentID, session.TaskEnvironmentID)
	}
	persistedEffective, err := repo.GetTaskSession(ctx, effective.ID)
	if err != nil {
		t.Fatalf("reload destination session: %v", err)
	}
	if !persistedEffective.IsPrimary {
		t.Fatal("destination session must become primary")
	}

	old, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if old.State != models.TaskSessionStateCompleted {
		t.Fatalf("source session state = %s, want completed", old.State)
	}
}

func TestApplyEngineTransitionRejectsPassthroughTargetProfileBeforePersistingStep(t *testing.T) {
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
	baseAgentManager := &mockAgentManager{}
	agentManager := &sessionScopedPassthroughAgentManager{
		mockAgentManager:     baseAgentManager,
		passthroughSessionID: session.ID,
	}
	log := testLogger()
	exec := executor.NewExecutor(agentManager, repo, log, executor.ExecutorConfig{})
	// The custom forge host has no persisted provider_host identity, so
	// preflightWorkflowStepCredentials rejects it before the transition is
	// committed. The credential broker is not called on this failure path.
	exec.SetGitHubCredentialBroker(fakeCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve")
	svc := &Service{
		logger: log, repo: repo, workflowStepGetter: steps, taskRepo: taskRepo, agentManager: agentManager,
		messageQueue: messagequeue.NewServiceMemory(log), executor: exec,
		workflowStore: newWorkflowStore(repo, steps, agentManager, noopPublisher, log),
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
	storedSession, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.State != models.TaskSessionStateRunning || !storedSession.IsPrimary {
		t.Fatalf("session after rejection = %#v, want running primary source session", storedSession)
	}
}

type fakeCredentialIssuer struct{}

func (fakeCredentialIssuer) Issue(
	context.Context, gitcredentials.Scope,
) (gitcredentials.Lease, error) {
	return gitcredentials.Lease{Token: "opaque-lease"}, nil
}
