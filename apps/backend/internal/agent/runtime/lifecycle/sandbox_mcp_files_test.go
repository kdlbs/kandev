//go:build linux

package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxMCPPiUsesPrivateAgentConfigWithoutProjectTrust(t *testing.T) {
	for _, mode := range []string{"default", "private override", "shared override", "missing state", "native"} {
		t.Run(mode, func(t *testing.T) {
			manager := newTestManager(t)
			home, work, shared := t.TempDir(), t.TempDir(), t.TempDir()
			attempt := &startSandboxFixture{launch: &contract.Launch{AttemptID: "attempt", ChildHome: home, MCPURL: "http://127.0.0.1:45679/mcp"}}
			execution := &AgentExecution{ID: "execution", SessionID: "session", TaskID: "task", WorkspacePath: work, standalonePort: 45678, SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}, sandboxAttempt: attempt}
			configDir := filepath.Join(home, ".pi", "agent")
			switch mode {
			case "private override":
				configDir = filepath.Join(home, "custom-pi")
				execution.setRuntimeEnvironment(map[string]string{"PI_CODING_AGENT_DIR": configDir})
			case "shared override":
				execution.setRuntimeEnvironment(map[string]string{"PI_CODING_AGENT_DIR": shared})
			case "missing state":
				attempt.launch.ChildHome = ""
			}
			agent, ok := manager.registry.Get("pi-acp")
			require.True(t, ok)
			var err error
			if mode == "native" {
				_, err = manager.applyPassthroughMCP(t.Context(), execution, agents.NewPiACP().PassthroughConfig(), agent, nil)
			} else {
				err = manager.materializeRuntimeProjectMCP(t.Context(), execution, agent, nil, "local")
			}
			if mode == "shared override" || mode == "missing state" {
				require.Error(t, err)
				require.NoFileExists(t, filepath.Join(shared, "mcp.json"))
			} else {
				require.NoError(t, err)
				content, readErr := os.ReadFile(filepath.Join(configDir, "mcp.json"))
				require.NoError(t, readErr)
				var payload struct {
					Servers map[string]struct {
						Type string `json:"type"`
						URL  string `json:"url"`
					} `json:"mcpServers"`
				}
				require.NoError(t, json.Unmarshal(content, &payload))
				require.Equal(t, "streamable-http", payload.Servers["kandev"].Type)
				require.Equal(t, attempt.launch.MCPURL, payload.Servers["kandev"].URL)
				manager.cleanupPassthroughMCPConfig(execution)
				require.NoFileExists(t, filepath.Join(configDir, "mcp.json"))
			}
			require.NoFileExists(t, filepath.Join(work, ".pi", "mcp.json"))
			require.NoFileExists(t, filepath.Join(home, ".pi", "agent", "trust.json"))
		})
	}
}

func TestSandboxNativeMCPFilesUseGrantedChildState(t *testing.T) {
	manager := newTestManager(t)
	manager.dataDir = t.TempDir()
	home, work := t.TempDir(), t.TempDir()
	attempt := &startSandboxFixture{launch: &contract.Launch{AttemptID: "attempt-1", ChildHome: home}}
	execution := &AgentExecution{ID: "execution", SessionID: "session", WorkspacePath: work, SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}, sandboxAttempt: attempt}
	paths := manager.passthroughMCPPaths(execution)
	require.Equal(t, filepath.Join(home, ".config", "kandev-mcp-attempt-1.json"), paths.TempConfigPath)
	require.Equal(t, work, paths.WorkspaceDir)
	body := []byte(`{"mcpServers":{"kandev":{"url":"http://127.0.0.1:45678/mcp"}}}`)
	require.NoError(t, manager.writePassthroughMCPFiles(t.Context(), execution, []mcpconfig.PassthroughConfigFile{{Path: paths.TempConfigPath, Content: body}}))
	content, err := os.ReadFile(paths.TempConfigPath)
	require.NoError(t, err)
	require.Equal(t, body, content)
	require.NoError(t, execution.closeSandbox())
	manager.cleanupPassthroughMCPConfig(execution)
	require.NoFileExists(t, paths.TempConfigPath)
	attempt.launch.ChildHome = ""
	require.Empty(t, manager.passthroughMCPPaths(execution).TempConfigPath, "no state grant must not fall back to private host storage")
}

func TestSandboxNativeMCPFilesRejectEscapedOrMalformedTargets(t *testing.T) {
	for _, mode := range []string{"outside", "parent symlink", "leaf symlink", "malformed merge"} {
		t.Run(mode, func(t *testing.T) {
			manager := newTestManager(t)
			home, work, private := t.TempDir(), t.TempDir(), t.TempDir()
			execution := &AgentExecution{ID: "execution", WorkspacePath: work, SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}, sandboxAttempt: &startSandboxFixture{launch: &contract.Launch{AttemptID: "attempt", ChildHome: home}}}
			target := filepath.Join(private, "secret.json")
			original := []byte("private sentinel")
			require.NoError(t, os.WriteFile(target, original, 0600))
			file := mcpconfig.PassthroughConfigFile{Path: target, Content: []byte(`{"mcpServers":{}}`)}
			switch mode {
			case "parent symlink":
				require.NoError(t, os.Symlink(private, filepath.Join(home, ".config")))
				file.Path = filepath.Join(home, ".config", "secret.json")
			case "leaf symlink":
				require.NoError(t, os.Mkdir(filepath.Join(home, ".config"), 0700))
				file.Path = filepath.Join(home, ".config", "link.json")
				require.NoError(t, os.Symlink(target, file.Path))
			case "malformed merge":
				file.Path = filepath.Join(work, "config.json")
				file.MergeKey = "mcpServers"
				require.NoError(t, os.WriteFile(file.Path, []byte("malformed"), 0600))
			}
			require.Error(t, manager.writePassthroughMCPFiles(t.Context(), execution, []mcpconfig.PassthroughConfigFile{file}))
			content, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, original, content)
		})
	}
}

func TestSandboxNativeMCPMissingStateGrantRefusesCommand(t *testing.T) {
	manager, execution, profile := newClaudePassthroughMCPTestManager(t)
	attempt := &startSandboxFixture{launch: &contract.Launch{AttemptID: "attempt", ChildHome: t.TempDir(), MCPURL: "http://127.0.0.1:45679/mcp"}}
	execution.SandboxSelection = &policy.SelectionContext{ExecutorID: "local"}
	execution.sandboxAttempt = attempt
	_, _, _, command, err := manager.passthroughAgentCommand(t.Context(), execution, profile)
	require.NoError(t, err)
	assertClaudePassthroughMCPConfig(t, command.Command, attempt.launch.MCPURL)
	manager.cleanupPassthroughMCPConfig(execution)
	attempt.launch.ChildHome = ""
	_, _, _, _, err = manager.passthroughAgentCommand(t.Context(), execution, profile)
	require.Error(t, err, "configured MCP must not silently disappear when no readable config location exists")
}

func TestSandboxNativeMCPCleanupCannotFollowReplacementDirectory(t *testing.T) {
	manager := newTestManager(t)
	home, private := t.TempDir(), t.TempDir()
	execution := &AgentExecution{ID: "execution", WorkspacePath: t.TempDir(), SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}, sandboxAttempt: &startSandboxFixture{launch: &contract.Launch{AttemptID: "attempt", ChildHome: home}}}
	path := manager.passthroughMCPPaths(execution).TempConfigPath
	content := []byte(`{"mcpServers":{}}`)
	require.NoError(t, manager.writePassthroughMCPFiles(t.Context(), execution, []mcpconfig.PassthroughConfigFile{{Path: path, Content: content}}))
	require.NoError(t, os.Rename(filepath.Dir(path), filepath.Join(home, "retained")))
	target := filepath.Join(private, filepath.Base(path))
	require.NoError(t, os.WriteFile(target, content, 0600))
	require.NoError(t, os.Symlink(private, filepath.Dir(path)))
	manager.cleanupPassthroughMCPConfig(execution)
	require.FileExists(t, target, "cleanup must leave files outside the captured child root untouched")
}
