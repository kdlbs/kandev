package instance

import (
	"context"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestRemotePassthroughInstanceTeardown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SSH targets use Unix PTYs")
	}
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{Ports: config.PortConfig{Base: 0, Max: 0}}, log)
	t.Cleanup(func() { require.NoError(t, mgr.Shutdown(context.Background())) })
	procMgr := process.NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1", InstanceID: "instance-1"}, log)
	t.Cleanup(func() { require.NoError(t, procMgr.StopForTeardown(context.Background())) })
	snapshot, err := procMgr.Passthrough().Start(context.Background(), types.PassthroughStartRequest{
		SessionID: "session-1", OperationID: "start-1", Command: []string{"/bin/sh", "-c", "cat"},
	})
	require.NoError(t, err)
	child, err := os.FindProcess(snapshot.OSPID)
	require.NoError(t, err)
	require.NoError(t, child.Signal(syscall.Signal(0)))
	mgr.instances["instance-1"] = &Instance{ID: "instance-1", Status: "running", manager: procMgr}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, mgr.StopInstance(ctx, "instance-1"))
	require.Error(t, child.Signal(syscall.Signal(0)), "instance teardown left the remote CLI alive")
}
