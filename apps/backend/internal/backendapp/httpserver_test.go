package backendapp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/goleak"
	"go.uber.org/zap"
)

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log
}

// freePort grabs a currently-free TCP port on loopback and releases it. There
// is an inherent race between release and re-bind, but it is acceptable for
// tests and keeps the shared-port multi-listener scenarios simple.
func freePort(t *testing.T) int {
	t.Helper()
	ln := listenOnFreePort(t)
	port := listenerPort(t, ln)
	if err := ln.Close(); err != nil {
		t.Fatalf("close temp listener: %v", err)
	}
	return port
}

func noContentServer() *http.Server {
	return &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
}

// getStatus dials host:port over HTTP and returns the status code. Keep-alives
// are disabled so no idle client connection lingers for goleak to flag.
func getStatus(host string, port int) (int, error) {
	url := fmt.Sprintf("http://%s/", net.JoinHostPort(host, fmt.Sprint(port)))
	client := &http.Client{
		Timeout:   500 * time.Millisecond,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	defer client.CloseIdleConnections()
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

// waitForStatus polls host:port until it returns wantStatus or the deadline
// expires.
func waitForStatus(t *testing.T, host string, port, wantStatus int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if got, err := getStatus(host, port); err == nil && got == wantStatus {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("address %s:%d never returned %d within %s", host, port, wantStatus, within)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// shutdown stops the retry loop and the shared server, then verifies no
// goroutines leaked. Registered first via t.Cleanup so it runs last.
func shutdown(t *testing.T, listeners *serverListeners, server *http.Server) {
	t.Helper()
	if listeners != nil {
		listeners.Stop()
	}
	if server != nil {
		if err := server.Shutdown(context.Background()); err != nil {
			t.Errorf("server.Shutdown: %v", err)
		}
	}
	goleak.VerifyNone(t)
}

func TestStartHTTPServersMultipleLoopbackAddresses(t *testing.T) {
	log := testLogger(t)
	port := freePort(t)
	server := noContentServer()

	listeners, ok := startHTTPServers(server, []string{"127.0.0.1", "127.0.0.2"}, port, log)
	if !ok {
		t.Fatal("startHTTPServers returned not-ok for two bindable loopback addresses")
	}
	defer shutdown(t, listeners, server)

	for _, host := range []string{"127.0.0.1", "127.0.0.2"} {
		got, err := getStatus(host, port)
		if err != nil {
			t.Fatalf("GET %s:%d: %v", host, port, err)
		}
		if got != http.StatusNoContent {
			t.Fatalf("GET %s:%d status = %d, want %d", host, port, got, http.StatusNoContent)
		}
	}
}

func TestStartHTTPServersAllFailIsFatal(t *testing.T) {
	defer goleak.VerifyNone(t)
	log := testLogger(t)

	// Occupy the port on 127.0.0.1 so the only requested bind fails.
	blocker := listenOnFreePort(t)
	t.Cleanup(func() { _ = blocker.Close() })
	port := listenerPort(t, blocker)

	server := noContentServer()
	listeners, ok := startHTTPServers(server, []string{"127.0.0.1"}, port, log)
	if ok {
		listeners.Stop()
		_ = server.Shutdown(context.Background())
		t.Fatal("startHTTPServers returned ok when every bind failed; want fatal")
	}
	if listeners != nil {
		t.Fatalf("startHTTPServers returned non-nil manager on all-fail: %v", listeners)
	}
}

func TestStartHTTPServersPartialFailSelfHeals(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Shorten the retry cadence for the test and restore it after.
	prev := serverBindRetryInterval
	serverBindRetryInterval = 15 * time.Millisecond
	t.Cleanup(func() { serverBindRetryInterval = prev })

	log := testLogger(t)
	port := freePort(t)

	// Block 127.0.0.2:port so its initial bind fails; 127.0.0.1 succeeds.
	blocker, err := net.Listen("tcp", net.JoinHostPort("127.0.0.2", fmt.Sprint(port)))
	if err != nil {
		t.Fatalf("occupy 127.0.0.2:%d: %v", port, err)
	}
	t.Cleanup(func() { _ = blocker.Close() })

	server := noContentServer()
	listeners, ok := startHTTPServers(server, []string{"127.0.0.1", "127.0.0.2"}, port, log)
	if !ok {
		_ = blocker.Close()
		t.Fatal("startHTTPServers returned not-ok on partial failure; want serve-the-good-one")
	}
	defer shutdown(t, listeners, server)

	// The bound address serves immediately.
	if got, err := getStatus("127.0.0.1", port); err != nil || got != http.StatusNoContent {
		t.Fatalf("GET 127.0.0.1:%d = %d (err %v), want %d", port, got, err, http.StatusNoContent)
	}

	// Release the blocked address; the background retry should bind it.
	if err := blocker.Close(); err != nil {
		t.Fatalf("release blocker: %v", err)
	}
	waitForStatus(t, "127.0.0.2", port, http.StatusNoContent, 2*time.Second)
}

func TestServerListenersStopClosesListenersAndDrains(t *testing.T) {
	defer goleak.VerifyNone(t)

	log := testLogger(t)
	port := freePort(t)
	server := noContentServer()

	listeners, ok := startHTTPServers(server, []string{"127.0.0.1", "127.0.0.2"}, port, log)
	if !ok {
		t.Fatal("startHTTPServers not-ok")
	}
	waitForStatus(t, "127.0.0.1", port, http.StatusNoContent, time.Second)

	listeners.Stop()
	// Stop is idempotent.
	listeners.Stop()
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("server.Shutdown: %v", err)
	}

	// After shutdown, both listeners are closed.
	if _, err := getStatus("127.0.0.1", port); err == nil {
		t.Fatal("expected connection failure on 127.0.0.1 after shutdown")
	}
	if _, err := getStatus("127.0.0.2", port); err == nil {
		t.Fatal("expected connection failure on 127.0.0.2 after shutdown")
	}
}

func TestProbeAddrPrefersLoopback(t *testing.T) {
	sl := &serverListeners{bound: []string{"100.64.0.1:8080", "127.0.0.1:8080"}}
	if got := sl.probeAddr(); got != "127.0.0.1:8080" {
		t.Fatalf("probeAddr() = %q, want 127.0.0.1:8080", got)
	}

	wildcard := &serverListeners{bound: []string{"0.0.0.0:8080"}}
	if got := wildcard.probeAddr(); got != "127.0.0.1:8080" {
		t.Fatalf("probeAddr() wildcard = %q, want 127.0.0.1:8080", got)
	}

	wildcard6 := &serverListeners{bound: []string{"[::]:8080"}}
	if got := wildcard6.probeAddr(); got != "[::1]:8080" {
		t.Fatalf("probeAddr() ipv6 wildcard = %q, want [::1]:8080", got)
	}

	tailnetOnly := &serverListeners{bound: []string{"100.64.0.1:8080"}}
	if got := tailnetOnly.probeAddr(); got != "100.64.0.1:8080" {
		t.Fatalf("probeAddr() tailnet-only = %q, want 100.64.0.1:8080", got)
	}
}
