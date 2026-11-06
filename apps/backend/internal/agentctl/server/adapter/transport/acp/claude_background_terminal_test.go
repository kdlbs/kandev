package acp

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// claudeTerminalAdapter builds a Claude adapter whose initialize handshake
// reports the exact verified bridge tuple.
func claudeTerminalAdapter(t *testing.T) *Adapter {
	t.Helper()
	a := newTestAdapterForAgent(claudeAgentID)
	a.agentInfo = &AgentInfo{
		Name:    verifiedClaudeTerminalBridgeName,
		Version: verifiedClaudeTerminalBridgeVersion,
	}
	return a
}

type claudeBackgroundLaunch struct {
	toolCallID string
	command    string
	title      string
}

func startClaudeBackgroundLaunch(t *testing.T, a *Adapter, sessionID string, launch claudeBackgroundLaunch) *AgentEvent {
	t.Helper()
	call := &acp.SessionUpdateToolCall{
		ToolCallId:    acp.ToolCallId(launch.toolCallID),
		Kind:          acp.ToolKindExecute,
		Status:        acp.ToolCallStatusPending,
		Title:         "Terminal",
		SessionUpdate: "tool_call",
		RawInput:      map[string]any{},
	}
	require.NotNil(t, a.convertToolCallUpdate(sessionID, call))

	completed := acp.ToolCallStatusCompleted
	event := a.convertToolCallResultUpdate(sessionID, &acp.SessionToolCallUpdate{
		ToolCallId:    acp.ToolCallId(launch.toolCallID),
		Status:        &completed,
		SessionUpdate: "tool_call_update",
		Meta:          map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		RawInput: map[string]any{
			"command":           launch.command,
			"description":       launch.title,
			"run_in_background": true,
		},
	})
	require.NotNil(t, event)
	require.True(t, event.NormalizedPayload.IsDetachedBackgroundLaunch(),
		"the launch tool must be bound before terminal evidence can settle it")
	return event
}

func claudeTerminalExitUpdate(toolCallID string, exitCode, signal any) *acp.SessionToolCallUpdate {
	return &acp.SessionToolCallUpdate{
		ToolCallId:    acp.ToolCallId(toolCallID),
		SessionUpdate: "tool_call_update",
		Meta: map[string]any{
			"terminal_exit": map[string]any{
				"exit_code":   exitCode,
				"signal":      signal,
				"terminal_id": toolCallID,
			},
		},
	}
}

func activeClaudeToolCallCount(a *Adapter) int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.activeToolCalls)
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.3
// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalPromptCleanupScope(t *testing.T) {
	cleanups := []struct {
		name      string
		preserves bool
		run       func(*Adapter)
	}{
		{name: "prompt end", preserves: true, run: func(a *Adapter) { a.cancelPromptEndToolCalls("session-claude") }},
		{name: "explicit cancel", preserves: false, run: func(a *Adapter) { a.cancelActiveToolCalls("session-claude") }},
	}
	identities := []struct {
		name          string
		agentName     string
		agentVersion  string
		wantPreserved bool
	}{
		{
			name:          "verified bridge",
			agentName:     verifiedClaudeTerminalBridgeName,
			agentVersion:  verifiedClaudeTerminalBridgeVersion,
			wantPreserved: true,
		},
		{
			name:          "legacy bridge",
			agentName:     verifiedClaudeTerminalBridgeName,
			agentVersion:  "0.81.2",
			wantPreserved: false,
		},
		{
			name:          "missing identity",
			agentName:     "",
			agentVersion:  "",
			wantPreserved: false,
		},
	}
	for _, identity := range identities {
		for _, cleanup := range cleanups {
			t.Run(identity.name+" "+cleanup.name, func(t *testing.T) {
				a := claudeTerminalAdapter(t)
				a.agentInfo = &AgentInfo{Name: identity.agentName, Version: identity.agentVersion}
				launch := claudeBackgroundLaunch{
					toolCallID: "toolu_cleanup", command: "sleep 30", title: "Sleep",
				}
				startClaudeBackgroundLaunch(t, a, "session-claude", launch)
				drainEvents(a)

				cleanup.run(a)

				events := drainEvents(a)
				if identity.wantPreserved && cleanup.preserves {
					require.Equal(t, 1, activeClaudeToolCallCount(a),
						"the verified bridge can still report this shell's terminal exit")
					for _, event := range events {
						require.NotEqual(t, toolStatusCancelled, event.ToolStatus)
					}
					return
				}
				require.Equal(t, 0, activeClaudeToolCallCount(a),
					"unsupported cleanup paths keep the prompt-end/explicit-cancel fallback")
				require.Len(t, events, 1)
				require.Equal(t, launch.toolCallID, events[0].ToolCallID)
				require.Equal(t, toolStatusCancelled, events[0].ToolStatus)
			})
		}
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalExit(t *testing.T) {
	t.Run("null exit settles only its matching live sibling", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		first := claudeBackgroundLaunch{
			toolCallID: "toolu_first", command: "go test ./...", title: "Run the test suite",
		}
		second := claudeBackgroundLaunch{
			toolCallID: "toolu_second", command: "gh pr checks --watch", title: "Watch checks",
		}
		startClaudeBackgroundLaunch(t, a, "session-claude", first)
		startClaudeBackgroundLaunch(t, a, "session-claude", second)
		// The foreground turn can end while a detached command still runs; the
		// verified binding must survive prompt-end cleanup so its later terminal
		// frame can settle the exact job.
		a.cancelPromptEndToolCalls("session-claude")
		for _, event := range drainEvents(a) {
			require.NotEqual(t, toolStatusCancelled, event.ToolStatus,
				"a verified detached shell must not be cancelled at prompt end")
		}
		require.Equal(t, 2, activeClaudeToolCallCount(a))

		event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(first.toolCallID, nil, nil))
		require.NotNil(t, event)
		require.Equal(t, toolStatusComplete, event.ToolStatus)
		require.False(t, event.ToolStatusMissing,
			"the adapter synthesized a terminal status from exact evidence")
		require.NotNil(t, event.BackgroundWork)
		require.Equal(t, first.toolCallID, event.BackgroundWork.WorkID)
		require.Equal(t, streams.WorkloadKindShell, event.BackgroundWork.Kind)
		require.Equal(t, streams.RunStateEnded, event.BackgroundWork.State,
			"a null bridge exit keeps the outcome unavailable rather than completed")
		require.Nil(t, event.BackgroundWork.ExitCode, "null is not zero")
		require.Equal(t, first.toolCallID, event.BackgroundWork.SourceCallID)
		require.Equal(t, first.title, event.BackgroundWork.Title)
		require.NotNil(t, event.BackgroundWork.FinishedAt)
		require.Equal(t, "none", event.BackgroundWork.Capabilities.Output)
		require.False(t, event.NormalizedPayload.IsActiveBackgroundWork())
		require.Equal(t, 1, activeClaudeToolCallCount(a), "a sibling job must stay tracked")

		a.mu.RLock()
		_, firstTracked := a.activeToolCalls[first.toolCallID]
		_, secondTracked := a.activeToolCalls[second.toolCallID]
		a.mu.RUnlock()
		require.False(t, firstTracked)
		require.True(t, secondTracked)

		// Duplicate terminal evidence cannot recreate the retired launch.
		duplicate := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(first.toolCallID, nil, nil))
		if duplicate != nil {
			require.Nil(t, duplicate.BackgroundWork)
		}
		require.Equal(t, 1, activeClaudeToolCallCount(a))
	})

	t.Run("numeric exit keeps the existing terminal-code path", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		launch := claudeBackgroundLaunch{
			toolCallID: "toolu_numeric", command: "echo done", title: "Print done",
		}
		startClaudeBackgroundLaunch(t, a, "session-claude", launch)

		event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(launch.toolCallID, float64(0), nil))
		require.NotNil(t, event)
		require.Equal(t, toolStatusComplete, event.ToolStatus)
		require.Nil(t, event.BackgroundWork,
			"numeric evidence keeps the generic normalized projection")
		require.NotNil(t, event.NormalizedPayload.ShellExec().Output)
		require.NotNil(t, event.NormalizedPayload.ShellExec().Output.ExitCode)
		require.Equal(t, 0, *event.NormalizedPayload.ShellExec().Output.ExitCode)
		require.False(t, event.NormalizedPayload.IsActiveBackgroundWork())
	})

	t.Run("fast completion is terminal evidence on the launch result", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		call := &acp.SessionUpdateToolCall{
			ToolCallId:    "toolu_fast",
			Kind:          acp.ToolKindExecute,
			Status:        acp.ToolCallStatusPending,
			Title:         "Terminal",
			SessionUpdate: "tool_call",
			RawInput:      map[string]any{},
		}
		require.NotNil(t, a.convertToolCallUpdate("session-claude", call))

		completed := acp.ToolCallStatusCompleted
		update := claudeTerminalExitUpdate("toolu_fast", nil, nil)
		update.Status = &completed
		update.RawInput = map[string]any{
			"command":           "true",
			"description":       "Fast command",
			"run_in_background": true,
		}
		update.RawOutput = map[string]any{"output": "backgrounded"}
		event := a.convertToolCallResultUpdate("session-claude", update)
		require.NotNil(t, event)
		require.NotNil(t, event.BackgroundWork)
		require.Equal(t, streams.RunStateEnded, event.BackgroundWork.State)
		require.Nil(t, event.BackgroundWork.ExitCode)
	})

	t.Run("signal termination settles with an unavailable exit code", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		launch := claudeBackgroundLaunch{
			toolCallID: "toolu_signaled", command: "sleep 300", title: "Long sleep",
		}
		startClaudeBackgroundLaunch(t, a, "session-claude", launch)

		event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(launch.toolCallID, nil, "SIGTERM"))
		require.NotNil(t, event)
		require.False(t, event.ToolStatusMissing)
		require.NotNil(t, event.BackgroundWork)
		require.Equal(t, streams.RunStateEnded, event.BackgroundWork.State)
		require.Nil(t, event.BackgroundWork.ExitCode, "a signal carries no exit code")
	})

	for _, tc := range []struct {
		name         string
		agentID      string
		agentName    string
		agentVersion string
	}{
		{"legacy bridge launch exit", claudeAgentID, verifiedClaudeTerminalBridgeName, "0.81.2"},
		{"unverified bridge version", claudeAgentID, verifiedClaudeTerminalBridgeName, "0.98.0"},
		{"foreign provider", openCodeAgentID, verifiedClaudeTerminalBridgeName, verifiedClaudeTerminalBridgeVersion},
		{"missing connected identity", claudeAgentID, "", ""},
	} {
		t.Run(tc.name+" retains prior behavior", func(t *testing.T) {
			a := claudeTerminalAdapter(t)
			a.agentInfo = &AgentInfo{Name: tc.agentName, Version: tc.agentVersion}
			launch := claudeBackgroundLaunch{
				toolCallID: "toolu_unverified", command: "sleep 30", title: "Sleep",
			}
			startClaudeBackgroundLaunch(t, a, "session-claude", launch)
			// The launch is recognized under the Claude identity; switching the
			// configured provider afterwards isolates the dialect gate.
			a.agentID = tc.agentID

			event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(launch.toolCallID, nil, nil))
			require.NotNil(t, event)
			require.Nil(t, event.BackgroundWork)
			require.True(t, event.NormalizedPayload.IsActiveBackgroundWork(),
				"unverified evidence must not end the workload")
			require.Equal(t, 1, activeClaudeToolCallCount(a))
		})
	}

	for _, tc := range []struct {
		name  string
		build func(string) *acp.SessionToolCallUpdate
	}{
		{
			name: "mismatched terminal id",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				update := claudeTerminalExitUpdate(toolCallID, nil, nil)
				update.Meta["terminal_exit"].(map[string]any)["terminal_id"] = "toolu_other"
				return update
			},
		},
		{
			name: "missing terminal id",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return &acp.SessionToolCallUpdate{
					ToolCallId:    acp.ToolCallId(toolCallID),
					SessionUpdate: "tool_call_update",
					Meta: map[string]any{"terminal_exit": map[string]any{
						"exit_code": nil, "signal": nil,
					}},
				}
			},
		},
		{
			name: "malformed exit code type",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return claudeTerminalExitUpdate(toolCallID, "zero", nil)
			},
		},
		{
			name: "malformed signal type",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return claudeTerminalExitUpdate(toolCallID, nil, true)
			},
		},
		{
			name: "missing nullable fields",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return &acp.SessionToolCallUpdate{
					ToolCallId:    acp.ToolCallId(toolCallID),
					SessionUpdate: "tool_call_update",
					Meta: map[string]any{"terminal_exit": map[string]any{
						"terminal_id": toolCallID,
					}},
				}
			},
		},
	} {
		t.Run(tc.name+" establishes no termination", func(t *testing.T) {
			a := claudeTerminalAdapter(t)
			launch := claudeBackgroundLaunch{
				toolCallID: "toolu_malformed", command: "sleep 30", title: "Sleep",
			}
			startClaudeBackgroundLaunch(t, a, "session-claude", launch)

			event := a.convertToolCallResultUpdate("session-claude", tc.build(launch.toolCallID))
			require.NotNil(t, event)
			require.Nil(t, event.BackgroundWork)
			a.mu.RLock()
			tracked := a.activeToolCalls[launch.toolCallID]
			a.mu.RUnlock()
			require.NotNil(t, tracked, "the launch binding must survive unrelated evidence")
			require.True(t, tracked.IsActiveBackgroundWork())
			require.Equal(t, 1, activeClaudeToolCallCount(a))
		})
	}
}
