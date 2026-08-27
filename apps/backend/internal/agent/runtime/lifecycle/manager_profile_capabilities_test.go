package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
)

// capabilityExecutor reports fixed executor capabilities so the manager's
// pass-through accessors can be checked against a known backend.
type capabilityExecutor struct {
	MockExecutor
	requiresClone bool
	preferShell   bool
}

func (e *capabilityExecutor) RequiresCloneURL() bool          { return e.requiresClone }
func (e *capabilityExecutor) ShouldApplyPreferredShell() bool { return e.preferShell }

func TestManagerCapabilitiesDelegateToBackend(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&capabilityExecutor{
		MockExecutor:  MockExecutor{name: executor.NameSprites},
		requiresClone: true,
		preferShell:   false,
	})
	registry.Register(&capabilityExecutor{
		MockExecutor:  MockExecutor{name: executor.NameStandalone},
		requiresClone: false,
		preferShell:   true,
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	require.True(t, mgr.RequiresCloneURL(string(models.ExecutorTypeSprites)),
		"a clone-based executor owns its own filesystem and needs a clone URL")
	require.False(t, mgr.ShouldApplyPreferredShell(string(models.ExecutorTypeSprites)))

	require.False(t, mgr.RequiresCloneURL(string(models.ExecutorTypeLocal)))
	require.True(t, mgr.ShouldApplyPreferredShell(string(models.ExecutorTypeLocal)),
		"a host-local executor inherits the user's preferred shell")
}

func TestManagerCapabilitiesFailClosedForUnknownExecutor(t *testing.T) {
	mgr := newTestManager(t)

	require.False(t, mgr.RequiresCloneURL("not-an-executor"))
	require.False(t, mgr.ShouldApplyPreferredShell("not-an-executor"))
}

func TestResolveAgentProfileRequiresResolver(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = nil

	_, err := mgr.ResolveAgentProfile(context.Background(), "profile-1")

	require.ErrorContains(t, err, "profile resolver not configured")
}

func TestResolveAgentProfileDelegatesToResolver(t *testing.T) {
	mgr := newTestManager(t)

	info, err := mgr.ResolveAgentProfile(context.Background(), "profile-1")

	require.NoError(t, err)
	require.Equal(t, "profile-1", info.ProfileID)
	require.Equal(t, "auggie", info.AgentName)
}

func TestRuntimeNameOfNilBackend(t *testing.T) {
	require.Equal(t, executor.NameUnknown, runtimeName(nil))
	require.Equal(t, executor.NameDocker, runtimeName(&MockExecutor{name: executor.NameDocker}))
}

func TestResolveProfileSessionConfigAndPolicyReadsProfileOnce(t *testing.T) {
	resolver := &countingProfileResolver{info: &AgentProfileInfo{
		ProfileID:     "profile-1",
		Model:         "claude-opus-5",
		Mode:          "plan",
		ConfigOptions: map[string]string{"reasoning": "high"},
		FallbackModel: "claude-sonnet-5",
		AutoFallback:  true,
	}}
	mgr := newTestManager(t)
	mgr.profileResolver = resolver

	model, mode, options, policy := mgr.resolveProfileSessionConfigAndPolicy(context.Background(), "profile-1")

	require.Equal(t, "claude-opus-5", model)
	require.Equal(t, "plan", mode)
	require.Equal(t, map[string]string{"reasoning": "high"}, options)
	require.Equal(t, "claude-opus-5", policy.Model)
	require.Equal(t, "claude-sonnet-5", policy.FallbackModel)
	require.True(t, policy.AutoFallback)
	require.Equal(t, int32(1), resolver.calls.Load(),
		"session start needs config and policy together; a second read would be a wasted DB hit")
}

func TestResolveProfileSessionConfigAndPolicyDegradesGracefully(t *testing.T) {
	ctx := context.Background()

	t.Run("no profile id", func(t *testing.T) {
		mgr := newTestManager(t)
		model, mode, options, policy := mgr.resolveProfileSessionConfigAndPolicy(ctx, "")
		require.Empty(t, model)
		require.Empty(t, mode)
		require.Nil(t, options)
		require.Equal(t, StartModelPolicy{}, policy)
	})

	t.Run("resolver error", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.profileResolver = &countingProfileResolver{err: errors.New("profile deleted")}
		model, _, _, policy := mgr.resolveProfileSessionConfigAndPolicy(ctx, "profile-1")
		require.Empty(t, model)
		require.Equal(t, StartModelPolicy{}, policy,
			"a missing profile means no start model is configured, not a hard failure")
	})

	t.Run("no resolver wired", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.profileResolver = nil
		_, _, _, policy := mgr.resolveProfileSessionConfigAndPolicy(ctx, "profile-1")
		require.Equal(t, StartModelPolicy{}, policy)
	})
}

func TestApplyExecutorMcpPolicyOverridesDefault(t *testing.T) {
	mgr := newTestManager(t)
	base := mcpconfig.DefaultPolicyForRuntime(executor.NameUnknown)
	require.False(t, base.AllowStdio, "the unknown-runtime default denies stdio")

	policy, executorID, err := mgr.applyExecutorMcpPolicy("profile-1", "",
		map[string]interface{}{
			"executor_id":         "executor-7",
			"executor_mcp_policy": `{"allow_stdio":true}`,
		}, base)

	require.NoError(t, err)
	require.Equal(t, "executor-7", executorID,
		"the executor identity is read from metadata so warnings name the right executor")
	require.True(t, policy.AllowStdio)
}

func TestApplyExecutorMcpPolicyPassesThroughWithoutOverride(t *testing.T) {
	mgr := newTestManager(t)
	base := mcpconfig.DefaultPolicyForRuntime(executor.NameDocker)

	policy, executorID, err := mgr.applyExecutorMcpPolicy("profile-1", "executor-1", nil, base)
	require.NoError(t, err)
	require.Equal(t, base, policy)
	require.Equal(t, "executor-1", executorID)

	policy, executorID, err = mgr.applyExecutorMcpPolicy("profile-1", "executor-1",
		map[string]interface{}{"unrelated": "value"}, base)
	require.NoError(t, err)
	require.Equal(t, base, policy)
	require.Equal(t, "executor-1", executorID)
}

func TestApplyExecutorMcpPolicyRejectsMalformedOverride(t *testing.T) {
	mgr := newTestManager(t)
	base := mcpconfig.DefaultPolicyForRuntime(executor.NameDocker)

	_, _, err := mgr.applyExecutorMcpPolicy("profile-1", "", map[string]interface{}{
		"executor_mcp_policy": "{not json",
	}, base)

	require.ErrorContains(t, err, "invalid executor MCP policy")
}

func TestResolveMcpServersWithParamsShortCircuits(t *testing.T) {
	ctx := context.Background()
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	require.True(t, ok)

	t.Run("no provider", func(t *testing.T) {
		mgr := newTestManager(t)
		servers, err := mgr.resolveMcpServersWithParams(ctx, "profile-1", nil, agentConfig)
		require.NoError(t, err)
		require.Nil(t, servers)
	})

	t.Run("no agent config", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.mcpProvider = &fakeMcpConfigProvider{}
		servers, err := mgr.resolveMcpServersWithParams(ctx, "profile-1", nil, nil)
		require.NoError(t, err)
		require.Nil(t, servers)
	})

	t.Run("blank profile id", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.mcpProvider = &fakeMcpConfigProvider{}
		servers, err := mgr.resolveMcpServersWithParams(ctx, "   ", nil, agentConfig)
		require.NoError(t, err)
		require.Nil(t, servers)
	})

	t.Run("disabled config", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.mcpProvider = &fakeMcpConfigProvider{config: &mcpconfig.ProfileConfig{ProfileID: "profile-1"}}
		servers, err := mgr.resolveMcpServersWithParams(ctx, "profile-1", nil, agentConfig)
		require.NoError(t, err)
		require.Nil(t, servers, "a profile with MCP disabled contributes no servers")
	})
}

func TestResolveMcpServersFromExecutionUsesExecutionMetadata(t *testing.T) {
	mgr := newTestManager(t)
	mgr.mcpProvider = &fakeMcpConfigProvider{config: &mcpconfig.ProfileConfig{
		ProfileID: "profile-1",
		Enabled:   true,
		Servers: map[string]mcpconfig.ServerDef{
			"github": {Type: mcpconfig.ServerTypeStdio, Command: "npx", Args: []string{"-y", "gh"}},
		},
	}}
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	require.True(t, ok)

	execution := &AgentExecution{ID: "exec-1", AgentProfileID: "profile-1"}
	execution.setMetadataValue("executor_mcp_policy", `{"allow_stdio":true}`)

	servers, err := mgr.resolveMcpServers(context.Background(), execution, agentConfig)

	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "github", servers[0].Name)
	require.Equal(t, "npx", servers[0].Command,
		"the executor policy on the execution's own metadata is what unlocks stdio transports")

	nilServers, err := mgr.resolveMcpServers(context.Background(), nil, agentConfig)
	require.NoError(t, err)
	require.Nil(t, nilServers)
}

// TestPushBaseBranchesForTaskTargetsOnlyThatTask pins the fan-out: a base
// branch change must reach every live execution of the task and no other.
func TestPushBaseBranchesForTaskTargetsOnlyThatTask(t *testing.T) {
	var mu sync.Mutex
	var pushes []map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspace/base-branches" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body struct {
			BaseBranches map[string]string `json:"base_branches"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			// t.Errorf, not require: this runs on the server's goroutine and
			// require's FailNow/Goexit is only valid on the test goroutine.
			t.Errorf("decode base-branches request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		pushes = append(pushes, body.BaseBranches)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	client := newTestAgentctlClient(t, server.URL, newTestLogger())

	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1", agentctl: client,
	}))
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-2", TaskID: "task-1", SessionID: "session-2", agentctl: client,
	}))
	// Same-workspace execution on another task, plus one with no client.
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-3", TaskID: "task-other", SessionID: "session-3", agentctl: client,
	}))
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-4", TaskID: "task-1", SessionID: "session-4",
	}))

	branches := map[string]string{"": "main", "frontend": "develop"}
	mgr.PushBaseBranchesForTask(context.Background(), "task-1", branches)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, pushes, 2, "only the two client-bearing executions of task-1 are pushed to")
	for _, push := range pushes {
		require.Equal(t, branches, push)
	}
}

func TestPushBaseBranchesForTaskIgnoresEmptyTaskID(t *testing.T) {
	mgr := newTestManager(t)

	// No panic, no work: an empty task ID cannot identify executions.
	mgr.PushBaseBranchesForTask(context.Background(), "", map[string]string{"": "main"})
}

// TestPushBaseBranchesForTaskIsBestEffort pins that one failing agentctl does
// not stop the push to the task's other executions.
func TestPushBaseBranchesForTaskIsBestEffort(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := newTestAgentctlClient(t, server.URL, newTestLogger())

	mgr := newTestManager(t)
	for _, id := range []string{"session-1", "session-2"} {
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{
			ID: "exec-" + id, TaskID: "task-1", SessionID: id, agentctl: client,
		}))
	}

	mgr.PushBaseBranchesForTask(context.Background(), "task-1", map[string]string{"": "main"})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, calls,
		"a failed push must be logged and the loop continued; the DB remains the source of truth")
}

func TestGetRecoveredExecutionsProjectsOfficeIdentity(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1", ContainerID: "container-1",
		AgentProfileID: "execution-profile", OfficeAgentProfileID: "office-profile",
	}))

	recovered := mgr.GetRecoveredExecutions()

	require.Len(t, recovered, 1)
	require.Equal(t, "exec-1", recovered[0].ExecutionID)
	require.Equal(t, "task-1", recovered[0].TaskID)
	require.Equal(t, "session-1", recovered[0].SessionID)
	require.Equal(t, "container-1", recovered[0].ContainerID)
	require.Equal(t, "office-profile", recovered[0].AgentProfileID,
		"the orchestrator syncs on the stable Office identity")
	require.Equal(t, "execution-profile", recovered[0].ExecutionProfileID)
}

func TestExecutionStoreGetByContainerID(t *testing.T) {
	store := NewExecutionStore()
	require.NoError(t, store.Add(&AgentExecution{
		ID: "exec-1", SessionID: "session-1", ContainerID: "container-1",
	}))

	got, ok := store.GetByContainerID("container-1")
	require.True(t, ok)
	require.Equal(t, "exec-1", got.ID)

	_, ok = store.GetByContainerID("container-absent")
	require.False(t, ok)
}
