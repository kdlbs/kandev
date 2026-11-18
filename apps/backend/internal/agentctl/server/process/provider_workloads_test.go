//go:build linux

package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process/probe"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementWorkloadIdentityExcludesWorkspaceProcesses(t *testing.T) {
	cfg := persistentRestartConfig(t.TempDir())
	cfg.Protocol = agent.ProtocolCodexAppServer
	cfg.SessionID = "same-session"
	cfg.AgentEnv = append(cfg.AgentEnv, "KANDEV_SESSION_ID=same-session")
	mgr := newRestartManager(t, cfg)
	start := time.Now()
	startRestartManager(t, mgr)
	command, env := fixtureShellExec("sleep 60")
	_, err := mgr.StartProcess(context.Background(), StartProcessRequest{SessionID: "same-session", Kind: types.ProcessKindDev, Command: command, Env: env})
	require.NoError(t, err)
	providerEnv := mgr.cmd.Env
	require.Equal(t, mgr.ProcessManagerID(), environmentMapFromSlice(providerEnv)["KANDEV_PROVIDER_WORKLOAD_ID"], "only the provider command must carry its workload identity")
	workspace, err := mgr.CommandEnvironment()
	require.NoError(t, err)
	for key := range workspace {
		require.False(t, strings.HasPrefix(key, "KANDEV_PROVIDER_WORKLOAD"))
	}
	observer, ok := any(mgr).(interface {
		ProviderBackgroundWorkloads(time.Time) (probe.Result, error)
	})
	require.True(t, ok)
	result, err := observer.ProviderBackgroundWorkloads(start)
	require.NoError(t, err)
	require.Equal(t, probe.ResultSettled, result, "same-session workspace work must not block provider replacement")
	orphan := fixtureCmd("sleep 60")
	orphan.Env = append(orphan.Env, "KANDEV_PROVIDER_WORKLOAD_ID="+mgr.ProcessManagerID())
	require.NoError(t, orphan.Start())
	t.Cleanup(func() { _ = orphan.Process.Kill(); _ = orphan.Wait() })
	result, err = observer.ProviderBackgroundWorkloads(start)
	require.NoError(t, err)
	require.Equal(t, probe.ResultLive, result, "a reparented provider workload must still block retirement")
}
