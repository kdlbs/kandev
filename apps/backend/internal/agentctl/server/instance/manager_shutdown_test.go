package instance

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	closed      bool
}

func (s *fakeHTTPServer) Shutdown(context.Context) error {
	return s.shutdownErr
}

func (s *fakeHTTPServer) Close() error {
	s.closed = true
	return s.closeErr
}
