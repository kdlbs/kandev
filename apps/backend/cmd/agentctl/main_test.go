package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "warn",
		Format:     "console",
		OutputPath: "stdout",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func TestMonitorParentLiveness_NoEnvVar(t *testing.T) {
	t.Setenv("KANDEV_PARENT_PIPE_FD", "")
	ch := monitorParentLiveness(newTestLogger(t))
	if ch != nil {
		t.Fatal("expected nil channel when env var is absent")
	}
}

func TestMonitorParentLiveness_InvalidEnvVar(t *testing.T) {
	t.Setenv("KANDEV_PARENT_PIPE_FD", "not-a-number")
	ch := monitorParentLiveness(newTestLogger(t))
	if ch != nil {
		t.Fatal("expected nil channel for invalid FD value")
	}
}

func TestMonitorParentLiveness_PipeBreak(t *testing.T) {
	// Create a pipe: r is the read-end (what agentctl would inherit),
	// w is the write-end (what the parent keeps open).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	// Point the env var at the read-end's FD.
	t.Setenv("KANDEV_PARENT_PIPE_FD", fmt.Sprintf("%d", r.Fd()))

	ch := monitorParentLiveness(newTestLogger(t))
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	// Channel should not be closed yet.
	select {
	case <-ch:
		t.Fatal("channel closed before pipe break")
	case <-time.After(50 * time.Millisecond):
	}

	// Close write-end to simulate parent death.
	_ = w.Close()

	select {
	case <-ch:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after pipe break")
	}
}

func reservePprofAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve pprof address: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release reserved pprof address: %v", err)
	}
	return addr
}

func waitForPprofListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/debug/pprof/")
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pprof listener at %s was not reachable: %v", addr, lastErr)
}

func TestPprofListenerShutdownReleasesListener(t *testing.T) {
	addr := reservePprofAddr(t)
	t.Setenv(pprofAddrEnv, addr)

	stop := startPprofListenerIfEnabled(newTestLogger(t))
	waitForPprofListener(t, addr)

	stop()
	stop()

	if resp, err := http.Get("http://" + addr + "/debug/pprof/"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("pprof listener still serving after stop")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("pprof address %s was not released after stop: %v", addr, err)
	}
	_ = ln.Close()
}

func TestPprofListenerDisabledOrNonLoopbackStartsNoListener(t *testing.T) {
	for _, tc := range []struct {
		name string
		addr string
	}{
		{name: "unset", addr: ""},
		{name: "non-loopback", addr: "0.0.0.0:0"},
		{name: "malformed", addr: "not-an-address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(pprofAddrEnv, tc.addr)
			stop := startPprofListenerIfEnabled(newTestLogger(t))
			stop()
			stop()
		})
	}
}

func TestPprofListenerBindFailureLeavesExistingListenerAlone(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy pprof address: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	t.Setenv(pprofAddrEnv, occupied.Addr().String())

	stop := startPprofListenerIfEnabled(newTestLogger(t))
	stop()

	conn, err := net.DialTimeout("tcp", occupied.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("existing listener was disturbed after bind failure: %v", err)
	}
	_ = conn.Close()
}
