package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/tooltokens"
	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpsrv "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMCPAttachmentObserverEmitsSafeConnectionEvidence(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
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

func TestMCPAttachmentObserverPublishesToolSummaries(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	events := make(chan streams.MCPAttachmentEvidence, 4)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-1"})

	s.registerMCPConnection("connection-1")
	<-events
	tool := mcplib.Tool{
		Name:           "create_task_kandev",
		Description:    "Create a task",
		RawInputSchema: []byte(`{"type":"object","properties":{"secret":{"type":"string"}}}`),
		Meta:           &mcplib.Meta{AdditionalFields: map[string]any{"private_note": "must not persist"}},
	}
	s.observeMCPToolsList("connection-1", []mcplib.Tool{tool})

	evidence := <-events
	if evidence.Kind != streams.MCPAttachmentEvidenceToolsListObserved || evidence.ToolCount != 1 {
		t.Fatalf("evidence = %+v, want tools-list count", evidence)
	}
	if len(evidence.Tools) != 1 || evidence.Tools[0].Name != "create_task_kandev" || evidence.Tools[0].Description != "Create a task" {
		t.Fatalf("tool summaries = %+v, want name and description", evidence.Tools)
	}
	encoded, err := json.Marshal(evidence)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"input_schema":{"type":"object","properties":{"secret":{"type":"string"}}}`)
	assert.Contains(t, string(encoded), `"tool_token_estimator":"o200k_base:mcp-tool-json-v1"`)
	assert.NotContains(t, string(encoded), "must not persist")
	definition, err := json.Marshal(tool)
	require.NoError(t, err)
	wantTokens, err := tooltokens.EstimateToolJSON(definition)
	require.NoError(t, err)
	assert.Equal(t, wantTokens, evidence.Tools[0].EstimatedTokens)
}

func TestMCPAttachmentObserverPublishesStructuredInputSchema(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	events := make(chan streams.MCPAttachmentEvidence, 4)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-1"})
	s.registerMCPConnection("connection-1")
	<-events

	s.observeMCPToolsList("connection-1", []mcplib.Tool{{
		Name: "structured",
		InputSchema: mcplib.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{"title": map[string]any{"type": "string"}},
			Required:   []string{"title"},
		},
	}})
	evidence := <-events
	require.Len(t, evidence.Tools, 1)
	assert.JSONEq(t, `{"type":"object","properties":{"title":{"type":"string"}},"required":["title"]}`, string(evidence.Tools[0].InputSchema))
	assert.Positive(t, evidence.Tools[0].EstimatedTokens)
}

func TestMCPAttachmentObserverKeepsConnectionAttemptAcrossRollover(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
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

func TestSetPluginToolsReplacesRuntimeRegistry(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "task-tags.v1", LocalName: "add_tag", ExposedName: plugintools.ExposedName("task-tags.v1", "add_tag"),
		Description: "Add a tag", InputSchema: []byte(`{"type":"object","properties":{"tag":{"type":"string"}}}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition}}))
	if _, ok := s.mcpServer.ListTools()[definition.ExposedName]; !ok {
		t.Fatalf("plugin tool was not registered")
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 2}))
	if _, ok := s.mcpServer.ListTools()[definition.ExposedName]; ok {
		t.Fatalf("plugin tool was not removed on replacement")
	}
}

func TestSetPluginToolsRebuildsArgumentValidators(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "task-tags.v1", LocalName: "add_tag", ExposedName: plugintools.ExposedName("task-tags.v1", "add_tag"),
		Description: "Add a tag", InputSchema: []byte(`{"type":"object","properties":{"tag":{"type":"string"}},"required":["tag"]}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition}}))

	req := mcplib.CallToolRequest{Params: mcplib.CallToolParams{Name: definition.ExposedName, Arguments: map[string]any{"tag": "urgent"}}}
	if _, err := s.validateToolArguments(definition.ExposedName, req); err != nil {
		t.Fatalf("validateToolArguments() error = %v", err)
	}
}

func TestSetPluginToolsRejectsMalformedSnapshotAndPreservesRegistry(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "echo", LocalName: "echo", ExposedName: plugintools.ExposedName("echo", "echo"),
		Description: "Echo", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition},
	}))

	invalid := definition
	invalid.InputSchema = []byte(`{"type":`)
	err := s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 2, Tools: []plugintools.Definition{invalid},
	})
	require.ErrorContains(t, err, "input schema")
	require.Contains(t, s.mcpServer.ListTools(), definition.ExposedName)
	require.Equal(t, uint64(1), s.pluginTools.Revision)
}

func TestSetPluginToolsPublishesDeclaredOutputSchema(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "echo", LocalName: "echo", ExposedName: plugintools.ExposedName("echo", "echo"),
		Description: "Echo", InputSchema: []byte(`{"type":"object"}`),
		OutputSchema: []byte(`{"type":"object","properties":{"value":{"type":"string"}}}`),
		Surfaces:     []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition},
	}))

	tool := s.mcpServer.ListTools()[definition.ExposedName]
	require.JSONEq(t, string(definition.OutputSchema), string(tool.Tool.RawOutputSchema))
}

func TestSetPluginToolsSkipsEquivalentEffectiveRegistry(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	session := &providerRefreshTestSession{
		id: "initialized-plugin-noop", notifications: make(chan mcplib.JSONRPCNotification, 4),
	}
	require.NoError(t, s.mcpServer.RegisterSession(context.Background(), session))
	definition := plugintools.Definition{
		PluginID: "echo", LocalName: "echo", ExposedName: plugintools.ExposedName("echo", "echo"),
		Description: "Echo", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition},
	}))
	<-session.notifications

	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 2, Tools: []plugintools.Definition{definition},
	}))
	select {
	case notification := <-session.notifications:
		t.Fatalf("equivalent plugin catalog emitted notification %q", notification.Method)
	default:
	}
	require.Equal(t, uint64(2), s.pluginTools.Revision)
}

func TestPluginToolCallDoesNotLogArguments(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	backend := &testBackend{response: map[string]any{"text": "ok"}}
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "echo", LocalName: "echo", ExposedName: plugintools.ExposedName("echo", "echo"),
		Description: "Echo", InputSchema: []byte(`{"type":"object","properties":{"token":{"type":"string"}}}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition}}))
	callTool(t, s, definition.ExposedName, map[string]interface{}{"token": "secret-value"})

	entries := observed.FilterMessage("MCP tool call").All()
	require.Len(t, entries, 1)
	_, loggedArgs := entries[0].ContextMap()["args"]
	require.False(t, loggedArgs, "plugin arguments must not be attached to logs")
}

func TestPluginToolErrorDoesNotLogResult(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	backend := &testBackend{response: map[string]any{"text": "secret-result", "is_error": true}}
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "echo", LocalName: "echo", ExposedName: plugintools.ExposedName("echo", "echo"),
		Description: "Echo", InputSchema: []byte(`{"type":"object"}`), Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition}}))
	callTool(t, s, definition.ExposedName, map[string]interface{}{})

	entries := observed.FilterMessage("MCP tool returned error").All()
	require.Len(t, entries, 1)
	_, loggedResult := entries[0].ContextMap()["result"]
	require.False(t, loggedResult, "plugin results must not be attached to logs")
}

type staticPluginBackend struct{}

func (staticPluginBackend) RequestPayload(_ context.Context, _ string, _, result interface{}) error {
	response, ok := result.(*struct {
		Text              string         `json:"text"`
		StructuredContent map[string]any `json:"structured_content,omitempty"`
		IsError           bool           `json:"is_error"`
	})
	if ok {
		response.Text = "ok"
	}
	return nil
}

func TestPluginToolInvocationProfileAccessIsRaceFree(t *testing.T) {
	log := newTestLogger(t)
	s := New(staticPluginBackend{}, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "echo", LocalName: "echo", ExposedName: plugintools.ExposedName("echo", "echo"),
		Description: "Echo", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban, plugintools.SurfaceOffice},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition}}))
	tool := s.mcpServer.ListTools()[definition.ExposedName]
	require.NotNil(t, tool)
	req := mcplib.CallToolRequest{Params: mcplib.CallToolParams{Name: definition.ExposedName, Arguments: map[string]any{}}}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			s.SetProfile(mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil))
			s.SetProfile(mcpprofile.New(mcpprofile.SurfaceOfficeTask, nil, nil))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = tool.Handler(context.Background(), req)
		}
	}()
	wg.Wait()
}

func TestSetPluginToolsIgnoresStaleRevision(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	definition := plugintools.Definition{
		PluginID: "plugin", LocalName: "current", ExposedName: plugintools.ExposedName("plugin", "current"),
		Description: "Current", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 2, Tools: []plugintools.Definition{definition}}))
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1}))
	_, ok := s.mcpServer.ListTools()[definition.ExposedName]
	if !ok {
		t.Fatal("stale revision replaced the current registry")
	}
}

func TestSetPluginToolsIgnoresDivergentSnapshotAtCurrentRevision(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	current := plugintools.Definition{
		PluginID: "plugin", LocalName: "current", ExposedName: plugintools.ExposedName("plugin", "current"),
		Description: "Current", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	replacement := plugintools.Definition{
		PluginID: "plugin", LocalName: "replacement", ExposedName: plugintools.ExposedName("plugin", "replacement"),
		Description: "Replacement", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 2, Tools: []plugintools.Definition{current},
	}))
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 2, Tools: []plugintools.Definition{replacement},
	}))

	tools := s.mcpServer.ListTools()
	require.Contains(t, tools, current.ExposedName)
	require.NotContains(t, tools, replacement.ExposedName)
}

func TestSetPluginToolsPreservesCatalogAcrossSurfaceChange(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	kanban := plugintools.Definition{
		PluginID: "plugin", LocalName: "kanban", ExposedName: plugintools.ExposedName("plugin", "kanban"),
		Description: "Kanban", InputSchema: []byte(`{"type":"object"}`), Surfaces: []string{plugintools.SurfaceKanban},
	}
	office := plugintools.Definition{
		PluginID: "plugin", LocalName: "office", ExposedName: plugintools.ExposedName("plugin", "office"),
		Description: "Office", InputSchema: []byte(`{"type":"object"}`), Surfaces: []string{plugintools.SurfaceOffice},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{kanban, office}}))
	s.SetProfile(mcpprofile.New(mcpprofile.SurfaceOfficeTask, nil, nil))

	tools := s.mcpServer.ListTools()
	if _, ok := tools[office.ExposedName]; !ok {
		t.Fatal("office tool was lost when the MCP surface changed")
	}
	if _, ok := tools[kanban.ExposedName]; ok {
		t.Fatal("kanban-only tool remained on the office surface")
	}
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
	t.Cleanup(backend.Close)

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

func TestServerSurfaceAutomationHasFixedCoordinatorCatalog(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	profile := mcpprofile.NewAutomation()
	s := NewWithProfile(backend, "automation-session", "automation-task", 10005, log, "", false, profile)
	want := []string{
		"list_workspaces_kandev", "list_workflows_kandev", "list_workflow_steps_kandev",
		"list_repositories_kandev", "list_tasks_kandev", "list_agents_kandev",
		"list_executors_kandev", "list_executor_profiles_kandev", "list_related_tasks_kandev",
		"get_task_conversation_kandev", "list_task_sessions_kandev", "create_task_kandev",
		"update_task_kandev", "move_task_kandev", "archive_task_kandev",
		"add_task_dependency_kandev", "remove_task_dependency_kandev", "message_task_kandev",
		"stop_task_kandev", "spawn_session_kandev", "list_pending_questions_kandev",
		"answer_question_kandev", "list_pending_agent_permissions_kandev", "resolve_agent_permission_kandev",
	}
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))
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
	assert.Contains(t, tools, "export_workflow_kandev")
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
	require.Len(t, tools, 36, "final registry should contain the complete GitLab-only task tool set")
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
	// 1 publish_review_findings + 1 related-tasks + 1 diagnostic bundle
	// + 2 task-dependency (add/remove) + 1 rich-output = 38.
	// Task-document tools (list/get/write) are office-only.
	assert.Contains(t, tools, "step_complete_kandev", "ADR 0015 explicit-completion signal must be registered in task mode")
	assert.Contains(t, tools, "show_walkthrough_kandev", "walkthrough tool must be registered in task mode")
	assert.Contains(t, tools, "publish_review_findings_kandev", "native code-review publishing must be registered in task mode")
	assert.Contains(t, tools, "spawn_session_kandev", "spawn_session must be registered in task mode")
	assert.Contains(t, tools, "add_workspace_sources_kandev")
	assert.Contains(t, tools, "list_task_sessions_kandev", "session discovery must be registered in task mode")
	assert.Contains(t, tools, "add_task_dependency_kandev", "dependency edges must be manageable in task mode")
	assert.Contains(t, tools, "remove_task_dependency_kandev")
	assert.Contains(t, tools, "show_rich_output_kandev", "native rich output must be registered in task mode")
	assert.Equal(t, 38, len(tools))
}

func TestServerStepCompleteTool_TaskAndOfficeOnlyAndDiscoverable(t *testing.T) {
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

	officeServer := New(backend, "test-session", "test-task", 10005, log, "", false, ModeOffice)
	assert.Contains(t, officeServer.mcpServer.ListTools(), "step_complete_kandev", "office mode must register step_complete_kandev (ADR 0015)")

	for _, mode := range []string{ModeConfig, ModeExternal} {
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
	// 13 workflow (incl. list_repositories + import_workflow + export_workflow) + 4 agent + 4 mcp + 2 saved prompts + 5 executor + 7 task (incl. list_task_sessions) + 1 interaction = 36
	assert.NotContains(t, tools, "step_complete_kandev", "step_complete_kandev requires a live task session; must NOT register in config mode")
	assert.Equal(t, 36, len(tools))
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
	assert.Contains(t, tools, "show_rich_output_kandev")

	// delegate_task_kandev was removed from ModeOffice when the
	// agentctl CLI started covering task creation/delegation via
	// `agentctl kandev task create --parent $KANDEV_TASK_ID …`.
	assert.NotContains(t, tools, "delegate_task_kandev")

	// Office mode exposes the cross-task handoff tools.
	assert.Contains(t, tools, "list_related_tasks_kandev")
	assert.Contains(t, tools, "list_task_documents_kandev")
	assert.Contains(t, tools, "get_task_document_kandev")
	assert.Contains(t, tools, "write_task_document_kandev")

	// Office mode gets the ADR 0015 declarative completion signal too, so a
	// step gated by auto_advance_requires_signal can advance without relying
	// solely on the agent process exiting.
	assert.Contains(t, tools, "step_complete_kandev")

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
	// 4 plan + 1 interaction + 1 related-tasks + 3 task-documents
	// + 1 rich-output + 1 decisions + 1 step_complete (ADR 0015) = 12.
	// (delegate_task_kandev retired in favour of `agentctl kandev task create …`).
	// (list_task_comments_kandev retired in favour of `agentctl kandev comment list …`).
	assert.Contains(t, tools, "step_complete_kandev", "office mode must register the ADR 0015 completion signal")
	assert.Equal(t, 12, len(tools))
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
	// 4 plan + 1 related-tasks + 3 task-documents + 1 rich-output
	// + 1 decisions + 1 step_complete (ADR 0015) = 11
	// (no ask_user_question, no delegate, no list_task_comments)
	assert.Equal(t, 11, len(tools))
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
	assert.Contains(t, tools, "export_workflow_kandev")
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "get_mcp_config_kandev")
	assert.Contains(t, tools, "list_executors_kandev")
	assert.Contains(t, tools, "move_task_kandev")

	// External mode includes create_task_kandev so external agents can spawn tasks
	assert.Contains(t, tools, "create_task_kandev")
	createTask := s.mcpServer.ListTools()["create_task_kandev"]
	assert.Contains(t, createTask.Tool.Description, "external client")
	assert.Contains(t, createTask.Tool.Description, "native subagent mechanism")
	assert.Contains(t, createTask.Tool.Description, "external_id")
	agentProfile := toolInputProperties(t, s, "create_task_kandev")["agent_profile_id"].(map[string]interface{})
	agentProfileDescription := agentProfile["description"].(string)
	assert.Contains(t, agentProfileDescription, "current_task")
	assert.Contains(t, agentProfileDescription, "workspace_default")
	assert.Contains(t, agentProfileDescription, "workflow profiles")
	assert.Contains(t, agentProfileDescription, "parent task profile")
	assert.Contains(t, agentProfileDescription, "External mode never copies creator-session runtime values")

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

	// External mode includes the question-answering tools (spec S1/S2); the
	// agent-facing ask_user_question_kandev stays off this surface (spec S3).
	assert.Contains(t, tools, "list_pending_questions_kandev")
	assert.Contains(t, tools, "answer_question_kandev")
}

func TestServerModeExternal_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "", "", 0, log, "", true, ModeExternal)
	tools := getRegisteredToolNames(s)
	// 13 workflow (incl. list_repositories + import_workflow + export_workflow) + 4 agent + 4 mcp + 2 saved prompts + 5 executor + 7 task (incl. list_task_sessions) + 1 create_task + 2 task-dependency + 2 question-answering (list_pending_questions + answer_question) + 2 agent permission (list_pending_agent_permissions + resolve_agent_permission) = 42.
	// add_branch_to_task_kandev is task-mode only — external coding agents have no live session to attach a worktree to.
	assert.Equal(t, 42, len(tools))
	assert.NotContains(t, tools, "add_branch_to_task_kandev")
}

func TestExternalAnswerQuestionSchemaDoesNotRestrictOptionCardinality(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()
	s := New(backend, "", "", 0, log, "", true, ModeExternal)

	answers := toolInputProperties(t, s, "answer_question_kandev")["answers"].(map[string]interface{})
	answerItem := answers["items"].(map[string]interface{})
	answerProperties := answerItem["properties"].(map[string]interface{})
	selectedOptions := answerProperties["selected_options"].(map[string]interface{})
	description := selectedOptions["description"].(string)

	assert.Contains(t, description, "Zero or more option IDs")
	assert.NotContains(t, description, "single-choice")
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
