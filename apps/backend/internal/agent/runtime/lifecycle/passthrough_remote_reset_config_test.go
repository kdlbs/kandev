package lifecycle

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestSSHPassthroughResetReplacesChangedWorkspaceMCP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SSH passthrough runs on Unix targets")
	}
	for _, strategy := range []struct {
		name   string
		mcp    mcpconfig.PassthroughMCPStrategy
		shared bool
	}{
		{name: "cursor", mcp: mcpconfig.CursorStrategy{}},
		{name: "pi", mcp: mcpconfig.PiStrategy{}},
		{name: "cursor", mcp: mcpconfig.CursorStrategy{}, shared: true},
		{name: "pi", mcp: mcpconfig.PiStrategy{}, shared: true},
	} {
		t.Run(strategy.name+"_"+strconv.FormatBool(strategy.shared), func(t *testing.T) {
			mgr, _, execution := newPassthroughRunnerManager(t)
			controller, root := sshPassthroughTestController(t, execution)
			t.Cleanup(func() { _ = mgr.Stop() })
			const agentID = "remote-reset-config-test"
			pt := agents.PassthroughConfig{
				Supported: true, MCPStrategy: strategy.mcp,
				PassthroughCmd: agents.NewCommand("/bin/sh", "-c", "printf 'READY> '; cat"),
				PromptPattern:  "READY> $", IdleTimeout: time.Minute,
			}
			require.NoError(t, mgr.registry.Register(&testAgent{
				id: agentID, enabled: true, StandardPassthrough: agents.StandardPassthrough{Cfg: pt},
			}))
			mgr.profileResolver = &mockPassthroughProfileResolver{agentName: agentID, cliPassthrough: true}
			execution.AgentID = agentID
			execution.setMetadataValue("executor_mcp_policy", `{"allow_stdio":true}`)
			profile := &mcpconfig.ProfileConfig{Enabled: true, Servers: map[string]mcpconfig.ServerDef{
				"profile": {Type: mcpconfig.ServerTypeStdio, Command: "before"},
			}}
			mgr.mcpProvider = &fakeMcpConfigProvider{config: profile}
			_, _, _, cmd, err := mgr.passthroughAgentCommand(context.Background(), execution, nil)
			require.NoError(t, err)
			info, err := mgr.startInteractiveProcess(context.Background(), execution, pt, nil, cmd, nil, nil)
			require.NoError(t, err)
			execution.PassthroughProcessID = info.ID
			execution.Status = v1.AgentStatusReady
			path := filepath.Join(root, "."+strategy.name, "mcp.json")
			if strategy.shared {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, passthroughconfig.Prepare(context.Background(), root, root, "other:session:1", []passthroughconfig.File{{
					Path: path, Content: data, MergeKey: "mcpServers",
				}}))
			}
			profile.Servers["profile"] = mcpconfig.ServerDef{Type: mcpconfig.ServerTypeStdio, Command: "after"}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err = mgr.restartPassthroughProcess(ctx, execution)
			if strategy.shared {
				require.Error(t, err)
				require.Equal(t, v1.AgentStatusFailed, execution.Status)
				require.Empty(t, execution.PassthroughProcessID)
				require.False(t, controller.Passthrough().Live())
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Contains(t, string(data), "before")
				require.NotContains(t, string(data), "after")
				return
			}
			require.NoError(t, err)
			require.NotEqual(t, info.ID, execution.PassthroughProcessID)
			require.Equal(t, uint64(2), controller.Passthrough().Snapshot().Generation)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(data), "after")
			require.NotContains(t, string(data), "before")
		})
	}
}

func TestSSHPassthroughResetPreflightPreservesLiveMCP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SSH passthrough runs on Unix targets")
	}
	for _, failure := range []string{"profile_secret", "stop"} {
		t.Run(failure, func(t *testing.T) {
			mgr, _, execution := newPassthroughRunnerManager(t)
			controller, root := sshPassthroughTestControllerWithHandler(t, execution, func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if failure == "stop" && r.Method == http.MethodDelete {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					next.ServeHTTP(w, r)
				})
			})
			if failure == "profile_secret" {
				mgr.profileResolver = &mockPassthroughProfileResolver{
					agentName: "claude-acp", cliPassthrough: true,
					envVars: []settingsmodels.ProfileEnvVar{{Key: "PROFILE_ONLY", SecretID: "deleted-secret"}},
				}
			}
			file := passthroughconfig.File{
				Path: filepath.Join(root, ".cursor", "mcp.json"), MergeKey: "mcpServers",
				Content: []byte(`{"mcpServers":{"profile":{"command":"before"}}}`),
			}
			client, release := execution.AcquireAgentCtlClient()
			require.NoError(t, client.PreparePassthrough(context.Background(), types.PassthroughPrepareRequest{
				Generation: 1, Files: []passthroughconfig.File{file},
			}))
			release()
			info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, passthroughCommand{Command: agents.NewCommand("/bin/sh", "-c", "cat")}, nil, nil)
			require.NoError(t, err)
			execution.PassthroughProcessID = info.ID
			execution.PassthroughStartedAt = info.StartedAt
			execution.passthroughInitialPromptProcessID = info.ID
			registry := filepath.Join(root, ".kandev", "passthrough-claims.json")
			before, err := os.ReadFile(registry)
			require.NoError(t, err)
			err = mgr.restartPassthroughProcess(context.Background(), execution)
			require.Error(t, err)
			if failure == "profile_secret" {
				require.ErrorIs(t, err, ErrProfileSecretUnavailable)
			}
			require.Equal(t, info.ID, execution.PassthroughProcessID)
			require.Equal(t, info.StartedAt, execution.PassthroughStartedAt)
			require.Equal(t, info.ID, execution.passthroughInitialPromptProcessID)
			require.True(t, controller.Passthrough().Live())
			after, err := os.ReadFile(registry)
			require.NoError(t, err)
			require.Equal(t, before, after)
			content, err := os.ReadFile(file.Path)
			require.NoError(t, err)
			require.JSONEq(t, string(file.Content), string(content))
		})
	}
}
