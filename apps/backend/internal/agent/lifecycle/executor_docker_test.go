package lifecycle

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

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
	exec := NewDockerExecutor(config.DockerConfig{}, log)

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
	exec := NewDockerExecutor(config.DockerConfig{}, log)

	if exec.Name() != executor.NameDocker {
		t.Errorf("expected name %q, got %q", executor.NameDocker, exec.Name())
	}
}

func TestDockerExecutor_HealthCheck(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, log)

	if err := exec.HealthCheck(context.Background()); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestDockerExecutor_RecoverInstances(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, log)

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
	exec := NewDockerExecutor(config.DockerConfig{}, log)

	if runner := exec.GetInteractiveRunner(); runner != nil {
		t.Error("expected nil interactive runner for docker executor")
	}
}

func TestDockerExecutor_EnsureClient_Success(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, log)
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
	exec := NewDockerExecutor(config.DockerConfig{}, log)

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
	exec := NewDockerExecutor(config.DockerConfig{}, log)

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
	exec := NewDockerExecutor(config.DockerConfig{}, log)
	exec.newClientFunc = failingClientFactory("docker unavailable")

	if cli := exec.Client(); cli != nil {
		t.Error("expected nil client when Docker is unavailable")
	}
}

func TestDockerExecutor_ContainerMgr_ReturnsNilOnFailure(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, log)
	exec.newClientFunc = failingClientFactory("docker unavailable")

	if cm := exec.ContainerMgr(); cm != nil {
		t.Error("expected nil container manager when Docker is unavailable")
	}
}

func TestDockerExecutor_Client_ReturnsClientOnSuccess(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, log)

	cli := exec.Client()
	if cli == nil {
		t.Error("expected non-nil client with default config")
	}

	_ = exec.Close()
}

func TestDockerExecutor_Close_BeforeInit(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, log)

	if err := exec.Close(); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestDockerExecutor_Close_AfterInit(t *testing.T) {
	log := newTestDockerLogger()
	exec := NewDockerExecutor(config.DockerConfig{}, log)

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
	exec := NewDockerExecutor(config.DockerConfig{}, log)
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
	mgr := NewManager(nil, nil, nil, nil, nil, nil, ExecutorFallbackWarn, log)

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
	mgr := NewManager(nil, nil, registry, nil, nil, nil, ExecutorFallbackWarn, log)

	provider := mgr.DockerClientProvider()
	if client := provider(); client != nil {
		t.Error("expected nil client when no Docker executor is registered")
	}
}

func TestDockerClientProvider_WithDockerExecutor(t *testing.T) {
	log := newTestDockerLogger()
	registry := NewExecutorRegistry(log)
	dockerExec := NewDockerExecutor(config.DockerConfig{}, log)
	dockerExec.newClientFunc = failingClientFactory("docker unavailable")
	registry.Register(dockerExec)
	mgr := NewManager(nil, nil, registry, nil, nil, nil, ExecutorFallbackWarn, log)

	provider := mgr.DockerClientProvider()
	if client := provider(); client != nil {
		t.Error("expected nil client from provider with unavailable Docker")
	}
}

func TestDockerClientProvider_WithWorkingDocker(t *testing.T) {
	log := newTestDockerLogger()
	registry := NewExecutorRegistry(log)
	dockerExec := NewDockerExecutor(config.DockerConfig{}, log)
	registry.Register(dockerExec)
	mgr := NewManager(nil, nil, registry, nil, nil, nil, ExecutorFallbackWarn, log)

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

		dockerExec := NewDockerExecutor(config.DockerConfig{}, log)
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
