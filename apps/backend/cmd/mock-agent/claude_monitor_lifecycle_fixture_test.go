package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
)

func TestClaudeMonitorLifecycleFixture(t *testing.T) {
	resetState()
	configDir := t.TempDir()
	signalFile := filepath.Join(t.TempDir(), "monitor.signals")
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv(mockClaudeMonitorSignalFileEnv, signalFile)
	t.Setenv(mockClaudeBridgeVersionEnv, mockClaudeBridgeVersion)

	e, updater := newTestEmitter()
	var cancel context.CancelFunc
	workerDone := make(chan struct{})
	e.registerDetachedWork = func(_ acp.SessionId, stop context.CancelFunc) func() {
		cancel = stop
		return func() { close(workerDone) }
	}
	t.Cleanup(func() {
		if cancel != nil {
			cancel()
		}
		select {
		case <-workerDone:
		case <-time.After(time.Second):
			t.Error("Monitor fixture worker did not stop after test cleanup")
		}
	})
	emitClaudeMonitorLifecycleFixture(e)
	updates := updater.getUpdates()
	var calls []acp.SessionUpdateToolCall
	for _, update := range updates {
		if update.notification.Update.ToolCall != nil {
			calls = append(calls, *update.notification.Update.ToolCall)
		}
	}
	require.Len(t, calls, 2)
	require.Equal(t, "Monitor", calls[0].Meta["claudeCode"].(map[string]any)["toolName"])
	require.Equal(t, "Monitor", calls[1].Meta["claudeCode"].(map[string]any)["toolName"])
	var messages []string
	for _, update := range updates {
		if isTextUpdate(update) {
			messages = append(messages, getTextContent(update))
		}
	}
	require.Contains(t, strings.Join(messages, ""), "Two Claude Monitors are watching.")

	workDir, err := os.Getwd()
	require.NoError(t, err)
	transcript := filepath.Join(configDir, claudeMonitorSessionFilePath(workDir, string(e.sid)))
	require.Equal(t, "2.1.328", mockClaudeNativeVersion)
	initialUpdateCount := len(updates)
	handlePrompt(e, "/claude-monitor-stop-first", modelFast)
	stopUpdates := updater.getUpdates()[initialUpdateCount:]
	var taskStopCall *acp.SessionUpdateToolCall
	var taskStopUpdates []*acp.SessionToolCallUpdate
	for _, update := range stopUpdates {
		if call := update.notification.Update.ToolCall; call != nil && taskStopMetaName(call.Meta) == "TaskStop" {
			taskStopCall = call
		}
		if result := update.notification.Update.ToolCallUpdate; result != nil && taskStopMetaName(result.Meta) == "TaskStop" {
			taskStopUpdates = append(taskStopUpdates, result)
		}
	}
	require.NotNil(t, taskStopCall, "the TaskStop prompt must emit a tool_call")
	require.Len(t, taskStopUpdates, 3, "the TaskStop prompt must emit the captured three-update sequence")
	callInput, ok := taskStopCall.RawInput.(map[string]any)
	require.True(t, ok)
	require.Empty(t, callInput, "the captured tool_call starts with empty rawInput")
	inputUpdate := taskStopUpdates[0]
	updateInput, ok := inputUpdate.RawInput.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "monitor-task-first", updateInput["task_id"])
	require.Nil(t, inputUpdate.Status)
	require.NotContains(t, inputUpdate.Meta["claudeCode"].(map[string]any), "toolResponse")

	responseUpdate := taskStopUpdates[1]
	require.Nil(t, responseUpdate.RawInput)
	require.Nil(t, responseUpdate.Status)
	claudeCode := responseUpdate.Meta["claudeCode"].(map[string]any)
	require.Equal(t, "monitor-task-first", claudeCode["toolResponse"].(map[string]any)["task_id"])
	require.Nil(t, responseUpdate.RawOutput)

	completionUpdate := taskStopUpdates[2]
	require.Nil(t, completionUpdate.RawInput)
	require.Equal(t, acp.ToolCallStatusCompleted, *completionUpdate.Status)
	require.NotContains(t, completionUpdate.Meta["claudeCode"].(map[string]any), "toolResponse")
	completionOutput, ok := completionUpdate.RawOutput.(string)
	require.True(t, ok, "the captured terminal rawOutput is a JSON string")
	var taskStopOutput map[string]any
	require.NoError(t, json.Unmarshal([]byte(completionOutput), &taskStopOutput))
	require.Equal(t, "monitor-task-first", taskStopOutput["task_id"])
	require.Eventually(t, func() bool {
		content, readErr := os.ReadFile(signalFile)
		return readErr == nil && strings.Contains(string(content), "taskstop-first")
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, os.WriteFile(signalFile, []byte("taskstop-first\ncomplete-second\n"), 0o600))
	require.Eventually(t, func() bool {
		rows, readErr := os.ReadFile(transcript)
		return readErr == nil && strings.Count(string(rows), "\n") == 1
	}, 2*time.Second, 10*time.Millisecond)

	rows, err := os.ReadFile(transcript)
	require.NoError(t, err)
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(rows))), &second))
	require.Equal(t, "attachment", second["type"])
	require.Equal(t, mockClaudeNativeVersion, second["version"])
	require.Equal(t, string(e.sid), second["sessionId"])
	attachment := second["attachment"].(map[string]any)
	require.Equal(t, "queued_command", attachment["type"])
	require.Equal(t, "task-notification", attachment["origin"].(map[string]any)["kind"])
	require.Equal(t, "session-task", attachment["origin"].(map[string]any)["producer"])
	require.Contains(t, attachment["prompt"], "<task-id>monitor-task-second</task-id>")
	require.Contains(t, attachment["prompt"], "<tool-use-id>"+string(calls[1].ToolCallId)+"</tool-use-id>")
}

func taskStopMetaName(meta map[string]any) string {
	claudeCode, _ := meta["claudeCode"].(map[string]any)
	name, _ := claudeCode["toolName"].(string)
	return name
}

func claudeMonitorSessionFilePath(workDir, sessionID string) string {
	return filepath.Join("projects", encodeMockClaudeProjectName(filepath.Clean(workDir)), sessionID+".jsonl")
}
