package instance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

// A shutdown that stops some process groups but not all must report the
// failure, keep the unstopped instance for retry, and succeed once it stops.
func TestShutdownReportsAPartialStopAndRetainsTheUnstoppedInstance(t *testing.T) {
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 12360, Max: 12361},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, newTestLogger(t))
	stopped := addFakeInstance(t, mgr, "stops", nil)
	stuckErr := errors.New("process group still alive")
	stuck := addFakeInstance(t, mgr, "stuck", stuckErr)

	err := mgr.Shutdown(context.Background())
	require.ErrorIs(t, err, stuckErr, "a partial stop must not report a clean shutdown")
	require.True(t, stopped.stopped)
	_, retained := mgr.GetInstance("stuck")
	require.True(t, retained, "the instance whose process group is alive must stay registered")
	_, gone := mgr.GetInstance("stops")
	require.False(t, gone)

	stuck.stopErr = nil
	require.NoError(t, mgr.Shutdown(context.Background()))
	_, retained = mgr.GetInstance("stuck")
	require.False(t, retained)
}

func addFakeInstance(t *testing.T, mgr *Manager, id string, stopErr error) *fakeProcessManager {
	t.Helper()
	lease, err := mgr.portAlloc.Allocate(id)
	require.NoError(t, err)
	procMgr := &fakeProcessManager{stopErr: stopErr}
	mgr.mu.Lock()
	mgr.instances[id] = &Instance{
		ID: id, Port: lease.Port, lease: lease, Status: "running", CreatedAt: time.Now(),
		manager: procMgr, server: &fakeHTTPServer{},
	}
	mgr.mu.Unlock()
	return procMgr
}
