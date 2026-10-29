//go:build linux

package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/hostutility"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	pluginruntime "github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxBackendFactoryCapturesLocalExecutionAndRestart(t *testing.T) {
	archivePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if archivePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to the dedicated provider package")
	}
	h := newBootStateTestHarness(t)
	cfg := &config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{SandboxPolicies: true}, Server: config.ServerConfig{Port: 39991}, Agent: config.AgentConfig{StandalonePort: 39992}}
	policies, err := newSandboxPolicyService(cfg, h.db)
	require.NoError(t, err)
	policies.SetTargetResolver(func(context.Context, string, string) (policy.TargetScope, error) {
		return policy.TargetScope{OrgID: "org"}, nil
	})
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	root := t.TempDir()
	providers := plugins.NewService(store.NewFSStore(root), plugins.NewRegistry(), nil, log)
	require.NoError(t, providers.SetPluginsDir(root))
	providers.SetSandboxProvidersEnabled(true)
	providers.SetRuntime(pluginruntime.NewManager(root, nil, log))
	t.Cleanup(func() { require.NoError(t, providers.Close()) })
	archive, err := os.Open(archivePath)
	require.NoError(t, err)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	_, err = providers.Install(ctx, archive)
	require.NoError(t, archive.Close())
	require.NoError(t, err)
	listed, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	configureSandboxProviderCatalog(policies, providers)
	catalog, err := policies.ListProviders(ctx)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, listed[0].Binding.InstallationID, catalog[0].InstallationID)
	require.Equal(t, listed[0].Binding.ProviderKey, catalog[0].ProviderKey)
	require.Equal(t, listed[0].PluginRunning, catalog[0].Available)
	binding := listed[0].Binding
	work := t.TempDir()
	require.NoError(t, h.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace", OrgID: "org"}))
	require.NoError(t, h.taskRepo.CreateTask(ctx, &models.Task{ID: "task", Title: "Task", WorkspaceID: "workspace"}))
	revision, err := policies.Publish(ctx, policy.Policy{ID: "work", Name: "Work", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: "local", Executor: &policy.ExecutorPolicy{Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: revision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	require.NoError(t, h.taskRepo.CreateExecutor(ctx, &models.Executor{ID: "local", Name: "Local", Type: models.ExecutorTypeLocal}))
	availability, err := policies.CheckAvailability(ctx, policy.PreviewContext{ExecutorID: "local"})
	require.NoError(t, err)
	require.True(t, availability.Available, "installed provider runtime must pass the explicit read-only check")
	runtime, err := newSandboxRuntime(cfg, policies, providers)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	factory := sandboxRuntimeFactory{runtime: runtime, policies: policies, tasks: h.taskRepo, config: cfg}
	t.Run("admission captures only current server selection", func(t *testing.T) {
		admission := sandboxLaunchAdmission{policies: policies, tasks: h.taskRepo, runtime: runtime}
		request := &lifecycle.LaunchRequest{TaskID: "task", Metadata: map[string]any{"executor_id": "local"}, SandboxSelection: &policy.SelectionContext{OrgID: "foreign", ExecutorID: "unrestricted"}}
		require.NoError(t, admission.admit(ctx, request))
		require.NotNil(t, request.SandboxSelection)
		require.Equal(t, "org", request.SandboxSelection.OrgID)
		require.Equal(t, "local", request.SandboxSelection.ExecutorID)
		require.Equal(t, "workspace", request.SandboxSelection.WorkspaceID)
		request.TaskID = "missing"
		require.Error(t, admission.admit(ctx, request))
		require.Nil(t, request.SandboxSelection)
		ordinary := &lifecycle.LaunchRequest{Metadata: map[string]any{"executor_id": "unrestricted"}, SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}}
		require.NoError(t, admission.admit(ctx, ordinary))
		require.Nil(t, ordinary.SandboxSelection)
		utilityAdmission := sandboxUtilityAdmission{launch: admission, targets: sandboxTargetResolver{tasks: h.taskSvc}}
		probe := &hostutility.ProfileProbeContext{ExecutorID: "local", SandboxSelection: &policy.SelectionContext{OrgID: "foreign"}, SandboxDigest: "stale"}
		require.NoError(t, utilityAdmission.admit(ctx, probe))
		require.NotNil(t, probe.SandboxSelection)
		require.Equal(t, "org", probe.SandboxSelection.OrgID)
		require.NotEmpty(t, probe.SandboxDigest)
		require.NotEqual(t, "stale", probe.SandboxDigest)
		providers.SetSandboxProvidersEnabled(false)
		t.Cleanup(func() { providers.SetSandboxProvidersEnabled(true) })
		require.Error(t, utilityAdmission.admit(ctx, probe), "an unavailable provider cannot serve a cached capability result")
		require.Empty(t, probe.SandboxDigest)
		providers.SetSandboxProvidersEnabled(true)
		require.NoError(t, utilityAdmission.admit(ctx, probe))
		probe.ExecutorID = "missing"
		require.Error(t, utilityAdmission.admit(ctx, probe))
		require.Nil(t, probe.SandboxSelection)
		require.Empty(t, probe.SandboxDigest)
	})
	transport := client.NewClient("127.0.0.1", 39993, log)
	execution := &lifecycle.AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", RuntimeName: executor.NameStandalone, ExecutorType: string(models.ExecutorTypeLocal), WorkspacePath: work, AgentArgs: []string{"/bin/agent"}, SandboxSelection: &policy.SelectionContext{OrgID: "org", WorkspaceID: "workspace", TaskID: "task", ExecutorID: "local"}}
	prepared, err := factory.prepareExecution(ctx, execution, 1, "", map[string]string{"HOME": "/host-home"}, transport)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	verifyPreparedExecutionSnapshot(t, h.taskRepo, policies, execution, prepared.Launch())
	require.Equal(t, filepath.Join(cfg.ResolvedDataDir(), "sandbox-runtime"), filepath.Dir(prepared.Launch().Root))
	_, err = factory.prepareExecution(ctx, execution, 2, "", nil, transport)
	require.Error(t, err, "a pending attempt cannot be replaced by a competing launch")
	retained, err := policies.ReadAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.Equal(t, policy.AttemptPrepared, retained.State)
	require.DirExists(t, prepared.Launch().Root)
	admission := sandboxLaunchAdmission{policies: policies, tasks: h.taskRepo}
	require.Error(t, admission.recover(ctx, execution), "a recovered execution cannot reuse backend-owned callback endpoints")
	interrupted, err := policies.ReadAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.Equal(t, policy.AttemptRecoveryRequired, interrupted.State)
	require.NoError(t, prepared.Close())
	restarted, err := factory.prepareExecution(ctx, execution, 2, prepared.Launch().AttemptID, nil, transport)
	require.NoError(t, err)
	require.NotNil(t, restarted)
	captured, err := policies.ReadAttempt(ctx, "org", restarted.Launch().AttemptID)
	require.NoError(t, err)
	require.Equal(t, prepared.Launch().AttemptID, captured.Snapshot.RestartOf)
	require.NoError(t, restarted.Close())
	t.Run("utility owns a separate sessionless attempt", func(t *testing.T) {
		selection := &policy.SelectionContext{OrgID: "org", WorkspaceID: "workspace", ExecutorID: "local"}
		probe := &hostutility.ProfileProbeContext{ExecutorID: "local"}
		admission := sandboxUtilityAdmission{launch: sandboxLaunchAdmission{policies: policies, tasks: h.taskRepo, runtime: runtime}, targets: sandboxTargetResolver{tasks: h.taskSvc}}
		require.NoError(t, admission.admit(ctx, probe))
		utilityConfig := &utility.InferenceConfigDTO{Command: []string{"/bin/agent"}, CLIFlags: []string{"--discovery"}, WorkDir: work}
		attempt, err := factory.prepareUtility(ctx, probe, utilityConfig, transport)
		require.NoError(t, err)
		require.NotNil(t, attempt)
		captured, err := policies.ReadAttempt(ctx, "org", attempt.Launch().AttemptID)
		require.NoError(t, err)
		require.Empty(t, captured.Snapshot.SessionID)
		require.Empty(t, captured.Snapshot.TaskID)
		require.NotEmpty(t, captured.Snapshot.ExecutionID)
		require.NotEqual(t, execution.ID, captured.Snapshot.ExecutionID)
		require.Equal(t, work, captured.Snapshot.WorkDir)
		require.NoError(t, attempt.Close())
		probe.SandboxDigest = "stale"
		_, err = factory.prepareUtility(ctx, probe, utilityConfig, transport)
		require.Error(t, err, "changed selection must not publish discovery under a stale cache key")
		probe.SandboxDigest = ""
		_, err = factory.prepareUtility(ctx, probe, utilityConfig, transport)
		require.Error(t, err, "a selected probe must carry its admitted context")
		require.NoError(t, admission.admit(ctx, probe))
		second, err := factory.prepareUtility(ctx, probe, utilityConfig, transport)
		require.NoError(t, err)
		require.NotEqual(t, attempt.Launch().AttemptID, second.Launch().AttemptID)
		require.NoError(t, second.Close())
		for _, invalid := range []*hostutility.ProfileProbeContext{nil, {}, {SandboxSelection: selection, ExecutorID: "different"}} {
			_, err = factory.prepareUtility(ctx, invalid, utilityConfig, transport)
			require.Error(t, err)
		}
	})
	execution.RuntimeName = executor.NameDocker
	_, err = factory.prepareExecution(ctx, execution, 3, "", nil, transport)
	require.Error(t, err, "local sandbox cannot be applied to a remote runtime")
}

func TestSandboxBackendWithoutPluginsPreservesOrdinaryStartup(t *testing.T) {
	cfg := &config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{SandboxPolicies: true}}
	runtime, err := newSandboxRuntime(cfg, nil, nil)
	require.NoError(t, err)
	require.Nil(t, runtime)
	_, err = os.Stat(filepath.Join(cfg.ResolvedDataDir(), "sandbox-runtime"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
