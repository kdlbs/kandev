package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A child session that inherited its parent's environment must still launch
// when it is elected onto a different executor from the one that materialized
// the inherited environment. Rejecting the launch strands the child: it cannot
// attach to a parent environment that lives on one executor while being elected
// onto another. Instead the child detaches and materializes a fresh environment
// on its elected executor, leaving the parent's environment untouched.
func TestLaunchPreparedSession_DetachesInheritedEnvironmentOnExecutorMismatch(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-parent"] = &models.Task{ID: "task-parent"}
	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:            "env-parent",
		TaskID:        "task-parent",
		ExecutorType:  string(models.ExecutorTypeLocal),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: "/parent/workspace",
	}
	repo.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID:     models.ExecutorIDLocalDocker,
		Type:   models.ExecutorTypeLocalDocker,
		Status: models.ExecutorStatusActive,
	}
	session := &models.TaskSession{
		ID:                "session-child",
		TaskID:            "task-child",
		AgentProfileID:    "profile-123",
		TaskEnvironmentID: "env-parent",
		WorkspacePath:     "/parent/workspace",
		State:             models.TaskSessionStateCreated,
		StartedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	repo.sessions[session.ID] = session

	var launchedExecutorType, launchedTaskEnvironmentID, launchedWorkspacePath string
	var launchedWorkspaceReuseRequired bool
	exec := newTestExecutor(t, &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchedExecutorType = req.ExecutorType
			launchedTaskEnvironmentID = req.TaskEnvironmentID
			launchedWorkspacePath = req.WorkspacePath
			launchedWorkspaceReuseRequired = req.WorkspaceReuseRequired
			return &LaunchAgentResponse{AgentExecutionID: "exec-child", Status: v1.AgentStatusStarting}, nil
		},
	}, repo)

	_, err := exec.LaunchPreparedSession(context.Background(),
		&v1.Task{ID: session.TaskID, WorkspaceID: "ws-1"}, session.ID,
		LaunchOptions{AgentProfileID: session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, Prompt: "test"})
	if err != nil {
		t.Fatalf("LaunchPreparedSession() error = %v, want nil after detaching inherited environment", err)
	}
	if launchedExecutorType != string(models.ExecutorTypeLocalDocker) {
		t.Fatalf("launched executor type = %q, want %q", launchedExecutorType, models.ExecutorTypeLocalDocker)
	}
	if launchedTaskEnvironmentID == "" || launchedTaskEnvironmentID == "env-parent" {
		t.Fatalf("launched task environment ID = %q, want a fresh child environment", launchedTaskEnvironmentID)
	}
	if launchedWorkspacePath != "" {
		t.Fatalf("launched workspace path = %q, want stale inherited path cleared", launchedWorkspacePath)
	}
	if launchedWorkspaceReuseRequired {
		t.Fatal("detached child launch must not require reuse of the inherited environment")
	}
	if len(repo.createTaskEnvironmentCalls) != 1 {
		t.Fatalf("created %d task environments, want 1 fresh environment for the detached child", len(repo.createTaskEnvironmentCalls))
	}
	created := repo.createTaskEnvironmentCalls[0]
	if created.TaskID != session.TaskID {
		t.Fatalf("created environment TaskID = %q, want %q (child owns its detach), not the parent's %q", created.TaskID, session.TaskID, "task-parent")
	}
	if created.ExecutorType != string(models.ExecutorTypeLocalDocker) {
		t.Fatalf("created environment ExecutorType = %q, want %q", created.ExecutorType, models.ExecutorTypeLocalDocker)
	}
	if created.ID == "env-parent" {
		t.Fatalf("detached child re-bound to the parent environment %q", created.ID)
	}
	if parent := repo.taskEnvironments["env-parent"]; parent.TaskID != "task-parent" || parent.ExecutorType != string(models.ExecutorTypeLocal) || parent.WorkspacePath != "/parent/workspace" {
		t.Fatalf("parent environment changed during detach: %+v", parent)
	}
	persistedSession := repo.sessions[session.ID]
	if persistedSession.TaskEnvironmentID != created.ID {
		t.Fatalf("persisted child session environment = %q, want %q", persistedSession.TaskEnvironmentID, created.ID)
	}
	if persistedSession.WorkspacePath != created.WorkspacePath {
		t.Fatalf("persisted child session workspace path = %q, want %q", persistedSession.WorkspacePath, created.WorkspacePath)
	}
}

// An inherit_parent child whose inherited environment lives on a different
// executor than the one the launch selected cannot attach to it. Rejecting the
// launch strands the child, so it detaches and materializes a fresh environment
// on the selected executor, leaving the inherited environment untouched. The
// Kubernetes unsupported-reuse refusal is covered separately.
// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.5
func TestLaunchPreparedSession_DetachesInheritedPolicyEnvironmentOnExecutorMismatch(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-parent"] = &models.Task{ID: "task-parent"}
	parent := &models.TaskEnvironment{
		ID:            "env-parent",
		TaskID:        "task-parent",
		ExecutorType:  string(models.ExecutorTypeLocal),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: "/parent/workspace",
	}
	repo.taskEnvironments[parent.ID] = parent
	repo.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID:     models.ExecutorIDLocalDocker,
		Type:   models.ExecutorTypeLocalDocker,
		Status: models.ExecutorStatusActive,
	}
	session := &models.TaskSession{
		ID:                "session-child",
		TaskID:            "task-child",
		AgentProfileID:    "profile-123",
		TaskEnvironmentID: parent.ID,
		WorkspacePath:     parent.WorkspacePath,
		State:             models.TaskSessionStateCreated,
		StartedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	repo.sessions[session.ID] = session
	runningUpdatedAt := time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		ID:               "runtime-child",
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		AgentExecutionID: "execution-parent-runtime",
		Status:           models.ExecutorRunningStatusPrepared,
		UpdatedAt:        runningUpdatedAt,
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyTaskEnvironmentID: parent.ID,
		},
	}

	var launchedExecutorType, launchedTaskEnvironmentID string
	var launchedWorkspaceReuseRequired bool
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			claimed, claimErr := repo.GetTaskEnvironmentByTaskID(ctx, session.TaskID)
			if claimErr != nil {
				t.Errorf("load child environment before physical launch: %v", claimErr)
			} else if claimed == nil || claimed.Status != models.TaskEnvironmentStatusCreating || claimed.MaterializationSessionID != session.ID {
				t.Errorf("child environment before physical launch = %+v, want a creating claim owned by %q", claimed, session.ID)
			}
			bound, bindErr := repo.GetTaskSession(ctx, session.ID)
			if bindErr != nil {
				t.Errorf("load child session before physical launch: %v", bindErr)
			} else if bound == nil || bound.TaskEnvironmentID == "" || (claimed != nil && bound.TaskEnvironmentID != claimed.ID) || bound.WorkspacePath != "" {
				t.Errorf("child session before physical launch = %+v, want its child environment and no inherited workspace path", bound)
			}
			launchedExecutorType = req.ExecutorType
			launchedTaskEnvironmentID = req.TaskEnvironmentID
			launchedWorkspaceReuseRequired = req.WorkspaceReuseRequired
			return &LaunchAgentResponse{AgentExecutionID: "exec-child", Status: v1.AgentStatusStarting}, nil
		},
	}
	agentManager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		return "", ErrExecutionNotFound
	}
	agentManager.cleanupStaleExecutionIfCurrentFunc = func(_ context.Context, sessionID, executionID string, updatedAt time.Time) error {
		if sessionID != session.ID || executionID != "execution-parent-runtime" || !updatedAt.Equal(runningUpdatedAt) {
			t.Fatalf("identity-aware runtime cleanup got session=%q execution=%q updated_at=%s", sessionID, executionID, updatedAt)
		}
		delete(repo.executorsRunning, session.ID)
		return nil
	}
	exec := newTestExecutor(t, agentManager, repo)
	task := &v1.Task{
		ID:          session.TaskID,
		WorkspaceID: "ws-1",
		Metadata: map[string]interface{}{
			"workspace": map[string]interface{}{"mode": "inherit_parent"},
		},
	}

	_, err := exec.LaunchPreparedSession(context.Background(), task, session.ID,
		LaunchOptions{AgentProfileID: session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, Prompt: "test"})
	if err != nil {
		t.Fatalf("LaunchPreparedSession() error = %v, want nil after detaching inherited policy environment", err)
	}
	if launchedExecutorType != string(models.ExecutorTypeLocalDocker) {
		t.Fatalf("launched executor type = %q, want %q", launchedExecutorType, models.ExecutorTypeLocalDocker)
	}
	if launchedTaskEnvironmentID == "" || launchedTaskEnvironmentID == parent.ID {
		t.Fatalf("launched task environment ID = %q, want a fresh child environment", launchedTaskEnvironmentID)
	}
	if launchedWorkspaceReuseRequired {
		t.Fatal("detached child launch must not require reuse of the inherited environment")
	}
	if len(repo.createTaskEnvironmentCalls) != 1 {
		t.Fatalf("created %d task environments, want 1 fresh environment for the detached child", len(repo.createTaskEnvironmentCalls))
	}
	if created := repo.createTaskEnvironmentCalls[0]; created.TaskID != session.TaskID || created.ExecutorType != string(models.ExecutorTypeLocalDocker) {
		t.Fatalf("created environment = %+v, want a fresh local_docker environment owned by the child", created)
	}
	if got := repo.taskEnvironments[parent.ID]; got.TaskID != parent.TaskID || got.ExecutorType != parent.ExecutorType || got.WorkspacePath != parent.WorkspacePath {
		t.Fatalf("parent environment changed after detached launch: %+v", got)
	}
	if agentManager.cleanupStaleExecutionCallCount != 1 {
		t.Fatalf("retired stale runtime %d times, want exactly once", agentManager.cleanupStaleExecutionCallCount)
	}
	if _, ok := repo.executorsRunning[session.ID]; ok {
		t.Fatal("old inherited runtime row survived the executor transition")
	}
}
