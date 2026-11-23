package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
)

func terminalExitUpdates(updates []recordedUpdate) []map[string]any {
	var exits []map[string]any
	for _, update := range updates {
		if update.notification.Update.ToolCallUpdate == nil {
			continue
		}
		exit, ok := update.notification.Update.ToolCallUpdate.Meta["terminal_exit"].(map[string]any)
		if ok {
			exits = append(exits, exit)
		}
	}
	return exits
}

func TestClaudeBackgroundTerminalFixtureEmitsCapturedShape(t *testing.T) {
	tempRoot := t.TempDir()
	tempDir := filepath.Join(tempRoot, "temp with spaces")
	require.NoError(t, os.MkdirAll(tempDir, 0o700))
	t.Setenv("TMPDIR", tempDir)

	e, updater := newTestEmitter()
	emitClaudeBackgroundTerminalFixture(e, "/claude-background-terminal 10ms 10ms")

	updates := updater.getUpdates()
	var launchToolCalls []acp.SessionUpdateToolCall
	for _, update := range updates {
		if update.notification.Update.ToolCall != nil {
			launchToolCalls = append(launchToolCalls, *update.notification.Update.ToolCall)
		}
	}
	require.Len(t, launchToolCalls, 2, "both launch frames are synchronous")
	require.Equal(t, acp.ToolKindExecute, launchToolCalls[0].Kind)
	input, ok := launchToolCalls[0].RawInput.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, input["run_in_background"])
	terminalInfo, ok := launchToolCalls[0].Meta["terminal_info"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, string(launchToolCalls[0].ToolCallId), terminalInfo["terminal_id"])

	var outputPaths []string
	for _, update := range updater.getUpdates() {
		result := update.notification.Update.ToolCallUpdate
		if result == nil {
			continue
		}
		raw, ok := result.RawOutput.(map[string]any)
		if !ok {
			continue
		}
		output, _ := raw["output"].(string)
		if !strings.Contains(output, "Output is being written to: ") {
			continue
		}
		path := mockClaudeBackgroundShellOutputPath(string(e.sid), string(launchToolCalls[len(outputPaths)].ToolCallId))
		require.Contains(t, output, "Output is being written to: "+path)
		outputPaths = append(outputPaths, path)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.Mode().IsRegular())
	}
	require.Len(t, outputPaths, 2, "each detached shell launch names its own output file")

	require.Eventually(t, func() bool {
		return len(terminalExitUpdates(updater.getUpdates())) == 2
	}, 2*time.Second, 10*time.Millisecond)

	exits := terminalExitUpdates(updater.getUpdates())
	require.Equal(t, string(launchToolCalls[0].ToolCallId), exits[0]["terminal_id"])
	require.Contains(t, exits[0], "exit_code")
	require.Contains(t, exits[0], "signal")
	require.Nil(t, exits[0]["exit_code"])
	require.Nil(t, exits[0]["signal"])
	require.Equal(t, string(launchToolCalls[1].ToolCallId), exits[1]["terminal_id"])
	require.Contains(t, exits[1], "exit_code")
	require.Contains(t, exits[1], "signal")
	require.Nil(t, exits[1]["exit_code"])
	require.Nil(t, exits[1]["signal"])
	firstOutput, err := os.ReadFile(outputPaths[0])
	require.NoError(t, err)
	require.Contains(t, string(firstOutput), "Mock test suite started")
	require.Contains(t, string(firstOutput), "Mock test suite is still running")
	require.Contains(t, string(firstOutput), "Mock test suite finished")
}

func TestMockClaudeBridgeAgentInfoRequiresVerifiedVersion(t *testing.T) {
	t.Setenv(mockClaudeBridgeVersionEnv, "")
	require.Nil(t, mockClaudeBridgeAgentInfo())

	t.Setenv(mockClaudeBridgeVersionEnv, "0.81.2")
	require.Nil(t, mockClaudeBridgeAgentInfo())
	t.Setenv(mockClaudeBridgeVersionEnv, "0.98.0")
	require.Nil(t, mockClaudeBridgeAgentInfo())

	for _, version := range []string{mockClaudeBridgeLegacyVersion, mockClaudeBridgeMonitorVersion, mockClaudeBridgeVersion} {
		t.Run(version, func(t *testing.T) {
			t.Setenv(mockClaudeBridgeVersionEnv, version)
			info := mockClaudeBridgeAgentInfo()
			require.NotNil(t, info)
			require.Equal(t, mockClaudeBridgeName, info.Name)
			require.Equal(t, version, info.Version)
		})
	}
}
