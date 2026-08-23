package backendapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"go.uber.org/goleak"
)

// getJSON dials host:port+path over HTTP and decodes a JSON object body.
// Keep-alives are disabled so no idle client connection lingers for goleak.
func getJSON(host string, port int, path string) (int, map[string]any, error) {
	return requestJSON(host, port, http.MethodGet, path)
}

func requestJSON(host string, port int, method string, path string) (int, map[string]any, error) {
	url := fmt.Sprintf("http://%s%s", net.JoinHostPort(host, fmt.Sprint(port)), path)
	client := &http.Client{
		Timeout:   500 * time.Millisecond,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	if decodeErr := json.NewDecoder(resp.Body).Decode(&body); decodeErr != nil {
		return resp.StatusCode, nil, decodeErr
	}
	return resp.StatusCode, body, nil
}

// TestBindBootstrapListenersServesLivenessBeforeSwap is the regression test
// for docs/plans/startup-listener-before-recovery/task-01-bind-before-
// startup.md: a caller must be able to bind the socket and get a 200 on the
// launcher's liveness path (GET /health) before the real router exists.
//
// Expected pre-fix failure: bindBootstrapListeners does not exist yet, so
// this fails to compile.
func TestBindBootstrapListenersServesLivenessBeforeSwap(t *testing.T) {
	defer goleak.VerifyNone(t)

	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	log := testLogger(t)

	handler, server, listeners, err := bindBootstrapListeners(cfg, log, "1.2.3-test")
	if err != nil {
		t.Fatalf("bindBootstrapListeners: %v", err)
	}
	defer shutdown(t, listeners, server)

	status, body, err := getJSON("127.0.0.1", cfg.Server.Port, "/health")
	if err != nil {
		t.Fatalf("GET /health before swap: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("bootstrap /health status = %d, want %d (launcher liveness probe requires 200)", status, http.StatusOK)
	}
	if got := body[versionFieldKey]; got != "1.2.3-test" {
		t.Fatalf("bootstrap /health version = %v, want 1.2.3-test", got)
	}
	if got := body[statusKey]; got != "ok" {
		t.Fatalf("bootstrap /health status field = %v, want %q (must match healthHandler's contract)", got, "ok")
	}
	if got := body["mode"]; got != "websocket+http" {
		t.Fatalf("bootstrap /health mode = %v, want %q (must match healthHandler's contract)", got, "websocket+http")
	}

	status, body, err = requestJSON("127.0.0.1", cfg.Server.Port, http.MethodPost, "/health")
	if err != nil {
		t.Fatalf("POST /health before swap: %v", err)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("bootstrap POST /health status = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if body[statusKey] != startingStatus {
		t.Fatalf("bootstrap POST /health body = %v, want status=%q", body, startingStatus)
	}

	// Every other path must answer deterministically, not hang or 404.
	status, body, err = getJSON("127.0.0.1", cfg.Server.Port, "/api/v1/whatever")
	if err != nil {
		t.Fatalf("GET non-health path before swap: %v", err)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("bootstrap non-health status = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if body["status"] != startingStatus {
		t.Fatalf("bootstrap non-health body = %v, want status=%q", body, startingStatus)
	}

	if handler == nil {
		t.Fatal("bindBootstrapListeners returned a nil handler switch")
	}
}

// TestBindBootstrapListenersEchoesDesktopHealthToken guards against the
// bootstrap handler poisoning the launcher's health probe: probeHealth
// (internal/launcher/health.go) treats a 200 with a missing/mismatched
// X-Kandev-Desktop-Health-Token as a foreign process and permanently sticks
// that verdict for the whole wait window.
func TestBindBootstrapListenersEchoesDesktopHealthToken(t *testing.T) {
	defer goleak.VerifyNone(t)
	t.Setenv(desktopHealthTokenEnv, "bootstrap-token")

	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	log := testLogger(t)

	_, server, listeners, err := bindBootstrapListeners(cfg, log, "1.2.3-test")
	if err != nil {
		t.Fatalf("bindBootstrapListeners: %v", err)
	}
	defer shutdown(t, listeners, server)

	url := fmt.Sprintf("http://%s/health", net.JoinHostPort("127.0.0.1", fmt.Sprint(cfg.Server.Port)))
	client := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{DisableKeepAlives: true}}
	defer client.CloseIdleConnections()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get(desktopHealthTokenHeader); got != "bootstrap-token" {
		t.Fatalf("bootstrap /health token header = %q, want bootstrap-token", got)
	}
}

// TestHandlerSwitchSwapsWithoutRebind is the regression test for the "no
// window where the socket is closed and reopened" acceptance criterion: the
// same listeners must serve the bootstrap handler and then the real handler
// with no gap and no second bind.
func TestHandlerSwitchSwapsWithoutRebind(t *testing.T) {
	defer goleak.VerifyNone(t)

	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	log := testLogger(t)

	handler, server, listeners, err := bindBootstrapListeners(cfg, log, "1.2.3-test")
	if err != nil {
		t.Fatalf("bindBootstrapListeners: %v", err)
	}
	defer shutdown(t, listeners, server)

	boundBefore := listeners.boundAddrs()

	status, _, err := getJSON("127.0.0.1", cfg.Server.Port, "/api/v1/whatever")
	if err != nil {
		t.Fatalf("GET before swap: %v", err)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status before swap = %d, want %d", status, http.StatusServiceUnavailable)
	}

	handler.Store(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	status, err = getStatus("127.0.0.1", cfg.Server.Port)
	if err != nil {
		t.Fatalf("GET after swap: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status after swap = %d, want %d", status, http.StatusOK)
	}

	if got := listeners.boundAddrs(); len(got) != len(boundBefore) || got[0] != boundBefore[0] {
		t.Fatalf("bound addresses changed across swap: before=%v after=%v (want no rebind)", boundBefore, got)
	}
}

// TestBindBootstrapListenersFailsWhenPortUnavailable pins today's bind-
// failure behaviour: bindBootstrapListeners returns errServerBindFailed
// (already logged by startHTTPServers) rather than a generic error, so the
// caller can skip the misleading "Failed to start orchestrator" log.
func TestBindBootstrapListenersFailsWhenPortUnavailable(t *testing.T) {
	defer goleak.VerifyNone(t)

	blocker := listenOnFreePort(t)
	t.Cleanup(func() { _ = blocker.Close() })
	port := listenerPort(t, blocker)

	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = port
	log := testLogger(t)

	handler, server, listeners, err := bindBootstrapListeners(cfg, log, "1.2.3-test")
	if !errors.Is(err, errServerBindFailed) {
		t.Fatalf("bindBootstrapListeners error = %v, want %v", err, errServerBindFailed)
	}
	if handler != nil || server != nil || listeners != nil {
		t.Fatalf("bindBootstrapListeners returned non-nil results on bind failure: handler=%v server=%v listeners=%v",
			handler, server, listeners)
	}
}

// TestCloseBoundListenersReleasesPortOnStartupFailure is the regression test
// for the listener-leak flagged in Build round 3 review: a step that fails
// after bindBootstrapListeners already bound the socket (orchestrator start,
// office services, storage composition, buildHTTPServer) must release the
// port before returning, not just stop the background bind-retry loop.
func TestCloseBoundListenersReleasesPortOnStartupFailure(t *testing.T) {
	defer goleak.VerifyNone(t)

	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	log := testLogger(t)

	_, server, listeners, err := bindBootstrapListeners(cfg, log, "1.2.3-test")
	if err != nil {
		t.Fatalf("bindBootstrapListeners: %v", err)
	}

	closeBoundListeners(server, listeners, log)

	// The port must become free: a later startup failure that leaks the
	// listener would leave a same-process retry (or even a fresh process
	// racing a restart) unable to bind here. server.Close() closing the
	// listener's fd and the OS releasing the address aren't guaranteed to be
	// observable in the same instant, so poll briefly rather than asserting
	// on the very next syscall.
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(cfg.Server.Port)))
		if err == nil {
			if closeErr := ln.Close(); closeErr != nil {
				t.Fatalf("close probe listener: %v", closeErr)
			}
			return
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("port still held after closeBoundListeners: %v", lastErr)
}

// TestCloseBoundListenersNoopWhenListenersNil covers the pure-bind-failure
// case: bindBootstrapListeners returns a nil listeners/server pair when the
// bind itself failed, so closeBoundListeners must not dereference server.
func TestCloseBoundListenersNoopWhenListenersNil(t *testing.T) {
	closeBoundListeners(nil, nil, testLogger(t))
}
