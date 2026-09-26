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

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
)

// registerRecoveryTestAgentProfile wires a resolvable agent profile onto mgr
// so a recovered execution's agent identity re-derivation
// (AC-EXECUTORS-SURVIVAL-002.14) succeeds. Tests that assert a recovered
// session lands in the store, but don't care about command-building details,
// use this instead of repeating the wiring; the matching fixture's
// AgentProfileID must be recoveryTestAgentProfileID.
const recoveryTestAgentProfileID = "profile-1"

func registerRecoveryTestAgentProfile(t *testing.T, mgr *Manager) {
	t.Helper()
	require.NoError(t, mgr.registry.Register(&cliFlagTestAgent{testAgent: testAgent{id: "recovery-test-agent"}}))
	mgr.profileResolver = &restartProfileResolver{profile: &AgentProfileInfo{
		ProfileID: recoveryTestAgentProfileID,
		AgentName: "recovery-test-agent",
	}}
}

type recordedRescan struct {
	WorkDir     string   `json:"work_dir"`
	SourceRoots []string `json:"workspace_source_roots"`
}

// newRescanServer serves a fixed status for the lifetime of the server. The
// status is passed by value rather than by pointer so the handler goroutine
// cannot race a mid-run write: these tests each want one status throughout.
// (fakeAgentctlProcessServer uses atomic.Int32 because it genuinely does flip
// its status mid-test; matching the shape to the requirement keeps the
// difference between the two helpers visible.)
func newRescanServer(t *testing.T, status int) (*[]recordedRescan, string) {
	t.Helper()
	var mu sync.Mutex
	recorded := &[]recordedRescan{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body recordedRescan
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		*recorded = append(*recorded, body)
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	return recorded, server.URL
}

// TestRescanWorkspaceForSessionForwardsWorkDirAndRoots pins what reaches
// agentctl and, critically, that the execution's recorded source roots are
// updated to match — a rescan that succeeded but left the old roots on the
// execution would send stale roots on the next call.
func TestRescanWorkspaceForSessionForwardsWorkDirAndRoots(t *testing.T) {
	recorded, url := newRescanServer(t, http.StatusOK)
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID: "exec-1", SessionID: "session-1",
		WorkspaceSourceRoots: []string{"/work/task-1/backend"},
		agentctl:             newTestAgentctlClient(t, url, newTestLogger()),
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	newRoots := []string{"/work/task-1/backend", "/work/task-1/frontend"}
	require.NoError(t, mgr.RescanWorkspaceForSession(
		context.Background(), "session-1", "/work/task-1", newRoots))

	require.Len(t, *recorded, 1)
	require.Equal(t, "/work/task-1", (*recorded)[0].WorkDir)
	require.Equal(t, newRoots, (*recorded)[0].SourceRoots)
	require.Equal(t, newRoots, execution.WorkspaceSourceRoots,
		"a successful rescan commits the new roots to the execution")
}

// TestRescanWorkspaceForSessionRollsBackRootsOnFailure pins the rollback: a
// failed rescan must not leave the execution claiming roots agentctl never
// accepted.
func TestRescanWorkspaceForSessionRollsBackRootsOnFailure(t *testing.T) {
	_, url := newRescanServer(t, http.StatusInternalServerError)
	mgr := newTestManager(t)
	original := []string{"/work/task-1/backend"}
	execution := &AgentExecution{
		ID: "exec-1", SessionID: "session-1",
		WorkspaceSourceRoots: original,
		agentctl:             newTestAgentctlClient(t, url, newTestLogger()),
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.RescanWorkspaceForSession(context.Background(), "session-1", "/work/task-1",
		[]string{"/work/task-1/backend", "/work/task-1/frontend"})

	require.ErrorContains(t, err, "rescan workspace via agentctl")
	require.Equal(t, original, execution.WorkspaceSourceRoots,
		"a failed rescan must restore the roots agentctl is actually tracking")
}

func TestRescanWorkspaceForSessionSkipsWithoutExecutionOrClient(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	require.ErrorContains(t, mgr.RescanWorkspaceForSession(ctx, "", ""), "session_id is required")
	require.NoError(t, mgr.RescanWorkspaceForSession(ctx, "session-absent", ""),
		"no execution means agentctl is not running; the next launch rebuilds trackers")

	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-1", SessionID: "session-1"}))
	require.NoError(t, mgr.RescanWorkspaceForSession(ctx, "session-1", ""),
		"an execution without an agentctl client has nothing to rescan")
}

func TestOptionalWorkspaceSourceRoots(t *testing.T) {
	current := []string{"/work/backend"}

	unchanged := optionalWorkspaceSourceRoots(current, nil)
	require.Equal(t, current, unchanged)
	require.NotSame(t, &current[0], &unchanged[0],
		"the caller must get a copy so a later mutation cannot reach the execution")

	replaced := optionalWorkspaceSourceRoots(current, [][]string{{"/work/frontend"}})
	require.Equal(t, []string{"/work/frontend"}, replaced)

	cleared := optionalWorkspaceSourceRoots(current, [][]string{nil})
	require.Empty(t, cleared, "an explicit empty root list clears the tracked roots")
}

// TestManagerStartRecoversInstancesIntoStore pins the restart path: instances
// a backend reports as still running are re-tracked so the frontend reattaches
// instead of creating a duplicate execution.
func TestManagerStartRecoversInstancesIntoStore(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-recovered",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: recoveryTestAgentProfileID,
			ContainerID:    "container-1",
			ContainerIP:    "10.0.0.2",
			WorkspacePath:  "/workspace",
			RuntimeName:    executor.NameStandalone,
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok, "a recovered instance must be re-tracked under its session")
	require.Equal(t, "exec-recovered", execution.ID)
	require.Equal(t, "container-1", execution.ContainerID)
	require.Equal(t, "/workspace", execution.WorkspacePath)
}

// TestManagerStartRecoveredExecutionCarriesEnvAndRunID pins
// AC-EXECUTORS-SURVIVAL-002.14's "runtime environment" and "run identity"
// reconstruction rows: both are sourced from the adopted instance's own
// environment (never the database), with run identity re-derived from its
// KANDEV_RUN_ID key.
func TestManagerStartRecoveredExecutionCarriesEnvAndRunID(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-recovered",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: recoveryTestAgentProfileID,
			RuntimeName:    executor.NameStandalone,
			Env:            map[string]string{"KANDEV_RUN_ID": "run-42", "PATH": "/usr/bin"},
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok)
	require.Equal(t, "run-42", execution.RunID)
	require.Equal(t, map[string]string{"KANDEV_RUN_ID": "run-42", "PATH": "/usr/bin"}, execution.RuntimeEnvironment())
}

// TestManagerStartRecoveredExecutionCarriesTaskEnvironmentID pins
// AC-EXECUTORS-SURVIVAL-002.14's "task-environment identity" reconstruction
// row: sourced from the durable store via the session's own task-environment
// reference (WorkspaceInfoProvider), never re-derived from the adopted
// instance, which carries no such identity at all.
func TestManagerStartRecoveredExecutionCarriesTaskEnvironmentID(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-recovered",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: recoveryTestAgentProfileID,
			RuntimeName:    executor.NameStandalone,
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetWorkspaceInfoProvider(&mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-1": {TaskEnvironmentID: "env-9"},
	}})

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok)
	require.Equal(t, "env-9", execution.TaskEnvironmentID)
}

// TestManagerStartRecoveredExecutionRefusesOnWorkspaceInfoLookupFailure pins
// Review round 3, finding 4: task-environment identity is one of
// AC-EXECUTORS-SURVIVAL-002.3's required reconstructed values, so a
// durable-store read failure during recovery must not be tolerated as a soft
// failure -- AC-EXECUTORS-SURVIVAL-002.13 forbids publishing the session as
// re-tracked when a required read fails, and this instance is instead routed
// to the AC-EXECUTORS-SURVIVAL-002.6 stop path. (An earlier version of this
// test pinned the opposite, buggy behavior.)
func TestManagerStartRecoveredExecutionRefusesOnWorkspaceInfoLookupFailure(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	mockExec := &MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-recovered",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: recoveryTestAgentProfileID,
			RuntimeName:    executor.NameStandalone,
		}},
	}
	registry.Register(mockExec)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetWorkspaceInfoProvider(&mockWorkspaceInfoProvider{err: errors.New("db down")})

	require.NoError(t, mgr.Start(context.Background()))

	_, ok := mgr.GetExecutionBySessionID("session-1")
	require.False(t, ok, "recovery must refuse to re-track when task-environment identity cannot be reconstructed")
	require.Len(t, mockExec.stopInstanceCalls, 1)
	require.Equal(t, "exec-recovered", mockExec.stopInstanceCalls[0].InstanceID)
}

// TestManagerStartRecoveredExecutionCarriesOfficeAgentProfileID pins
// AC-EXECUTORS-SURVIVAL-002.14's "Office profile identity" reconstruction
// row: sourced from the recovery-inventory record's own persisted metadata
// (a new key in the existing column), not re-derived or read from the
// instance.
func TestManagerStartRecoveredExecutionCarriesOfficeAgentProfileID(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-recovered",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: recoveryTestAgentProfileID,
			RuntimeName:    executor.NameStandalone,
			Metadata: map[string]interface{}{
				MetadataKeyOfficeAgentProfileID: "office-profile-9",
			},
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok)
	require.Equal(t, "office-profile-9", execution.OfficeAgentProfileID)
}

// TestManagerStartRecoveredExecutionLeavesOfficeAgentProfileIDEmptyForNonOffice
// pins that an absent metadata key (the common, non-Office case) is a
// legitimately empty value, not an error.
func TestManagerStartRecoveredExecutionLeavesOfficeAgentProfileIDEmptyForNonOffice(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-recovered",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: recoveryTestAgentProfileID,
			RuntimeName:    executor.NameStandalone,
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok)
	require.Empty(t, execution.OfficeAgentProfileID)
}

// TestManagerStartRecoveredExecutionCarriesWorkspaceSourceRoots pins
// AC-EXECUTORS-SURVIVAL-002.14's "workspace source roots" reconstruction
// row: sourced from the adopted instance, read back rather than pushed.
func TestManagerStartRecoveredExecutionCarriesWorkspaceSourceRoots(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:           "exec-recovered",
			TaskID:               "task-1",
			SessionID:            "session-1",
			AgentProfileID:       recoveryTestAgentProfileID,
			RuntimeName:          executor.NameStandalone,
			WorkspaceSourceRoots: []string{"/ws/task-1/backend", "/ws/task-1/frontend"},
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok)
	require.Equal(t, []string{"/ws/task-1/backend", "/ws/task-1/frontend"}, execution.WorkspaceSourceRoots)
}

// TestManagerStartRecoveredExecutionCarriesProviderSessionID pins
// AC-EXECUTORS-SURVIVAL-002.14's "provider session identity" reconstruction
// row: sourced from the adopted instance, which holds the live provider
// session, never from a durable/database value.
func TestManagerStartRecoveredExecutionCarriesProviderSessionID(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:        "exec-recovered",
			TaskID:            "task-1",
			SessionID:         "session-1",
			AgentProfileID:    recoveryTestAgentProfileID,
			RuntimeName:       executor.NameStandalone,
			ProviderSessionID: "provider-session-9",
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok)
	require.Equal(t, "provider-session-9", execution.ACPSessionID)
}

// agentIdentityHistoryTestAgent is a resolvable agent whose runtime config
// opts in to history-context injection, so
// TestManagerStartRecoveredExecutionCarriesAgentIdentityCommandsAndHistorySetting
// can assert historyEnabled alongside the built commands.
type agentIdentityHistoryTestAgent struct {
	testAgent
}

func (a *agentIdentityHistoryTestAgent) BuildCommand(_ agents.CommandOptions) agents.Command {
	return agents.Cmd("test-agent-cli", "--acp").Build()
}

// TestManagerStartRecoveredExecutionCarriesAgentIdentityCommandsAndHistorySetting
// pins AC-EXECUTORS-SURVIVAL-002.14's last reconstruction-table row: agent
// identity, the agent and continuation commands and their arguments, and the
// history setting are re-derived from the restored agent profile and the
// agent-type registry -- the same computation an ordinary launch performs --
// never read from the adopted instance.
func TestManagerStartRecoveredExecutionCarriesAgentIdentityCommandsAndHistorySetting(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-recovered",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: "profile-history",
			RuntimeName:    executor.NameStandalone,
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	require.NoError(t, mgr.registry.Register(&agentIdentityHistoryTestAgent{
		testAgent: testAgent{
			id: "history-agent",
			runtimeConfig: &agents.RuntimeConfig{
				Cmd:           agents.NewCommand("test-agent-cli"),
				SessionConfig: agents.SessionConfig{HistoryContextInjection: true},
			},
		},
	}))
	mgr.profileResolver = &restartProfileResolver{profile: &AgentProfileInfo{
		ProfileID: "profile-history",
		AgentName: "history-agent",
	}}

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok)
	require.Equal(t, "history-agent", execution.AgentID)
	require.Equal(t, "test-agent-cli --acp", execution.AgentCommand)
	require.Equal(t, []string{"test-agent-cli", "--acp"}, execution.AgentArgs)
	require.True(t, execution.historyEnabled, "history-context-injection agent must have historyEnabled restored")
}

// TestManagerStartRefusesAndStopsRecoveredExecutionWhenAgentIdentityCannotBeReconstructed
// pins AC-EXECUTORS-SURVIVAL-002.4's refusal path for
// AC-EXECUTORS-SURVIVAL-002.14's re-derivation row: when the restored
// AgentProfileID no longer resolves to a registered agent type (declared
// source answered, value not present), the instance is not re-tracked and is
// left to the stop path AC-EXECUTORS-SURVIVAL-002.6 defines.
func TestManagerStartRefusesAndStopsRecoveredExecutionWhenAgentIdentityCannotBeReconstructed(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	mockExecutor := &MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-unreconstructable",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: "profile-deleted",
			RuntimeName:    executor.NameStandalone,
		}},
	}
	registry.Register(mockExecutor)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	// No agent registered under "history-agent"/whatever this profile names,
	// and the profile resolver reports genuine failure -- either way,
	// getAgentConfigForExecution cannot answer.
	mgr.profileResolver = &restartProfileResolver{err: errors.New("profile deleted during outage")}

	require.NoError(t, mgr.Start(context.Background()))

	_, ok := mgr.GetExecutionBySessionID("session-1")
	require.False(t, ok, "an execution whose agent identity cannot be reconstructed must not be re-tracked")
	require.Len(t, mockExecutor.stopInstanceCalls, 1)
	require.Equal(t, "exec-unreconstructable", mockExecutor.stopInstanceCalls[0].InstanceID)
}

func TestManagerStartWithoutRegistryIsNoOp(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.Start(context.Background()))
	require.Empty(t, mgr.ListExecutions())
}
