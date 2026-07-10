package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
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

func TestReuseExistingEnvironment_WorktreeReuseKeepsTaskDirName(t *testing.T) {
	repo := newMockRepository()
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:      "task-1",
		UseWorktree: true,
		TaskDirName: "fresh-task-dir",
		Repositories: []RepoSpec{
			{RepositoryID: "repo-kandev", BranchIdentitySlug: "main"},
			{RepositoryID: "repo-docs", BranchIdentitySlug: "main"},
		},
	}
	env := &models.TaskEnvironment{
		ID:          "env-existing",
		TaskDirName: "persisted-task-dir",
		Repos: []*models.TaskEnvironmentRepo{
			{TaskEnvironmentID: "env-existing", RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-kandev"},
		},
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.TaskDirName != "persisted-task-dir" {
		t.Fatalf("TaskDirName = %q, want persisted-task-dir", req.TaskDirName)
	}
	if req.Repositories[0].WorktreeID != "wt-kandev" {
		t.Fatalf("first repo WorktreeID = %q, want wt-kandev", req.Repositories[0].WorktreeID)
	}
	if req.Repositories[1].WorktreeID != "" {
		t.Fatalf("second repo WorktreeID = %q, want empty for new checkout", req.Repositories[1].WorktreeID)
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
		Runtime:          models.ExecutorTypeLocalDocker.Runtime(),
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret: "secret-token",
			"task_description":                   "drop me",
		},
	}

	req, _, _, _, running, err := exec.buildResumeRequest(context.Background(), task, session, true)
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

// TestApplyExecutorRunningMetadata_SkipsSessionScopedKeys pins the guard
// that prevents a SECOND session on the same task from inheriting the FIRST
// session's session-scoped runtime resources — agentctl PID/port, remote
// session dir, local forward port. Without this filter, the SSH executor's
// ResumeRemoteInstance would interpret those keys as a resume hint and
// reattach to session-1's agentctl process, so session 2 would end up
// sharing session 1's ACP session and instance port and never finish its
// own initialize.
//
// Connection / task-environment-wide keys (host, port, user, fingerprint,
// remote task dir, workdir root, proxy jump) MUST still propagate so the
// second session connects to the same host and reuses the task dir.
func TestApplyExecutorRunningMetadata_SkipsSessionScopedKeys(t *testing.T) {
	req := &LaunchAgentRequest{TaskID: "task-1"}
	running := &models.ExecutorRunning{
		AgentExecutionID: "exec-prev",
		Metadata: map[string]interface{}{
			// Connection config — should propagate.
			lifecycle.MetadataKeySSHHost:            "example.com",
			lifecycle.MetadataKeySSHPort:            "2200",
			lifecycle.MetadataKeySSHUser:            "deploy",
			lifecycle.MetadataKeySSHHostFingerprint: "SHA256:aaa",
			lifecycle.MetadataKeySSHRemoteTaskDir:   "/home/deploy/.kandev/tasks/task-1",
			lifecycle.MetadataKeySSHWorkdirRoot:     "/home/deploy/.kandev",
			lifecycle.MetadataKeySSHProxyJump:       "bastion",
			// Session-scoped runtime resources — must NOT propagate.
			lifecycle.MetadataKeySSHRemoteSessionDir:   "/home/deploy/.kandev/tasks/task-1/.kandev/sessions/sess-1",
			lifecycle.MetadataKeySSHRemoteAgentctlPort: "41001",
			lifecycle.MetadataKeySSHRemoteAgentctlPID:  "12345",
			lifecycle.MetadataKeySSHLocalForwardPort:   "59123",
			lifecycle.MetadataKeySSHRemoteAgentctlURL:  "http://127.0.0.1:59123",
			// Non-persistent key — must NOT propagate (not in persistentMetadataKeys).
			"task_description": "session 1 prompt",
		},
	}

	applyExecutorRunningMetadata(req, running)

	if req.PreviousExecutionID != "exec-prev" {
		t.Errorf("PreviousExecutionID = %q, want exec-prev", req.PreviousExecutionID)
	}
	if req.Metadata == nil {
		t.Fatal("req.Metadata is nil; expected propagated keys")
	}

	propagated := []string{
		lifecycle.MetadataKeySSHHost,
		lifecycle.MetadataKeySSHPort,
		lifecycle.MetadataKeySSHUser,
		lifecycle.MetadataKeySSHHostFingerprint,
		lifecycle.MetadataKeySSHRemoteTaskDir,
		lifecycle.MetadataKeySSHWorkdirRoot,
		lifecycle.MetadataKeySSHProxyJump,
	}
	for _, k := range propagated {
		if _, ok := req.Metadata[k]; !ok {
			t.Errorf("expected connection key %q to propagate", k)
		}
	}

	sessionScoped := []string{
		lifecycle.MetadataKeySSHRemoteSessionDir,
		lifecycle.MetadataKeySSHRemoteAgentctlPort,
		lifecycle.MetadataKeySSHRemoteAgentctlPID,
		lifecycle.MetadataKeySSHLocalForwardPort,
		lifecycle.MetadataKeySSHRemoteAgentctlURL,
	}
	for _, k := range sessionScoped {
		if v, ok := req.Metadata[k]; ok {
			t.Errorf("session-scoped key %q leaked into sibling-session request (value=%v)", k, v)
		}
		if !lifecycle.IsSessionScopedMetadataKey(k) {
			t.Errorf("IsSessionScopedMetadataKey(%q) = false; expected true", k)
		}
	}

	// task_description is not in persistentMetadataKeys at all, so the
	// pre-existing ShouldPersistMetadataKey gate already drops it.
	if _, ok := req.Metadata["task_description"]; ok {
		t.Error("task_description (non-persistent) leaked into sibling-session request")
	}
}

// TestApplyExecutorRunningMetadata_RequestKeysWin documents that an explicit
// value already on the request (e.g. set by the caller from launch options)
// is not overwritten by the previous ExecutorRunning record. This applies to
// every persistent key, not just the connection ones.
func TestApplyExecutorRunningMetadata_RequestKeysWin(t *testing.T) {
	req := &LaunchAgentRequest{
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeySSHHost: "user-override.example.com",
		},
	}
	running := &models.ExecutorRunning{
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeySSHHost: "stale.example.com",
		},
	}

	applyExecutorRunningMetadata(req, running)

	if got := req.Metadata[lifecycle.MetadataKeySSHHost]; got != "user-override.example.com" {
		t.Errorf("ssh_host = %q, want user-override.example.com (request value should win)", got)
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

// TestApplyRepositoryConfig_PropagatesRepositoryID asserts that
// applyRepositoryConfig copies RepositoryID from the resolved repoInfo onto
// the launch request. The lifecycle layer carries this field through to the
// worktree manager's runWorktreeSetupScript, which uses it to look up the
// repository's setup script. When the field is empty the manager silently
// skips the script — manifesting as "the start script is not run" for the
// user who configured one on their repo.
func TestApplyRepositoryConfig_PropagatesRepositoryID(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	info := &repoInfo{
		RepositoryID:   "repo-abc",
		RepositoryPath: "/repos/myrepo",
		BaseBranch:     "main",
		Repository: &models.Repository{
			ID:          "repo-abc",
			Name:        "myrepo",
			SetupScript: "npm install",
		},
	}
	execCfg := executorConfig{ExecutorID: "exec-1", ExecutorType: string(models.ExecutorTypeLocal)}

	if _, err := e.applyRepositoryConfig(req, task, info, execCfg, nil); err != nil {
		t.Fatalf("applyRepositoryConfig: %v", err)
	}

	if req.RepositoryID != "repo-abc" {
		t.Errorf("req.RepositoryID = %q, want %q", req.RepositoryID, "repo-abc")
	}
}
