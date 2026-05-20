package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestDockerLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

// failingClientFactory returns a factory that always fails.
func failingClientFactory(errMsg string) func(config.DockerConfig, *logger.Logger) (*docker.Client, error) {
	return func(_ config.DockerConfig, _ *logger.Logger) (*docker.Client, error) {
		return nil, fmt.Errorf("%s", errMsg)
	}
}

func TestNewDockerExecutor(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	if exec == nil {
		t.Fatal("expected non-nil executor")
	}
	if exec.initialized {
		t.Error("expected initialized to be false")
	}
	if exec.docker != nil {
		t.Error("expected docker client to be nil before first use")
	}
	if exec.containerMgr != nil {
		t.Error("expected container manager to be nil before first use")
	}
	if exec.newClientFunc == nil {
		t.Error("expected newClientFunc to be set")
	}
}

func TestDockerExecutor_Name(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	if exec.Name() != executor.NameDocker {
		t.Errorf("expected name %q, got %q", executor.NameDocker, exec.Name())
	}
}

func TestDockerExecutor_HealthCheck(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	if err := exec.HealthCheck(context.Background()); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestDockerExecutor_RecoverInstances(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	instances, err := exec.RecoverInstances(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if instances != nil {
		t.Errorf("expected nil instances, got: %v", instances)
	}
}

func TestDockerExecutor_GetInteractiveRunner(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	if runner := exec.GetInteractiveRunner(); runner != nil {
		t.Error("expected nil interactive runner for docker executor")
	}
}

func TestDockerExecutor_EnsureClient_Success(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)
	// Default factory uses docker.NewClient which succeeds even without Docker running

	cli, cm, err := exec.ensureClient()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if cli == nil {
		t.Error("expected non-nil client")
	}
	if cm == nil {
		t.Error("expected non-nil container manager")
	}
	if !exec.initialized {
		t.Error("expected initialized to be true after success")
	}

	// Second call should return cached values
	cli2, cm2, err2 := exec.ensureClient()
	if err2 != nil {
		t.Fatalf("expected nil error on second call, got: %v", err2)
	}
	if cli2 != cli {
		t.Error("expected same client on second call")
	}
	if cm2 != cm {
		t.Error("expected same container manager on second call")
	}

	// Clean up
	_ = exec.Close()
}

func TestDockerExecutor_EnsureClient_RetriesOnFailure(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	var callCount atomic.Int32
	exec.newClientFunc = func(_ config.DockerConfig, _ *logger.Logger) (*docker.Client, error) {
		callCount.Add(1)
		return nil, fmt.Errorf("docker daemon not running")
	}

	// First call should fail
	cli, cm, err := exec.ensureClient()
	if err == nil {
		t.Fatal("expected error on first call")
	}
	if cli != nil || cm != nil {
		t.Error("expected nil client and container manager on failure")
	}
	if exec.initialized {
		t.Error("expected initialized to be false after failure")
	}

	// Second call should retry (not return a cached error like sync.Once would)
	_, _, err2 := exec.ensureClient()
	if err2 == nil {
		t.Fatal("expected error on second call")
	}
	if callCount.Load() != 2 {
		t.Errorf("expected factory to be called twice (retry), got %d calls", callCount.Load())
	}
}

func TestDockerExecutor_EnsureClient_RecoversAfterTransientFailure(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	var callCount atomic.Int32
	exec.newClientFunc = func(cfg config.DockerConfig, l *logger.Logger) (*docker.Client, error) {
		n := callCount.Add(1)
		if n == 1 {
			return nil, fmt.Errorf("transient error")
		}
		// Succeed on second call using real factory
		return docker.NewClient(cfg, l)
	}

	// First call fails
	_, _, err := exec.ensureClient()
	if err == nil {
		t.Fatal("expected error on first call")
	}

	// Second call succeeds
	cli, cm, err := exec.ensureClient()
	if err != nil {
		t.Fatalf("expected nil error after recovery, got: %v", err)
	}
	if cli == nil || cm == nil {
		t.Error("expected non-nil client and container manager after recovery")
	}
	if !exec.initialized {
		t.Error("expected initialized to be true after recovery")
	}

	_ = exec.Close()
}

func TestDockerExecutor_Client_ReturnsNilOnFailure(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)
	exec.newClientFunc = failingClientFactory("docker unavailable")

	if cli := exec.Client(); cli != nil {
		t.Error("expected nil client when Docker is unavailable")
	}
}

func TestDockerExecutor_ContainerMgr_ReturnsNilOnFailure(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)
	exec.newClientFunc = failingClientFactory("docker unavailable")

	if cm := exec.ContainerMgr(); cm != nil {
		t.Error("expected nil container manager when Docker is unavailable")
	}
}

func TestDockerExecutor_Client_ReturnsClientOnSuccess(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	cli := exec.Client()
	if cli == nil {
		t.Error("expected non-nil client with default config")
	}

	_ = exec.Close()
}

func TestDockerExecutor_Close_BeforeInit(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	if err := exec.Close(); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestDockerExecutor_Close_AfterInit(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	// Initialize the client
	_, _, _ = exec.ensureClient()
	if !exec.initialized {
		t.Fatal("expected initialized to be true")
	}

	// Close should succeed and reset state
	if err := exec.Close(); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if exec.initialized {
		t.Error("expected initialized to be false after close")
	}
	if exec.docker != nil {
		t.Error("expected docker to be nil after close")
	}
	if exec.containerMgr != nil {
		t.Error("expected containerMgr to be nil after close")
	}
}

func TestDockerExecutor_Close_AfterFailedInit(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)
	exec.newClientFunc = failingClientFactory("docker unavailable")

	// Trigger failed init
	_, _, _ = exec.ensureClient()

	// Close after failed init should be a no-op
	if err := exec.Close(); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestDockerClientProvider_NilRegistry(t *testing.T) {
	log := newTestDockerLogger()
	mgr := NewManager(nil, nil, nil, nil, nil, nil, ExecutorFallbackWarn, "", log)

	provider := mgr.DockerClientProvider()
	if provider == nil {
		t.Fatal("expected non-nil provider function")
	}
	if client := provider(); client != nil {
		t.Error("expected nil client from provider with nil registry")
	}
}

func TestDockerClientProvider_NoDockerExecutor(t *testing.T) {
	log := newTestDockerLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{name: executor.NameStandalone})
	mgr := NewManager(nil, nil, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)

	provider := mgr.DockerClientProvider()
	if client := provider(); client != nil {
		t.Error("expected nil client when no Docker executor is registered")
	}
}

func TestDockerClientProvider_WithDockerExecutor(t *testing.T) {
	log := newTestDockerLogger()
	registry := NewExecutorRegistry(log)
	dockerExec := NewDockerExecutor(config.DockerConfig{}, "", log)
	dockerExec.newClientFunc = failingClientFactory("docker unavailable")
	registry.Register(dockerExec)
	mgr := NewManager(nil, nil, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)

	provider := mgr.DockerClientProvider()
	if client := provider(); client != nil {
		t.Error("expected nil client from provider with unavailable Docker")
	}
}

func TestDockerClientProvider_WithWorkingDocker(t *testing.T) {
	log := newTestDockerLogger()
	registry := NewExecutorRegistry(log)
	dockerExec := NewDockerExecutor(config.DockerConfig{}, "", log)
	registry.Register(dockerExec)
	mgr := NewManager(nil, nil, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)

	provider := mgr.DockerClientProvider()
	client := provider()
	if client == nil {
		t.Error("expected non-nil client from provider with working Docker executor")
	}

	_ = dockerExec.Close()
}

func TestExecutorRegistry_CloseAll(t *testing.T) {
	t.Run("closes closeable backends", func(t *testing.T) {
		log := newTestDockerLogger()
		registry := NewExecutorRegistry(log)

		dockerExec := NewDockerExecutor(config.DockerConfig{}, "", log)
		registry.Register(dockerExec)
		registry.Register(&MockExecutor{name: executor.NameStandalone})

		// Should not panic
		registry.CloseAll()
	})

	t.Run("empty registry", func(t *testing.T) {
		log := newTestDockerLogger()
		registry := NewExecutorRegistry(log)

		// Should not panic
		registry.CloseAll()
	})
}

func TestInjectTokenIntoURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		env      map[string]string
		expected string
	}{
		{
			name:     "HTTPS URL with GITHUB_TOKEN",
			url:      "https://github.com/org/repo.git",
			env:      map[string]string{"GITHUB_TOKEN": "ghp_test123"},
			expected: "https://x-access-token:ghp_test123@github.com/org/repo.git",
		},
		{
			name:     "HTTPS URL with GH_TOKEN fallback",
			url:      "https://github.com/org/repo.git",
			env:      map[string]string{"GH_TOKEN": "ghp_ghtoken"},
			expected: "https://x-access-token:ghp_ghtoken@github.com/org/repo.git",
		},
		{
			name:     "GITHUB_TOKEN takes priority over GH_TOKEN",
			url:      "https://github.com/org/repo.git",
			env:      map[string]string{"GITHUB_TOKEN": "ghp_primary", "GH_TOKEN": "ghp_secondary"},
			expected: "https://x-access-token:ghp_primary@github.com/org/repo.git",
		},
		{
			name:     "SSH URL converted to authenticated HTTPS",
			url:      "git@github.com:org/repo.git",
			env:      map[string]string{"GITHUB_TOKEN": "ghp_test123"},
			expected: "https://x-access-token:ghp_test123@github.com/org/repo.git",
		},
		{
			name:     "no token returns URL unchanged",
			url:      "https://github.com/org/repo.git",
			env:      map[string]string{},
			expected: "https://github.com/org/repo.git",
		},
		{
			name:     "nil env returns URL unchanged",
			url:      "https://github.com/org/repo.git",
			env:      nil,
			expected: "https://github.com/org/repo.git",
		},
		{
			name:     "non-GitHub URL unchanged",
			url:      "https://gitlab.com/org/repo.git",
			env:      map[string]string{"GITHUB_TOKEN": "ghp_test123"},
			expected: "https://gitlab.com/org/repo.git",
		},
		{
			name:     "SSH URL without token unchanged",
			url:      "git@github.com:org/repo.git",
			env:      map[string]string{},
			expected: "git@github.com:org/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectGitHubTokenIntoCloneURL(tt.url, tt.env)
			if got != tt.expected {
				t.Errorf("injectTokenIntoURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestReconnectToContainer_ValidationErrors(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	t.Run("short execution ID rejected", func(t *testing.T) {
		req := &ExecutorCreateRequest{
			PreviousExecutionID: "short",
		}
		_, err := exec.reconnectToContainer(context.Background(), nil, req)
		if err == nil {
			t.Fatal("expected error for short execution ID")
		}
		if !strings.Contains(err.Error(), "too short") {
			t.Errorf("expected 'too short' error, got: %v", err)
		}
	})

	t.Run("empty execution ID rejected", func(t *testing.T) {
		req := &ExecutorCreateRequest{
			PreviousExecutionID: "",
		}
		_, err := exec.reconnectToContainer(context.Background(), nil, req)
		if err == nil {
			t.Fatal("expected error for empty execution ID")
		}
	})
}

func TestResolveReconnectContainerRef(t *testing.T) {
	t.Run("uses stored container id before derived execution name", func(t *testing.T) {
		req := &ExecutorCreateRequest{
			PreviousExecutionID: "exec-previous",
			Metadata: map[string]interface{}{
				MetadataKeyContainerID: "container-real",
			},
		}
		got, err := resolveReconnectContainerRef(req)
		if err != nil {
			t.Fatalf("resolveReconnectContainerRef returned error: %v", err)
		}
		if got != "container-real" {
			t.Fatalf("expected stored container id, got %q", got)
		}
	})

	t.Run("falls back to legacy name from previous execution id", func(t *testing.T) {
		req := &ExecutorCreateRequest{PreviousExecutionID: "12345678-abcdef"}
		got, err := resolveReconnectContainerRef(req)
		if err != nil {
			t.Fatalf("resolveReconnectContainerRef returned error: %v", err)
		}
		if got != "kandev-agent-12345678" {
			t.Fatalf("expected derived container name, got %q", got)
		}
	})
}

func TestReconnectInstanceID(t *testing.T) {
	t.Run("uses requested instance for new session in existing container", func(t *testing.T) {
		req := &ExecutorCreateRequest{InstanceID: "exec-new"}
		got := reconnectInstanceID(req, "exec-old")
		if got != "exec-new" {
			t.Fatalf("reconnectInstanceID = %q, want exec-new", got)
		}
	})

	t.Run("falls back to previous execution for legacy callers", func(t *testing.T) {
		got := reconnectInstanceID(&ExecutorCreateRequest{}, "exec-old")
		if got != "exec-old" {
			t.Fatalf("reconnectInstanceID = %q, want exec-old", got)
		}
	})
}

func TestShouldStartExistingDockerContainer(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{state: "created", want: true},
		{state: "exited", want: true},
		{state: "running", want: false},
		{state: "paused", want: false},
		{state: "dead", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := shouldStartExistingDockerContainer(tt.state); got != tt.want {
				t.Fatalf("shouldStartExistingDockerContainer(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestBuildReconnectCreateInstanceRequest(t *testing.T) {
	req := &ExecutorCreateRequest{
		TaskID:    "task-1",
		SessionID: "session-1",
		AgentConfig: &testAgent{
			id:      "codex",
			enabled: true,
			runtimeConfig: &agents.RuntimeConfig{
				AssumeMcpSse: true,
			},
		},
		Env: map[string]string{
			"OPENAI_API_KEY": "token",
		},
		McpServers: []McpServerConfig{{Name: "test-mcp"}},
		McpMode:    "config",
	}

	got := buildReconnectCreateInstanceRequest(req, "previous-exec")
	if got.ID != "previous-exec" {
		t.Fatalf("ID = %q, want previous-exec", got.ID)
	}
	if got.WorkspacePath != dockerWorkspacePath {
		t.Fatalf("WorkspacePath = %q, want %q", got.WorkspacePath, dockerWorkspacePath)
	}
	if got.AgentType != "codex" {
		t.Fatalf("AgentType = %q, want codex", got.AgentType)
	}
	if got.SessionID != "session-1" || got.TaskID != "task-1" {
		t.Fatalf("task/session not propagated: task=%q session=%q", got.TaskID, got.SessionID)
	}
	if got.Env["OPENAI_API_KEY"] != "token" {
		t.Fatalf("env not propagated: %v", got.Env)
	}
	if len(got.McpServers) != 1 || got.McpServers[0].Name != "test-mcp" {
		t.Fatalf("mcp servers not propagated: %v", got.McpServers)
	}
	if !got.AssumeMcpSse {
		t.Fatal("expected AssumeMcpSse to be propagated")
	}
	if got.McpMode != "config" {
		t.Fatalf("McpMode = %q, want config", got.McpMode)
	}
}

func TestResolvePrepareScript(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, "", log)

	t.Run("uses default docker script when no metadata", func(t *testing.T) {
		req := &ExecutorCreateRequest{
			Metadata: map[string]interface{}{
				"repository_path":      "/tmp/repo",
				"base_branch":          "main",
				"repository_clone_url": "https://github.com/org/repo.git",
				"worktree_branch":      "feature/task-abc",
			},
			Env: map[string]string{"GH_TOKEN": "ghp_test"},
		}
		script := exec.resolvePrepareScript(req)
		if script == "" {
			t.Fatal("expected non-empty script")
		}
		// Should contain clone command
		if !strings.Contains(script, "git clone") {
			t.Error("expected git clone in script")
		}
		if !strings.Contains(script, "git config --global --add safe.directory '*'") {
			t.Error("expected default Docker prepare script to trust mounted local repositories")
		}
		// Should contain token stripping
		if !strings.Contains(script, "git remote set-url") {
			t.Error("expected token stripping after clone")
		}
		if !strings.Contains(script, "git checkout -b \"feature/task-abc\"") {
			t.Fatalf("expected Docker prepare script to create task branch, got:\n%s", script)
		}
	})

	t.Run("empty metadata uses defaults", func(t *testing.T) {
		req := &ExecutorCreateRequest{
			Metadata: map[string]interface{}{},
			Env:      map[string]string{},
		}
		script := exec.resolvePrepareScript(req)
		// Should still produce a script (default docker script)
		if script == "" {
			t.Fatal("expected non-empty default script")
		}
	})
}

func TestLocalPathFromCloneURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "file URL", raw: "file:///tmp/e2e-docker-remote.git", want: "/tmp/e2e-docker-remote.git"},
		{name: "escaped file URL", raw: "file:///tmp/repo%20remote.git", want: "/tmp/repo remote.git"},
		{name: "localhost file URL", raw: "file://localhost/tmp/repo.git", want: "/tmp/repo.git"},
		{name: "plain absolute path", raw: "/tmp/repo.git", want: "/tmp/repo.git"},
		{name: "https URL", raw: "https://github.com/org/repo.git", want: ""},
		{name: "remote file host", raw: "file://server/tmp/repo.git", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localPathFromCloneURL(tt.raw); got != tt.want {
				t.Fatalf("localPathFromCloneURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
