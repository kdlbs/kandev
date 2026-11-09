package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type failingCreateExecutor struct {
	MockExecutor
	createCount int
	createErr   error
}

func (e *failingCreateExecutor) CreateInstance(context.Context, *ExecutorCreateRequest) (*ExecutorInstance, error) {
	e.createCount++
	return nil, e.createErr
}

// identitylessDockerExecutor models a Docker backend that reports success
// without a daemon-verifiable container identity.
type identitylessDockerExecutor struct {
	MockExecutor
	createCount int
	stopCount   int
}

func (e *identitylessDockerExecutor) CreateInstance(_ context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	e.createCount++
	return &ExecutorInstance{InstanceID: req.InstanceID, RuntimeName: executor.NameDocker}, nil
}

func (e *identitylessDockerExecutor) StopInstance(context.Context, *ExecutorInstance, bool) error {
	e.stopCount++
	return nil
}

func newRuntimeSelectionManager(t *testing.T, backends ...ExecutorBackend) (*Manager, *MockEventBus) {
	t.Helper()
	log := newTestLogger()
	registry := NewExecutorRegistry(log)
	for _, backend := range backends {
		registry.Register(backend)
	}
	eventBus := &MockEventBus{}
	mgr := NewManager(
		newTestRegistry(), eventBus, registry,
		&MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackAllow, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	return mgr, eventBus
}

func dockerSelectionWorkspaceProvider() *mockWorkspaceInfoProvider {
	return &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-docker": {
			TaskID: "task-1", SessionID: "session-docker", TaskEnvironmentID: "env-1",
			WorkspacePath: "/workspace/task-1", AgentID: "auggie",
			ExecutorType: string(models.ExecutorTypeLocalDocker),
		},
	}}
}

func dockerSelectionProfileReader() *fakeExecutorProfileReader {
	return &fakeExecutorProfileReader{
		session: &models.TaskSession{
			ID: "session-docker", TaskID: "task-1", TaskEnvironmentID: "env-1",
			State: models.TaskSessionStateRunning,
		},
		env: &models.TaskEnvironment{
			ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
			Status: models.TaskEnvironmentStatusReady, OwnershipGeneration: 1,
		},
	}
}

// TestExecutorRuntimeSelectionPolicies proves every internal fallback policy
// requires the mapped backend for a nonempty selection. The default and every
// alias keep their existing standalone behavior; unknown and unregistered
// plugin selections stay refused.
func TestExecutorRuntimeSelectionPolicies(t *testing.T) {
	standalone := &MockExecutor{name: executor.NameStandalone}
	for _, policy := range []ExecutorFallbackPolicy{
		ExecutorFallbackAllow, ExecutorFallbackWarn, ExecutorFallbackDeny, "",
	} {
		t.Run("policy_"+string(policy), func(t *testing.T) {
			mgr, _ := newRuntimeSelectionManager(t, standalone)
			mgr.executorFallbackPolicy = policy

			if _, err := mgr.getExecutorBackend(string(models.ExecutorTypeLocalDocker)); !errors.Is(err, ErrExecutorRuntimeUnavailable) {
				t.Fatalf("local_docker under policy %q error = %v, want unavailable", policy, err)
			}
			if err := mgr.ValidateExecutorRuntime(string(models.ExecutorTypeLocalDocker)); !errors.Is(err, ErrExecutorRuntimeUnavailable) {
				t.Fatalf("ValidateExecutorRuntime(local_docker) error = %v, want unavailable", err)
			}
			for _, selection := range []string{"", "local", "local_pc", "worktree", "mock_remote"} {
				backend, err := mgr.getExecutorBackend(selection)
				if err != nil || backend.Name() != executor.NameStandalone {
					t.Fatalf("selection %q = %v, %v; want standalone", selection, backend, err)
				}
			}
			for _, selection := range []string{"unknown-runtime", "plugin_remote"} {
				if _, err := mgr.getExecutorBackend(selection); !errors.Is(err, ErrExecutorRuntimeUnavailable) {
					t.Fatalf("selection %q error = %v, want unavailable", selection, err)
				}
			}
		})
	}

	t.Run("registered_docker", func(t *testing.T) {
		mgr, _ := newRuntimeSelectionManager(t, standalone,
			&MockExecutor{name: executor.NameDocker})
		backend, err := mgr.getExecutorBackend(string(models.ExecutorTypeLocalDocker))
		if err != nil || backend.Name() != executor.NameDocker {
			t.Fatalf("registered docker selection = %v, %v", backend, err)
		}
	})
}

// TestExplicitDockerCreationFailureDoesNotUseStandalone proves a registered
// Docker backend whose creation fails never falls back to a host worker.
func TestExplicitDockerCreationFailureDoesNotUseStandalone(t *testing.T) {
	docker := &failingCreateExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker},
		createErr:    errors.New("docker daemon unavailable"),
	}
	standalone := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, newTestLogger()),
	}
	mgr, _ := newRuntimeSelectionManager(t, docker, standalone)
	mgr.workspaceInfoProvider = dockerSelectionWorkspaceProvider()
	mgr.SetExecutorProfileReader(dockerSelectionProfileReader())

	if _, err := mgr.GetOrEnsureExecution(context.Background(), "session-docker"); err == nil {
		t.Fatal("expected Docker creation failure")
	}
	if docker.createCount != 1 {
		t.Fatalf("docker create calls = %d, want 1", docker.createCount)
	}
	if got := standalone.createCount.Load(); got != 0 {
		t.Fatalf("standalone create calls = %d, want 0", got)
	}
}

// TestDockerRuntimeIdentityRequired refuses a Docker success without a
// daemon-verifiable container ID and rolls back only the created resource.
func TestDockerRuntimeIdentityRequired(t *testing.T) {
	docker := &identitylessDockerExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}}
	mgr, _ := newRuntimeSelectionManager(t, docker)
	mgr.workspaceInfoProvider = dockerSelectionWorkspaceProvider()
	mgr.SetExecutorProfileReader(dockerSelectionProfileReader())

	execution, err := mgr.GetOrEnsureExecution(context.Background(), "session-docker")
	if err == nil || execution != nil {
		t.Fatalf("GetOrEnsureExecution = %v, %v; want identity refusal", execution, err)
	}
	if !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("error = %v, want identity unverified", err)
	}
	if docker.createCount != 1 || docker.stopCount != 1 {
		t.Fatalf("create/stop calls = %d/%d, want 1/1", docker.createCount, docker.stopCount)
	}
	if _, exists := mgr.GetExecutionBySessionID("session-docker"); exists {
		t.Fatal("identity refusal registered an execution")
	}
}

// TestExecutorRuntimeReuseAdmission proves cached access compares the recorded
// runtime with the requesting session's selection and never removes retained
// work on refusal.
func TestExecutorRuntimeReuseAdmission(t *testing.T) {
	standalone := &MockExecutor{name: executor.NameStandalone}
	docker := &MockExecutor{name: executor.NameDocker}

	t.Run("retained_standalone_selected_docker", func(t *testing.T) {
		mgr, _ := newRuntimeSelectionManager(t, standalone, docker)
		mgr.workspaceInfoProvider = dockerSelectionWorkspaceProvider()
		mgr.SetExecutorProfileReader(dockerSelectionProfileReader())
		execution := &AgentExecution{
			ID: "exec-mismatch", TaskID: "task-1", SessionID: "session-docker",
			TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
			RuntimeName: agentruntime.RuntimeStandalone, WorkspacePath: "/workspace/task-1",
			Status: v1.AgentStatusRunning, StartedAt: time.Now(),
		}
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}

		if _, err := mgr.GetOrEnsureExecution(context.Background(), "session-docker"); !errors.Is(err, ErrExecutorRuntimeMismatch) {
			t.Fatalf("cached access error = %v, want runtime mismatch", err)
		}
		if _, exists := mgr.GetExecutionBySessionID("session-docker"); !exists {
			t.Fatal("mismatch refusal removed retained execution")
		}
	})

	t.Run("retained_docker_selected_docker", func(t *testing.T) {
		mgr, _ := newRuntimeSelectionManager(t, standalone, docker)
		mgr.workspaceInfoProvider = dockerSelectionWorkspaceProvider()
		mgr.SetExecutorProfileReader(dockerSelectionProfileReader())
		execution := &AgentExecution{
			ID: "exec-docker", TaskID: "task-1", SessionID: "session-docker",
			TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
			RuntimeName: agentruntime.RuntimeDocker, ContainerID: "container-1",
			WorkspacePath: "/workspace/task-1", Status: v1.AgentStatusRunning, StartedAt: time.Now(),
		}
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}

		got, err := mgr.GetOrEnsureExecution(context.Background(), "session-docker")
		if err != nil || got != execution {
			t.Fatalf("healthy Docker reuse = %v, %v", got, err)
		}
	})

	t.Run("retained_standalone_selected_local", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-local": {
				TaskID: "task-1", SessionID: "session-local", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1", ExecutorType: string(models.ExecutorTypeLocal),
			},
		}}
		reader := dockerSelectionProfileReader()
		reader.session.ID = "session-local"
		reader.session.TaskEnvironmentID = "env-1"
		reader.env.ExecutorType = string(models.ExecutorTypeLocal)
		mgr, _ := newRuntimeSelectionManager(t, standalone)
		mgr.workspaceInfoProvider = provider
		mgr.SetExecutorProfileReader(reader)
		execution := &AgentExecution{
			ID: "exec-local", TaskID: "task-1", SessionID: "session-local",
			TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocal),
			RuntimeName: agentruntime.RuntimeStandalone, WorkspacePath: "/workspace/task-1",
			Status: v1.AgentStatusRunning, StartedAt: time.Now(),
		}
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}

		got, err := mgr.GetOrEnsureExecution(context.Background(), "session-local")
		if err != nil || got != execution {
			t.Fatalf("standalone reuse = %v, %v", got, err)
		}
	})
}

// TestExecutorRuntimeMismatchPreservesStop proves Stop still targets the exact
// recorded worker and does not require selected-runtime admission.
func TestExecutorRuntimeMismatchPreservesStop(t *testing.T) {
	standalone := &MockExecutor{name: executor.NameStandalone}
	mgr, _ := newRuntimeSelectionManager(t, standalone)
	execution := &AgentExecution{
		ID: "exec-mismatch", TaskID: "task-1", SessionID: "session-docker",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
		RuntimeName: agentruntime.RuntimeStandalone, WorkspacePath: "/workspace/task-1",
		Status: v1.AgentStatusRunning, StartedAt: time.Now(),
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	if err := mgr.StopAgent(context.Background(), execution.ID, true); err != nil {
		if errors.Is(err, ErrExecutorRuntimeMismatch) || errors.Is(err, ErrExecutorRuntimeUnavailable) {
			t.Fatalf("Stop required selected-runtime admission: %v", err)
		}
	}
	if len(standalone.stopInstanceCalls) != 1 {
		t.Fatalf("standalone stop calls = %d, want 1", len(standalone.stopInstanceCalls))
	}
}

// TestExecutorRuntimeRecoveryObservation proves a recovered mismatch does not
// publish availability while a compatible recovery still does.
func TestExecutorRuntimeRecoveryObservation(t *testing.T) {
	t.Run("mismatch_suppresses_availability", func(t *testing.T) {
		mgr, eventBus := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
		execution := &AgentExecution{
			ID: "exec-recovered-mismatch", TaskID: "task-1", SessionID: "session-docker",
			ExecutorType: string(models.ExecutorTypeLocalDocker),
			RuntimeName:  agentruntime.RuntimeStandalone,
			Status:       v1.AgentStatusRunning, StartedAt: time.Now(),
		}
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}
		mgr.publishRecoveredExecutionReady(context.Background(), execution)
		if len(eventBus.PublishedEvents) != 0 {
			t.Fatalf("mismatched recovery published %d events, want 0", len(eventBus.PublishedEvents))
		}
	})

	t.Run("compatible_publishes_availability", func(t *testing.T) {
		mgr, eventBus := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
		execution := &AgentExecution{
			ID: "exec-recovered-local", TaskID: "task-1", SessionID: "session-local",
			ExecutorType: string(models.ExecutorTypeLocal),
			RuntimeName:  agentruntime.RuntimeStandalone,
			Status:       v1.AgentStatusRunning, StartedAt: time.Now(),
		}
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}
		mgr.publishRecoveredExecutionReady(context.Background(), execution)
		if len(eventBus.PublishedEvents) == 0 {
			t.Fatal("compatible recovery published no availability event")
		}
	})
}

// TestExecutorRuntimePassthroughAdmission refuses a retained standalone
// passthrough worker selected as Docker before any PTY process starts.
func TestExecutorRuntimePassthroughAdmission(t *testing.T) {
	mgr, execution, profile, pty, _ := newCopilotPassthroughInitialManager(t)
	execution.ExecutorType = string(models.ExecutorTypeLocalDocker)
	execution.RuntimeName = agentruntime.RuntimeStandalone

	err := mgr.startPassthroughSession(context.Background(), execution, profile)
	if !errors.Is(err, ErrExecutorRuntimeMismatch) {
		t.Fatalf("startPassthroughSession error = %v, want runtime mismatch", err)
	}
	if pty.startCalls != 0 {
		t.Fatalf("PTY start calls = %d, want 0", pty.startCalls)
	}
}

// TestExecutorRuntimeInventoryReadFailureFailsClosed proves storage and
// cancellation errors never authorize a retained execution.
func TestExecutorRuntimeInventoryReadFailureFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		readErr error
	}{
		{name: "storage_error", readErr: errors.New("inventory database unavailable")},
		{name: "cancelled_context", readErr: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docker := &MockExecutor{name: executor.NameDocker}
			mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone}, docker)
			mgr.workspaceInfoProvider = dockerSelectionWorkspaceProvider()
			mgr.SetExecutorProfileReader(dockerSelectionProfileReader())
			mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{readErr: tc.readErr})
			execution := &AgentExecution{
				ID: "exec-docker", TaskID: "task-1", SessionID: "session-docker",
				TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
				RuntimeName: agentruntime.RuntimeDocker, ContainerID: "container-1",
				WorkspacePath: "/workspace/task-1", Status: v1.AgentStatusRunning,
			}
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatal(err)
			}

			if _, err := mgr.GetOrEnsureExecution(context.Background(), "session-docker"); !errors.Is(err, ErrExecutorIdentityUnverified) {
				t.Fatalf("cached access error = %v, want identity unverified", err)
			}
			if err := mgr.ValidateExecutionRuntime(
				context.Background(), "session-docker", "env-1", string(models.ExecutorTypeLocalDocker),
			); !errors.Is(err, ErrExecutorIdentityUnverified) {
				t.Fatalf("durable validation error = %v, want identity unverified", err)
			}
			if _, exists := mgr.GetExecutionBySessionID("session-docker"); !exists {
				t.Fatal("read failure removed retained execution")
			}
		})
	}
}

// TestExecutorRuntimeUnknownRemoteIdentityFailsClosed proves an SSH-bound
// execution with no observed runtime cannot pass by treating missing evidence
// as compatibility.
func TestExecutorRuntimeUnknownRemoteIdentityFailsClosed(t *testing.T) {
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{readErr: errors.New("inventory database unavailable")})
	execution := &AgentExecution{
		ID: "exec-ssh", TaskID: "task-1", SessionID: "session-ssh",
		ExecutorType: string(models.ExecutorTypeSSH), WorkspacePath: "/workspace",
		Status: v1.AgentStatusRunning,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := mgr.requireExecutionSelectionCompatibility(context.Background(), execution); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("SSH identity check = %v, want identity unverified", err)
	}
}

// TestExecutorRuntimeRetainedRowExecutionMismatchFailsClosed proves durable
// evidence must describe the exact candidate execution.
func TestExecutorRuntimeRetainedRowExecutionMismatchFailsClosed(t *testing.T) {
	docker := &MockExecutor{name: executor.NameDocker}
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone}, docker)
	mgr.workspaceInfoProvider = dockerSelectionWorkspaceProvider()
	mgr.SetExecutorProfileReader(dockerSelectionProfileReader())
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "running-session-docker", SessionID: "session-docker", TaskID: "task-1",
		Runtime: agentruntime.RuntimeDocker, ContainerID: "container-1",
		AgentExecutionID: "exec-other", Status: models.ExecutorRunningStatusRunning,
	}})
	execution := &AgentExecution{
		ID: "exec-docker", TaskID: "task-1", SessionID: "session-docker",
		TaskEnvironmentID: "env-1", ExecutorType: string(models.ExecutorTypeLocalDocker),
		RuntimeName: agentruntime.RuntimeDocker, ContainerID: "container-1",
		WorkspacePath: "/workspace/task-1", Status: v1.AgentStatusRunning,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GetOrEnsureExecution(context.Background(), "session-docker"); !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("mismatched execution ID error = %v, want identity unverified", err)
	}
}

func coldRetainedMismatchManager(t *testing.T) (*Manager, *createInstanceExecutor) {
	t.Helper()
	docker := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker},
		client:       newReadyAgentctlClient(t, newTestLogger()),
	}
	mgr, _ := newRuntimeSelectionManager(t, &MockExecutor{name: executor.NameStandalone}, docker)
	provider := dockerSelectionWorkspaceProvider()
	mgr.workspaceInfoProvider = provider
	mgr.SetExecutorProfileReader(dockerSelectionProfileReader())
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "running-session-docker", SessionID: "session-docker", TaskID: "task-1",
		Runtime: agentruntime.RuntimeStandalone, AgentExecutionID: "exec-old",
		Status: models.ExecutorRunningStatusRunning,
	}})
	return mgr, docker
}

// TestColdWorkspaceEnsureRefusesRetainedMismatch proves a cold session or
// environment ensure compares durable inventory before preparation or
// backend creation.
func TestColdWorkspaceEnsureRefusesRetainedMismatch(t *testing.T) {
	t.Run("session_ensure", func(t *testing.T) {
		mgr, docker := coldRetainedMismatchManager(t)
		if _, err := mgr.GetOrEnsureExecution(context.Background(), "session-docker"); !errors.Is(err, ErrExecutorRuntimeMismatch) {
			t.Fatalf("cold session ensure error = %v, want runtime mismatch", err)
		}
		if got := docker.createCount.Load(); got != 0 {
			t.Fatalf("cold session ensure created %d runtimes, want 0", got)
		}
	})

	t.Run("environment_ensure", func(t *testing.T) {
		mgr, docker := coldRetainedMismatchManager(t)
		if _, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-1"); !errors.Is(err, ErrExecutorRuntimeMismatch) {
			t.Fatalf("cold environment ensure error = %v, want runtime mismatch", err)
		}
		if got := docker.createCount.Load(); got != 0 {
			t.Fatalf("cold environment ensure created %d runtimes, want 0", got)
		}
	})
}

// TestColdLaunchRefusesRetainedMismatch proves launch admission precedes
// profile resolution, workspace mutation, and runtime allocation.
func TestColdLaunchRefusesRetainedMismatch(t *testing.T) {
	mgr, docker := coldRetainedMismatchManager(t)
	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID: "task-1", SessionID: "session-docker", AgentProfileID: "profile-1",
		ExecutorType: string(models.ExecutorTypeLocalDocker), StartAgent: true,
	})
	if !errors.Is(err, ErrExecutorRuntimeMismatch) {
		t.Fatalf("cold Launch error = %v, want runtime mismatch", err)
	}
	if got := docker.createCount.Load(); got != 0 {
		t.Fatalf("cold Launch created %d runtimes, want 0", got)
	}
}
