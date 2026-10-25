package executor

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestLaunchPreparedSession_RejectsInheritedEnvironmentExecutorMismatch(t *testing.T) {
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

	launched := false
	exec := newTestExecutor(t, &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launched = true
			return nil, nil
		},
	}, repo)
	task := &v1.Task{
		ID:          session.TaskID,
		WorkspaceID: "ws-1",
		Metadata: map[string]interface{}{
			"workspace": map[string]interface{}{"mode": "inherit_parent"},
		},
	}

	_, err := exec.LaunchPreparedSession(context.Background(), task, session.ID,
		LaunchOptions{AgentProfileID: session.AgentProfileID, ExecutorID: models.ExecutorIDLocalDocker, Prompt: "test"})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("LaunchPreparedSession() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if launched {
		t.Fatal("LaunchAgent was called for an inherited environment on a different executor")
	}
	if len(repo.createTaskEnvironmentCalls) != 0 {
		t.Fatalf("created %d task environments, want none", len(repo.createTaskEnvironmentCalls))
	}
	if got := repo.taskEnvironments[parent.ID]; got.TaskID != parent.TaskID || got.ExecutorType != parent.ExecutorType || got.WorkspacePath != parent.WorkspacePath {
		t.Fatalf("parent environment changed after rejected launch: %+v", got)
	}
	if got := repo.sessions[session.ID]; got.TaskEnvironmentID != parent.ID || got.WorkspacePath != parent.WorkspacePath {
		t.Fatalf("child session changed after rejected launch: %+v", got)
	}
}
