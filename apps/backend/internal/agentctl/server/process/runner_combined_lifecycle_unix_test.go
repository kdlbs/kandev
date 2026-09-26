//go:build !windows

package process

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	tools "github.com/kandev/kandev/internal/tools/installer"
	"github.com/stretchr/testify/require"
)

func TestManagerCombinedOutputTeardownReapsDescendants(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1"}, newTestLogger(t))
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	command, env := fixtureExec("sleep-with-child " + pidFile + " 30")

	runDone := make(chan error, 1)
	go func() {
		_, err := mgr.CombinedOutput(context.Background(), tools.CommandSpec{
			Path: command[0],
			Args: command[1:],
			Env:  env,
		})
		runDone <- err
	}()
	childPID := waitForChildPID(t, pidFile, 5*time.Second)

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, mgr.StopForTeardown(stopCtx))
	if err := <-runDone; err == nil {
		t.Fatal("CombinedOutput() error = nil after teardown killed the command")
	}
	require.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond, "child process %d should be reaped", childPID)
}
