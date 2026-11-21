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
	configDir := t.TempDir()
	signalFile := filepath.Join(t.TempDir(), "monitor.signals")
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv(mockClaudeMonitorSignalFileEnv, signalFile)
	t.Setenv(mockClaudeBridgeVersionEnv, mockClaudeBridgeMonitorVersion)

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
	require.NoError(t, os.WriteFile(signalFile, []byte("complete-first\n"), 0o600))
	require.Eventually(t, func() bool {
		rows, readErr := os.ReadFile(transcript)
		return readErr == nil && strings.Contains(string(rows), "monitor-task-first")
	}, 2*time.Second, 10*time.Millisecond)

	rows, err := os.ReadFile(transcript)
	require.NoError(t, err)
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(rows))), &first))
	require.Equal(t, "attachment", first["type"])
	require.Equal(t, mockClaudeNativeVersion, first["version"])
	require.Equal(t, string(e.sid), first["sessionId"])
	attachment := first["attachment"].(map[string]any)
	require.Equal(t, "queued_command", attachment["type"])
	require.Equal(t, "task-notification", attachment["origin"].(map[string]any)["kind"])
	require.Equal(t, "session-task", attachment["origin"].(map[string]any)["producer"])
	require.Contains(t, attachment["prompt"], "<tool-use-id>"+string(calls[0].ToolCallId)+"</tool-use-id>")

	require.NoError(t, os.WriteFile(signalFile, []byte("complete-first\ncomplete-second\n"), 0o600))
	require.Eventually(t, func() bool {
		rows, readErr := os.ReadFile(transcript)
		return readErr == nil && strings.Count(string(rows), "\n") == 2
	}, 2*time.Second, 10*time.Millisecond)
}

func claudeMonitorSessionFilePath(workDir, sessionID string) string {
	return filepath.Join("projects", encodeMockClaudeProjectName(filepath.Clean(workDir)), sessionID+".jsonl")
}
