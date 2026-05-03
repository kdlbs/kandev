package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
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

	e.applyExistingEnvironment(req, nil)

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
		WorktreeID: "wt-1",
	}

	e.applyExistingEnvironment(req, env)

	if req.WorktreeID != "wt-1" {
		t.Errorf("expected WorktreeID=wt-1, got %s", req.WorktreeID)
	}
}

func TestApplyExistingEnvironment_SkipsReuseOnExecutorTypeMismatch(t *testing.T) {
	// Switching the task's executor profile to a different type must invalidate
	// reuse: stale PreviousExecutionID/ContainerID/sprite_name from the old
	// backend would otherwise leak into the new launch and overwrite the
	// persisted env with mixed resource IDs on the next save.
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{
		TaskID:       "task-1",
		ExecutorType: "local_docker",
		UseWorktree:  true,
	}
	env := &models.TaskEnvironment{
		ExecutorType:     "sprites",
		ContainerID:      "container-abc",
		WorktreeID:       "wt-1",
		AgentExecutionID: "exec-123",
	}

	e.applyExistingEnvironment(req, env)

	if req.WorktreeID != "" {
		t.Errorf("expected WorktreeID to be empty on executor mismatch, got %q", req.WorktreeID)
	}
	if req.PreviousExecutionID != "" {
		t.Errorf("expected PreviousExecutionID empty on mismatch, got %q", req.PreviousExecutionID)
	}
	if req.Metadata != nil {
		t.Errorf("expected nil metadata on mismatch, got %v", req.Metadata)
	}
}

func TestApplyExistingEnvironment_WorktreeSkippedWhenNotRequested(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: false}
	env := &models.TaskEnvironment{
		WorktreeID: "wt-1",
	}

	e.applyExistingEnvironment(req, env)

	if req.WorktreeID != "" {
		t.Errorf("expected empty WorktreeID when UseWorktree=false, got %s", req.WorktreeID)
	}
}

func TestApplyExistingEnvironment_ContainerReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		ContainerID: "container-abc",
	}

	e.applyExistingEnvironment(req, env)

	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["container_id"] != "container-abc" {
		t.Errorf("expected metadata container_id=container-abc, got %v", req.Metadata["container_id"])
	}
}

func TestApplyExistingEnvironmentRuntimeMetadata_CarriesPersistentSecrets(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	repo := newMockRepository()
	now := time.Now().UTC()
	repo.sessions["session-old"] = &models.TaskSession{
		ID:                "session-old",
		TaskID:            "task-1",
		TaskEnvironmentID: "env-1",
		StartedAt:         now,
		UpdatedAt:         now,
	}
	repo.executorsRunning["session-old"] = &models.ExecutorRunning{
		SessionID:        "session-old",
		AgentExecutionID: "exec-old",
		ContainerID:      "container-old",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret:      "secret-token",
			lifecycle.MetadataKeyBootstrapNonceSecret: "secret-nonce",
			"task_description":                        "drop me",
		},
	}
	e := &Executor{logger: log, repo: repo}
	req := &LaunchAgentRequest{TaskID: "task-1"}

	e.applyExistingEnvironmentRuntimeMetadata(context.Background(), req, &models.TaskEnvironment{
		ID: "env-1",
	})

	if req.PreviousExecutionID != "exec-old" {
		t.Fatalf("PreviousExecutionID = %q, want exec-old", req.PreviousExecutionID)
	}
	if req.Metadata[lifecycle.MetadataKeyContainerID] != "container-old" {
		t.Fatalf("container metadata = %v, want container-old", req.Metadata[lifecycle.MetadataKeyContainerID])
	}
	if req.Metadata[lifecycle.MetadataKeyAuthTokenSecret] != "secret-token" {
		t.Fatalf("auth token secret missing: %v", req.Metadata)
	}
	if req.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] != "secret-nonce" {
		t.Fatalf("bootstrap nonce secret missing: %v", req.Metadata)
	}
	if _, ok := req.Metadata["task_description"]; ok {
		t.Fatalf("launch-only metadata should be filtered out: %v", req.Metadata)
	}
}

func TestApplyExistingEnvironment_SandboxReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		SandboxID: "kandev-sprite-abc",
	}

	e.applyExistingEnvironment(req, env)

	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["sprite_name"] != "kandev-sprite-abc" {
		t.Errorf("expected metadata sprite_name=kandev-sprite-abc, got %v", req.Metadata["sprite_name"])
	}
}

func TestApplyExistingEnvironment_WorktreeAndContainer(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		WorktreeID:  "wt-1",
		ContainerID: "container-abc",
	}

	e.applyExistingEnvironment(req, env)

	if req.WorktreeID != "wt-1" {
		t.Errorf("expected WorktreeID=wt-1, got %s", req.WorktreeID)
	}
	if req.Metadata["container_id"] != "container-abc" {
		t.Errorf("expected metadata container_id=container-abc, got %v", req.Metadata["container_id"])
	}
	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
}

func TestApplyExistingEnvironment_EmptyEnvFieldsDoNothing(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{}

	e.applyExistingEnvironment(req, env)

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
