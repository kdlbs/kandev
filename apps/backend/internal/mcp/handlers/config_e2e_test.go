package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	bus "github.com/kandev/kandev/internal/events/bus"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock event bus ---

type mockEventBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (m *mockEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBus) Subscribe(_ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *mockEventBus) QueueSubscribe(_, _ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *mockEventBus) Request(_ context.Context, _ string, _ *bus.Event, _ time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *mockEventBus) Close()            {}
func (m *mockEventBus) IsConnected() bool { return true }

// --- mock session checker ---

type mockSessionChecker struct{}

func (m *mockSessionChecker) HasActiveTaskSessionsByAgentProfile(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// --- test environment ---

type testEnv struct {
	handlers     *Handlers
	taskSvc      *service.Service
	workflowCtrl *workflowctrl.Controller
	agentCtrl    *agentsettingscontroller.Controller
	mcpConfigSvc *mcpconfig.Service
	db           *sql.DB
	sqlxDB       *sqlx.DB
	log          *logger.Logger
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)

	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")

	// Task repository (also provides workspace/workflow repos)
	taskRepo, taskCleanup, err := taskrepository.Provide(sqlxDB, sqlxDB)
	require.NoError(t, err)

	// Worktree store (required by task repo schema)
	_, err = worktree.NewSQLiteStore(sqlxDB, sqlxDB)
	require.NoError(t, err)

	// Workflow repo + service
	_, workflowSvc, wfCleanup, err := workflow.Provide(sqlxDB, sqlxDB, testLogger(t))
	require.NoError(t, err)
	wfCtrl := workflowctrl.NewController(workflowSvc)

	// Agent settings store + controller
	agentSettingsRepo, agentCleanup, err := store.Provide(sqlxDB, sqlxDB)
	require.NoError(t, err)

	agentRegistry := registry.NewRegistry(testLogger(t))
	agentRegistry.LoadDefaults()

	// Enable the mock agent so it appears in discovery
	if mockAg, ok := agentRegistry.Get("mock-agent"); ok {
		mockAg.(*agents.MockAgent).SetEnabled(true)
	}

	// Create a discovery registry from enabled agents
	discoveryReg, err := discovery.LoadRegistry(context.Background(), agentRegistry, testLogger(t))
	require.NoError(t, err)

	agentCtrl := agentsettingscontroller.NewController(
		agentSettingsRepo,
		discoveryReg,
		agentRegistry,
		&mockSessionChecker{},
		testLogger(t),
	)

	// MCP config service
	mcpSvc := mcpconfig.NewService(agentSettingsRepo)

	// Task service
	eventBus := &mockEventBus{}
	log := testLogger(t)
	taskSvc := service.NewService(service.Repos{
		Workspaces:   taskRepo,
		Tasks:        taskRepo,
		TaskRepos:    taskRepo,
		Workflows:    taskRepo,
		Messages:     taskRepo,
		Turns:        taskRepo,
		Sessions:     taskRepo,
		GitSnapshots: taskRepo,
		RepoEntities: taskRepo,
		Executors:    taskRepo,
		Environments: taskRepo,
		Reviews:      taskRepo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})

	// Create handlers with all deps
	h := NewHandlers(taskSvc, wfCtrl, nil, nil, nil, nil, eventBus, nil, log)
	h.SetConfigDeps(workflowSvc, agentCtrl, mcpSvc)

	t.Cleanup(func() {
		_ = sqlxDB.Close()
		_ = taskCleanup()
		_ = wfCleanup()
		_ = agentCleanup()
	})

	return &testEnv{
		handlers:     h,
		taskSvc:      taskSvc,
		workflowCtrl: wfCtrl,
		agentCtrl:    agentCtrl,
		mcpConfigSvc: mcpSvc,
		db:           dbConn,
		sqlxDB:       sqlxDB,
		log:          log,
	}
}

// callHandler invokes a handler via ws.Message and returns the response.
func callHandler(t *testing.T, h *Handlers, action string, payload interface{}) *ws.Message {
	t.Helper()
	msg := makeWSMessage(t, action, payload)
	resp, err := dispatchAction(h, action, context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

// dispatchAction routes an action to the correct handler method.
func dispatchAction(h *Handlers, action string, ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	switch action {
	// Workflow
	case ws.ActionMCPCreateWorkflowStep:
		return h.handleCreateWorkflowStep(ctx, msg)
	case ws.ActionMCPUpdateWorkflowStep:
		return h.handleUpdateWorkflowStep(ctx, msg)
	case ws.ActionMCPDeleteWorkflowStep:
		return h.handleDeleteWorkflowStep(ctx, msg)
	case ws.ActionMCPReorderWorkflowStep:
		return h.handleReorderWorkflowSteps(ctx, msg)
	// Agent
	case ws.ActionMCPListAgents:
		return h.handleListAgents(ctx, msg)
	case ws.ActionMCPCreateAgent:
		return h.handleCreateAgent(ctx, msg)
	case ws.ActionMCPUpdateAgent:
		return h.handleUpdateAgent(ctx, msg)
	case ws.ActionMCPDeleteAgent:
		return h.handleDeleteAgent(ctx, msg)
	case ws.ActionMCPListAgentProfiles:
		return h.handleListAgentProfiles(ctx, msg)
	case ws.ActionMCPUpdateAgentProfile:
		return h.handleUpdateAgentProfile(ctx, msg)
	// MCP Config
	case ws.ActionMCPGetMcpConfig:
		return h.handleGetMcpConfig(ctx, msg)
	case ws.ActionMCPUpdateMcpConfig:
		return h.handleUpdateMcpConfig(ctx, msg)
	// Task
	case ws.ActionMCPMoveTask:
		return h.handleMoveTask(ctx, msg)
	case ws.ActionMCPDeleteTask:
		return h.handleDeleteTask(ctx, msg)
	case ws.ActionMCPArchiveTask:
		return h.handleArchiveTask(ctx, msg)
	case ws.ActionMCPUpdateTaskState:
		return h.handleUpdateTaskState(ctx, msg)
	default:
		return nil, nil
	}
}

func assertWSSuccess(t *testing.T, resp *ws.Message) {
	t.Helper()
	if resp.Type == ws.MessageTypeError {
		var ep ws.ErrorPayload
		_ = json.Unmarshal(resp.Payload, &ep)
		t.Fatalf("expected success but got error: code=%s message=%s", ep.Code, ep.Message)
	}
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)
}

func decodePayload(t *testing.T, resp *ws.Message) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	return result
}

// seedWorkspace creates a workspace and workflow for task tests.
func seedWorkspace(t *testing.T, env *testEnv) (workspaceID, workflowID string) {
	t.Helper()
	ctx := context.Background()

	ws, err := env.taskSvc.CreateWorkspace(ctx, &service.CreateWorkspaceRequest{
		Name:    "Test Workspace",
		OwnerID: "test-owner",
	})
	require.NoError(t, err)

	wf, err := env.taskSvc.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID: ws.ID,
		Name:        "Test Workflow",
	})
	require.NoError(t, err)

	return ws.ID, wf.ID
}

// =============================================================================
// Workflow Step E2E Tests
// =============================================================================

func TestE2E_WorkflowStep_CreateAndRead(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Review",
		"color":       "#3b82f6",
		"position":    0,
	})
	assertWSSuccess(t, resp)

	payload := decodePayload(t, resp)
	step, ok := payload["step"].(map[string]interface{})
	require.True(t, ok, "expected step in response")
	assert.Equal(t, "Review", step["name"])
	assert.NotEmpty(t, step["id"])
}

func TestE2E_WorkflowStep_CreateWithPrompt(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Deploy",
		"color":       "#ef4444",
		"position":    0,
		"prompt":      "Deploy the application to production",
	})
	assertWSSuccess(t, resp)

	payload := decodePayload(t, resp)
	step := payload["step"].(map[string]interface{})
	assert.Equal(t, "Deploy", step["name"])
	assert.Equal(t, "Deploy the application to production", step["prompt"])
}

func TestE2E_WorkflowStep_CreateAsStartStep(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	isStartStep := true
	resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id":   workflowID,
		"name":          "Backlog",
		"color":         "#6b7280",
		"position":      0,
		"is_start_step": isStartStep,
	})
	assertWSSuccess(t, resp)

	payload := decodePayload(t, resp)
	step := payload["step"].(map[string]interface{})
	assert.Equal(t, true, step["is_start_step"])
}

func TestE2E_WorkflowStep_Update(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	// Create step first
	createResp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Original",
		"color":       "#000000",
		"position":    0,
	})
	assertWSSuccess(t, createResp)
	createPayload := decodePayload(t, createResp)
	stepID := createPayload["step"].(map[string]interface{})["id"].(string)

	// Update the step
	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateWorkflowStep, map[string]interface{}{
		"step_id": stepID,
		"name":    "Updated",
		"color":   "#ffffff",
	})
	assertWSSuccess(t, updateResp)

	updatePayload := decodePayload(t, updateResp)
	step := updatePayload["step"].(map[string]interface{})
	assert.Equal(t, "Updated", step["name"])
	assert.Equal(t, "#ffffff", step["color"])
}

func TestE2E_WorkflowStep_UpdatePartial(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	createResp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Step1",
		"color":       "#aaa",
		"position":    0,
	})
	assertWSSuccess(t, createResp)
	stepID := decodePayload(t, createResp)["step"].(map[string]interface{})["id"].(string)

	// Only update color, name should stay the same
	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateWorkflowStep, map[string]interface{}{
		"step_id": stepID,
		"color":   "#bbb",
	})
	assertWSSuccess(t, updateResp)

	step := decodePayload(t, updateResp)["step"].(map[string]interface{})
	assert.Equal(t, "Step1", step["name"]) // unchanged
	assert.Equal(t, "#bbb", step["color"]) // updated
}

func TestE2E_WorkflowStep_Delete(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	// Create step
	createResp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Doomed",
		"color":       "#000",
		"position":    0,
	})
	assertWSSuccess(t, createResp)
	stepID := decodePayload(t, createResp)["step"].(map[string]interface{})["id"].(string)

	// Delete it
	deleteResp := callHandler(t, env.handlers, ws.ActionMCPDeleteWorkflowStep, map[string]interface{}{
		"step_id": stepID,
	})
	assertWSSuccess(t, deleteResp)
	deletePayload := decodePayload(t, deleteResp)
	assert.Equal(t, true, deletePayload["success"])

	// Verify it's gone — update should fail
	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateWorkflowStep, map[string]interface{}{
		"step_id": stepID,
		"name":    "Ghost",
	})
	assert.Equal(t, ws.MessageTypeError, updateResp.Type)
}

func TestE2E_WorkflowStep_DeleteNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPDeleteWorkflowStep, map[string]interface{}{
		"step_id": "nonexistent-step-id",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestE2E_WorkflowStep_Reorder(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	// Create 3 steps
	var stepIDs []string
	for _, name := range []string{"Step1", "Step2", "Step3"} {
		resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
			"workflow_id": workflowID,
			"name":        name,
			"color":       "#000",
			"position":    len(stepIDs),
		})
		assertWSSuccess(t, resp)
		id := decodePayload(t, resp)["step"].(map[string]interface{})["id"].(string)
		stepIDs = append(stepIDs, id)
	}

	// Reorder: reverse
	reorderResp := callHandler(t, env.handlers, ws.ActionMCPReorderWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"step_ids":    []string{stepIDs[2], stepIDs[1], stepIDs[0]},
	})
	assertWSSuccess(t, reorderResp)
	reorderPayload := decodePayload(t, reorderResp)
	assert.Equal(t, true, reorderPayload["success"])
}

func TestE2E_WorkflowStep_UpdateNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPUpdateWorkflowStep, map[string]interface{}{
		"step_id": "nonexistent-id",
		"name":    "Ghost",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

// =============================================================================
// Agent E2E Tests
// =============================================================================

func TestE2E_Agent_ListEmpty(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPListAgents, map[string]interface{}{})
	assertWSSuccess(t, resp)

	payload := decodePayload(t, resp)
	agents, ok := payload["agents"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, agents)
}

func TestE2E_Agent_CreateAndList(t *testing.T) {
	env := setupTestEnv(t)

	// Create an agent
	createResp := callHandler(t, env.handlers, ws.ActionMCPCreateAgent, map[string]interface{}{
		"name": "mock-agent",
	})
	assertWSSuccess(t, createResp)

	createPayload := decodePayload(t, createResp)
	assert.NotEmpty(t, createPayload["id"])
	assert.Equal(t, "mock-agent", createPayload["name"])

	// List agents
	listResp := callHandler(t, env.handlers, ws.ActionMCPListAgents, map[string]interface{}{})
	assertWSSuccess(t, listResp)

	listPayload := decodePayload(t, listResp)
	agents := listPayload["agents"].([]interface{})
	assert.Len(t, agents, 1)
}

func TestE2E_Agent_CreateWithWorkspaceID(t *testing.T) {
	env := setupTestEnv(t)
	workspaceID, _ := seedWorkspace(t, env)

	resp := callHandler(t, env.handlers, ws.ActionMCPCreateAgent, map[string]interface{}{
		"name":         "mock-agent",
		"workspace_id": workspaceID,
	})
	assertWSSuccess(t, resp)

	payload := decodePayload(t, resp)
	assert.Equal(t, "mock-agent", payload["name"])
}

func TestE2E_Agent_Update(t *testing.T) {
	env := setupTestEnv(t)

	// Create
	createResp := callHandler(t, env.handlers, ws.ActionMCPCreateAgent, map[string]interface{}{
		"name": "mock-agent",
	})
	assertWSSuccess(t, createResp)
	agentID := decodePayload(t, createResp)["id"].(string)

	// Update supports_mcp
	supportsMCP := true
	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateAgent, map[string]interface{}{
		"agent_id":     agentID,
		"supports_mcp": supportsMCP,
	})
	assertWSSuccess(t, updateResp)

	updatePayload := decodePayload(t, updateResp)
	assert.Equal(t, true, updatePayload["supports_mcp"])
}

func TestE2E_Agent_UpdateNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPUpdateAgent, map[string]interface{}{
		"agent_id":     "nonexistent",
		"supports_mcp": true,
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestE2E_Agent_Delete(t *testing.T) {
	env := setupTestEnv(t)

	// Create
	createResp := callHandler(t, env.handlers, ws.ActionMCPCreateAgent, map[string]interface{}{
		"name": "mock-agent",
	})
	assertWSSuccess(t, createResp)
	agentID := decodePayload(t, createResp)["id"].(string)

	// Delete
	deleteResp := callHandler(t, env.handlers, ws.ActionMCPDeleteAgent, map[string]interface{}{
		"agent_id": agentID,
	})
	assertWSSuccess(t, deleteResp)

	// List should be empty
	listResp := callHandler(t, env.handlers, ws.ActionMCPListAgents, map[string]interface{}{})
	assertWSSuccess(t, listResp)
	agents := decodePayload(t, listResp)["agents"].([]interface{})
	assert.Empty(t, agents)
}

func TestE2E_Agent_DeleteNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPDeleteAgent, map[string]interface{}{
		"agent_id": "nonexistent",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestE2E_Agent_CreateMultipleAndList(t *testing.T) {
	env := setupTestEnv(t)

	// Use distinct valid agent type IDs from the registry
	agentNames := []string{"mock-agent", "claude-code", "codex"}
	for _, name := range agentNames {
		resp := callHandler(t, env.handlers, ws.ActionMCPCreateAgent, map[string]interface{}{
			"name": name,
		})
		assertWSSuccess(t, resp)
	}

	listResp := callHandler(t, env.handlers, ws.ActionMCPListAgents, map[string]interface{}{})
	assertWSSuccess(t, listResp)
	agents := decodePayload(t, listResp)["agents"].([]interface{})
	assert.Len(t, agents, 3)
}

// createAgentWithProfile creates an agent using the controller directly (with a default profile).
func createAgentWithProfile(t *testing.T, env *testEnv) (agentID, profileID string) {
	t.Helper()
	ctx := context.Background()
	agent, err := env.agentCtrl.CreateAgent(ctx, agentsettingscontroller.CreateAgentRequest{
		Name: "mock-agent",
		Profiles: []agentsettingscontroller.CreateAgentProfileRequest{
			{Name: "Default", Model: "mock-default"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, agent.Profiles)
	return agent.ID, agent.Profiles[0].ID
}

// =============================================================================
// Agent Profile E2E Tests
// =============================================================================

func TestE2E_AgentProfile_ListProfiles(t *testing.T) {
	env := setupTestEnv(t)

	agentID, _ := createAgentWithProfile(t, env)

	// List profiles via handler
	listResp := callHandler(t, env.handlers, ws.ActionMCPListAgentProfiles, map[string]interface{}{
		"agent_id": agentID,
	})
	assertWSSuccess(t, listResp)

	payload := decodePayload(t, listResp)
	profiles := payload["profiles"].([]interface{})
	assert.GreaterOrEqual(t, len(profiles), 1)
}

func TestE2E_AgentProfile_ListProfilesNonexistentAgent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPListAgentProfiles, map[string]interface{}{
		"agent_id": "nonexistent",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestE2E_AgentProfile_Update(t *testing.T) {
	env := setupTestEnv(t)

	_, profileID := createAgentWithProfile(t, env)

	// Update the profile via handler
	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateAgentProfile, map[string]interface{}{
		"profile_id": profileID,
		"name":       "Custom Profile",
		"model":      "claude-3.5-sonnet",
	})
	assertWSSuccess(t, updateResp)

	updatePayload := decodePayload(t, updateResp)
	assert.Equal(t, "Custom Profile", updatePayload["name"])
	assert.Equal(t, "claude-3.5-sonnet", updatePayload["model"])
}

func TestE2E_AgentProfile_UpdateAutoApprove(t *testing.T) {
	env := setupTestEnv(t)

	_, profileID := createAgentWithProfile(t, env)

	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateAgentProfile, map[string]interface{}{
		"profile_id":   profileID,
		"auto_approve": true,
	})
	assertWSSuccess(t, updateResp)

	payload := decodePayload(t, updateResp)
	assert.Equal(t, true, payload["auto_approve"])
}

func TestE2E_AgentProfile_UpdateNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPUpdateAgentProfile, map[string]interface{}{
		"profile_id": "nonexistent",
		"name":       "Ghost",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

// =============================================================================
// MCP Config E2E Tests
// =============================================================================

func TestE2E_McpConfig_GetDefault(t *testing.T) {
	env := setupTestEnv(t)

	_, profileID := createAgentWithProfile(t, env)

	// Get MCP config — should return defaults
	getResp := callHandler(t, env.handlers, ws.ActionMCPGetMcpConfig, map[string]interface{}{
		"profile_id": profileID,
	})
	assertWSSuccess(t, getResp)

	payload := decodePayload(t, getResp)
	_, hasEnabled := payload["enabled"]
	assert.True(t, hasEnabled)
}

func TestE2E_McpConfig_UpdateAndGet(t *testing.T) {
	env := setupTestEnv(t)

	_, profileID := createAgentWithProfile(t, env)

	// Update MCP config
	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateMcpConfig, map[string]interface{}{
		"profile_id": profileID,
		"enabled":    true,
		"servers": map[string]interface{}{
			"my-server": map[string]interface{}{
				"command": "npx",
				"args":    []string{"-y", "@my/mcp-server"},
			},
		},
	})
	assertWSSuccess(t, updateResp)

	// Get and verify
	getResp := callHandler(t, env.handlers, ws.ActionMCPGetMcpConfig, map[string]interface{}{
		"profile_id": profileID,
	})
	assertWSSuccess(t, getResp)

	payload := decodePayload(t, getResp)
	assert.Equal(t, true, payload["enabled"])
	servers, ok := payload["servers"].(map[string]interface{})
	require.True(t, ok)
	_, hasMyServer := servers["my-server"]
	assert.True(t, hasMyServer, "expected my-server in servers")
}

func TestE2E_McpConfig_UpdateToggleEnabled(t *testing.T) {
	env := setupTestEnv(t)

	_, profileID := createAgentWithProfile(t, env)

	// Enable
	callHandler(t, env.handlers, ws.ActionMCPUpdateMcpConfig, map[string]interface{}{
		"profile_id": profileID,
		"enabled":    true,
	})

	// Disable
	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateMcpConfig, map[string]interface{}{
		"profile_id": profileID,
		"enabled":    false,
	})
	assertWSSuccess(t, updateResp)

	getResp := callHandler(t, env.handlers, ws.ActionMCPGetMcpConfig, map[string]interface{}{
		"profile_id": profileID,
	})
	assertWSSuccess(t, getResp)
	payload := decodePayload(t, getResp)
	assert.Equal(t, false, payload["enabled"])
}

func TestE2E_McpConfig_GetNonexistentProfile(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPGetMcpConfig, map[string]interface{}{
		"profile_id": "nonexistent",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}
