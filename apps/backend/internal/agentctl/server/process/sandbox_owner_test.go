package process

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/stretchr/testify/require"
)

func TestSandboxSupervisorIdentityBoundAtStart(t *testing.T) {
	launch, receipt := processSandboxFixture(t)
	launch.MCPURL = "http://127.0.0.1:31234/" + strings.Repeat("a", 64) + "/mcp"
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), Sandbox: launch, AgentArgs: []string{"/bin/sh", "-c", "read line"}, AgentEnv: []string{"SANDBOX_RECEIPT=" + receipt}}, newTestLogger(t))
	manager.adapter = newStubAdapter()
	require.NoError(t, manager.buildFinalCommand())
	_, err := manager.startManagedProcess(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = manager.Stop(ctx)
	})
	owner, err := manager.SandboxOwner()
	require.NoError(t, err)
	require.Equal(t, launch.AttemptID, owner.AttemptID)
	require.Equal(t, launch.Generation, owner.Generation)
	require.Equal(t, manager.cmd.Process.Pid, owner.Supervisor.PID)
	require.Equal(t, owner.Supervisor.PID, owner.Supervisor.SessionID)
	receiver := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), Sandbox: launch}, newTestLogger(t))
	altered := *owner
	altered.Supervisor.BirthToken = "different"
	require.Error(t, receiver.SetSandboxOwner(&altered))
	altered = *owner
	altered.Generation++
	require.Error(t, receiver.SetSandboxOwner(&altered))
	require.NoError(t, receiver.SetSandboxOwner(owner))
	retained, retainErr := receiver.SandboxOwner()
	require.NoError(t, retainErr)
	require.Equal(t, owner, retained)

	state, err := processidentity.Inspect(owner.Supervisor)
	require.NoError(t, err)
	require.Equal(t, processidentity.StateAlive, state)
	require.True(t, manager.AdmitSandboxMCP(launch.AttemptID, strconv.FormatUint(launch.Generation, 10), launch.PolicyDigest))
	require.False(t, manager.AdmitSandboxMCP("other", strconv.FormatUint(launch.Generation, 10), launch.PolicyDigest))
	require.False(t, manager.AdmitSandboxMCP(launch.AttemptID, "2", launch.PolicyDigest))
	require.NoError(t, manager.cmd.Process.Kill())
	require.Eventually(t, func() bool { _, e := manager.SandboxOwner(); return e != nil }, time.Second, 10*time.Millisecond)
	require.False(t, manager.AdmitSandboxMCP(launch.AttemptID, strconv.FormatUint(launch.Generation, 10), launch.PolicyDigest))
	require.Error(t, receiver.SetSandboxOwner(owner))
}

func TestSandboxOneShotStartupKeepsFilteredEnvironment(t *testing.T) {
	launch, receipt := processSandboxFixture(t)
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), Sandbox: launch, AgentArgs: []string{"/bin/sh", "-c", "exit 0"}, AgentEnv: []string{"SANDBOX_RECEIPT=" + receipt, "UNAPPROVED_SECRET=private"}}
	cfg.ContinueArgs = cfg.AgentArgs
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newOneShotStubAdapter()
	manager.adapterCfg = &adapter.Config{OneShotConfig: &adapter.OneShotConfig{InitialArgs: cfg.AgentArgs}}
	require.NoError(t, manager.startOneShot())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = manager.Stop(ctx)
	})
	require.Empty(t, envValue(manager.adapterCfg.OneShotConfig.Env, "UNAPPROVED_SECRET"))
	require.Equal(t, launch.Root+"/home", envValue(manager.adapterCfg.OneShotConfig.Env, "HOME"))
}
