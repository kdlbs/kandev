package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func retiredTaskEnvironment(id, taskID string) *models.TaskEnvironment {
	retiredAt := time.Now().UTC()
	return &models.TaskEnvironment{
		ID: id, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusFailed, RetiredAt: &retiredAt,
	}
}

// A session still bound to a parent's retired environment must not attach a
// launch to that retained history, even though the parent itself is live.
func TestResolveLaunchTaskEnvironment_RefusesRetiredInheritedEnvironment(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["parent"] = &models.Task{ID: "parent"}
	repo.tasks["child"] = &models.Task{ID: "child", ParentID: "parent"}
	env := retiredTaskEnvironment("env-parent", "parent")
	repo.taskEnvironments[env.ID] = env
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "sess-child", TaskID: "child", TaskEnvironmentID: env.ID}

	got, err := e.resolveLaunchTaskEnvironment(context.Background(), &v1.Task{ID: "child"}, session)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("resolveLaunchTaskEnvironment() = %v, %v; want ErrWorkspaceReuseUnsafe for retired history", got, err)
	}
}

func TestValidateInheritedEnvironmentOwner_LiveOwnerStillAccepted(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["parent"] = &models.Task{ID: "parent"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	env := &models.TaskEnvironment{ID: "env-parent", TaskID: "parent", Status: models.TaskEnvironmentStatusReady}

	if err := e.validateInheritedEnvironmentOwner(context.Background(), &v1.Task{ID: "child"}, env); err != nil {
		t.Fatalf("validateInheritedEnvironmentOwner(active) = %v, want nil", err)
	}
}

// Worktree recovery admission selects an environment by its persisted ID; a
// historical session may still reference a retired row.
func TestResolveEnvironmentForAdmission_RefusesRetiredSelectedEnvironment(t *testing.T) {
	repo := newMockRepository()
	env := retiredTaskEnvironment("env-retired", "task-1")
	repo.taskEnvironments[env.ID] = env
	e := newTestExecutor(t, &mockAgentManager{}, repo)

	got, err := e.resolveEnvironmentForAdmission(context.Background(), "task-1", env.ID)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) || got != nil {
		t.Fatalf("resolveEnvironmentForAdmission() = %v, %v; want nil, ErrWorkspaceReuseUnsafe", got, err)
	}
}
