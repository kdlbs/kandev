package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestLaunchPreparedSession_InheritedRuntimeRetirementRetriesAfterFailure(t *testing.T) {
	fixture := newInheritedRuntimeRetirementFixture(t, models.TaskSessionStateCreated, models.ExecutorRunningStatusPrepared, false)
	runtimeAvailable := true
	cleanupErr := errors.New("injected runtime cleanup failure")
	var cleanupCalls int
	fixture.agentManager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		if runtimeAvailable {
			return "execution-inherited", nil
		}
		return "", ErrExecutionNotFound
	}
	cleanup := func(sessionID, executionID string, updatedAt time.Time) error {
		cleanupCalls++
		if sessionID != fixture.session.ID || executionID != "execution-inherited" || !updatedAt.Equal(fixture.running.UpdatedAt) {
			t.Fatalf("identity-aware cleanup got session=%q execution=%q updated_at=%s", sessionID, executionID, updatedAt)
		}
		if cleanupCalls == 1 {
			return cleanupErr
		}
		runtimeAvailable = false
		return nil
	}
	fixture.agentManager.cleanupStaleExecutionFunc = func(context.Context, string) error {
		return cleanup(fixture.session.ID, "execution-inherited", fixture.running.UpdatedAt)
	}
	fixture.agentManager.cleanupStaleExecutionIfCurrentFunc = func(_ context.Context, sessionID, executionID string, updatedAt time.Time) error {
		return cleanup(sessionID, executionID, updatedAt)
	}
	fixture.agentManager.launchAgentFunc = func(_ context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		if request.PreviousExecutionID != "" {
			t.Errorf("launch reused inherited execution %q after retirement", request.PreviousExecutionID)
		}
		if request.TaskEnvironmentID == fixture.inherited.ID || request.TaskEnvironmentID != fixture.repo.sessions[fixture.session.ID].TaskEnvironmentID {
			t.Errorf("launch environment = %q, want the persisted child environment", request.TaskEnvironmentID)
		}
		return &LaunchAgentResponse{
			AgentExecutionID: "execution-new",
			Status:           v1.AgentStatusStarting,
			WorkspacePath:    "/child/workspace",
		}, nil
	}

	_, err := fixture.executor.LaunchPreparedSession(context.Background(), fixture.task, fixture.session.ID,
		LaunchOptions{AgentProfileID: fixture.session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, StartAgent: true})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("first launch error = %v, want injected runtime cleanup failure", err)
	}
	canonicalID := fixture.repo.sessions[fixture.session.ID].TaskEnvironmentID
	if canonicalID == "" || canonicalID == fixture.inherited.ID {
		t.Fatalf("session binding after committed claim = %q, want a child-owned environment", canonicalID)
	}
	if !runtimeAvailable {
		t.Fatal("failed cleanup removed the inherited runtime")
	}

	execution, err := fixture.executor.LaunchPreparedSession(context.Background(), fixture.task, fixture.session.ID,
		LaunchOptions{AgentProfileID: fixture.session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, StartAgent: true})
	if err != nil {
		t.Fatalf("retry after cleanup failure: %v", err)
	}
	if cleanupCalls != 2 {
		t.Fatalf("identity-aware cleanup attempts = %d, want retry after the claim was already committed", cleanupCalls)
	}
	if fixture.agentManager.launchAgentCallCount != 1 || execution.AgentExecutionID != "execution-new" {
		t.Fatalf("retry reused inherited runtime: launches=%d execution=%q", fixture.agentManager.launchAgentCallCount, execution.AgentExecutionID)
	}
}

func TestLaunchPreparedSession_InheritedRuntimeRetirementFollowsExecutorRefusal(t *testing.T) {
	fixture := newInheritedRuntimeRetirementFixture(t, models.TaskSessionStateCreated, models.ExecutorRunningStatusPrepared, true)
	runtimeAvailable := true
	fixture.agentManager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		if runtimeAvailable {
			return "execution-inherited", nil
		}
		return "", ErrExecutionNotFound
	}
	cleanup := func(sessionID, executionID string, updatedAt time.Time) error {
		if sessionID != fixture.session.ID || executionID != "execution-inherited" || !updatedAt.Equal(fixture.running.UpdatedAt) {
			t.Fatalf("identity-aware cleanup got session=%q execution=%q updated_at=%s", sessionID, executionID, updatedAt)
		}
		runtimeAvailable = false
		return nil
	}
	fixture.agentManager.cleanupStaleExecutionFunc = func(context.Context, string) error {
		return cleanup(fixture.session.ID, "execution-inherited", fixture.running.UpdatedAt)
	}
	fixture.agentManager.cleanupStaleExecutionIfCurrentFunc = func(_ context.Context, sessionID, executionID string, updatedAt time.Time) error {
		return cleanup(sessionID, executionID, updatedAt)
	}
	fixture.agentManager.launchAgentFunc = func(_ context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		if request.PreviousExecutionID != "" {
			t.Errorf("launch reused inherited execution %q after executor-refusal retry", request.PreviousExecutionID)
		}
		if request.TaskEnvironmentID == fixture.inherited.ID || request.TaskEnvironmentID != fixture.repo.sessions[fixture.session.ID].TaskEnvironmentID {
			t.Errorf("launch environment = %q, want the persisted child environment", request.TaskEnvironmentID)
		}
		return &LaunchAgentResponse{AgentExecutionID: "execution-new", Status: v1.AgentStatusStarting}, nil
	}

	_, err := fixture.executor.LaunchPreparedSession(context.Background(), fixture.task, fixture.session.ID,
		LaunchOptions{AgentProfileID: fixture.session.AgentProfileID, ExecutorID: "executor-transition-ssh", StartAgent: true})
	if !errors.Is(err, models.ErrWorkspaceExecutorMismatch) {
		t.Fatalf("first launch error = %v, want selected-executor mismatch", err)
	}
	if fixture.agentManager.cleanupStaleExecutionCallCount != 0 || !runtimeAvailable {
		t.Fatalf("executor refusal retired the existing runtime: cleanup calls=%d runtime_available=%v",
			fixture.agentManager.cleanupStaleExecutionCallCount, runtimeAvailable)
	}

	execution, err := fixture.executor.LaunchPreparedSession(context.Background(), fixture.task, fixture.session.ID,
		LaunchOptions{AgentProfileID: fixture.session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, StartAgent: true})
	if err != nil {
		t.Fatalf("launch on canonical executor: %v", err)
	}
	if fixture.agentManager.cleanupStaleExecutionCallCount != 1 || runtimeAvailable {
		t.Fatalf("retry did not retire the mismatched runtime: cleanup calls=%d runtime_available=%v",
			fixture.agentManager.cleanupStaleExecutionCallCount, runtimeAvailable)
	}
	if fixture.agentManager.launchAgentCallCount != 1 || execution.AgentExecutionID != "execution-new" {
		t.Fatalf("retry reused inherited runtime: launches=%d execution=%q", fixture.agentManager.launchAgentCallCount, execution.AgentExecutionID)
	}
}

func TestLaunchPreparedSession_RefusesInheritedRuntimeWithoutEnvironmentIdentity(t *testing.T) {
	fixture := newInheritedRuntimeRetirementFixture(t, models.TaskSessionStateCreated, models.ExecutorRunningStatusPrepared, false)
	fixture.running.Metadata = nil
	fixture.agentManager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		return "execution-inherited", nil
	}

	_, err := fixture.executor.LaunchPreparedSession(context.Background(), fixture.task, fixture.session.ID,
		LaunchOptions{AgentProfileID: fixture.session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, StartAgent: true})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("launch error = %v, want unsafe refusal for a runtime without durable environment identity", err)
	}
	if fixture.agentManager.cleanupStaleExecutionCallCount != 0 || fixture.agentManager.launchAgentCallCount != 0 {
		t.Fatalf("unknown runtime was used or retired: cleanups=%d launches=%d", fixture.agentManager.cleanupStaleExecutionCallCount, fixture.agentManager.launchAgentCallCount)
	}
}

func TestLaunchPreparedSession_DoesNotRetireInheritedRuntimeBeforeAgentAdmission(t *testing.T) {
	fixture := newInheritedRuntimeRetirementFixture(t, models.TaskSessionStateRunning, models.ExecutorRunningStatusRunning, false)
	fixture.agentManager.isAgentRunningForSessionFunc = func(context.Context, string) bool { return true }

	_, err := fixture.executor.LaunchPreparedSession(context.Background(), fixture.task, fixture.session.ID,
		LaunchOptions{AgentProfileID: fixture.session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, StartAgent: true})
	if !errors.Is(err, ErrExecutionAlreadyRunning) {
		t.Fatalf("launch error = %v, want active-agent admission refusal", err)
	}
	if fixture.agentManager.cleanupStaleExecutionCallCount != 0 {
		t.Fatalf("retired runtime %d times before admission refusal, want zero", fixture.agentManager.cleanupStaleExecutionCallCount)
	}
}

type inheritedRuntimeRetirementFixture struct {
	repo         *mockRepository
	agentManager *mockAgentManager
	executor     *Executor
	task         *v1.Task
	session      *models.TaskSession
	inherited    *models.TaskEnvironment
	running      *models.ExecutorRunning
}

func newInheritedRuntimeRetirementFixture(
	t *testing.T,
	state models.TaskSessionState,
	runtimeStatus string,
	canonical bool,
) inheritedRuntimeRetirementFixture {
	t.Helper()
	repo := newMockRepository()
	parent := &models.Task{ID: "task-retirement-parent", WorkspaceID: "workspace-retirement"}
	child := &models.Task{ID: "task-retirement-child", WorkspaceID: parent.WorkspaceID, ParentID: parent.ID}
	repo.tasks[parent.ID] = parent
	repo.tasks[child.ID] = child
	inherited := &models.TaskEnvironment{
		ID: "environment-retirement-parent", TaskID: parent.ID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/parent/workspace",
	}
	repo.taskEnvironments[inherited.ID] = inherited
	if canonical {
		repo.taskEnvironments["environment-retirement-child"] = &models.TaskEnvironment{
			ID: "environment-retirement-child", TaskID: child.ID,
			ExecutorType: string(models.ExecutorTypeLocalDocker), Status: models.TaskEnvironmentStatusReady,
			WorkspacePath: "/child/workspace",
		}
	}
	for _, executor := range []*models.Executor{
		{ID: models.ExecutorIDLocalDocker, Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive},
		{ID: "executor-transition-ssh", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive},
	} {
		repo.executors[executor.ID] = executor
	}
	session := &models.TaskSession{
		ID: "session-retirement", TaskID: child.ID, AgentProfileID: "profile-retirement",
		TaskEnvironmentID: inherited.ID, WorkspacePath: inherited.WorkspacePath, State: state,
	}
	repo.sessions[session.ID] = session
	running := &models.ExecutorRunning{
		ID: session.ID, SessionID: session.ID, TaskID: child.ID, AgentExecutionID: "execution-inherited",
		Status: runtimeStatus, UpdatedAt: time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC),
		Metadata: map[string]interface{}{"task_environment_id": inherited.ID},
	}
	repo.executorsRunning[session.ID] = running
	agentManager := &mockAgentManager{}
	exec := newTestExecutor(t, agentManager, repo)
	task := &v1.Task{
		ID: child.ID, WorkspaceID: child.WorkspaceID,
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}
	return inheritedRuntimeRetirementFixture{
		repo: repo, agentManager: agentManager, executor: exec, task: task,
		session: session, inherited: inherited, running: running,
	}
}
