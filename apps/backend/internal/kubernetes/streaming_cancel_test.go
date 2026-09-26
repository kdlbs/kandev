package kubernetes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestProbePortForwardUpgradeCancelsBlockedWebSocketHandshake(t *testing.T) {
	testProbePortForwardUpgradeCancelsBlockedHandshake(t, http.MethodGet)
}

func TestProbePortForwardUpgradeCancelsBlockedSPDYFallbackHandshake(t *testing.T) {
	testProbePortForwardUpgradeCancelsBlockedHandshake(t, http.MethodPost)
}

func testProbePortForwardUpgradeCancelsBlockedHandshake(t *testing.T, blockedMethod string) {
	t.Helper()
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var startOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRequest) }) }
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if blockedMethod == http.MethodPost && request.Method == http.MethodGet {
			http.Error(response, "WebSocket upgrade refused", http.StatusBadRequest)
			return
		}
		if request.Method != blockedMethod {
			http.Error(response, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		startOnce.Do(func() { close(requestStarted) })
		select {
		case <-request.Context().Done():
		case <-releaseRequest:
		}
	}))
	t.Cleanup(func() {
		release()
		server.Close()
	})
	config := &rest.Config{Host: server.URL}
	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	client := &agentkubernetes.Client{RESTConfig: config, Clientset: clientset}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() {
		result <- probePortForwardUpgrade(ctx, client, "kandev", "probe-pod")
	}()

	select {
	case <-requestStarted:
	case err := <-result:
		release()
		require.FailNow(t, "port-forward probe returned before blocked handshake", "error: %v", err)
	case <-time.After(time.Second):
		release()
		select {
		case <-result:
		case <-time.After(time.Second):
		}
		require.FailNow(t, "blocked port-forward handshake did not start")
	}
	cancel()

	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(300 * time.Millisecond):
		release()
		err := <-result
		require.FailNow(t, "blocked port-forward handshake ignored cancellation", "eventual error: %v", err)
	}
}
