package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

func newEnvTestExecutor(t *testing.T) *Executor {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return &Executor{logger: log}
}

func TestApplyExistingEnvironment_NilEnv(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}

	e.applyExistingEnvironment(req, nil, LaunchOptions{})

	if req.Metadata != nil {
		t.Error("expected nil metadata for nil env")
	}
	if req.PreviousExecutionID != "" {
		t.Error("expected empty PreviousExecutionID for nil env")
	}
}

func TestApplyExistingEnvironment_WorktreeReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		WorktreeID:       "wt-1",
		AgentExecutionID: "exec-123",
	}

	e.applyExistingEnvironment(req, env, LaunchOptions{})

	if req.WorktreeID != "wt-1" {
		t.Errorf("expected WorktreeID=wt-1, got %s", req.WorktreeID)
	}
}

func TestApplyExistingEnvironment_WorktreeSkippedWhenNotRequested(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: false}
	env := &models.TaskEnvironment{
		WorktreeID:       "wt-1",
		AgentExecutionID: "exec-123",
	}

	e.applyExistingEnvironment(req, env, LaunchOptions{})

	if req.WorktreeID != "" {
		t.Errorf("expected empty WorktreeID when UseWorktree=false, got %s", req.WorktreeID)
	}
}

func TestApplyExistingEnvironment_ContainerReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		ContainerID:      "container-abc",
		AgentExecutionID: "exec-123",
	}

	e.applyExistingEnvironment(req, env, LaunchOptions{})

	if req.PreviousExecutionID != "exec-123" {
		t.Errorf("expected PreviousExecutionID=exec-123, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["container_id"] != "container-abc" {
		t.Errorf("expected metadata container_id=container-abc, got %v", req.Metadata["container_id"])
	}
}

func TestApplyExistingEnvironment_SandboxReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		SandboxID:        "kandev-sprite-abc",
		AgentExecutionID: "exec-456",
	}

	e.applyExistingEnvironment(req, env, LaunchOptions{})

	if req.PreviousExecutionID != "exec-456" {
		t.Errorf("expected PreviousExecutionID=exec-456, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["sprite_name"] != "kandev-sprite-abc" {
		t.Errorf("expected metadata sprite_name=kandev-sprite-abc, got %v", req.Metadata["sprite_name"])
	}
}

func TestApplyExistingEnvironment_WorktreeAndContainer(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		WorktreeID:       "wt-1",
		ContainerID:      "container-abc",
		AgentExecutionID: "exec-123",
	}

	e.applyExistingEnvironment(req, env, LaunchOptions{})

	if req.WorktreeID != "wt-1" {
		t.Errorf("expected WorktreeID=wt-1, got %s", req.WorktreeID)
	}
	if req.Metadata["container_id"] != "container-abc" {
		t.Errorf("expected metadata container_id=container-abc, got %v", req.Metadata["container_id"])
	}
	if req.PreviousExecutionID != "exec-123" {
		t.Errorf("expected PreviousExecutionID=exec-123, got %s", req.PreviousExecutionID)
	}
}

func TestApplyExistingEnvironment_FreshSkipsAll(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		WorktreeID:       "wt-1",
		ContainerID:      "container-abc",
		SandboxID:        "sandbox-xyz",
		AgentExecutionID: "exec-789",
	}

	e.applyExistingEnvironment(req, env, LaunchOptions{FreshEnvironment: true})

	if req.WorktreeID != "" {
		t.Errorf("expected empty WorktreeID when FreshEnvironment=true, got %s", req.WorktreeID)
	}
	if req.Metadata != nil {
		t.Errorf("expected nil metadata when FreshEnvironment=true, got %v", req.Metadata)
	}
	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID when FreshEnvironment=true, got %s", req.PreviousExecutionID)
	}
}

func TestApplyExistingEnvironment_EmptyEnvFieldsDoNothing(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		AgentExecutionID: "exec-123",
		// No worktree, container, or sandbox
	}

	e.applyExistingEnvironment(req, env, LaunchOptions{})

	if req.Metadata != nil {
		t.Error("expected nil metadata when no container/sandbox IDs")
	}
	if req.PreviousExecutionID != "" {
		t.Error("expected empty PreviousExecutionID when no container/sandbox IDs")
	}
}

func TestExtractSandboxID(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     string
	}{
		{"nil metadata", nil, ""},
		{"no sprite_name", map[string]interface{}{"other": "val"}, ""},
		{"with sprite_name", map[string]interface{}{"sprite_name": "kandev-abc"}, "kandev-abc"},
		{"non-string sprite_name", map[string]interface{}{"sprite_name": 42}, ""},
		{"empty sprite_name", map[string]interface{}{"sprite_name": ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSandboxID(tt.metadata)
			if got != tt.want {
				t.Errorf("extractSandboxID() = %q, want %q", got, tt.want)
			}
		})
	}
}
