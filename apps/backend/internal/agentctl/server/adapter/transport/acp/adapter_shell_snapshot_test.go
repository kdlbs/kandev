package acp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.4
func TestShellSnapshotCoalescingStorageBound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := newTestAdapter()
		defer func() { require.NoError(t, a.Close()) }()
		seedSnapshotTool(a, "shell")
		a.activeToolCalls["shell"].ShellExec().Output = &streams.ShellExecOutput{Stdout: strings.Repeat("x", maxShellOutputBytes)}
		var expected strings.Builder
		expected.WriteString(strings.Repeat("x", maxShellOutputBytes-160000))
		var count, bytes int
		var last AgentEvent
		for index := range 10000 {
			text := fmt.Sprintf("%016d", index)
			expected.WriteString(text)
			a.handleACPUpdate(shellSnapshotNotification(t, "shell", map[string]any{
				"terminal_output_delta": map[string]any{"data": text},
			}), 1)
			for _, event := range drainEvents(a) {
				encoded, err := json.Marshal(event)
				require.NoError(t, err)
				bytes += len(encoded)
				count++
				last = event
			}
			require.LessOrEqual(t, count, 42, "one accumulated snapshot per provider chunk amplifies storage")
			time.Sleep(time.Millisecond)
		}
		require.True(t, a.syncNotifQueue())
		for _, event := range drainEvents(a) {
			encoded, err := json.Marshal(event)
			require.NoError(t, err)
			bytes += len(encoded)
			count++
			last = event
		}
		require.LessOrEqual(t, count, 42, "one accumulated snapshot per provider chunk amplifies storage")
		require.Less(t, bytes, 16<<20)
		require.Equal(t, expected.String(), last.NormalizedPayload.ShellExec().Output.Stdout)
		t.Logf("snapshots=%d serialized_bytes=%d", count, bytes)
	})
}

func seedSnapshotTool(a *Adapter, id string) {
	a.activeToolCalls[id] = streams.NewShellExec("test-command", "/tmp/test", "", 0, false)
}

func TestShellSnapshotQuietFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := newTestAdapter()
		defer func() { require.NoError(t, a.Close()) }()
		seedSnapshotTool(a, "shell")
		a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("first")), 1)
		require.Len(t, drainEvents(a), 1)
		a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("second")), 1)
		require.Empty(t, drainEvents(a))
		time.Sleep(249 * time.Millisecond)
		synctest.Wait()
		require.Empty(t, drainEvents(a))
		time.Sleep(time.Millisecond)
		synctest.Wait()
		events := drainEvents(a)
		require.Len(t, events, 1)
		require.Equal(t, "firstsecond", events[0].NormalizedPayload.ShellExec().Output.Stdout)
	})
}

// @covers AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.5
func TestShellSnapshotOrderedBoundaries(t *testing.T) {
	for _, eventType := range []string{streams.EventTypeMessageChunk, streams.EventTypeReasoning,
		streams.EventTypeComplete, streams.EventTypeError, streams.EventTypeSessionMode} {
		t.Run(eventType, func(t *testing.T) {
			a := newTestAdapter()
			defer func() { require.NoError(t, a.Close()) }()
			seedSnapshotTool(a, "shell")
			a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("first")), 1)
			a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("second")), 1)
			a.sendUpdate(AgentEvent{Type: eventType, SessionID: "snapshot-session"})
			events := drainEvents(a)
			require.Len(t, events, 3)
			require.Equal(t, "firstsecond", events[1].NormalizedPayload.ShellExec().Output.Stdout)
			require.Equal(t, eventType, events[2].Type)
		})
	}
}

func TestShellSnapshotGenerationAndUnknownMetadata(t *testing.T) {
	a := newTestAdapter()
	defer func() { require.NoError(t, a.Close()) }()
	seedSnapshotTool(a, "shell")
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("one")), 1)
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("two")), 1)
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("three")), 2)
	require.Len(t, drainEvents(a), 3)
	meta := shellDelta("four")
	meta["provider_extension"] = "preserve"
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", meta), 2)
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", meta), 2)
	require.Len(t, drainEvents(a), 2, "unknown metadata must remain lossless")
}

func TestShellSnapshotTerminalAndBarrier(t *testing.T) {
	a := newTestAdapter()
	defer func() { require.NoError(t, a.Close()) }()
	seedSnapshotTool(a, "shell")
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("one")), 1)
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("two")), 1)
	require.True(t, a.syncNotifQueue())
	require.Len(t, drainEvents(a), 2)
	final := shellSnapshotNotification(t, "shell", nil)
	final.Update.ToolCallUpdate.RawOutput = map[string]any{"stdout": "final", "stderr": "error", "exit_code": 1}
	a.handleACPUpdate(final, 1)
	events := drainEvents(a)
	require.Len(t, events, 1)
	output := events[0].NormalizedPayload.ShellExec().Output
	require.Equal(t, "final", output.Stdout)
	require.Equal(t, "error", output.Stderr)
	require.Equal(t, 1, *output.ExitCode)
}

func TestShellSnapshotShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := newTestAdapter()
		defer func() { require.NoError(t, a.Close()) }()
		seedSnapshotTool(a, "shell")
		a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("one")), 1)
		drainEvents(a)
		a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("two")), 1)
		for len(a.updatesCh) < cap(a.updatesCh) {
			a.updatesCh <- AgentEvent{Type: streams.EventTypeReasoning}
		}
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		require.NoError(t, a.Close(), "shutdown must cancel a blocked timer flush")
	})
}

func shellDelta(text string) map[string]any {
	return map[string]any{"terminal_output_delta": map[string]any{"data": text}}
}

func TestShellSnapshotProviderShapes(t *testing.T) {
	for _, carrier := range []string{"terminal_output", "terminal_output_delta", "content"} {
		t.Run(carrier, func(t *testing.T) {
			a := newTestAdapter()
			defer func() { require.NoError(t, a.Close()) }()
			seedSnapshotTool(a, "shell")
			for _, text := range []string{"one", "two"} {
				n := shellSnapshotNotification(t, "shell", map[string]any{carrier: map[string]any{"data": text}})
				if carrier == "content" {
					n = shellSnapshotNotification(t, "shell", nil)
					raw := `[{"type":"content","content":{"type":"text","text":"` + text + `"}}]`
					require.NoError(t, json.Unmarshal([]byte(raw), &n.Update.ToolCallUpdate.Content))
				}
				a.handleACPUpdate(n, 1)
			}
			require.True(t, a.syncNotifQueue())
			events := drainEvents(a)
			require.Len(t, events, 2)
			want := "onetwo"
			if carrier == "content" {
				want = "two"
			}
			require.Equal(t, want, events[1].NormalizedPayload.ShellExec().Output.Stdout)
		})
	}
}

func TestShellSnapshotUnknownOutputFieldsRemainLossless(t *testing.T) {
	a := newTestAdapter()
	defer func() { require.NoError(t, a.Close()) }()
	seedSnapshotTool(a, "shell")
	for range 2 {
		a.handleACPUpdate(shellSnapshotNotification(t, "shell", map[string]any{
			"terminal_output_delta": map[string]any{"data": "output", "provider_extension": true},
		}), 1)
	}
	require.Len(t, drainEvents(a), 2)
}

func TestShellSnapshotCodexTerminalCarrier(t *testing.T) {
	for _, terminalID := range []string{"shell", "different-terminal"} {
		t.Run(terminalID, func(t *testing.T) {
			a := newTestAdapter()
			defer func() { require.NoError(t, a.Close()) }()
			seedSnapshotTool(a, "shell")
			for _, text := range []string{"one", "two"} {
				n := shellSnapshotNotification(t, "shell", map[string]any{
					"terminal_output": map[string]any{"data": text, "terminal_id": terminalID},
				})
				n.Update.ToolCallUpdate.Status = nil
				a.handleACPUpdate(n, 1)
			}
			want := 2
			if terminalID == "shell" {
				want = 1
			}
			require.Len(t, drainEvents(a), want, "Codex terminal carrier is a supported output shape only for the exact tool")
			require.True(t, a.syncNotifQueue())
			if terminalID == "shell" {
				events := drainEvents(a)
				require.Len(t, events, 1)
				require.Equal(t, "onetwo", events[0].NormalizedPayload.ShellExec().Output.Stdout)
			}
		})
	}
}

func TestShellSnapshotKeepsAdmissionGeneration(t *testing.T) {
	a := newTestAdapter()
	defer func() { require.NoError(t, a.Close()) }()
	seedSnapshotTool(a, "shell")
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("one")), 17)
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("two")), 17)
	a.sendUpdate(AgentEvent{Type: streams.EventTypeForegroundIdle, PromptGeneration: 18})
	events := drainEvents(a)
	require.Len(t, events, 3)
	require.Equal(t, uint64(17), events[0].PromptGeneration)
	require.Equal(t, uint64(17), events[1].PromptGeneration, "delayed output must retain its captured submission generation")
	require.Equal(t, uint64(18), events[2].PromptGeneration)
}

func shellSnapshotNotification(t *testing.T, id string, meta map[string]any) sdk.SessionNotification {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"sessionId": "snapshot-session", "update": map[string]any{
			"sessionUpdate": "tool_call_update", "toolCallId": id, "status": "in_progress", "_meta": meta,
		},
	})
	require.NoError(t, err)
	var notification sdk.SessionNotification
	require.NoError(t, json.Unmarshal(encoded, &notification))
	return notification
}
