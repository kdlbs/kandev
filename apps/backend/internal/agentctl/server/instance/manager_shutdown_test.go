package instance

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceInfoSynchronizesConcurrentStatusChanges(t *testing.T) {
	inst := &Instance{ID: "instance-race", Status: "running"}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1_000 {
			inst.setStatus("stopping")
			inst.setStatus("running")
		}
	}()
	go func() {
		defer wg.Done()
		for range 1_000 {
			status := inst.Info().Status
			if status != "running" && status != "stopping" {
				t.Errorf("Info().Status = %q", status)
				return
			}
		}
	}()
	wg.Wait()
}

func TestStopInstanceBoundsHTTPServerShutdown(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() {
		_ = mgr.Shutdown(context.Background())
	})

	requestStarted := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		}
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: handler}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	inst := &Instance{
		ID:        "slow-http-shutdown",
		Port:      listener.Addr().(*net.TCPAddr).Port,
		Status:    "running",
		CreatedAt: time.Now(),
		server:    server,
	}
	mgr.mu.Lock()
	mgr.instances[inst.ID] = inst
	mgr.mu.Unlock()

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		resp, err := http.Get("http://" + listener.Addr().String() + "/probe")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("test HTTP request did not reach server")
	}

	start := time.Now()
	require.NoError(t, mgr.StopInstance(context.Background(), inst.ID))
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 700*time.Millisecond,
		"StopInstance should not wait for long-running probe handlers during shutdown")

	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("test HTTP client did not finish after instance shutdown")
	}

	select {
	case err := <-serveErr:
		require.ErrorIs(t, err, http.ErrServerClosed)
	case <-time.After(time.Second):
		t.Fatal("test HTTP server did not stop")
	}
}

func TestStopHTTPServerReturnsCloseError(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)

	closeErr := errors.New("listener close failed")
	server := &fakeHTTPServer{
		shutdownErr: context.DeadlineExceeded,
		closeErr:    closeErr,
	}

	err := mgr.stopHTTPServer(context.Background(), "close-failure", 12345, server)
	require.ErrorIs(t, err, closeErr)
	require.True(t, server.closed)
}

func TestStopInstanceRetainsPortWhenHTTPServerCloseFails(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 12345, Max: 12345},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)

	port, err := mgr.portAlloc.Allocate("close-failure")
	require.NoError(t, err)

	closeErr := errors.New("listener close failed")
	inst := &Instance{
		ID:        "close-failure",
		Port:      port,
		Status:    "running",
		CreatedAt: time.Now(),
		server: &fakeHTTPServer{
			shutdownErr: context.DeadlineExceeded,
			closeErr:    closeErr,
		},
		manager: &fakeProcessManager{stopErr: errors.New("process teardown failed")},
	}

	mgr.mu.Lock()
	mgr.instances[inst.ID] = inst
	mgr.mu.Unlock()

	err = mgr.StopInstance(context.Background(), inst.ID)
	require.ErrorIs(t, err, closeErr)

	_, err = mgr.portAlloc.Allocate("next-instance")
	require.Error(t, err)
}

func TestStopInstanceRetainsPortAfterProcessTeardownFailure(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 12345, Max: 12345},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)

	port, err := mgr.portAlloc.Allocate("process-cleanup-failure")
	require.NoError(t, err)
	processErr := errors.New("process teardown failed")
	procMgr := &fakeProcessManager{stopErr: legacyResourceReleaseError{err: processErr}}
	server := &fakeHTTPServer{}
	inst := &Instance{
		ID:        "process-cleanup-failure",
		Port:      port,
		Status:    "running",
		CreatedAt: time.Now(),
		manager:   procMgr,
		server:    server,
	}
	mgr.mu.Lock()
	mgr.instances[inst.ID] = inst
	mgr.mu.Unlock()

	err = mgr.StopInstance(context.Background(), inst.ID)
	require.ErrorIs(t, err, processErr)
	require.True(t, procMgr.stopped)
	require.True(t, server.shutdown)
	_, ok := mgr.GetInstance(inst.ID)
	require.True(t, ok, "process teardown failure must retain the instance for retry")
	_, err = mgr.portAlloc.Allocate("next-instance")
	require.Error(t, err, "process teardown failure must retain the instance port")

	procMgr.stopErr = nil
	require.NoError(t, mgr.StopInstance(context.Background(), inst.ID))
	_, ok = mgr.GetInstance(inst.ID)
	require.False(t, ok)
	reusedPort, err := mgr.portAlloc.Allocate("third-instance")
	require.NoError(t, err)
	require.Equal(t, port, reusedPort)
}

func TestStopHTTPServerTreatsCanceledShutdownAsStoppedAfterClose(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)

	server := &fakeHTTPServer{shutdownErr: context.Canceled}

	err := mgr.stopHTTPServer(context.Background(), "canceled-shutdown", 12345, server)
	require.NoError(t, err)
	require.True(t, server.closed)
}

type fakeHTTPServer struct {
	shutdownErr error
	closeErr    error
	shutdown    bool
	closed      bool
}

func (s *fakeHTTPServer) Shutdown(context.Context) error {
	s.shutdown = true
	return s.shutdownErr
}

func (s *fakeHTTPServer) Close() error {
	s.closed = true
	return s.closeErr
}

type fakeProcessManager struct {
	stopErr error
	stopped bool
}

// legacyResourceReleaseError proves a process teardown error cannot authorize
// releasing the instance's port while its process ownership is unresolved.
type legacyResourceReleaseError struct {
	err error
}

func (e legacyResourceReleaseError) Error() string { return e.err.Error() }
func (e legacyResourceReleaseError) Unwrap() error { return e.err }
func (e legacyResourceReleaseError) CanReleaseInstanceResources() bool {
	return true
}

func (m *fakeProcessManager) CloseAdmission() {}

func (m *fakeProcessManager) StopForTeardown(context.Context) error {
	m.stopped = true
	return m.stopErr
}
