package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestResolveResumeTaskEnvironment_ReusesInheritedEnvironment is a regression
// test for "UNIQUE constraint failed: task_environments.id" on resume. A child
// task created by an office task-handoff (inherit_parent / shared_group) has its
// session.TaskEnvironmentID rewritten to point at the parent's / group's env
// row, which is owned by a *different* task. GetTaskEnvironmentByTaskID(childID)
// therefore returns nil, and without the inherited-env fallback the resume path
// treated the env as absent and re-created a row using the inherited ID, which
// already exists. The resolver must instead return the inherited row so the
// persist path takes its update branch.
func TestResolveResumeTaskEnvironment_ReusesInheritedEnvironment(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "task-parent",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-child", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env == nil || env.ID != "env-parent" {
		t.Fatalf("resolved env = %+v, want inherited env-parent", env)
	}
	if len(repo.createTaskEnvironmentCalls) != 0 {
		t.Fatalf("expected no CreateTaskEnvironment calls, got %d", len(repo.createTaskEnvironmentCalls))
	}
	if session.TaskEnvironmentID != "env-parent" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-parent", session.TaskEnvironmentID)
	}
}

// TestResolveResumeTaskEnvironment_MissingReferenceFallsThroughToCreate covers a
// session that references an env ID with no matching row (deleted env, or an env
// that was never persisted). The referenced ID is free, so the resolver returns
// nil and lets the caller create a fresh row using it — preserving the prior
// resume behavior rather than failing the resume.
func TestResolveResumeTaskEnvironment_MissingReferenceFallsThroughToCreate(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-missing",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-child", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil so the create path runs", env)
	}
}

// TestResolveResumeTaskEnvironment_OwnedEnvironmentUnchanged locks in the
// pre-existing behavior for a normal (non-inherited) session: the env is found
// by task id and no inherited fallback lookup occurs.
func TestResolveResumeTaskEnvironment_OwnedEnvironmentUnchanged(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.taskEnvironments["env-own"] = &models.TaskEnvironment{
		ID:           "env-own",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-1", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env == nil || env.ID != "env-own" {
		t.Fatalf("resolved env = %+v, want env-own", env)
	}
	if session.TaskEnvironmentID != "env-own" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-own", session.TaskEnvironmentID)
	}
}

// TestResolveResumeTaskEnvironment_NoEnvironmentAndNoReference returns nil so the
// caller can create a fresh environment (the ordinary cold-resume case).
func TestResolveResumeTaskEnvironment_NoEnvironmentAndNoReference(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-1", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}
