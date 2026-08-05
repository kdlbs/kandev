//go:build !windows

package process

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestProcessRunnerStartPipedTeardownReapsDescendants(t *testing.T) {
	runner := NewProcessRunner(nil, newTestLogger(t), 2*1024*1024)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	command, env := fixtureExec("sleep-with-child " + pidFile + " 30")
	proc, err := runner.StartPiped(PipedStartRequest{
		SessionID:  "session-1",
		Kind:       types.ProcessKindCustom,
		ScriptName: "test-lsp-tree",
		Command:    command[0],
		Args:       command[1:],
		Env:        env,
	})
	require.NoError(t, err)
	childPID := waitForChildPID(t, pidFile, 5*time.Second)

	runner.BeginStop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, runner.StopAllAndWait(ctx))

	select {
	case <-proc.Done:
	case <-ctx.Done():
		t.Fatal("piped process was not reaped before timeout")
	}
	require.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond, "child process %d should be reaped", childPID)
}
