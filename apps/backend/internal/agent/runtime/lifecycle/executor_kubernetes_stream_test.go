package lifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	kubernetesclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestKubernetesStreamingConfigClearsRequestTimeoutWithoutMutatingAPIConfig(t *testing.T) {
	apiConfig := &rest.Config{Host: "https://cluster.example", Timeout: 30 * time.Second}

	streamingConfig := kubernetesStreamingRESTConfig(apiConfig)

	require.NotSame(t, apiConfig, streamingConfig)
	require.Equal(t, 30*time.Second, apiConfig.Timeout, "ordinary API calls retain their bounded timeout")
	require.Zero(t, streamingConfig.Timeout, "long-lived streams must not inherit a whole-response deadline")
}

func TestKubernetesPortForwardCancellationClosesBlockedWebSocketHandshake(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		select {
		case <-request.Context().Done():
			close(requestCanceled)
		case <-releaseHandler:
		}
	}))
	t.Cleanup(func() {
		close(releaseHandler)
		server.Close()
	})
	config := &rest.Config{Host: server.URL}
	clientset, err := kubernetesclient.NewForConfig(config)
	require.NoError(t, err)
	forwarder := &liveKubernetesForwarder{
		config: config, handshakeTimeout: 50 * time.Millisecond,
		resources: &liveKubernetesResources{client: &kubeexecutor.Client{
			RESTConfig: config,
			Clientset:  clientset,
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	session, err := forwarder.Forward(ctx, kubeexecutor.PortForwardRequest{
		Namespace: "kandev-agents", Pod: "agent-pod", LocalAddress: "127.0.0.1", RemotePort: 8765,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("WebSocket handshake did not reach test server")
	}
	require.NoError(t, session.Close())
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("canceled port-forward startup left WebSocket handshake running")
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("Close returned before the blocked port-forward startup goroutine joined")
	}
	cancel()
}

func TestKubernetesPortForwardFallsBackToSPDYAfterWebSocketHandshakeTimeout(t *testing.T) {
	webSocketStarted := make(chan struct{})
	spdyStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			close(webSocketStarted)
			<-request.Context().Done()
		case http.MethodPost:
			close(spdyStarted)
			http.Error(response, "test SPDY rejection", http.StatusBadRequest)
		default:
			http.Error(response, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	config := &rest.Config{Host: server.URL}
	clientset, err := kubernetesclient.NewForConfig(config)
	require.NoError(t, err)
	forwarder := &liveKubernetesForwarder{
		config: config, handshakeTimeout: 50 * time.Millisecond,
		resources: &liveKubernetesResources{client: &kubeexecutor.Client{
			RESTConfig: config,
			Clientset:  clientset,
		}},
	}
	session, err := forwarder.Forward(context.Background(), kubeexecutor.PortForwardRequest{
		Namespace: "kandev-agents", Pod: "agent-pod", LocalAddress: "127.0.0.1", RemotePort: 8765,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	select {
	case <-webSocketStarted:
	case <-time.After(time.Second):
		t.Fatal("WebSocket primary did not reach test server")
	}
	select {
	case <-spdyStarted:
	case <-time.After(time.Second):
		t.Fatal("timed-out WebSocket primary did not fall back to SPDY")
	}
}

func TestKubernetesHealthWaitUsesCallerContextBeyondDockerRetryCap(t *testing.T) {
	executor := NewKubernetesExecutor(nil, newTestLogger())
	executor.healthRetryDelay = time.Nanosecond
	checker := &stubHealthChecker{
		becomesHealthyAfter: agentctlHealthMaxRetries + 1,
		failErr:             context.DeadlineExceeded,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := executor.waitForKubernetesAgentctlHealth(ctx, checker)

	require.NoError(t, err)
	require.EqualValues(t, agentctlHealthMaxRetries+1, checker.calls)
}

func TestKubernetesCreateRebuildsControlForwardThatDiesBeforeRemoteHealth(t *testing.T) {
	firstHealth := make(chan struct{})
	unhealthy := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		select {
		case <-firstHealth:
		default:
			close(firstHealth)
		}
		http.Error(response, "agentctl not listening remotely", http.StatusServiceUnavailable)
	}))
	t.Cleanup(unhealthy.Close)
	_, unhealthyPort := testServerHostPort(t, unhealthy)
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	forwarder := &recoveringControlKubernetesForwarder{
		firstHealth:  firstHealth,
		controlPorts: []uint16{unhealthyPort, controlPort},
		instancePort: instancePort,
	}
	executor := NewKubernetesExecutor(nil, newTestLogger())
	executor.healthRetryDelay = 40 * time.Millisecond
	executor.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		return &kubernetesRuntimeClient{
			resources: &fakeKubernetesResources{},
			streams: kubeexecutor.NewStreamOperations(
				&recordingKubernetesExec{}, forwarder,
			),
		}, nil
	}
	executor.resolveBinary = func(kubeexecutor.Platform) ([]byte, error) { return []byte("agentctl"), nil }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	instance, err := executor.CreateInstance(ctx, validKubernetesCreateRequest())

	require.NoError(t, err)
	require.NotNil(t, instance.Client)
	require.Equal(t, []uint16{
		uint16(kubeexecutor.DefaultAgentctlPort),
		uint16(kubeexecutor.DefaultAgentctlPort),
		41001,
	}, forwarder.remotePorts())
	require.True(t, forwarder.firstSession().isClosed())
	createdAt := forwarder.createdAtTimes()
	require.Len(t, createdAt, 3)
	require.GreaterOrEqual(t, createdAt[1].Sub(createdAt[0]), executor.healthRetryDelay,
		"dead control-forward recreation must not hot-loop against the API server")
}

func TestKubernetesRetainedForwardOutlivesCreateContext(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	forwarder := &contextBoundKubernetesForwarder{recordingKubernetesForwarder: recordingKubernetesForwarder{
		localPorts: map[uint16]uint16{
			uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
			41001:                                    instancePort,
		},
	}}
	executor := NewKubernetesExecutor(nil, newTestLogger())
	executor.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		return &kubernetesRuntimeClient{
			resources: &fakeKubernetesResources{},
			streams: kubeexecutor.NewStreamOperations(
				&recordingKubernetesExec{}, forwarder,
			),
		}, nil
	}
	executor.resolveBinary = func(kubeexecutor.Platform) ([]byte, error) { return []byte("agentctl"), nil }
	ctx, cancel := context.WithCancel(context.Background())
	instance, err := executor.CreateInstance(ctx, validKubernetesCreateRequest())
	require.NoError(t, err)
	require.NotNil(t, instance.Client)
	retained := forwarder.lastSession()

	cancel()
	time.Sleep(20 * time.Millisecond)
	require.False(t, retained.isClosed(), "launch timeout cancellation must not tear down the retained forward")

	require.NoError(t, executor.Close())
	require.True(t, retained.isClosed(), "backend Close owns retained-forward shutdown")
}

type contextBoundKubernetesForwarder struct {
	recordingKubernetesForwarder
}

type recoveringControlKubernetesForwarder struct {
	mu           sync.Mutex
	firstHealth  <-chan struct{}
	controlPorts []uint16
	instancePort uint16
	requests     []kubeexecutor.PortForwardRequest
	sessions     []*fakeKubernetesForwardSession
	createdAt    []time.Time
}

func (f *recoveringControlKubernetesForwarder) Forward(
	_ context.Context,
	request kubeexecutor.PortForwardRequest,
) (kubeexecutor.PortForwardSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request)
	f.createdAt = append(f.createdAt, time.Now())
	localPort := f.instancePort
	controlIndex := 0
	if request.RemotePort == uint16(kubeexecutor.DefaultAgentctlPort) {
		controlIndex = len(f.sessions)
		if controlIndex >= len(f.controlPorts) {
			controlIndex = len(f.controlPorts) - 1
		}
		localPort = f.controlPorts[controlIndex]
	}
	session := newFakeKubernetesForwardSession(localPort)
	f.sessions = append(f.sessions, session)
	if len(f.sessions) == 1 {
		go func() {
			<-f.firstHealth
			session.done <- errors.New("remote port refused forwarded stream")
		}()
	}
	return session, nil
}

func (f *recoveringControlKubernetesForwarder) createdAtTimes() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.createdAt...)
}

func (f *recoveringControlKubernetesForwarder) remotePorts() []uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	ports := make([]uint16, 0, len(f.requests))
	for _, request := range f.requests {
		ports = append(ports, request.RemotePort)
	}
	return ports
}

func (f *recoveringControlKubernetesForwarder) firstSession() *fakeKubernetesForwardSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[0]
}

func testServerHostPort(t *testing.T, server *httptest.Server) (string, uint16) {
	t.Helper()
	host, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	return host, uint16(port)
}

func (f *contextBoundKubernetesForwarder) Forward(
	ctx context.Context,
	request kubeexecutor.PortForwardRequest,
) (kubeexecutor.PortForwardSession, error) {
	session, err := f.recordingKubernetesForwarder.Forward(ctx, request)
	if err != nil {
		return nil, err
	}
	if done := ctx.Done(); done != nil {
		go func() {
			<-done
			_ = session.Close()
		}()
	}
	return session, nil
}
