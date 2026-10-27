package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestSandboxPersistentFailedSpawnHasNonReplayableReceipt(t *testing.T) {
	launch, wrapperReceipt := processSandboxFixture(t)
	work := t.TempDir()
	marker := filepath.Join(t.TempDir(), "started")
	cfg := &config.InstanceConfig{Protocol: agent.ProtocolACP, WorkDir: work, Sandbox: launch, AgentArgs: []string{filepath.Join(work, "missing-agent"), "-c", "printf started > \"$1\"; read line", "fixture", marker}, AgentEnv: []string{"SANDBOX_RECEIPT=" + wrapperReceipt}}
	manager := NewManager(cfg, newTestLogger(t))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = manager.Stop(ctx)
	})
	_, err := manager.StartWithGeneration(t.Context())
	require.Error(t, err)
	receipt, err := manager.SandboxStartReceipt(launch.AttemptID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, sandbox.StartNotStarted, receipt.Outcome)
	require.NoError(t, receipt.ValidateStopped(launch))
	require.Nil(t, receipt.Owner)
	cfg.AgentArgs[0] = "/bin/sh"
	_, err = manager.StartWithGeneration(t.Context())
	require.Error(t, err, "the same attempt cannot start after its no-spawn receipt")
	require.NoFileExists(t, marker)
	next, nextWrapperReceipt := processSandboxFixture(t)
	next.AttemptID = "next-attempt"
	require.NoError(t, manager.ConfigureExecution(ExecutionConfiguration{ArgsPresent: true, Args: cfg.AgentArgs, Sandbox: next, Environment: map[string]string{"SANDBOX_RECEIPT": nextWrapperReceipt}, ReplaceEnvironment: true}))
	_, err = manager.StartWithGeneration(t.Context())
	require.NoError(t, err, "a distinct attempt may start")
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, time.Second, 10*time.Millisecond)
	_, err = manager.SandboxStartReceipt(next.AttemptID)
	require.Error(t, err, "a running attempt has no stopped receipt")
}
