package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/processcontainment"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func processSandboxFixture(t *testing.T) (*sandbox.Launch, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process fixture")
	}
	group, groupErr := processcontainment.Create()
	if groupErr != nil && os.Getenv("KANDEV_TEST_SANDBOX_CGROUP") != "1" {
		t.Skip("requires delegated Linux cgroup v2")
	}
	require.NoError(t, groupErr)
	t.Cleanup(func() { require.NoError(t, group.Kill(context.Background())); require.NoError(t, group.Remove()) })
	receipt := filepath.Join(t.TempDir(), "receipt")
	content := []byte("#!/bin/sh\n[ -z \"$UNAPPROVED_SECRET\" ] || exit 61\nprintf 'sandbox\\n' >> \"$SANDBOX_RECEIPT\"\nexec \"$@\"\n")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(context.Background(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	return &sandbox.Launch{Containment: group, Version: 1, AttemptID: "process-attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests, EnvironmentKeys: []string{"SANDBOX_RECEIPT"}}, receipt
}

func TestSandboxSessionAndOneShotCommands(t *testing.T) {
	launch, receipt := processSandboxFixture(t)
	raw, err := json.Marshal(map[string]any{"sandbox": launch, "sandbox_required": true})
	require.NoError(t, err)
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentArgs: []string{"/bin/sh", "-c", "printf allowed"}, ContinueArgs: []string{"/bin/sh", "-c", "printf continued"}, AgentEnv: []string{"SANDBOX_RECEIPT=" + receipt, "UNAPPROVED_SECRET=private"}}
	require.NoError(t, json.Unmarshal(raw, cfg))
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newStubAdapter()
	for range 2 {
		manager.adapterCfg = &adapter.Config{OneShotConfig: &adapter.OneShotConfig{InitialArgs: cfg.AgentArgs, ContinueArgs: cfg.ContinueArgs}}
		require.NoError(t, manager.buildFinalCommand())
		output, err := manager.cmd.Output()
		require.NoError(t, err)
		require.Equal(t, "allowed", string(output))
		require.NotContains(t, manager.GetFinalCommand(), launch.Root)
		for _, args := range [][]string{manager.adapterCfg.OneShotConfig.InitialArgs, manager.adapterCfg.OneShotConfig.ContinueArgs} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Env = manager.adapterCfg.OneShotConfig.Env
			require.NoError(t, cmd.Run())
		}
	}
	got, err := os.ReadFile(receipt)
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("sandbox\n", 6), string(got))
}

func TestSandboxSessionRejectsLostRecoveryEnvelope(t *testing.T) {
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentArgs: []string{"/bin/sh", "-c", "exit 0"}}
	require.NoError(t, json.Unmarshal([]byte(`{"sandbox_required":true}`), cfg))
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newStubAdapter()
	require.Error(t, manager.buildFinalCommand())
	require.Nil(t, manager.cmd)
}

func TestSandboxInteractiveInputAndDeferredValidation(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(map[bool]string{false: "input", true: "changed-before-resize"}[tamper], func(t *testing.T) {
			launch, receipt := processSandboxFixture(t)
			input := filepath.Join(t.TempDir(), "input")
			req := InteractiveStartRequest{SessionID: "sandbox-session", Command: []string{"/bin/sh", "-c", `IFS= read -r line; printf '%s' "$line" > "$1"`, "fixture", input}, WorkingDir: t.TempDir(), Env: map[string]string{"SANDBOX_RECEIPT": receipt, "UNAPPROVED_SECRET": "private"}}
			raw, err := json.Marshal(map[string]any{"sandbox": launch})
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &req))
			runner := NewInteractiveRunner(nil, newTestLogger(t), 1024)
			info, err := runner.Start(context.Background(), req)
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = runner.Stop(ctx, info.ID)
			})
			if tamper {
				require.NoError(t, os.Chmod(launch.Prefix[0], 0700))
				require.Error(t, runner.ResizeByProcessID(info.ID, 90, 30))
				_, err = os.Stat(receipt)
				require.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			require.NoError(t, runner.ResizeByProcessID(info.ID, 90, 30))
			owner, err := runner.SandboxOwner(info.ID)
			require.NoError(t, err)
			require.Equal(t, launch.AttemptID, owner.AttemptID)
			require.NotEmpty(t, owner.Supervisor.BirthToken)
			writer, err := runner.GetPtyWriter(info.ID)
			require.NoError(t, err)
			_, err = writer.Write([]byte("hello sandbox\n"))
			require.NoError(t, err)
			require.Eventually(t, func() bool { got, err := os.ReadFile(input); return err == nil && string(got) == "hello sandbox" }, 3*time.Second, 10*time.Millisecond)
			got, err := os.ReadFile(receipt)
			require.NoError(t, err)
			require.Equal(t, "sandbox\n", string(got))
		})
	}
}

func TestSandboxSessionStderrRetainsOnlySafeDiagnostics(t *testing.T) {
	manager := &Manager{cfg: &config.InstanceConfig{Sandbox: &sandbox.Launch{}},
		stderr: io.NopCloser(strings.NewReader("private-supervisor-value\nnpm error code ECONNREFUSED\n")), logger: newTestLogger(t)}
	manager.wg.Add(1)
	manager.readStderr(make(chan stderrReadResult, 1))
	require.Equal(t, []string{"npm error code ECONNREFUSED"}, manager.GetRecentStderr())
}

func TestSandboxMCPConfigurationReplacesAutomaticRoute(t *testing.T) {
	launch, _ := processSandboxFixture(t)
	launch.MCPURL = "http://127.0.0.1:31234/" + strings.Repeat("a", 64) + "/mcp"
	cfg := &config.InstanceConfig{Port: 43210, InjectedKandevMCP: true, McpServers: []config.McpServerConfig{{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp"}, {Name: "kandev", Type: "sse", URL: "http://localhost:43210/sse"}, {Name: "external", Type: "http", URL: "https://example.test/mcp"}}}
	manager := NewManager(cfg, newTestLogger(t))
	require.NoError(t, manager.ConfigureExecution(ExecutionConfiguration{ArgsPresent: true, Args: []string{"agent"}, Sandbox: launch}))
	require.Len(t, cfg.McpServers, 2)
	require.Equal(t, launch.MCPURL, cfg.McpServers[0].URL)
	require.True(t, injectedKandevMCPConfigured(cfg))
	cfg.McpServers[0].URL = "http://localhost:43210/mcp"
	require.False(t, injectedKandevMCPConfigured(cfg))
	launch.MCPURL = ""
	require.NoError(t, manager.ConfigureExecution(ExecutionConfiguration{ArgsPresent: true, Args: []string{"agent"}, Sandbox: launch}))
	require.Len(t, cfg.McpServers, 1)
	require.Equal(t, "external", cfg.McpServers[0].Name)
	require.False(t, cfg.InjectedKandevMCP)
}
