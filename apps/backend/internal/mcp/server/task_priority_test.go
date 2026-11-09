package mcp

import (
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var taskPriorityValues = []string{
	models.TaskPriorityCritical,
	models.TaskPriorityHigh,
	models.TaskPriorityMedium,
	models.TaskPriorityLow,
}

func TestMCPTaskPrioritySchema(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	createProperties := toolInputProperties(t, s, "create_task_kandev")
	createPriority, ok := createProperties["priority"].(map[string]interface{})
	require.True(t, ok, "create priority should be advertised")
	assert.Equal(t, []interface{}{"critical", "high", "medium", "low"}, createPriority["enum"])
	assert.Equal(t, "medium", createPriority["default"])

	updateProperties := toolInputProperties(t, s, "update_task_kandev")
	updatePriority, ok := updateProperties["priority"].(map[string]interface{})
	require.True(t, ok, "update priority should be advertised")
	assert.Equal(t, []interface{}{"critical", "high", "medium", "low"}, updatePriority["enum"])
	assert.NotContains(t, updatePriority, "default", "update omission must preserve the stored value")
}

func TestMCPTaskPriorityArguments(t *testing.T) {
	for _, toolName := range []string{"create_task_kandev", "update_task_kandev"} {
		for _, priority := range taskPriorityValues {
			t.Run(toolName+"/accepts_"+priority, func(t *testing.T) {
				backend := &testBackend{}
				s := newTaskModeServer(t, backend, "task-current")
				args := map[string]interface{}{"priority": priority}
				if toolName == "create_task_kandev" {
					args["title"] = "Priority task"
				} else {
					args["task_id"] = "task-target"
				}

				result := callTool(t, s, toolName, args)
				assert.False(t, result.IsError, firstText(t, result))
				assert.NotEmpty(t, backend.lastAction)
				payload, ok := backend.lastPayload.(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, priority, payload["priority"])
			})
		}
	}

	invalid := []struct {
		name  string
		value interface{}
	}{
		{name: "empty", value: ""},
		{name: "padded", value: " high "},
		{name: "uppercase", value: "HIGH"},
		{name: "unknown", value: "urgent"},
		{name: "null", value: nil},
		{name: "non_string", value: 1},
	}
	for _, toolName := range []string{"create_task_kandev", "update_task_kandev"} {
		for _, tc := range invalid {
			t.Run(toolName+"/rejects_"+tc.name, func(t *testing.T) {
				backend := &testBackend{}
				s := newTaskModeServer(t, backend, "task-current")
				args := map[string]interface{}{"priority": tc.value}
				if toolName == "create_task_kandev" {
					args["title"] = "Priority task"
				} else {
					args["task_id"] = "task-target"
				}

				result := callTool(t, s, toolName, args)
				assert.True(t, result.IsError)
				assert.Empty(t, backend.lastAction, "invalid schema values must not dispatch")
			})
		}
	}
}

func TestMCPTaskPriorityForwarding(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"id": "task-created", "priority": "high"}}
	s := newTaskModeServer(t, backend, "task-current")

	create := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":    "Priority task",
		"priority": models.TaskPriorityHigh,
	})
	require.False(t, create.IsError, firstText(t, create))
	createPayload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, models.TaskPriorityHigh, createPayload["priority"])

	update := callTool(t, s, "update_task_kandev", map[string]interface{}{
		"task_id":  "task-current",
		"priority": models.TaskPriorityCritical,
	})
	require.False(t, update.IsError, firstText(t, update))
	assert.Equal(t, ws.ActionMCPUpdateTask, backend.lastAction)
	updatePayload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, models.TaskPriorityCritical, updatePayload["priority"])
}

func TestMCPTaskPriorityCatalog(t *testing.T) {
	log := newTestLogger(t)
	newChannelServer := func(profile mcpprofile.Context) *Server {
		backend := NewChannelBackendClient(log)
		t.Cleanup(backend.Close)
		return NewWithProfile(backend, "session", "task", 10005, log, "", false, profile)
	}
	newModeServer := func(mode string) *Server {
		backend := NewChannelBackendClient(log)
		t.Cleanup(backend.Close)
		return New(backend, "session", "task", 10005, log, "", false, mode)
	}

	surfaces := []struct {
		name      string
		server    *Server
		hasCreate bool
		hasUpdate bool
	}{
		{name: "task", server: newModeServer(ModeTask), hasCreate: true, hasUpdate: true},
		{name: "automation", server: newChannelServer(mcpprofile.NewAutomation()), hasCreate: true, hasUpdate: true},
		{name: "external", server: NewExternal(&testBackend{}, log, ""), hasCreate: true, hasUpdate: true},
		{name: "configuration", server: newModeServer(ModeConfig)},
		{name: "office", server: newModeServer(ModeOffice)},
		{name: "coordinator", server: newChannelServer(mcpprofile.NewCoordinator())},
		{name: "managed conversation", server: newChannelServer(mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil))},
	}

	for _, surface := range surfaces {
		t.Run(surface.name, func(t *testing.T) {
			tools := surface.server.mcpServer.ListTools()
			_, hasCreate := tools["create_task_kandev"]
			_, hasUpdate := tools["update_task_kandev"]
			assert.Equal(t, surface.hasCreate, hasCreate)
			assert.Equal(t, surface.hasUpdate, hasUpdate)
		})
	}
}
