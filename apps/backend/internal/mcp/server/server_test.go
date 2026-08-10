package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpsrv "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPAttachmentObserverEmitsSafeConnectionEvidence(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	events := make(chan streams.MCPAttachmentEvidence, 4)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-1"})

	s.registerMCPConnection("agent session containing secret")
	<-events
	s.observeMCPConnection("agent session containing secret", streams.MCPAttachmentEvidenceInitializeObserved, 0, "")
	s.observeMCPConnection("agent session containing secret", streams.MCPAttachmentEvidenceToolsListObserved, 7, "")

	first := <-events
	second := <-events
	if first.AttemptID != "attempt-1" || first.ServerName != "kandev" || first.ConnectionID == "agent session containing secret" {
		t.Fatalf("first evidence = %+v", first)
	}
	if second.Kind != streams.MCPAttachmentEvidenceToolsListObserved || second.ToolCount != 7 {
		t.Fatalf("second evidence = %+v", second)
	}
	if first.OccurredAt.IsZero() || time.Since(first.OccurredAt) > time.Second {
		t.Fatalf("timestamp = %v", first.OccurredAt)
	}
}

func TestMCPAttachmentObserverKeepsConnectionAttemptAcrossRollover(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	events := make(chan streams.MCPAttachmentEvidence, 8)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })

	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-old"})
	s.registerMCPConnection("old")
	if event := <-events; event.AttemptID != "attempt-old" {
		t.Fatalf("old register attempt = %q", event.AttemptID)
	}
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-new"})
	s.registerMCPConnection("new")
	if event := <-events; event.AttemptID != "attempt-new" {
		t.Fatalf("new register attempt = %q", event.AttemptID)
	}

	s.observeMCPConnection("old", streams.MCPAttachmentEvidenceToolsListObserved, 1, "")
	if event := <-events; event.AttemptID != "attempt-old" {
		t.Fatalf("old delayed event attempt = %q", event.AttemptID)
	}
	s.unregisterMCPConnection("old")
	if event := <-events; event.AttemptID != "attempt-old" || event.Kind != streams.MCPAttachmentEvidenceConnectionClosed {
		t.Fatalf("old close event = %+v", event)
	}
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	return log
}

// getRegisteredToolNames returns the names of all tools registered on the MCP server.
func getRegisteredToolNames(s *Server) []string {
	toolsMap := s.mcpServer.ListTools()
	names := make([]string, 0, len(toolsMap))
	for name := range toolsMap {
		names = append(names, name)
	}
	return names
}

func TestServerModeTask_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask, []string{"github", "gitlab"})
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// Task mode should have kanban tools
	assert.Contains(t, tools, "list_workspaces_kandev")
	assert.Contains(t, tools, "list_workflows_kandev")
	assert.Contains(t, tools, "list_workflow_steps_kandev")
	assert.Contains(t, tools, "list_tasks_kandev")
	assert.Contains(t, tools, "create_task_kandev")
	assert.Contains(t, tools, "update_task_kandev")
	assert.Contains(t, tools, "move_task_kandev")
	assert.Contains(t, tools, "message_task_kandev")
	assert.Contains(t, tools, "stop_task_kandev")
	assert.Contains(t, tools, "get_task_conversation_kandev")
	assert.Contains(t, tools, "get_task_pr_automation_kandev")
	assert.Contains(t, tools, "update_task_pr_automation_kandev")
	assert.Contains(t, tools, "get_diagnostic_bundle_kandev")
	assert.Contains(t, tools, "get_task_mr_automation_kandev")
	assert.Contains(t, tools, "update_task_mr_automation_kandev")

	// Task mode should have plan tools
	assert.Contains(t, tools, "create_task_plan_kandev")
	assert.Contains(t, tools, "get_task_plan_kandev")
	assert.Contains(t, tools, "update_task_plan_kandev")
	assert.Contains(t, tools, "delete_task_plan_kandev")

	// Task mode should have interaction tools
	assert.Contains(t, tools, "ask_user_question_kandev")

	// Task mode should have profile listing tools (needed for create_task)
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "list_executor_profiles_kandev")

	// Task mode keeps list_related_tasks_kandev (sibling discovery) but
	// drops the task-document tools — those are office-only.
	assert.Contains(t, tools, "list_related_tasks_kandev")
	assert.NotContains(t, tools, "list_task_documents_kandev")
	assert.NotContains(t, tools, "get_task_document_kandev")
	assert.NotContains(t, tools, "write_task_document_kandev")

	// Task mode exposes delete + archive so agents can clean up the tasks
	// they fan out. Restore/unarchive is intentionally NOT exposed via MCP —
	// it stays a user action in the UI.
	assert.Contains(t, tools, "delete_task_kandev")
	assert.Contains(t, tools, "archive_task_kandev")
	assert.NotContains(t, tools, "restore_task_kandev")

	// Task mode should NOT have config/mutation tools
	assert.NotContains(t, tools, "create_workflow_kandev")
	assert.NotContains(t, tools, "update_workflow_kandev")
	assert.NotContains(t, tools, "delete_workflow_kandev")
	assert.NotContains(t, tools, "create_workflow_step_kandev")
	assert.NotContains(t, tools, "update_workflow_step_kandev")
	assert.NotContains(t, tools, "update_agent_kandev")
	assert.NotContains(t, tools, "create_agent_profile_kandev")
	assert.NotContains(t, tools, "delete_agent_profile_kandev")
	assert.NotContains(t, tools, "list_agent_profiles_kandev")
	assert.NotContains(t, tools, "update_agent_profile_kandev")
	assert.NotContains(t, tools, "get_mcp_config_kandev")
	assert.NotContains(t, tools, "update_mcp_config_kandev")
	assert.NotContains(t, tools, "list_executors_kandev")
	assert.NotContains(t, tools, "create_executor_profile_kandev")
	assert.NotContains(t, tools, "update_executor_profile_kandev")
	assert.NotContains(t, tools, "delete_executor_profile_kandev")
	assert.NotContains(t, tools, "update_task_state_kandev")
	assert.NotContains(t, tools, "delete_workflow_step_kandev")
	assert.NotContains(t, tools, "reorder_workflow_steps_kandev")
}

func TestServerProfile_AutopilotChildHasOnlyParentQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityParentQuestion}, nil)
	s := NewWithProfile(backend, "child-session", "child-task", 10005, log, "", false, profile)
	tools := getRegisteredToolNames(s)

	assert.Contains(t, tools, "ask_parent_question_kandev")
	assert.NotContains(t, tools, "ask_user_question_kandev")

	rootProfile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil)
	root := NewWithProfile(backend, "root-session", "root-task", 10006, log, "", false, rootProfile)
	rootTools := getRegisteredToolNames(root)
	assert.NotContains(t, rootTools, "ask_parent_question_kandev")
	assert.NotContains(t, rootTools, "ask_user_question_kandev")
}

func TestServerModeConfig_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// Config mode should have workflow config tools
	assert.Contains(t, tools, "list_workspaces_kandev")
	assert.Contains(t, tools, "list_workflows_kandev")
	assert.Contains(t, tools, "list_repositories_kandev")
	assert.Contains(t, tools, "create_workflow_kandev")
	assert.Contains(t, tools, "update_workflow_kandev")
	assert.Contains(t, tools, "delete_workflow_kandev")
	assert.Contains(t, tools, "import_workflow_kandev")
	assert.Contains(t, tools, "list_workflow_steps_kandev")
	assert.Contains(t, tools, "create_workflow_step_kandev")
	assert.Contains(t, tools, "update_workflow_step_kandev")
	assert.Contains(t, tools, "delete_workflow_step_kandev")
	assert.Contains(t, tools, "reorder_workflow_steps_kandev")

	// Config mode should have agent tools
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "update_agent_kandev")
	assert.Contains(t, tools, "create_agent_profile_kandev")
	assert.Contains(t, tools, "delete_agent_profile_kandev")

	// Config mode should have MCP config tools
	assert.Contains(t, tools, "list_agent_profiles_kandev")
	assert.Contains(t, tools, "update_agent_profile_kandev")
	assert.Contains(t, tools, "get_mcp_config_kandev")
	assert.Contains(t, tools, "update_mcp_config_kandev")

	// Config mode should have executor profile tools
	assert.Contains(t, tools, "list_executors_kandev")
	assert.Contains(t, tools, "list_executor_profiles_kandev")
	assert.Contains(t, tools, "create_executor_profile_kandev")
	assert.Contains(t, tools, "update_executor_profile_kandev")
	assert.Contains(t, tools, "delete_executor_profile_kandev")

	// Config mode should have task tools
	assert.Contains(t, tools, "list_tasks_kandev")
	assert.Contains(t, tools, "move_task_kandev")
	assert.Contains(t, tools, "delete_task_kandev")
	assert.Contains(t, tools, "archive_task_kandev")
	assert.Contains(t, tools, "update_task_state_kandev")
	assert.Contains(t, tools, "get_task_conversation_kandev")

	// Config mode should have interaction tools
	assert.Contains(t, tools, "ask_user_question_kandev")

	// Config mode should NOT have plan tools
	assert.NotContains(t, tools, "create_task_plan_kandev")
	assert.NotContains(t, tools, "get_task_plan_kandev")
	assert.NotContains(t, tools, "update_task_plan_kandev")
	assert.NotContains(t, tools, "delete_task_plan_kandev")

	// Config mode should NOT have task-mode kanban create/update tools
	assert.NotContains(t, tools, "create_task_kandev")
	assert.NotContains(t, tools, "update_task_kandev")
	assert.NotContains(t, tools, "stop_task_kandev")
}

func TestServerModeDefault_DefaultsToTask(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, "")
	require.NotNil(t, s)
	assert.Equal(t, ModeTask, s.mode)

	tools := getRegisteredToolNames(s)
	assert.Contains(t, tools, "create_task_kandev")
	assert.Contains(t, tools, "create_task_plan_kandev")
	assert.Contains(t, tools, "stop_task_kandev")
	assert.NotContains(t, tools, "create_workflow_step_kandev")
}

func TestServerModeTask_AbsentProvidersFailClosedForReviewAutomation(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	tools := getRegisteredToolNames(s)

	assert.NotContains(t, tools, "get_task_pr_automation_kandev")
	assert.NotContains(t, tools, "update_task_pr_automation_kandev")
	assert.NotContains(t, tools, "get_task_mr_automation_kandev")
	assert.NotContains(t, tools, "update_task_mr_automation_kandev")
}

func TestServerModeTask_ProviderMembership(t *testing.T) {
	log := newTestLogger(t)

	tests := []struct {
		name      string
		providers []string
		wantPR    bool
		wantMR    bool
	}{
		{name: "github only", providers: []string{" GITHUB "}, wantPR: true},
		{name: "gitlab only", providers: []string{"gitlab"}, wantMR: true},
		{name: "mixed", providers: []string{"gitlab", "github", "github"}, wantPR: true, wantMR: true},
		{name: "empty", providers: []string{}, wantPR: false, wantMR: false},
		{name: "unsupported", providers: []string{"local", "azure"}, wantPR: false, wantMR: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewChannelBackendClient(log)
			t.Cleanup(backend.Close)
			s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask, tt.providers)
			tools := getRegisteredToolNames(s)
			assert.Equal(t, tt.wantPR, containsTool(tools, "get_task_pr_automation_kandev"))
			assert.Equal(t, tt.wantPR, containsTool(tools, "update_task_pr_automation_kandev"))
			assert.Equal(t, tt.wantMR, containsTool(tools, "get_task_mr_automation_kandev"))
			assert.Equal(t, tt.wantMR, containsTool(tools, "update_task_mr_automation_kandev"))
			assert.Contains(t, tools, "stop_task_kandev")
		})
	}
}

func TestServerSetProvidersPreservesModeAndRebuildsTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTaskTitlePending, []string{"github"})
	s.SetProviders([]string{"gitlab"})

	tools := getRegisteredToolNames(s)
	assert.Equal(t, ModeTaskTitlePending, s.mode)
	assert.Contains(t, tools, "set_task_title_kandev")
	assert.NotContains(t, tools, "get_task_pr_automation_kandev")
	assert.Contains(t, tools, "get_task_mr_automation_kandev")
}

type providerRefreshTestSession struct {
	id            string
	notifications chan mcplib.JSONRPCNotification
}

func (s *providerRefreshTestSession) Initialize() {}

func (s *providerRefreshTestSession) Initialized() bool { return true }

func (s *providerRefreshTestSession) NotificationChannel() chan<- mcplib.JSONRPCNotification {
	return s.notifications
}

func (s *providerRefreshTestSession) SessionID() string { return s.id }

var _ mcpsrv.ClientSession = (*providerRefreshTestSession)(nil)

func TestServerSetProvidersNotifiesInitializedSessionOnceWithCompleteTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask, []string{"github"})
	session := &providerRefreshTestSession{
		id:            "initialized-provider-refresh",
		notifications: make(chan mcplib.JSONRPCNotification, 128),
	}
	require.NoError(t, s.mcpServer.RegisterSession(context.Background(), session))

	s.SetProviders([]string{"gitlab"})

	var notifications []mcplib.JSONRPCNotification
	for {
		select {
		case notification := <-session.notifications:
			notifications = append(notifications, notification)
		default:
			goto drained
		}
	}

drained:
	require.Len(t, notifications, 1, "a live provider rebuild must emit one tools/list_changed notification")
	require.Equal(t, mcplib.MethodNotificationToolsListChanged, notifications[0].Method)
	tools := getRegisteredToolNames(s)
	// Absolute count: a tool added to task mode must be reflected here as well
	// as in TestServerModeTask_ToolCount and
	// TestRegisterTools_LoggedCountMatchesRegisteredTools (list_task_sessions_test.go),
	// which pin the per-mode registration rather than this SetProviders rebuild.
	require.Len(t, tools, 33, "final registry should contain the complete GitLab-only task tool set")
	assert.Contains(t, tools, "get_task_mr_automation_kandev")
	assert.NotContains(t, tools, "get_task_pr_automation_kandev")
}

func TestServerSetProvidersAndModeSkipNormalizedNoOps(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask, []string{"github", "gitlab"})
	session := &providerRefreshTestSession{
		id:            "initialized-noop-refresh",
		notifications: make(chan mcplib.JSONRPCNotification, 128),
	}
	require.NoError(t, s.mcpServer.RegisterSession(context.Background(), session))

	s.SetProviders([]string{" GITLAB ", "github", "github"})
	s.SetMode("unknown-mode")

	select {
	case notification := <-session.notifications:
		t.Fatalf("normalized no-op emitted notification %q", notification.Method)
	default:
	}
}

func TestServerSetProfileSkipsEquivalentProfile(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}, []string{"github"})
	s := NewWithProfile(backend, "test-session", "test-task", 10005, log, "", false, profile)
	session := &providerRefreshTestSession{
		id:            "initialized-profile-noop",
		notifications: make(chan mcplib.JSONRPCNotification, 128),
	}
	require.NoError(t, s.mcpServer.RegisterSession(context.Background(), session))

	s.SetProfile(mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}, []string{"github"}))

	select {
	case notification := <-session.notifications:
		t.Fatalf("equivalent profile emitted notification %q", notification.Method)
	default:
	}
}

func TestServerSetModePreservesAdditiveCapabilities(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityParentQuestion}, nil)
	s := NewWithProfile(backend, "child-session", "child-task", 10005, log, "", false, profile)

	s.SetMode(ModeConfig)
	assert.True(t, s.Profile().HasCapability(mcpprofile.CapabilityParentQuestion))
	assert.NotContains(t, getRegisteredToolNames(s), "ask_parent_question_kandev")

	s.SetMode(ModeTask)
	assert.True(t, s.Profile().HasCapability(mcpprofile.CapabilityParentQuestion))
	assert.Contains(t, getRegisteredToolNames(s), "ask_parent_question_kandev")
	assert.NotContains(t, getRegisteredToolNames(s), "ask_user_question_kandev")
}

func containsTool(tools []string, name string) bool {
	for _, tool := range tools {
		if tool == name {
			return true
		}
	}
	return false
}

func TestServerModeConfig_DisableAskQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", true, ModeConfig)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "create_workflow_step_kandev")
}

func TestServerModeTask_DisableAskQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", true, ModeTask)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.Contains(t, tools, "create_task_kandev")
	assert.Contains(t, tools, "create_task_plan_kandev")
}

func TestServerModeTaskTitlePending_RegistersTitleToolOnlyForPendingTaskMode(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	pending := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTaskTitlePending)
	titleTool, ok := pending.mcpServer.ListTools()["set_task_title_kandev"]
	require.True(t, ok, "pending task mode must register set_task_title_kandev")
	assert.Contains(t, titleTool.Tool.Description, "first action")
	assert.Contains(t, titleTool.Tool.Description, "6 words")
	assert.Contains(t, titleTool.Tool.Description, "sentence case")
	assert.Contains(t, titleTool.Tool.Description, "Improve task title casing")
	assert.Contains(t, titleTool.Tool.Description, "short title phrase")
	assert.NotContains(t, titleTool.Tool.Description, "short noun phrase")

	titleProperties := toolInputProperties(t, pending, "set_task_title_kandev")
	titleProperty, ok := titleProperties["title"].(map[string]interface{})
	require.True(t, ok, "title argument should have a schema property")
	assert.Contains(t, titleProperty["description"], "targeting about 6 words")
	assert.Contains(t, titleProperty["description"], "sentence-case")

	ordinary := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	assert.NotContains(t, ordinary.mcpServer.ListTools(), "set_task_title_kandev")
	for _, mode := range []string{ModeConfig, ModeOffice, ModeExternal} {
		t.Run(mode, func(t *testing.T) {
			restricted := New(backend, "test-session", "test-task", 10005, log, "", false, mode)
			assert.NotContains(t, restricted.mcpServer.ListTools(), "set_task_title_kandev")
		})
	}
}

func TestServerSetTaskTitle_ForwardsBoundTaskAndSession(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"accepted": true,
		"task_id":  "task-1",
		"title":    "Short title",
	}}
	log := newTestLogger(t)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTaskTitlePending)

	result := callTool(t, s, "set_task_title_kandev", map[string]interface{}{"title": "Short title"})
	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPSetTaskTitle, backend.lastAction)
	assert.Equal(t, map[string]interface{}{
		"task_id":    "task-1",
		"session_id": "session-1",
		"title":      "Short title",
	}, backend.lastPayload)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, `"accepted": true`)
}

func TestServerModeTask_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask, []string{"github", "gitlab"})
	tools := getRegisteredToolNames(s)
	// 20 kanban (incl. delete + archive task + stop_task + spawn_session +
	// list_task_sessions + PR automation + MR automation) + 1 add_branch_to_task +
	// 1 add_workspace_sources + 1 update_repository_base_branch +
	// 1 step_complete (ADR 0015) + 1 interaction + 4 plan + 3 walkthrough +
	// 1 publish_review_findings + 1 related-tasks + 1 diagnostic bundle = 35.
	// Task-document tools (list/get/write) are office-only.
	assert.Contains(t, tools, "step_complete_kandev", "ADR 0015 explicit-completion signal must be registered in task mode")
	assert.Contains(t, tools, "show_walkthrough_kandev", "walkthrough tool must be registered in task mode")
	assert.Contains(t, tools, "publish_review_findings_kandev", "native code-review publishing must be registered in task mode")
	assert.Contains(t, tools, "spawn_session_kandev", "spawn_session must be registered in task mode")
	assert.Contains(t, tools, "add_workspace_sources_kandev")
	assert.Contains(t, tools, "list_task_sessions_kandev", "session discovery must be registered in task mode")
	assert.Equal(t, 35, len(tools))
}

func TestServerStepCompleteTool_TaskOnlyAndDiscoverable(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	taskServer := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	stepComplete, ok := taskServer.mcpServer.ListTools()["step_complete_kandev"]
	require.True(t, ok, "task mode must register step_complete_kandev")

	serialized, err := json.Marshal(stepComplete.Tool)
	require.NoError(t, err)
	var tool map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(serialized, &tool))
	assert.NotContains(t, tool, "_meta", "task tools should use normal client discovery")

	for _, mode := range []string{ModeOffice, ModeConfig, ModeExternal} {
		t.Run(mode, func(t *testing.T) {
			restrictedServer := New(backend, "test-session", "test-task", 10005, log, "", false, mode)
			assert.NotContains(t, restrictedServer.mcpServer.ListTools(), "step_complete_kandev")
		})
	}
}

func TestServerModeConfig_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	tools := getRegisteredToolNames(s)
	// 12 workflow (incl. list_repositories + import_workflow) + 4 agent + 4 mcp + 5 executor + 7 task (incl. list_task_sessions) + 1 interaction = 33
	assert.NotContains(t, tools, "step_complete_kandev", "step_complete_kandev requires a live task session; must NOT register in config mode")
	assert.Equal(t, 33, len(tools))
}

func TestServerModeConfig_ToolDescriptions(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)

	toolsMap := s.mcpServer.ListTools()

	assert.Contains(t, toolsMap["create_workflow_step_kandev"].Tool.Description, "Create a new workflow step")
	assert.Contains(t, toolsMap["list_agents_kandev"].Tool.Description, "List all configured agents")
	assert.Contains(t, toolsMap["get_mcp_config_kandev"].Tool.Description, "Get MCP server configuration")
}

func TestServerModeOffice_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeOffice)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// Office mode should have plan tools
	assert.Contains(t, tools, "create_task_plan_kandev")
	assert.Contains(t, tools, "get_task_plan_kandev")
	assert.Contains(t, tools, "update_task_plan_kandev")
	assert.Contains(t, tools, "delete_task_plan_kandev")

	// Office mode should have interaction tools
	assert.Contains(t, tools, "ask_user_question_kandev")

	// delegate_task_kandev was removed from ModeOffice when the
	// agentctl CLI started covering task creation/delegation via
	// `agentctl kandev task create --parent $KANDEV_TASK_ID …`.
	assert.NotContains(t, tools, "delegate_task_kandev")

	// Office mode exposes the cross-task handoff tools.
	assert.Contains(t, tools, "list_related_tasks_kandev")
	assert.Contains(t, tools, "list_task_documents_kandev")
	assert.Contains(t, tools, "get_task_document_kandev")
	assert.Contains(t, tools, "write_task_document_kandev")

	// Office mode should NOT have kanban tools
	assert.NotContains(t, tools, "create_task_kandev")
	assert.NotContains(t, tools, "list_tasks_kandev")
	assert.NotContains(t, tools, "update_task_kandev")
	assert.NotContains(t, tools, "list_workspaces_kandev")
	assert.NotContains(t, tools, "list_workflows_kandev")
	assert.NotContains(t, tools, "list_workflow_steps_kandev")
	assert.NotContains(t, tools, "list_agents_kandev")
	assert.NotContains(t, tools, "list_executor_profiles_kandev")
	assert.NotContains(t, tools, "stop_task_kandev")

	// Office mode should NOT have config tools
	assert.NotContains(t, tools, "create_workflow_kandev")
	assert.NotContains(t, tools, "update_workflow_kandev")
	assert.NotContains(t, tools, "update_agent_kandev")
}

func TestServerModeOffice_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeOffice)
	tools := getRegisteredToolNames(s)
	// 4 plan + 1 interaction + 1 related-tasks + 3 task-documents = 9
	// (delegate_task_kandev retired in favour of `agentctl kandev task create …`).
	assert.NotContains(t, tools, "step_complete_kandev", "step_complete_kandev is kanban-task-only; office mode advances tasks via its own approval surface")
	assert.Equal(t, 9, len(tools))
}

func TestServerModeOffice_DisableAskQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", true, ModeOffice)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.Contains(t, tools, "create_task_plan_kandev")
	// delegate_task_kandev was retired from ModeOffice (now lives in
	// the agentctl CLI as `agentctl kandev task create --parent …`).
	assert.NotContains(t, tools, "delegate_task_kandev")
	// 4 plan + 1 related-tasks + 3 task-documents = 8 (no ask_user_question, no delegate)
	assert.Equal(t, 8, len(tools))
}

func TestServerModeConstants(t *testing.T) {
	assert.Equal(t, "task", ModeTask)
	assert.Equal(t, "task-title-pending", ModeTaskTitlePending)
	assert.Equal(t, "config", ModeConfig)
	assert.Equal(t, "external", ModeExternal)
	assert.Equal(t, "office", ModeOffice)
}

func TestServerModeExternal_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "", "", 0, log, "", true, ModeExternal)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// External mode includes all config tools
	assert.Contains(t, tools, "list_workspaces_kandev")
	assert.Contains(t, tools, "list_repositories_kandev")
	assert.Contains(t, tools, "create_workflow_kandev")
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "get_mcp_config_kandev")
	assert.Contains(t, tools, "list_executors_kandev")
	assert.Contains(t, tools, "move_task_kandev")

	// External mode includes create_task_kandev so external agents can spawn tasks
	assert.Contains(t, tools, "create_task_kandev")
	createTask := s.mcpServer.ListTools()["create_task_kandev"]
	assert.Contains(t, createTask.Tool.Description, "outranks an explicit agent_profile_id")
	assert.Contains(t, createTask.Tool.Description, "current_task")
	assert.Contains(t, createTask.Tool.Description, "workspace_default")
	assert.Contains(t, createTask.Tool.Description, "workflow profiles first")

	// External mode does NOT include session-scoped tools
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.NotContains(t, tools, "create_task_plan_kandev")
	assert.NotContains(t, tools, "get_task_plan_kandev")
	assert.NotContains(t, tools, "update_task_plan_kandev")
	assert.NotContains(t, tools, "delete_task_plan_kandev")

	// External mode does NOT include kanban update_task_kandev (config has its own update_task_state)
	assert.NotContains(t, tools, "update_task_kandev")

	// External mode does NOT include message_task_kandev (no live session context)
	assert.NotContains(t, tools, "message_task_kandev")
	assert.NotContains(t, tools, "stop_task_kandev")
}

func TestServerModeExternal_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "", "", 0, log, "", true, ModeExternal)
	tools := getRegisteredToolNames(s)
	// 12 workflow (incl. list_repositories + import_workflow) + 4 agent + 4 mcp + 5 executor + 7 task (incl. list_task_sessions) + 1 create_task = 33.
	// add_branch_to_task_kandev is task-mode only — external coding agents have no live session to attach a worktree to.
	assert.Equal(t, 33, len(tools))
	assert.NotContains(t, tools, "add_branch_to_task_kandev")
}

func TestNewExternal_Constructs(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := NewExternal(backend, log, "")
	require.NotNil(t, s)
	assert.Equal(t, ModeExternal, s.mode)
	assert.True(t, s.disableAskQuestion)
	assert.Empty(t, s.sessionID)
	assert.Empty(t, s.taskID)
	assert.NotNil(t, s.sseServer)
	assert.NotNil(t, s.httpServer)
	assert.Equal(t, "/mcp/message?sessionId=session-1", s.sseServer.GetMessageEndpointForClient(nil, "session-1"))
}
