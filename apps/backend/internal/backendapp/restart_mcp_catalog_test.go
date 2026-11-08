package backendapp

import (
	"context"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	actlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentctlapi "github.com/kandev/kandev/internal/agentctl/server/api"
	agentctlconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	agentctlprocess "github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/events/bus"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/kandev/kandev/internal/orchestrator"
	executorpkg "github.com/kandev/kandev/internal/orchestrator/executor"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const resumeCatalogExecutionID = "resume-catalog-helper"

// resumeCatalogHelper is one workspace-only agentctl helper plus the lifecycle
// execution that owns it. The helper is created with the same base profile
// workspace access uses, so optional tools can only arrive through the
// production binding operation.
type resumeCatalogHelper struct {
	harness   bootStateTestHarness
	server    *httptest.Server
	mcp       *mcpserver.Server
	execution *lifecycle.AgentExecution
	executor  *executorpkg.Executor
	task      *models.Task
	session   *models.TaskSession
	adapter   *lifecycleAdapter
	profile   p.Context
}

type resumeCatalogCase struct {
	name                 string
	pendingTitle         bool
	ownerIsSelf          bool
	canvases             bool
	configurationPackage bool
	wantTitle            bool
	wantCanvas           bool
}

type resumeCatalogPreferenceWriter interface {
	PutBuiltinTaskPreferences(context.Context, *mcpconfig.BuiltinTaskPreferences, int64, int64, p.Surface) (*mcpconfig.BuiltinPolicySnapshot, error)
}

// @covers AC-AGENTS-MCP-PROFILE-CONTINUITY-001.1
// @covers AC-AGENTS-MCP-PROFILE-CONTINUITY-001.2
// @covers AC-AGENTS-MCP-PROFILE-CONTINUITY-001.3
func TestResumeWorkspaceHelperMCPToolInventory(t *testing.T) {
	cases := resumeCatalogToolCases()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			helper := newResumeCatalogHelper(t, tc)
			before := listMCPToolNames(t, helper.server.URL+"/mcp")
			require.NotContains(t, before, "list_canvases_kandev", "a workspace helper must start without optional canvas tools")
			require.NotContains(t, before, "set_task_title_kandev", "a workspace helper must start without the title tool")

			promoteResumeCatalogHelper(t, helper)

			names := listMCPToolNames(t, helper.server.URL+"/mcp")
			resolved, err := helper.executor.PrepareTaskSessionMCPProfile(context.Background(), helper.task.ID, helper.session, true)
			require.NoError(t, err)
			require.Equal(t, tc.wantTitle, resolved.HasCapability(p.CapabilityTaskTitle),
				"promotion must preserve the owned title capability")
			require.Equal(t, tc.wantCanvas, resolved.HasCapability(p.CapabilityCanvas),
				"promotion must preserve the canvas capability gate")
			direct := listMCPToolNames(t, freshResumeCatalogURL(t, helper.profile))
			require.ElementsMatch(t, direct, names,
				"a promoted workspace helper must expose the same inventory as a direct start")
			assertResumeOptionalTools(t, names, tc)
			assertResumeBinding(t, helper)
		})
	}
}

// @covers AC-AGENTS-MCP-PROFILE-CONTINUITY-001.4
func TestResumeWorkspaceHelperMCPGuidance(t *testing.T) {
	cases := resumeCatalogToolCases()
	cases = append(cases, resumeCatalogCase{
		name: "task configuration package", pendingTitle: true, ownerIsSelf: true, canvases: true,
		configurationPackage: true, wantTitle: true, wantCanvas: true,
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			helper := newResumeCatalogHelper(t, tc)
			promoteResumeCatalogHelper(t, helper)
			names := listMCPToolNames(t, helper.server.URL+"/mcp")
			startupContext := captureResumeCatalogStartupContext(t, helper)
			assertResumeCapabilityGuidance(t, startupContext, names, tc)
			assertResumeGuidanceMatchesProfile(t, startupContext, helper.profile.BuiltinPackages, helper.mcp.BuiltinCatalog())
			require.Contains(t, startupContext, "MCP DISCOVERY:")
			require.Contains(t, startupContext, "native tool search or discovery")
			require.Contains(t, startupContext, "BUILT-IN PACKAGES:")
			require.Contains(t, startupContext, "supersedes earlier optional-tool guidance")
			require.NotContains(t, startupContext, "Do not infer tool availability from previous turns")
		})
	}
}

func resumeCatalogToolCases() []resumeCatalogCase {
	return []resumeCatalogCase{
		{name: "pending title owner", pendingTitle: true, ownerIsSelf: true, canvases: true, wantTitle: true, wantCanvas: true},
		{name: "non-owner session", pendingTitle: true, ownerIsSelf: false, canvases: true, wantTitle: false, wantCanvas: true},
		{name: "settled title", pendingTitle: false, ownerIsSelf: true, canvases: true, wantTitle: false, wantCanvas: true},
		{name: "canvas disabled", pendingTitle: true, ownerIsSelf: true, canvases: false, wantTitle: true, wantCanvas: false},
	}
}

func captureResumeCatalogStartupContext(t *testing.T, helper *resumeCatalogHelper) string {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, helper.harness.taskRepo.UpdateTaskSessionState(ctx, helper.session.ID, models.TaskSessionStateCreated, ""))
	helper.session.State = models.TaskSessionStateCreated
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	taskhandlers.RegisterMessageRoutes(router, dispatcher, helper.harness.taskSvc, &resumeCatalogMessageOrchestrator{
		executor: helper.executor, task: helper.task, session: helper.session,
	}, logger.Default())
	request, err := ws.NewRequest("resume-catalog-guidance", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": helper.task.ID, "session_id": helper.session.ID, "content": "resume this work",
	})
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(ctx, request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	messages, err := helper.harness.taskRepo.ListMessages(ctx, helper.session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	return messages[0].Content
}

// resumeCatalogMessageOrchestrator drives the production message handler. It
// resolves title and canvas guidance through the same task state and executor
// profile the production orchestrator uses.
type resumeCatalogMessageOrchestrator struct {
	executor *executorpkg.Executor
	task     *models.Task
	session  *models.TaskSession
	started  []string
}

func (o *resumeCatalogMessageOrchestrator) PromptTask(_ context.Context, _, _, prompt, _ string, _ bool, _ []v1.MessageAttachment, _ bool) (*orchestrator.PromptResult, error) {
	o.started = append(o.started, prompt)
	return &orchestrator.PromptResult{}, nil
}

func (o *resumeCatalogMessageOrchestrator) ResumeTaskSession(context.Context, string, string) error {
	return nil
}

func (o *resumeCatalogMessageOrchestrator) StartCreatedSession(_ context.Context, _, _, _, prompt string, _, _, _ bool, _ []v1.MessageAttachment, _ []v1.EntityReference) error {
	o.started = append(o.started, prompt)
	return nil
}

func (o *resumeCatalogMessageOrchestrator) StartCreatedSessionWithDeliverySubmission(_ context.Context, _, _, _, prompt string, _ orchestrator.DirectPromptStartOptions) (*executorpkg.TaskExecution, error) {
	o.started = append(o.started, prompt)
	return &executorpkg.TaskExecution{}, nil
}

func (o *resumeCatalogMessageOrchestrator) ProcessOnTurnStart(context.Context, string, string) (orchestrator.ProcessOnTurnStartResult, error) {
	return orchestrator.ProcessOnTurnStartResult{}, nil
}

func (o *resumeCatalogMessageOrchestrator) QueueUserPrompt(_ context.Context, _, _, prompt, _ string, _ bool, _ []v1.MessageAttachment, _ map[string]interface{}, _ bool) error {
	o.started = append(o.started, prompt)
	return nil
}

func (o *resumeCatalogMessageOrchestrator) StepRequiresCompletionSignal(context.Context, string) bool {
	return false
}

func (o *resumeCatalogMessageOrchestrator) ForegroundActivity(string) v1.ForegroundActivity {
	return v1.ForegroundActivityGenerating
}

func (o *resumeCatalogMessageOrchestrator) SteerEligible(string, models.TaskSessionState) bool {
	return false
}

func (o *resumeCatalogMessageOrchestrator) SteerTask(_ context.Context, _, _, prompt, _ string, _ bool, _ []v1.MessageAttachment) (*orchestrator.PromptResult, error) {
	o.started = append(o.started, prompt)
	return &orchestrator.PromptResult{}, nil
}

func (o *resumeCatalogMessageOrchestrator) ClaimTaskTitleSession(_ context.Context, _, sessionID string) (bool, error) {
	return models.IsAgentTitleOwner(o.task.Metadata, sessionID), nil
}

func (o *resumeCatalogMessageOrchestrator) TaskSessionCanvasGuidanceEnabled(ctx context.Context, taskID, _ string) (bool, error) {
	profile, err := o.executor.ResolveTaskSessionMCPProfile(ctx, taskID, o.session, true)
	if err != nil {
		return false, err
	}
	return profile.HasCapability(p.CapabilityCanvas), nil
}

func (o *resumeCatalogMessageOrchestrator) PrepareTaskSessionMCPPrompt(ctx context.Context, taskID, _ string) (context.Context, error) {
	return o.executor.PrepareMCPLaunch(ctx, taskID, o.session, true)
}

func assertResumeCapabilityGuidance(t *testing.T, startupContext string, names []string, tc resumeCatalogCase) {
	t.Helper()
	require.Equal(t, tc.wantCanvas, strings.Contains(startupContext, "CANVAS AUTHORING:"),
		"canvas guidance must follow the resolved capability")
	// Guidance advertises a selected canvas subset, not the complete catalog.
	for _, name := range canvasGuidanceToolNames() {
		if tc.wantCanvas {
			require.Contains(t, startupContext, name)
			require.Contains(t, names, name, "canvas guidance must advertise only delivered tools")
			continue
		}
		require.NotContains(t, startupContext, name)
		require.NotContains(t, names, name)
	}
	require.Equal(t, tc.wantTitle, strings.Contains(startupContext, "- set_task_title_kandev:"),
		"title guidance must follow owned eligibility")
	if tc.wantTitle {
		require.Contains(t, startupContext, "Call this as your first action")
		require.Contains(t, names, "set_task_title_kandev")
		return
	}
	require.NotContains(t, startupContext, "set_task_title_kandev")
	require.NotContains(t, names, "set_task_title_kandev")
}

func canvasGuidanceToolNames() []string {
	return []string{"create_canvas_kandev", "read_canvas_authoring_skill_kandev", "publish_canvas_kandev"}
}

func promoteResumeCatalogHelper(t *testing.T, helper *resumeCatalogHelper) {
	t.Helper()
	executionID, sessionID := helper.execution.ID, helper.execution.SessionID
	nativeConversationID := helper.execution.ACPSessionID
	profile := helper.profile
	_, err := helper.adapter.LaunchAgent(context.Background(), &executorpkg.LaunchAgentRequest{
		TaskID: helper.execution.TaskID, SessionID: sessionID,
		WorkspaceID: helper.execution.WorkspaceID, AgentProfileID: "claude-acp",
		ACPSessionID: nativeConversationID, McpProfile: &profile,
	})
	require.NoError(t, err)
	require.NotEmpty(t, helper.execution.AgentCommand, "promotion must configure the helper command after binding")
	require.Equal(t, executionID, helper.execution.ID, "resume must preserve the execution identity")
	require.Equal(t, sessionID, helper.execution.SessionID, "resume must preserve the task session identity")
	require.Equal(t, nativeConversationID, helper.execution.ACPSessionID,
		"resume must preserve the saved native conversation identity")
}

func assertResumeOptionalTools(t *testing.T, names []string, tc resumeCatalogCase) {
	t.Helper()
	for _, name := range canvasToolNames() {
		if tc.wantCanvas {
			require.Contains(t, names, name)
		} else {
			require.NotContains(t, names, name)
		}
	}
	if tc.wantTitle {
		require.Contains(t, names, "set_task_title_kandev")
	} else {
		require.NotContains(t, names, "set_task_title_kandev")
	}
	require.Contains(t, names, "list_workspaces_kandev")
}

func assertResumeBinding(t *testing.T, helper *resumeCatalogHelper) {
	t.Helper()
	value, present := helper.execution.MetadataSnapshot()[p.BuiltinBindingMetadataKey]
	require.True(t, present, "the promoted helper must persist its accepted profile")
	bound, err := p.ParseBuiltinBindingMetadata(value)
	require.NoError(t, err)
	expected, err := p.MarshalBuiltinBinding(p.Normalize(helper.profile))
	require.NoError(t, err)
	actual, err := p.MarshalBuiltinBinding(*bound)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func assertResumeGuidanceMatchesProfile(t *testing.T, startupContext string, manifest *p.BuiltinManifest, catalog map[p.PackageID][]string) {
	t.Helper()
	require.NotNil(t, manifest)
	require.NotEmpty(t, catalog[p.PackageCore])
	for _, descriptor := range p.BuiltinPackageCatalog() {
		if descriptor.ID == p.PackageCore {
			continue
		}
		if containsPackage(manifest.Packages, descriptor.ID) {
			require.Contains(t, startupContext, descriptor.RuntimeName)
			require.NotEmpty(t, catalog[descriptor.ID], "startup context for %s must match a delivered package endpoint", descriptor.RuntimeName)
			continue
		}
		require.NotContains(t, startupContext, descriptor.RuntimeName)
		require.NotContains(t, catalog, descriptor.ID)
	}
	require.Len(t, catalog, len(manifest.Packages))
}

func containsPackage(packages []p.PackageID, id p.PackageID) bool {
	for _, candidate := range packages {
		if candidate == id {
			return true
		}
	}
	return false
}

func canvasToolNames() []string {
	return []string{
		"list_canvases_kandev",
		"read_canvas_authoring_skill_kandev",
		"create_canvas_kandev",
		"get_canvas_kandev",
		"publish_canvas_kandev",
		"get_canvas_state_kandev",
		"set_canvas_state_kandev",
	}
}

func newResumeCatalogHelper(t *testing.T, tc resumeCatalogCase) *resumeCatalogHelper {
	t.Helper()
	harness := newBootStateTestHarness(t)
	workspaceID, task, session := createResumeCatalogTask(t, harness, tc)
	log := logger.Default()
	settings, cleanup, err := agentsettingsstore.Provide(harness.db, harness.db, log)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	if tc.configurationPackage {
		enableResumeCatalogConfiguration(t, settings, workspaceID, task.ID)
	}
	executor := executorpkg.NewExecutor(nil, harness.taskRepo, log, executorpkg.ExecutorConfig{})
	executor.SetBuiltinPolicyReader(settings)
	executor.SetCanvasesEnabled(tc.canvases)
	profile, err := executor.PrepareTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.NoError(t, err)
	require.NotNil(t, profile.BuiltinPackages)

	server, mcp := newResumeCatalogAgentctl(t, task.ID, session.ID)
	mgr := newResumeCatalogLifecycle(t, workspaceID, log)
	execution := registerResumeCatalogExecution(t, mgr, server, task.ID, session.ID, workspaceID)
	return &resumeCatalogHelper{
		harness: harness, server: server, mcp: mcp, execution: execution, executor: executor,
		task: task, session: session,
		adapter: newLifecycleAdapter(mgr, nil, log), profile: profile,
	}
}

func createResumeCatalogTask(t *testing.T, harness bootStateTestHarness, tc resumeCatalogCase) (string, *models.Task, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps)
	sessionID := "resume-catalog-session"
	ownerID := "other-session"
	if tc.ownerIsSelf {
		ownerID = sessionID
	}
	result, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, WorkflowStepID: steps[0].ID,
		Title: "Resume catalog task",
		Metadata: map[string]interface{}{
			models.MetaKeyAgentTitlePending:        tc.pendingTitle,
			models.MetaKeyAgentTitleOwnerSessionID: ownerID,
		},
	})
	require.NoError(t, err)
	session := &models.TaskSession{
		ID: sessionID, TaskID: result.Task.ID, AgentProfileID: "resume-profile",
		State: models.TaskSessionStateRunning, IsPrimary: true,
	}
	require.NoError(t, harness.taskRepo.CreateTaskSession(ctx, session))
	return workspaces[0].ID, result.Task, session
}

func enableResumeCatalogConfiguration(t *testing.T, writer resumeCatalogPreferenceWriter, workspaceID, taskID string) {
	t.Helper()
	_, err := writer.PutBuiltinTaskPreferences(context.Background(), &mcpconfig.BuiltinTaskPreferences{
		WorkspaceID: workspaceID, TaskID: taskID, Version: 1, UpdatedBy: "resume-catalog-test",
		Packages: mcpconfig.BuiltinPackageChoices{
			p.PackageConfiguration: mcpconfig.BuiltinPreferenceOn,
		},
	}, 0, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
}

func newResumeCatalogAgentctl(t *testing.T, taskID, sessionID string) (*httptest.Server, *mcpserver.Server) {
	t.Helper()
	log := logger.Default()
	cfg, err := (&agentctlconfig.Config{}).NewInstanceConfig(0, &agentctlconfig.InstanceOverrides{
		InstanceID: resumeCatalogExecutionID, WorkDir: t.TempDir(),
		TaskID: taskID, SessionID: sessionID, McpMode: mcpmode.Task,
	})
	require.NoError(t, err)
	backend := mcpserver.NewChannelBackendClient(log)
	helperMCP := mcpserver.NewWithProfile(backend, sessionID, taskID, 0, log, "", false, p.Legacy(mcpmode.Task, false, nil))
	api := agentctlapi.NewServer(cfg, agentctlprocess.NewManager(cfg, log), helperMCP, nil, log)
	server := httptest.NewServer(api.Router())
	t.Cleanup(func() {
		server.Close()
		require.NoError(t, helperMCP.Close(context.Background()))
		backend.Close()
	})
	return server, helperMCP
}

func newResumeCatalogLifecycle(t *testing.T, workspaceID string, log *logger.Logger) *lifecycle.Manager {
	t.Helper()
	agentRegistry := registry.NewRegistry(log)
	agentRegistry.LoadDefaults()
	mgr := lifecycle.NewManager(
		agentRegistry, bus.NewMemoryEventBus(log), nil, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), log,
	)
	t.Cleanup(func() { _ = mgr.Stop() })
	mgr.SetMCPPrincipalScoper(func(ctx context.Context, taskID, sessionID string) (context.Context, error) {
		return mcpscope.WithPrincipal(ctx, mcpscope.Principal{
			WorkspaceID: workspaceID, CallerTaskID: taskID, CallerSessionID: sessionID,
			Surface: p.SurfaceKanbanTask,
		}), nil
	})
	mgr.SetBuiltinActionResolver(func(profile p.Context) ([]string, error) {
		return mcpserver.ResolveBuiltinActions(profile, log)
	})
	return mgr
}

func registerResumeCatalogExecution(
	t *testing.T, mgr *lifecycle.Manager, server *httptest.Server,
	taskID, sessionID, workspaceID string,
) *lifecycle.AgentExecution {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	client := actlclient.NewClient(parsed.Hostname(), port, logger.Default(),
		actlclient.WithExecutionID(resumeCatalogExecutionID))
	instance := &lifecycle.ExecutorInstance{
		InstanceID: resumeCatalogExecutionID, Client: client, WorkspacePath: t.TempDir(),
	}
	execution := instance.ToAgentExecution(&lifecycle.ExecutorCreateRequest{
		TaskID: taskID, SessionID: sessionID, WorkspaceID: workspaceID, AgentProfileID: "claude-acp",
	})
	execution.ACPSessionID = "native-conversation-1"
	require.NoError(t, mgr.ExecutionStoreForTesting().Add(execution))
	return execution
}

func freshResumeCatalogURL(t *testing.T, profile p.Context) string {
	t.Helper()
	log := logger.Default()
	backend := mcpserver.NewChannelBackendClient(log)
	direct := mcpserver.NewWithProfile(backend, "direct-session", profile.BuiltinPackages.TaskID, 0, log, "", false, profile)
	router := gin.New()
	direct.RegisterRoutes(router)
	server := httptest.NewServer(router)
	t.Cleanup(func() {
		server.Close()
		require.NoError(t, direct.Close(context.Background()))
		backend.Close()
	})
	return server.URL + "/mcp"
}

func listMCPToolNames(t *testing.T, endpoint string) []string {
	t.Helper()
	client, err := mcpclient.NewStreamableHttpClient(endpoint)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() { _ = client.Close() })
	_, err = client.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: "2025-03-26",
		Capabilities:    mcp.ClientCapabilities{},
		ClientInfo:      mcp.Implementation{Name: "resume-catalog-test", Version: "1"},
	}})
	require.NoError(t, err)
	result, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}
