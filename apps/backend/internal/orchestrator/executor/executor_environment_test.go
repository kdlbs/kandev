package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newEnvTestExecutor(t *testing.T) *Executor {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return &Executor{logger: log}
}

func TestReuseExistingEnvironment_NilEnv(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}

	e.reuseExistingEnvironment(context.Background(), req, nil)

	if req.Metadata != nil {
		t.Error("expected nil metadata for nil env")
	}
	if req.PreviousExecutionID != "" {
		t.Error("expected empty PreviousExecutionID for nil env")
	}
}

func TestReuseExistingEnvironment_WorktreeReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		WorktreeID: "wt-1",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "wt-1" {
		t.Errorf("expected WorktreeID=wt-1, got %s", req.WorktreeID)
	}
}

func TestReuseExistingEnvironment_SkipsReuseOnExecutorTypeMismatch(t *testing.T) {
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
		ExecutorType: "sprites",
		ContainerID:  "container-abc",
		WorktreeID:   "wt-1",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

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

func TestReuseExistingEnvironment_WorktreeSkippedWhenNotRequested(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: false}
	env := &models.TaskEnvironment{
		WorktreeID: "wt-1",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "" {
		t.Errorf("expected empty WorktreeID when UseWorktree=false, got %s", req.WorktreeID)
	}
}

func TestReuseExistingEnvironment_ContainerReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		ContainerID: "container-abc",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["container_id"] != "container-abc" {
		t.Errorf("expected metadata container_id=container-abc, got %v", req.Metadata["container_id"])
	}
}

func TestReuseExistingEnvironment_DockerBranchReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", ExecutorType: "local_docker"}
	env := &models.TaskEnvironment{
		ExecutorType:   "local_docker",
		ContainerID:    "container-abc",
		WorktreeBranch: "feature/existing-task-abc",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.Metadata[lifecycle.MetadataKeyWorktreeBranch] != "feature/existing-task-abc" {
		t.Fatalf("metadata worktree_branch = %v, want existing branch", req.Metadata[lifecycle.MetadataKeyWorktreeBranch])
	}
}

func TestReuseExistingEnvironment_RuntimeMetadata_CarriesPersistentSecrets(t *testing.T) {
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

	e.reuseExistingEnvironment(context.Background(), req, &models.TaskEnvironment{
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

func TestReuseExistingEnvironment_RuntimeMetadata_FallsBackToMatchingContainer(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	repo := newMockRepository()
	now := time.Now().UTC()
	repo.sessions["session-old"] = &models.TaskSession{
		ID:        "session-old",
		TaskID:    "task-1",
		StartedAt: now,
		UpdatedAt: now,
	}
	repo.executorsRunning["session-old"] = &models.ExecutorRunning{
		SessionID:        "session-old",
		AgentExecutionID: "exec-old",
		ContainerID:      "container-old",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret:      "secret-token",
			lifecycle.MetadataKeyBootstrapNonceSecret: "secret-nonce",
		},
	}
	e := &Executor{logger: log, repo: repo}
	req := &LaunchAgentRequest{TaskID: "task-1"}

	e.reuseExistingEnvironment(context.Background(), req, &models.TaskEnvironment{
		ID:          "env-1",
		ContainerID: "container-old",
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
}

func TestBuildResumeRequest_ReusesTaskEnvironmentRuntimeMetadata(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	exec := newTestExecutor(t, agentManager, repo)
	now := time.Now().UTC()
	task := &v1.Task{
		ID:          "task-1",
		WorkspaceID: "workspace-1",
		Title:       "Task 1",
	}
	session := &models.TaskSession{
		ID:                "session-new",
		TaskID:            "task-1",
		AgentProfileID:    "profile-1",
		ExecutorID:        models.ExecutorIDLocalDocker,
		TaskEnvironmentID: "env-1",
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	repo.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID:        models.ExecutorIDLocalDocker,
		Type:      models.ExecutorTypeLocalDocker,
		Status:    models.ExecutorStatusActive,
		Resumable: true,
	}
	repo.taskEnvironments["env-1"] = &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		ContainerID:  "container-old",
		Status:       models.TaskEnvironmentStatusReady,
	}
	repo.sessions["session-old"] = &models.TaskSession{
		ID:                "session-old",
		TaskID:            "task-1",
		TaskEnvironmentID: "env-1",
		StartedAt:         now.Add(-time.Minute),
		UpdatedAt:         now.Add(-time.Minute),
	}
	repo.executorsRunning["session-old"] = &models.ExecutorRunning{
		SessionID:        "session-old",
		TaskID:           "task-1",
		AgentExecutionID: "exec-old",
		ContainerID:      "container-old",
		Runtime:          string(models.ExecutorTypeLocalDocker),
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret: "secret-token",
			"task_description":                   "drop me",
		},
	}

	req, _, _, running, err := exec.buildResumeRequest(context.Background(), task, session, true)
	if err != nil {
		t.Fatalf("buildResumeRequest returned error: %v", err)
	}

	if running != nil {
		t.Fatalf("current session should not have an ExecutorRunning row")
	}
	if req.TaskEnvironmentID != "env-1" {
		t.Fatalf("TaskEnvironmentID = %q, want env-1", req.TaskEnvironmentID)
	}
	if req.PreviousExecutionID != "exec-old" {
		t.Fatalf("PreviousExecutionID = %q, want latest environment execution exec-old", req.PreviousExecutionID)
	}
	if req.Metadata[lifecycle.MetadataKeyContainerID] != "container-old" {
		t.Fatalf("container metadata = %v, want container-old", req.Metadata[lifecycle.MetadataKeyContainerID])
	}
	if req.Metadata[lifecycle.MetadataKeyAuthTokenSecret] != "secret-token" {
		t.Fatalf("auth token secret missing: %v", req.Metadata)
	}
	if _, ok := req.Metadata["task_description"]; ok {
		t.Fatalf("launch-only metadata should be filtered out: %v", req.Metadata)
	}
}

func TestReuseExistingEnvironment_SandboxReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		SandboxID: "kandev-sprite-abc",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["sprite_name"] != "kandev-sprite-abc" {
		t.Errorf("expected metadata sprite_name=kandev-sprite-abc, got %v", req.Metadata["sprite_name"])
	}
}

func TestReuseExistingEnvironment_WorktreeAndContainer(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		WorktreeID:  "wt-1",
		ContainerID: "container-abc",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

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

func TestReuseExistingEnvironment_EmptyEnvFieldsDoNothing(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{}

	e.reuseExistingEnvironment(context.Background(), req, env)

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
