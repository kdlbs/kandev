//go:build unix

package process

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/stretchr/testify/require"
)

func TestNodePolicyRejectsSessionWithoutProfileWrapper(t *testing.T) {
	t.Setenv(launchwrapper.NodePolicyEnv, filepath.Join(t.TempDir(), "missing-node-policy"))
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentArgs: []string{"/bin/echo", "never-start"}, AgentEnv: []string{launchwrapper.NodePolicyEnv + "="}}
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newStubAdapter()
	require.ErrorIs(t, manager.buildFinalCommand(), launchwrapper.ErrNodePolicy)
	require.Nil(t, manager.cmd)
}

func TestNodePolicyRejectsInteractiveWithoutProfile(t *testing.T) {
	t.Setenv(launchwrapper.NodePolicyEnv, filepath.Join(t.TempDir(), "missing-node-policy"))
	runner := NewInteractiveRunner(nil, newTestLogger(t), 1024)
	_, err := runner.Start(context.Background(), InteractiveStartRequest{SessionID: "node-policy", Command: []string{"/bin/echo", "never-start"}, WorkingDir: t.TempDir(), ImmediateStart: true})
	require.ErrorIs(t, err, launchwrapper.ErrNodePolicy)
}

func TestNodePolicyOneShotRebuildUsesUnwrappedArguments(t *testing.T) {
	t.Setenv(launchwrapper.NodePolicyEnv, "/administrator/node-policy.json")
	cfg := &config.InstanceConfig{AgentArgs: []string{"original-agent"}, ContinueArgs: []string{"original-continuation"}}
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapterCfg = &adapter.Config{OneShotConfig: &adapter.OneShotConfig{InitialArgs: []string{"already-wrapped"}, ContinueArgs: []string{"already-wrapped-continuation"}}}
	initial, continuation := manager.oneShotBaseArgs()
	require.Equal(t, cfg.AgentArgs, initial)
	require.Equal(t, cfg.ContinueArgs, continuation)
}
