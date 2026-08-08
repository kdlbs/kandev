package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestListTaskSessions_RegisteredWhereverConversationIs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(t *testing.T) *Server
	}{
		{"task", func(t *testing.T) *Server { return newTaskModeServer(t, &testBackend{}, "task-current") }},
		{"config", func(t *testing.T) *Server { return newTestServer(t, &testBackend{}) }},
		{"external", func(t *testing.T) *Server {
			return New(&testBackend{}, "test-session", "", 10005, newTestLogger(t), "", false, ModeExternal)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tools := getRegisteredToolNames(tc.build(t))
			require.Contains(t, tools, "get_task_conversation_kandev")
			assert.Contains(t, tools, "list_task_sessions_kandev",
				"session discovery must ship with the tools that consume a session_id")
		})
	}
}

// Office mode deliberately exposes neither the conversation reader nor session
// discovery — office agents coordinate through `agentctl kandev …`.
func TestListTaskSessions_NotRegisteredInOfficeMode(t *testing.T) {
	s := New(&testBackend{}, "test-session", "task-current", 10005, newTestLogger(t), "", false, ModeOffice)

	tools := getRegisteredToolNames(s)
	require.NotContains(t, tools, "get_task_conversation_kandev")
	assert.NotContains(t, tools, "list_task_sessions_kandev")
}

func TestListTaskSessions_ToolSchemaIsTaskIDOnly(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	tool, ok := s.mcpServer.ListTools()["list_task_sessions_kandev"]
	require.True(t, ok)

	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	properties, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok, "schema must declare properties")
	require.Len(t, properties, 1, "callers must not be able to set current_session_id")
	assert.Contains(t, properties, "task_id")
	assert.Equal(t, []interface{}{"task_id"}, parsed["required"])

	assert.Contains(t, tool.Tool.Description, "get_task_conversation_kandev")
	assert.Contains(t, tool.Tool.Description, "message_task_kandev")
	assert.Contains(t, tool.Tool.Description, "is_primary")
}

func TestListTaskSessions_ForwardsTaskIDAndBoundSession(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"total": 2}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_task_sessions_kandev", map[string]interface{}{
		"task_id": "task-target",
	})
	require.False(t, result.IsError)

	assert.Equal(t, ws.ActionMCPListTaskSessions, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-target", payload["task_id"])
	assert.Equal(t, "test-session", payload["current_session_id"],
		"is_current must be derived from the bound session, not the arguments")
}

// current_session_id is not part of the advertised schema, so a caller cannot
// smuggle one in to make another session look like its own.
func TestListTaskSessions_RejectsCallerSuppliedCurrentSession(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"total": 2}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_task_sessions_kandev", map[string]interface{}{
		"task_id":            "task-target",
		"current_session_id": "spoofed-session",
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "a rejected call must not reach the backend")
}

func TestListTaskSessions_MissingTaskID_ReturnsError(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	result := callTool(t, s, "list_task_sessions_kandev", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestListTaskSessions_BackendError_ReturnsError(t *testing.T) {
	backend := &testBackend{err: assert.AnError}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_task_sessions_kandev", map[string]interface{}{
		"task_id": "task-target",
	})

	assert.True(t, result.IsError)
}

// The per-mode tool counts in registerTools are hand-maintained; this pins them
// to what was actually registered so adding a tool cannot silently skew the
// number operators read out of the startup log.
func TestRegisterTools_LoggedCountMatchesRegisteredTools(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeTaskTitlePending, ModeConfig, ModeExternal, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)
			log, err := logger.NewFromZap(zap.New(core))
			require.NoError(t, err)

			s := New(&testBackend{}, "test-session", "task-current", 10005, log, "", false, mode, []string{"github", "gitlab"})

			entries := logs.FilterMessage("registered MCP tools").All()
			require.Len(t, entries, 1)
			count, err := strconv.Atoi(fmt.Sprint(entries[0].ContextMap()["count"]))
			require.NoError(t, err)

			assert.Equal(t, len(s.mcpServer.ListTools()), count,
				"logged tool count must match the tools actually registered")
		})
	}
}
