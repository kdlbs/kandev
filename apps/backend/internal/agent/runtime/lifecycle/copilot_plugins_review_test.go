package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/copilotplugins"
)

// @covers AC-AGENTS-COPILOT-PLUGINS-004.4
func TestCopilotPluginPassthroughRefreshesSelection(t *testing.T) {
	for _, boundary := range []string{"initial", "resume", "fallback"} {
		for _, selected := range []bool{true, false} {
			name := boundary + "/remove"
			if selected {
				name = boundary + "/add"
			}
			t.Run(name, func(t *testing.T) {
				mgr, execution, profile, pty, root := newCopilotPassthroughInitialManager(t)
				execution.CopilotSelectedPlugins = []string{}
				if !selected {
					execution.CopilotSelectedPlugins = append([]string{}, profile.CopilotSelectedPlugins...)
					profile.CopilotSelectedPlugins = []string{}
				}
				switch boundary {
				case "initial":
					require.NoError(t, mgr.startPassthroughSession(context.Background(), execution, profile))
				case "resume":
					require.NoError(t, mgr.ResumePassthroughSession(context.Background(), execution.SessionID))
				case "fallback":
					pty.activeWebSocket = true
					execution.passthroughLaunchUsedResume = true
					execution.PassthroughStartedAt = time.Now()
					mgr.attemptResumeFallback(execution, pty, execution.SessionID, 1, time.Millisecond)
				}
				require.Equal(t, 1, pty.startCalls)
				require.Equal(t, selected, containsSequence(pty.startReq.Command,
					"--plugin-dir", canonicalCopilotPluginRootForTest(t, root)))
				require.Equal(t, profile.CopilotSelectedPlugins, execution.CopilotSelectedPlugins)
			})
		}
	}
}

// @covers AC-AGENTS-COPILOT-PLUGINS-004.8, AC-AGENTS-COPILOT-PLUGINS-005.5
func TestCopilotPluginRestartChecksReplacementWrapper(t *testing.T) {
	for _, wrapped := range []bool{true, false} {
		name := "remove"
		if wrapped {
			name = "add"
		}
		t.Run(name, func(t *testing.T) {
			mgr, execution, mock, runner, _, _ := newCopilotRestartFixture(t, "1.0.104")
			profile := mgr.profileResolver.(*restartProfileResolver).profile
			if wrapped {
				profile.CommandPrefix = "sandbox --"
			} else {
				execution.LaunchWrapper = copilotWrapperForTest()
			}
			err := mgr.RestartAgentProcess(context.Background(), execution.ID)
			if !wrapped {
				require.NoError(t, err)
				require.Equal(t, []string{"stop", "configure", "start"}, mock.getHTTPActions())
				return
			}
			var pluginErr *copilotplugins.Error
			require.ErrorAs(t, err, &pluginErr)
			require.Equal(t, copilotplugins.ReasonUnsupportedLaunch, pluginErr.Reason)
			require.Empty(t, runner.calls)
			require.Empty(t, mock.getHTTPActions())
			require.Equal(t, "old-session", execution.ACPSessionID)
		})
	}
}

// @covers AC-AGENTS-COPILOT-PLUGINS-004.8, AC-AGENTS-COPILOT-PLUGINS-005.5
func TestCopilotPluginPassthroughResetChecksReplacementWrapper(t *testing.T) {
	for _, wrapped := range []bool{true, false} {
		name := "remove"
		if wrapped {
			name = "add"
		}
		t.Run(name, func(t *testing.T) {
			mgr, execution, pty, runner, _, _ := newCopilotPassthroughResetFixture(t, "1.0.104", "local")
			profile := mgr.profileResolver.(*mockPassthroughProfileResolver).profile
			if wrapped {
				profile.CommandPrefix = "sandbox --"
			} else {
				execution.LaunchWrapper = copilotWrapperForTest()
			}
			err := mgr.restartPassthroughProcess(context.Background(), execution)
			if !wrapped {
				require.NoError(t, err)
				require.Equal(t, 1, pty.stopCalls)
				require.Equal(t, 1, pty.startCalls)
				return
			}
			var pluginErr *copilotplugins.Error
			require.ErrorAs(t, err, &pluginErr)
			require.Equal(t, copilotplugins.ReasonUnsupportedLaunch, pluginErr.Reason)
			require.Empty(t, runner.calls)
			require.Zero(t, pty.stopCalls)
			require.Zero(t, pty.startCalls)
			require.Equal(t, "pty-old", execution.PassthroughProcessID)
		})
	}
}

// @covers AC-AGENTS-COPILOT-PLUGINS-004.5
func TestCopilotPluginRestartRetryRefreshesPluginRoot(t *testing.T) {
	mgr, execution, mock, _, root, configDir := newCopilotRestartFixture(t, "1.0.104")
	require.NoError(t, mgr.RestartAgentProcess(context.Background(), execution.ID))
	newRoot := filepath.Join(t.TempDir(), "replacement")
	require.NoError(t, os.Rename(root, newRoot))
	configPath := filepath.Join(configDir, "config.json")
	config, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath,
		[]byte(strings.ReplaceAll(string(config), quoteJSON(root), quoteJSON(newRoot))), 0o600))
	_, err = mgr.configureAndStartAgent(context.Background(), execution)
	require.NoError(t, err)
	_, args, _ := mock.getConfiguredProcess()
	require.True(t, containsSequence(args, "--plugin-dir", canonicalCopilotPluginRootForTest(t, newRoot)))
	require.Equal(t, 1, countArg(args, "--plugin-dir"))
}
