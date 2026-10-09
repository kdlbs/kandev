package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPrepareProgressCarriesNativeMCPDiagnostic(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-mcp-diagnostic-progress")
	var step PrepareStep
	require.NoError(t, json.Unmarshal([]byte(`{"name":"Cursor MCP approval","kind":"agent_mcp_approval","mcp_provider":"cursor","mcp_server_id":"plugin-example","status":"failed","mcp_diagnostic":{"operation":"enable","stage":"wait","kind":"wait_failed","message":"native helper failed"}}`), &step))
	mgr.newProgressCallback("task-mcp", "session-mcp")(step, 0, 1)

	payloads := prepareProgressPayloads(eventBus)
	require.Len(t, payloads, 1)
	encoded, err := json.Marshal(payloads[0])
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &payload))
	require.Contains(t, payload, "mcp_diagnostic")
}

func TestPreparationAttemptOptionalMCPFailureKeepsOverallSuccess(t *testing.T) {
	mgr, _ := newPrepareEventsTestManager(t, "profile-mcp-degraded")
	recorder := mgr.newPreparationAttemptRecorder("task-mcp", "session-mcp")
	recorder.AppendStep(PrepareStep{
		Name:   "MCP server",
		Kind:   PrepareStepKindAgentMCPVerification,
		Status: PrepareStepFailed,
	})

	result := mergePreparationAttemptResult(nil, recorder, "/workspace")

	require.True(t, result.Success)
	require.Equal(t, PrepareStepFailed, result.Steps[0].Status)
}

func TestPreparationAttemptKeepsEnvironmentOutcome(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "optional script failure", false: "fatal environment failure"}[success], func(t *testing.T) {
			recorder := newPrepareProgressRecorder(nil)
			recorder.AppendStep(PrepareStep{Name: "Run setup script", Status: PrepareStepFailed})
			result := mergePreparationAttemptResult(&EnvPrepareResult{Success: success}, recorder, "/workspace")
			require.Equal(t, success, result.Success)
			require.Equal(t, PrepareStepFailed, result.Steps[0].Status)
		})
	}
}

func TestExecutionPrepareCompletionFatalErrorUpdatesResultAndEvent(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-mcp-materialization-error")
	recorder := mgr.newPreparationAttemptRecorder("task-mcp", "session-mcp")
	recorder.AppendStep(PrepareStep{
		Name:   "MCP discovery",
		Kind:   PrepareStepKindAgentMCPDiscovery,
		Status: PrepareStepFailed,
	})
	execution := &AgentExecution{
		TaskID:        "task-mcp",
		SessionID:     "session-mcp",
		WorkspacePath: "/workspace",
		PrepareResult: &EnvPrepareResult{Success: true},
	}

	mgr.publishExecutionPrepareCompleted(execution, recorder, errors.New("materialization failed"))

	require.False(t, execution.PrepareResult.Success)
	require.Equal(t, "materialization failed", execution.PrepareResult.ErrorMessage)
	completed := prepareCompletedPayloads(eventBus)
	require.Len(t, completed, 1)
	require.False(t, completed[0].Success)
	require.Equal(t, "materialization failed", completed[0].ErrorMessage)
}

func TestLaunch_WorktreeResumePublishesPrepareCompleted(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-worktree-resume")
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register(models.ExecutorTypeWorktree, &progressPreparer{})

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-worktree-resume",
		SessionID:      "session-worktree-resume",
		ACPSessionID:   "acp-session-worktree-resume",
		AgentProfileID: "profile-worktree-resume",
		ExecutorType:   string(models.ExecutorTypeWorktree),
		RepositoryPath: "/tmp/repo",
		UseWorktree:    true,
		BaseBranch:     "main",
	})
	require.NoError(t, err)
	require.NotEmpty(t, prepareProgressPayloads(eventBus))

	completed := prepareCompletedPayloads(eventBus)
	require.Len(t, completed, 1)
	require.True(t, completed[0].Success)
	requirePrepareStep(t, completed[0].Steps, "Validate Docker")
}

func TestLaunch_PromotionPublishesPrepareCompletedAfterCursorMCPPreparation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	writeCursorMCPRecoveryPlugin(t, cursorHome)
	workspace := t.TempDir()
	profile := &AgentProfileInfo{
		ProfileID: "profile-promotion-mcp", AgentName: "cursor-acp",
		CursorPluginsMCPEnabled: true, MCPSelectionMode: "selected",
		MCPSelectedServers: []string{"plugin-harness-figma"},
	}
	manager, eventBus := newPrepareEventsTestManager(t, profile.ProfileID)
	manager.profileResolver = &mockPassthroughProfileResolver{profile: profile}
	manager.eventPublisher = NewEventPublisher(eventBus, manager.logger)
	inventoryStarted := make(chan struct{})
	releaseInventory := make(chan struct{})
	manager.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		close(inventoryStarted)
		<-releaseInventory
		return mcpconfig.CursorNativeInventory{}, nil
	}
	execution := &AgentExecution{
		ID: "workspace-only-mcp", TaskID: "task-promotion-mcp", SessionID: "session-promotion-mcp",
		AgentProfileID: profile.ProfileID, WorkspacePath: workspace, ExecutorType: "local",
		metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false},
	}
	execution.setRuntimeEnvironment(map[string]string{"HOME": home})
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	require.NoError(t, manager.executionStore.Add(execution))
	type launchOutcome struct {
		execution *AgentExecution
		err       error
	}
	done := make(chan launchOutcome, 1)
	go func() {
		got, err := manager.Launch(context.Background(), &LaunchRequest{
			TaskID: execution.TaskID, SessionID: execution.SessionID,
			AgentProfileID: profile.ProfileID, ExecutorType: "local",
		})
		done <- launchOutcome{execution: got, err: err}
	}()
	select {
	case <-inventoryStarted:
	case <-time.After(5 * time.Second):
		close(releaseInventory)
		t.Fatal("promotion did not reach Cursor MCP discovery")
	}
	require.Empty(t, prepareCompletedPayloads(eventBus), "promotion must not complete while MCP discovery is blocked")
	close(releaseInventory)
	outcome := <-done
	require.NoError(t, outcome.err)
	require.Same(t, execution, outcome.execution)
	completed := prepareCompletedPayloads(eventBus)
	require.Len(t, completed, 1)
	require.True(t, completed[0].Success)
	requirePrepareStep(t, completed[0].Steps, "Cursor MCP verification")
}

func TestLaunch_ResumeWithoutPreparationPublishesNoPrepareEvents(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-resume-without-preparation")

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-resume-without-preparation",
		SessionID:      "session-resume-without-preparation",
		ACPSessionID:   "acp-session-resume-without-preparation",
		AgentProfileID: "profile-resume-without-preparation",
		ExecutorType:   string(models.ExecutorTypeLocal),
		IsEphemeral:    true,
	})
	require.NoError(t, err)
	require.Empty(t, prepareProgressPayloads(eventBus))
	require.Empty(t, prepareCompletedPayloads(eventBus))
}

func TestPrepareProgressPublishesTypedRemoteHelperFields(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-remote-helper-progress")
	mgr.newProgressCallback("task-1", "session-1")(
		PrepareStep{
			Kind:           PrepareStepKindRemoteHelperDownload,
			RemotePlatform: "linux/amd64",
			FailureCode:    "timeout",
			Status:         PrepareStepFailed,
			Error:          "context deadline exceeded",
		},
		0,
		1,
	)

	payloads := prepareProgressPayloads(eventBus)
	require.Len(t, payloads, 1)
	require.Equal(t, PrepareStepKindRemoteHelperDownload, payloads[0].StepKind)
	require.Equal(t, "linux/amd64", payloads[0].RemotePlatform)
	require.Equal(t, "timeout", payloads[0].FailureCode)
}

func TestPrepareAttemptEventsCarryDurableOrderedIdentityAndMCPFields(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-mcp-events")
	recorder := mgr.newPreparationAttemptRecorder("task-mcp", "session-mcp")
	startedAt := recorder.preparationStartedAt
	index := recorder.AppendStep(PrepareStep{
		Name:        "MCP server",
		Kind:        PrepareStepKindAgentMCPVerification,
		MCPProvider: "cursor",
		MCPServerID: "plugin-fixture-server",
		Status:      PrepareStepRunning,
	})
	recorder.UpdateStep(index, PrepareStep{
		Name:        "MCP server",
		Kind:        PrepareStepKindAgentMCPVerification,
		MCPProvider: "cursor",
		MCPServerID: "plugin-fixture-server",
		Status:      PrepareStepCompleted,
	})
	mgr.publishLaunchPrepareCompleted(&LaunchRequest{TaskID: "task-mcp", SessionID: "session-mcp"}, nil, recorder, "/workspace", true, nil)

	progress := prepareProgressPayloads(eventBus)
	require.Len(t, progress, 2)
	require.NotEmpty(t, recorder.preparationID)
	for _, payload := range progress {
		require.Equal(t, recorder.preparationID, payload.PreparationID)
		require.Equal(t, startedAt.Format(time.RFC3339Nano), payload.PreparationStartedAt)
		require.Equal(t, "cursor", payload.MCPProvider)
		require.Equal(t, "plugin-fixture-server", payload.MCPServerID)
	}
	completed := prepareCompletedPayloads(eventBus)
	require.Len(t, completed, 1)
	require.Equal(t, recorder.preparationID, completed[0].PreparationID)
	require.Equal(t, startedAt.Format(time.RFC3339Nano), completed[0].PreparationStartedAt)
	require.Equal(t, PrepareStepKindAgentMCPVerification, completed[0].Steps[0].Kind)
	require.Equal(t, "plugin-fixture-server", completed[0].Steps[0].MCPServerID)

	serialized := SerializePrepareResult(&EnvPrepareResult{
		Success:              true,
		Steps:                recorder.Steps(),
		PreparationID:        recorder.preparationID,
		PreparationStartedAt: startedAt,
	})
	require.Equal(t, recorder.preparationID, serialized["preparation_id"])
	require.Equal(t, startedAt.Format(time.RFC3339Nano), serialized["preparation_started_at"])
}

func newPrepareEventsTestManager(t *testing.T, profileID string) (*Manager, *MockEventBusWithTracking) {
	t.Helper()
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(&createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, log),
	})
	eventBus := &MockEventBusWithTracking{}
	mgr := NewManager(
		newTestRegistry(), eventBus, execRegistry,
		&MockCredentialsManager{}, &countingProfileResolver{info: &AgentProfileInfo{
			ProfileID: profileID,
			AgentName: "auggie",
		}}, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	return mgr, eventBus
}

func prepareProgressPayloads(eventBus *MockEventBusWithTracking) []*PrepareProgressEventPayload {
	eventBus.mu.Lock()
	defer eventBus.mu.Unlock()
	var out []*PrepareProgressEventPayload
	for _, tracked := range eventBus.PublishedEvents {
		payload, ok := tracked.Event.Data.(*PrepareProgressEventPayload)
		if ok {
			out = append(out, payload)
		}
	}
	return out
}
