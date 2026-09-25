package codexappserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

func TestCodexBackgroundPaginationAndPartialFailure(t *testing.T) {
	pageCount := 0
	failOnPage2 := false

	server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
		method := readString(req, "method")
		id := req["id"]
		switch method {
		case "initialize":
			return write(resultFrame(id, map[string]any{"userAgent": "Codex CLI 0.154.0"}))
		case "initialized":
			return nil
		case "model/list":
			return write(resultFrame(id, map[string]any{"data": []any{}}))
		case "thread/start":
			return write(resultFrame(id, map[string]any{"thread": map[string]any{"id": "thread-1"}}))
		case "thread/backgroundTerminals/list":
			pageCount++
			var params protocol.BackgroundTerminalsListParams
			_ = json.Unmarshal(req["params"], &params)
			if params.Cursor == nil {
				// Page 1
				nextCur := "page-2-cur"
				return write(resultFrame(id, map[string]any{
					"data": []any{
						map[string]any{"itemId": "term-1", "processId": "proc-1", "command": "npm test", "cwd": "/repo"},
					},
					"nextCursor": nextCur,
				}))
			}
			if failOnPage2 {
				return write(errorFrame(id, -32603, "internal error fetching page 2"))
			}
			// Page 2
			return write(resultFrame(id, map[string]any{
				"data": []any{
					map[string]any{"itemId": "term-2", "processId": "proc-2", "command": "npm build", "cwd": "/repo"},
				},
				"nextCursor": nil,
			}))
		default:
			return write(errorFrame(id, -32601, "unsupported"))
		}
	})
	defer server.close()

	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()
	require.NoError(t, adapter.Connect(server.clientWriter, server.clientReader))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, adapter.Initialize(ctx))
	_, err := adapter.NewSession(ctx, nil)
	require.NoError(t, err)

	// 1. Full successful pagination & snapshot load
	adapter.refreshBackgroundTerminals(ctx, "thread-1")
	workloads, err := adapter.ListBackgroundWorkloads(ctx, "thread-1")
	require.NoError(t, err)
	require.Len(t, workloads, 2)
	require.Equal(t, "term-1", workloads[0].WorkID)
	require.Equal(t, "term-2", workloads[1].WorkID)

	// 2. Partial failure on second page
	failOnPage2 = true
	_, err = adapter.FetchAllBackgroundTerminals(ctx, "thread-1")
	require.Error(t, err)

	// Verify adapter refresh does not falsely drop existing backgrounds on error
	adapter.refreshBackgroundTerminals(ctx, "thread-1")
	workloadsAfterError, err := adapter.ListBackgroundWorkloads(ctx, "thread-1")
	require.NoError(t, err)
	require.Len(t, workloadsAfterError, 2)
}

func TestCodexBackgroundSnapshotGeneration(t *testing.T) {
	terminals := []any{
		map[string]any{"itemId": "term-1", "processId": "proc-1", "command": "npm run watch", "cwd": "/repo"},
	}

	server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
		method := readString(req, "method")
		id := req["id"]
		switch method {
		case "initialize":
			return write(resultFrame(id, map[string]any{"userAgent": "Codex CLI 0.154.0"}))
		case "initialized":
			return nil
		case "model/list":
			return write(resultFrame(id, map[string]any{"data": []any{}}))
		case "thread/start":
			return write(resultFrame(id, map[string]any{"thread": map[string]any{"id": "thread-1"}}))
		case "thread/backgroundTerminals/list":
			return write(resultFrame(id, map[string]any{
				"data": terminals,
			}))
		default:
			return write(errorFrame(id, -32601, "unsupported"))
		}
	})
	defer server.close()

	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()
	require.NoError(t, adapter.Connect(server.clientWriter, server.clientReader))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, adapter.Initialize(ctx))
	_, err := adapter.NewSession(ctx, nil)
	require.NoError(t, err)

	// Initial refresh -> 1 terminal running
	adapter.refreshBackgroundTerminals(ctx, "thread-1")
	list, err := adapter.ListBackgroundWorkloads(ctx, "thread-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "term-1", list[0].WorkID)
	require.Equal(t, streams.RunStateRunning, list[0].State)

	// Terminals finish (empty list) -> poller shuts down and background is completed
	terminals = []any{}
	adapter.refreshBackgroundTerminals(ctx, "thread-1")
	list, err = adapter.ListBackgroundWorkloads(ctx, "thread-1")
	require.NoError(t, err)
	require.Empty(t, list)

	adapter.mu.RLock()
	cancelFunc := adapter.backgroundCancel
	adapter.mu.RUnlock()
	require.Nil(t, cancelFunc, "poller should be stopped when no background terminals are running")
}

func TestCodexChildInterruptExactTurn(t *testing.T) {
	interruptedThreadID := ""
	interruptedTurnID := ""
	terminatedProcessID := ""

	server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
		method := readString(req, "method")
		id := req["id"]
		switch method {
		case "initialize":
			return write(resultFrame(id, map[string]any{"userAgent": "Codex CLI 0.154.0"}))
		case "initialized":
			return nil
		case "model/list":
			return write(resultFrame(id, map[string]any{"data": []any{}}))
		case "thread/start":
			return write(resultFrame(id, map[string]any{"thread": map[string]any{"id": "thread-root"}}))
		case "thread/backgroundTerminals/terminate":
			var params protocol.BackgroundTerminalTerminateParams
			_ = json.Unmarshal(req["params"], &params)
			terminatedProcessID = params.ProcessID
			return write(resultFrame(id, map[string]any{"terminated": true}))
		case "turn/interrupt":
			var params protocol.TurnInterruptParams
			_ = json.Unmarshal(req["params"], &params)
			interruptedThreadID = params.ThreadID
			interruptedTurnID = params.TurnID
			return write(resultFrame(id, map[string]any{"success": true}))
		default:
			return write(errorFrame(id, -32601, "unsupported"))
		}
	})
	defer server.close()

	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()
	require.NoError(t, adapter.Connect(server.clientWriter, server.clientReader))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, adapter.Initialize(ctx))
	_, err := adapter.NewSession(ctx, nil)
	require.NoError(t, err)

	// Register a child subagent
	adapter.emitCollabToolCall(map[string]any{
		"id":                "call-child-1",
		"tool":              "explore",
		"prompt":            "find files",
		"receiverThreadIds": []any{"thread-child-1"},
	}, "thread-root", "turn-1", false)

	adapter.handleTurnStarted("thread-child-1", "thread-root", "turn-child-1", false)

	// Execute interrupt action on child subagent
	resp, err := adapter.ExecuteBackgroundAction(ctx, streams.BackgroundWorkActionRequest{
		WorkID: "thread-child-1",
		Action: streams.WorkloadActionInterrupt,
	}, "thread-root")
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, "thread-child-1", interruptedThreadID)
	require.Equal(t, "turn-child-1", interruptedTurnID)

	// Register background terminal
	adapter.mu.Lock()
	adapter.backgrounds["term-1"] = protocol.BackgroundTerminal{
		ItemID:    "term-1",
		ProcessID: "proc-999",
		Command:   "watch tests",
		CWD:       "/repo",
	}
	adapter.mu.Unlock()

	// Execute stop action on background terminal
	respTerm, err := adapter.ExecuteBackgroundAction(ctx, streams.BackgroundWorkActionRequest{
		WorkID: "term-1",
		Action: streams.WorkloadActionStop,
	}, "thread-root")
	require.NoError(t, err)
	require.True(t, respTerm.Success)
	require.Equal(t, "proc-999", terminatedProcessID)

	// Unsupported action write_input
	respInput, err := adapter.ExecuteBackgroundAction(ctx, streams.BackgroundWorkActionRequest{
		WorkID: "term-1",
		Action: streams.WorkloadActionWriteInput,
		Data:   "stdin content",
	}, "thread-root")
	require.NoError(t, err)
	require.False(t, respInput.Success)
	require.Contains(t, respInput.Error, "unsupported")
}

func TestCodexChildBeforeBinding(t *testing.T) {
	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()

	// Emit subagent activity for child thread before collabToolCall binds it
	adapter.emitSubagentActivity(map[string]any{
		"agentThreadId": "early-child-1",
		"kind":          "running",
	})

	adapter.mu.RLock()
	earlyKind := adapter.earlyChildActivities["early-child-1"]
	adapter.mu.RUnlock()
	require.Equal(t, "running", earlyKind)

	// Now collabToolCall arrives and binds
	adapter.emitCollabToolCall(map[string]any{
		"id":                "call-early-1",
		"tool":              "planner",
		"prompt":            "create plan",
		"receiverThreadIds": []any{"early-child-1"},
	}, "thread-root", "turn-1", false)

	adapter.mu.RLock()
	binding, exists := adapter.children["early-child-1"]
	_, stillEarly := adapter.earlyChildActivities["early-child-1"]
	adapter.mu.RUnlock()

	require.True(t, exists)
	require.False(t, stillEarly, "early activity should be drained and cleared")
	require.Equal(t, "call-early-1", binding.toolCallID)
}

func TestCodexNestedChildAndMultipleCalls(t *testing.T) {
	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()

	// Parent spawns 2 child subagents
	adapter.emitCollabToolCall(map[string]any{
		"id":                "call-1",
		"tool":              "explore",
		"prompt":            "search a",
		"receiverThreadIds": []any{"child-thread-a"},
	}, "root-thread", "turn-1", false)

	adapter.emitCollabToolCall(map[string]any{
		"id":                "call-2",
		"tool":              "explore",
		"prompt":            "search b",
		"receiverThreadIds": []any{"child-thread-b"},
	}, "root-thread", "turn-1", false)

	adapter.mu.RLock()
	bindingA := adapter.children["child-thread-a"]
	bindingB := adapter.children["child-thread-b"]
	adapter.mu.RUnlock()

	require.Equal(t, "call-1", bindingA.toolCallID)
	require.Equal(t, "root-thread", bindingA.parentThreadID)
	require.Equal(t, "call-2", bindingB.toolCallID)
	require.Equal(t, "root-thread", bindingB.parentThreadID)
}
