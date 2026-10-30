package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/logger"
)

func passthroughStart(t *testing.T, runner *process.InteractiveRunner) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	require.NoError(t, os.WriteFile(child, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	_, err := runner.Start(context.Background(), process.InteractiveStartRequest{
		SessionID: "s", Command: []string{child}, WorkingDir: dir, ImmediateStart: true,
	})
	return err
}

// @covers AC-AGENTS-NODEWRAP-005.2 AC-AGENTS-NODEWRAP-005.3
func TestBackendPassthroughRunnerFailsClosedWhenPeerHasNodeWrapper(t *testing.T) {
	log := logger.Default()

	refused := process.NewInteractiveRunner(nil, log, 1024)
	configurePassthroughRunner(refused, []string{"agent-survival.v1", launchwrapper.NodeCapability})
	require.ErrorIs(t, passthroughStart(t, refused), launchwrapper.ErrNodeRequired)

	for name, capabilities := range map[string][]string{"no peer": nil, "peer without wrapper": {"agent-survival.v1"}} {
		allowed := process.NewInteractiveRunner(nil, log, 1024)
		configurePassthroughRunner(allowed, capabilities)
		require.NoError(t, passthroughStart(t, allowed), name)
	}
}
