package process

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-MCP-APPLY-001.6
func TestMCPProviderReplacementPreservesWorkspaceProtocolProcess(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	child, err := mgr.StartPipedProcess(PipedStartRequest{SessionID: "workspace-protocol", Kind: types.ProcessKindCustom, Command: fixtureArgs()[0], Env: environmentMapFromSlice(fixtureEnvSlice("cat"))})
	require.NoError(t, err)
	defer func() { _ = child.Stdin.Close(); _ = child.Stdout.Close() }()
	before := child.process.cmd.Process.Pid
	source, err := mgr.stopProviderForReplacement(context.Background(), mgr.ProcessGeneration())
	require.NoError(t, err)
	_, err = mgr.startProviderForReplacement(context.Background(), source)
	require.NoError(t, err)
	require.Equal(t, before, child.process.cmd.Process.Pid)
	select {
	case <-child.Exited:
		t.Fatal("workspace protocol process exited during provider replacement")
	default:
	}
	want := []byte("protocol connection survived\n")
	_, err = child.Stdin.Write(want)
	require.NoError(t, err)
	result := make(chan error, 1)
	got := make([]byte, len(want))
	go func() { _, err := io.ReadFull(child.Stdout, got); result <- err }()
	select {
	case err := <-result:
		require.NoError(t, err)
		require.Equal(t, want, got)
	case <-time.After(10 * time.Second):
		t.Fatal("existing workspace protocol connection did not respond")
	}
}
