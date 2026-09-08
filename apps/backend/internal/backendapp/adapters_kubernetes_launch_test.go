package backendapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	agentexecutor "github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var errKubernetesLaunchCaptured = errors.New("kubernetes launch captured")

type capturingKubernetesBackend struct {
	req *lifecycle.ExecutorCreateRequest
}

func (b *capturingKubernetesBackend) Name() agentexecutor.Name          { return agentexecutor.NameKubernetes }
func (b *capturingKubernetesBackend) HealthCheck(context.Context) error { return nil }
func (b *capturingKubernetesBackend) CreateInstance(
	_ context.Context,
	req *lifecycle.ExecutorCreateRequest,
) (*lifecycle.ExecutorInstance, error) {
	b.req = req
	return nil, errKubernetesLaunchCaptured
}
func (b *capturingKubernetesBackend) StopInstance(context.Context, *lifecycle.ExecutorInstance, bool) error {
	return nil
}
func (b *capturingKubernetesBackend) RecoverInstances(context.Context) ([]*lifecycle.ExecutorInstance, error) {
	return nil, nil
}
func (b *capturingKubernetesBackend) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (b *capturingKubernetesBackend) RequiresCloneURL() bool                           { return true }
func (b *capturingKubernetesBackend) ShouldApplyPreferredShell() bool                  { return false }
func (b *capturingKubernetesBackend) IsAlwaysResumable() bool                          { return true }

type kubernetesLaunchProfileResolver struct{}

func (kubernetesLaunchProfileResolver) ResolveProfile(
	context.Context,
	string,
) (*lifecycle.AgentProfileInfo, error) {
	return &lifecycle.AgentProfileInfo{
		ProfileID: "agent-profile", AgentID: "mock-agent", AgentName: "mock-agent",
	}, nil
}

type kubernetesLaunchProfileReader struct{}

func (kubernetesLaunchProfileReader) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: "session-1", TaskID: "task-1", ExecutorProfileID: "profile-k8s"}, nil
}
func (kubernetesLaunchProfileReader) HasActiveTaskResourceCleanupJob(context.Context, string) (bool, error) {
	return false, nil
}
func (kubernetesLaunchProfileReader) GetTaskEnvironment(context.Context, string) (*models.TaskEnvironment, error) {
	return nil, nil
}
func (kubernetesLaunchProfileReader) GetExecutorProfile(context.Context, string) (*models.ExecutorProfile, error) {
	return &models.ExecutorProfile{
		ID: "profile-k8s", ExecutorID: "executor-k8s",
		Config: map[string]string{
			lifecycle.MetadataKeyKubernetesProfilePlatform:      "linux/amd64",
			lifecycle.MetadataKeyKubernetesProfileMainContainer: "kandev-agent",
			lifecycle.MetadataKeyKubernetesPodTemplateYAML:      "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example.test/agent:latest\n",
			lifecycle.MetadataKeyKubernetesWorkspaceMode:        "empty_dir",
		},
	}, nil
}

// Reviewer-requested boundary contract: orchestrator request -> backend adapter
// -> lifecycle runtime request must keep authoritative Kubernetes connection keys.
func TestKubernetesLaunchCarriesConnectionConfigThroughAdapterAndLifecycle(t *testing.T) {
	log := newTestLogger()
	agentRegistry := registry.NewRegistry(log)
	mockAgent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	mockAgent.SetEnabled(true)
	require.NoError(t, agentRegistry.Register(mockAgent))
	backend := &capturingKubernetesBackend{}
	executorRegistry := lifecycle.NewExecutorRegistry(log)
	executorRegistry.Register(backend)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	mgr := lifecycle.NewManager(
		agentRegistry, eventBus, executorRegistry, nil, kubernetesLaunchProfileResolver{}, nil,
		lifecycle.ExecutorFallbackWarn, "", log,
	)
	mgr.SetExecutorProfileReader(kubernetesLaunchProfileReader{})
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
	adapter := newLifecycleAdapter(mgr, agentRegistry, log)
	wantConfig := map[string]string{
		lifecycle.MetadataKeyKubernetesAuthMode:              "kubeconfig",
		lifecycle.MetadataKeyKubernetesKubeconfigPath:        "/etc/kandev/kind.yaml",
		lifecycle.MetadataKeyKubernetesKubeContext:           "kind-kandev",
		lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
		lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "45",
	}

	_, err := adapter.LaunchAgent(context.Background(), &orchestratorexecutor.LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		AgentProfileID: "agent-profile", ExecutorType: string(models.ExecutorTypeKubernetes),
		ExecutorConfig: wantConfig,
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyExecutorProfileID: "profile-k8s",
			"executor_id":                          "executor-k8s",
		},
	})

	require.ErrorIs(t, err, errKubernetesLaunchCaptured)
	require.NotNil(t, backend.req)
	for key, want := range wantConfig {
		require.Equal(t, want, backend.req.Metadata[key], key)
	}
}

// Reviewer-requested full-path contract: persisted executor selection must
// survive orchestrator request construction, backend adapter conversion, and
// lifecycle metadata projection before the Kubernetes client is constructed.
func TestPreparedKubernetesLaunchCarriesPersistedConnectionConfigToLifecycle(t *testing.T) {
	tests := []struct {
		name           string
		executorConfig map[string]string
		wantMetadata   map[string]string
	}{
		{
			name: "kubeconfig",
			executorConfig: map[string]string{
				lifecycle.MetadataKeyKubernetesAuthMode:              "kubeconfig",
				lifecycle.MetadataKeyKubernetesKubeconfigPath:        "/etc/kandev/kind.yaml",
				lifecycle.MetadataKeyKubernetesKubeContext:           "kind-kandev",
				lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
				lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "45",
			},
		},
		{
			name: "in cluster clears kubeconfig fields",
			executorConfig: map[string]string{
				lifecycle.MetadataKeyKubernetesAuthMode:              "in_cluster",
				lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
				lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "30",
			},
			wantMetadata: map[string]string{
				lifecycle.MetadataKeyKubernetesKubeconfigPath: "",
				lifecycle.MetadataKeyKubernetesKubeContext:    "",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantMetadata := make(map[string]string, len(test.wantMetadata)+len(test.executorConfig))
			for key, value := range test.wantMetadata {
				wantMetadata[key] = value
			}
			for key, value := range test.executorConfig {
				wantMetadata[key] = value
			}
			testPreparedKubernetesLaunchCarriesConnectionConfig(
				t, test.executorConfig, wantMetadata,
			)
		})
	}
}

func testPreparedKubernetesLaunchCarriesConnectionConfig(
	t *testing.T,
	executorConfig map[string]string,
	wantMetadata map[string]string,
) {
	t.Helper()
	ctx := context.Background()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "kubernetes-launch.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, sqlxDB.Close()) })
	repo, err := sqlitetaskrepo.NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-1", Name: "Workspace"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1", Title: "Kubernetes launch", State: v1.TaskStateCreated,
	}))
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-k8s", Name: "Kind", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: executorConfig,
	}))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "profile-k8s", ExecutorID: "executor-k8s", Name: "Kind workload",
		Config: map[string]string{
			lifecycle.MetadataKeyKubernetesProfilePlatform:      "linux/amd64",
			lifecycle.MetadataKeyKubernetesProfileMainContainer: "kandev-agent",
			lifecycle.MetadataKeyKubernetesPodTemplateYAML:      "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example.test/agent:latest\n",
			lifecycle.MetadataKeyKubernetesWorkspaceMode:        "empty_dir",
		},
	}))
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", AgentProfileID: "agent-profile",
		ExecutorID: "executor-k8s", ExecutorProfileID: "profile-k8s",
		State: models.TaskSessionStateCreated, StartedAt: now, UpdatedAt: now,
	}))

	log := newTestLogger()
	agentRegistry := registry.NewRegistry(log)
	mockAgent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	mockAgent.SetEnabled(true)
	require.NoError(t, agentRegistry.Register(mockAgent))
	backend := &capturingKubernetesBackend{}
	executorRegistry := lifecycle.NewExecutorRegistry(log)
	executorRegistry.Register(backend)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	mgr := lifecycle.NewManager(
		agentRegistry, eventBus, executorRegistry, nil, kubernetesLaunchProfileResolver{}, nil,
		lifecycle.ExecutorFallbackWarn, "", log,
	)
	mgr.SetExecutorProfileReader(repo)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
	adapter := newLifecycleAdapter(mgr, agentRegistry, log)
	orchestrator := orchestratorexecutor.NewExecutor(
		adapter, repo, log, orchestratorexecutor.ExecutorConfig{},
	)

	_, err = orchestrator.LaunchPreparedSession(ctx, &v1.Task{
		ID: "task-1", WorkspaceID: "workspace-1", Title: "Kubernetes launch",
	}, "session-1", orchestratorexecutor.LaunchOptions{AgentProfileID: "agent-profile"})

	require.ErrorIs(t, err, errKubernetesLaunchCaptured)
	require.NotNil(t, backend.req)
	for key, want := range wantMetadata {
		require.Equal(t, want, backend.req.Metadata[key], key)
	}
}
