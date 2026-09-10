package lifecycle

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/ssh"
)

// Tests in this file exercise SSH session transport liveness
// (docs/specs/executors/requirements/ssh-transport-liveness.md). None use
// t.Parallel(): sshKeepaliveInterval, sshKeepaliveDeadline, sshCleanupTimeout,
// sshAgentctlCleanupTimeout and sshBrokerPreflightTimeout are package-level
// and mutable, and goleak_test.go's TestMain zeroes the keepalive pair for
// every other test in this package.

// withSSHKeepaliveTuning sets the keepalive interval/deadline pair for the
// duration of one test and restores the package defaults (zero, per
// TestMain) afterward.
func withSSHKeepaliveTuning(t *testing.T, interval, deadline time.Duration) {
	t.Helper()
	prevInterval, prevDeadline := sshKeepaliveInterval, sshKeepaliveDeadline
	sshKeepaliveInterval, sshKeepaliveDeadline = interval, deadline
	t.Cleanup(func() { sshKeepaliveInterval, sshKeepaliveDeadline = prevInterval, prevDeadline })
}

// withSSHRemoteCleanupTimeouts sets the three remote-cleanup timeouts for the
// duration of one test and restores their real defaults afterward.
func withSSHRemoteCleanupTimeouts(t *testing.T, cleanup, agentctl, brokerPreflight time.Duration) {
	t.Helper()
	prevCleanup, prevAgentctl, prevPreflight := sshCleanupTimeout, sshAgentctlCleanupTimeout, sshBrokerPreflightTimeout
	sshCleanupTimeout, sshAgentctlCleanupTimeout, sshBrokerPreflightTimeout = cleanup, agentctl, brokerPreflight
	t.Cleanup(func() {
		sshCleanupTimeout, sshAgentctlCleanupTimeout, sshBrokerPreflightTimeout = prevCleanup, prevAgentctl, prevPreflight
	})
}

// newTrackedSSHSession inserts a session and starts its watchdog exactly as
// CreateInstance/ResumeRemoteInstance do: under the executor mutex, in the
// same critical section as the record.
func newTrackedSSHSession(exec *SSHExecutor, instanceID string, client *ssh.Client) *sshSessionState {
	state := &sshSessionState{
		target:        &SSHTarget{Host: "build.example", User: "deploy", PinnedFingerprint: "SHA256:x"},
		client:        client,
		pid:           4242,
		remoteDir:     "/remote/session",
		remoteTaskDir: "/remote/task",
	}
	exec.mu.Lock()
	exec.sessions[instanceID] = state
	exec.startWatchdogLocked(instanceID, state)
	exec.mu.Unlock()
	return state
}

// newObservedSSHExecutor builds an SSHExecutor whose logger records every
// Warn-and-above entry, so a test can assert on the transport-loss warning
// AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.9 requires instead of only on the
// in-memory transportLost marker.
func newObservedSSHExecutor(t *testing.T) (*SSHExecutor, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	return NewSSHExecutor(nil, nil, nil, log), logs
}

// newTrackedSSHSessionWithForwarder inserts a session with both a real
// session SSH client and a real local port forward — the same pair
// CreateInstance/ResumeRemoteInstance record — and starts its watchdog in the
// same critical section as the record. Unlike newTrackedSSHSession, this lets
// a test observe the forwarder half of transport teardown
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5/-001.6).
func newTrackedSSHSessionWithForwarder(exec *SSHExecutor, instanceID string, client *ssh.Client, fwd *SSHPortForwarder) *sshSessionState {
	state := &sshSessionState{
		target:        &SSHTarget{Host: "build.example", User: "deploy", PinnedFingerprint: "SHA256:x"},
		client:        client,
		forwarder:     fwd,
		pid:           4242,
		remoteDir:     "/remote/session",
		remoteTaskDir: "/remote/task",
	}
	exec.mu.Lock()
	exec.sessions[instanceID] = state
	exec.startWatchdogLocked(instanceID, state)
	exec.mu.Unlock()
	return state
}

// dialForwarder reports whether a new TCP connection to fwd's local port is
// currently accepted, closing it immediately if so.
func dialForwarder(fwd *SSHPortForwarder) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(fwd.LocalPort())), time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// waitClosedWithin blocks until ch is closed, failing the test if timeout
// elapses first.
func waitClosedWithin(t *testing.T, ch <-chan struct{}, timeout time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("%s did not happen within %s", what, timeout)
	}
}

func TestSSHKeepaliveTuningValid(t *testing.T) {
	tests := []struct {
		name               string
		interval, deadline time.Duration
		want               bool
	}{
		{"positive pair with deadline > 2x interval", 10 * time.Millisecond, 25 * time.Millisecond, true},
		{"deadline exactly 2x interval is invalid", 10 * time.Millisecond, 20 * time.Millisecond, false},
		{"zero interval", 0, 45 * time.Second, false},
		{"zero deadline", 15 * time.Second, 0, false},
		{"negative interval", -1, 45 * time.Second, false},
		{"deadline less than interval", 30 * time.Millisecond, 10 * time.Millisecond, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sshKeepaliveTuningValid(tc.interval, tc.deadline); got != tc.want {
				t.Fatalf("sshKeepaliveTuningValid(%v, %v) = %v, want %v", tc.interval, tc.deadline, got, tc.want)
			}
		})
	}
}

// TestSSHKeepaliveWatchdogDeclaresLossOnDeadline exercises the raw watchdog's
// own decision timing in isolation from SSHExecutor — its onLost callback
// here is a test channel, not the production transportTeardown, so it does
// not exercise AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.9's warning log (see
// TestSSHKeepaliveDeadlineTearsDownTrackedSessionWithinBoundedTime for that,
// wired through the real executor).
//
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.3
func TestSSHKeepaliveWatchdogDeclaresLossOnDeadline(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	server.setSilent(true) // every probe goes unanswered from the start

	lost := make(chan struct{}, 1)
	var reason string
	var silence time.Duration
	w := startSSHKeepaliveWatchdog(client, 5*time.Millisecond, 20*time.Millisecond, time.Now(), nil,
		func(r string, s time.Duration) {
			reason, silence = r, s
			lost <- struct{}{}
		})
	select {
	case <-lost:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not declare transport loss within the deadline")
	}
	if reason != sshTransportLostReasonDeadline {
		t.Fatalf("reason = %q, want %q", reason, sshTransportLostReasonDeadline)
	}
	if silence <= 20*time.Millisecond {
		t.Fatalf("silence = %s, want strictly greater than the 20ms deadline", silence)
	}
	// The prober is still blocked in its SendRequest; only closing the
	// client releases it.
	_ = client.Close()
	w.stopAndAwaitLoop()
	w.awaitProbeExit()
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.4
func TestSSHKeepaliveWatchdogDeclaresLossOnProbeError(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)

	lost := make(chan struct{}, 1)
	var reason string
	w := startSSHKeepaliveWatchdog(client, 5*time.Millisecond, 500*time.Millisecond, time.Now(), nil,
		func(r string, _ time.Duration) {
			reason = r
			lost <- struct{}{}
		})
	time.Sleep(15 * time.Millisecond) // let at least one probe complete normally
	_ = client.Close()

	select {
	case <-lost:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not declare transport loss after the client closed")
	}
	if reason != sshTransportLostReasonProbeError {
		t.Fatalf("reason = %q, want %q", reason, sshTransportLostReasonProbeError)
	}
	w.stopAndAwaitLoop()
	w.awaitProbeExit()
}

// TestSSHKeepaliveDeadlineTearsDownTrackedSessionWithinBoundedTime is the
// capability this card exists for: a session whose transport falls silent is
// torn down automatically, within a bounded time, with no disposal call
// (StopInstance/Close) driving it. It ends without stopping the session — per
// the system design's Testing section — so goleak.VerifyTestMain proves the
// prober's own error exit (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.12) rather
// than a disposal call retiring it first.
//
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.6
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.9
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.12
func TestSSHKeepaliveDeadlineTearsDownTrackedSessionWithinBoundedTime(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Millisecond, 20*time.Millisecond)

	exec, logs := newObservedSSHExecutor(t)
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	fwd, err := StartPortForward(client, 41234, newTestLogger())
	if err != nil {
		t.Fatalf("StartPortForward: %v", err)
	}
	state := newTrackedSSHSessionWithForwarder(exec, "instance-1", client, fwd)

	if !dialForwarder(fwd) {
		t.Fatal("forwarder port must accept connections before teardown")
	}

	server.setSilent(true) // every probe goes unanswered from here on

	waitClosedWithin(t, state.watchdog.loopDone, 2*time.Second, "the watchdog loop declaring transport loss")
	waitClosedWithin(t, state.watchdog.proberDone, 2*time.Second, "the prober exiting after teardown closed the client")

	if !exec.isTransportLost(state) {
		t.Fatal("isTransportLost = false, want true after an internally-triggered teardown")
	}
	if dialForwarder(fwd) {
		t.Fatal("forwarder port still accepts connections after teardown, want it refused")
	}

	var warnings []observer.LoggedEntry
	for _, entry := range logs.All() {
		if entry.Message == "ssh session transport lost" {
			warnings = append(warnings, entry)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("transport-loss warnings = %d, want exactly 1 (entries: %v)", len(warnings), logs.All())
	}
	fields := warnings[0].ContextMap()
	if fields["instance_id"] != "instance-1" {
		t.Fatalf("warning instance_id = %v, want instance-1", fields["instance_id"])
	}
	if fields["host"] != "build.example" {
		t.Fatalf("warning host = %v, want build.example", fields["host"])
	}
	if _, ok := fields["silence_interval"]; !ok {
		t.Fatal("warning missing silence_interval field")
	}
	if _, ok := fields["forwarder_close_error"]; ok {
		t.Fatalf("warning unexpectedly named a forwarder_close_error: %v", fields)
	}
	if _, ok := fields["client_close_error"]; ok {
		t.Fatalf("warning unexpectedly named a client_close_error: %v", fields)
	}
}

// TestSSHKeepaliveProbeErrorTearsDownTrackedSessionWithinBoundedTime exercises
// AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.4's real teardown path — the
// session's transport terminating on its own — through the full production
// wiring (a real SSHExecutor and forwarder), not just the raw watchdog struct
// TestSSHKeepaliveWatchdogDeclaresLossOnProbeError exercises in isolation. It
// also ends without stopping the session.
//
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.4
func TestSSHKeepaliveProbeErrorTearsDownTrackedSessionWithinBoundedTime(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Millisecond, 2*time.Second) // deadline kept out of reach: only a probe error should trip this

	exec, logs := newObservedSSHExecutor(t)
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	fwd, err := StartPortForward(client, 41234, newTestLogger())
	if err != nil {
		t.Fatalf("StartPortForward: %v", err)
	}
	state := newTrackedSSHSessionWithForwarder(exec, "instance-1", client, fwd)

	time.Sleep(15 * time.Millisecond) // let at least one probe complete normally
	_ = client.Close()                // simulate the transport terminating on its own

	waitClosedWithin(t, state.watchdog.loopDone, 2*time.Second, "the watchdog loop declaring transport loss")
	waitClosedWithin(t, state.watchdog.proberDone, 2*time.Second, "the prober exiting after the probe error")

	if !exec.isTransportLost(state) {
		t.Fatal("isTransportLost = false, want true after a probe-error teardown")
	}
	if dialForwarder(fwd) {
		t.Fatal("forwarder port still accepts connections after teardown, want it refused")
	}
	found := false
	for _, entry := range logs.All() {
		if entry.Message == "ssh session transport lost" && entry.ContextMap()["reason"] == sshTransportLostReasonProbeError {
			found = true
		}
	}
	if !found {
		t.Fatalf("no transport-loss warning recorded with reason=%q, entries: %v", sshTransportLostReasonProbeError, logs.All())
	}
}

// TestSSHKeepaliveHealthyTransportKeepsAnsweringPastTwiceTheInterval pins
// reply accounting: state.lastProbeReply only advances because
// startWatchdogLocked's recordReply callback runs on every completed probe
// reply (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.2/-001.3). A regression that
// stops recording replies leaves lastProbeReply frozen at session start,
// which classifySSHTransportLocked misreads as "unresponsive" once twice the
// probe interval has elapsed — even though the transport is healthy — and
// StopInstance would then skip its remote cleanup/stop calls entirely. This
// test sleeps well past that threshold on an answering server and asserts
// the remote calls still ran, so it fails under that regression even though
// isTransportLost alone would not catch it (classification, not the
// transport-lost marker, is what a regression here corrupts).
//
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.2
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.3
func TestSSHKeepaliveHealthyTransportKeepsAnsweringPastTwiceTheInterval(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Millisecond, 200*time.Millisecond) // unresponsive threshold 10ms, deadline far out of reach

	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	cleanupCalls, stopCalls := 0, 0
	exec.cleanupScript = func(context.Context, *ssh.Client, string, map[string]interface{}, map[string]string, SSHRemotePlatform, string, string) error {
		cleanupCalls++
		return nil
	}
	exec.stopRemote = func(context.Context, *ssh.Client, string, int) error {
		stopCalls++
		return nil
	}
	client := server.dial(t)
	state := newTrackedSSHSession(exec, "instance-1", client)

	time.Sleep(30 * time.Millisecond) // several times the 10ms unresponsive threshold; the server stays healthy throughout

	if err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		StopReason: StopReasonTaskDeleted, // triggers the cleanup script
	}, false); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if cleanupCalls != 1 || stopCalls != 1 {
		t.Fatalf("cleanup calls = %d, stop calls = %d, want both 1 — a healthy transport must still classify as answering", cleanupCalls, stopCalls)
	}
	if exec.isTransportLost(state) {
		t.Fatal("a deliberate stop on a healthy transport must not declare transport loss")
	}
}

func TestSSHExecutorCreateInstanceRefusesATransportLostSession(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.sessions["instance-1"] = &sshSessionState{
		authToken:     "resumed-token",
		transportLost: true,
	}
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		Metadata: map[string]interface{}{
			MetadataKeySSHRemoteTaskDir: "/remote/task",
		},
	}
	_, err := exec.CreateInstance(context.Background(), req)
	if !errors.Is(err, ErrSSHTransportLost) {
		t.Fatalf("error = %v, want ErrSSHTransportLost", err)
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.1
func TestSSHExecutorResumeRemoteInstanceInsertIfAbsentUnderRace(t *testing.T) {
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = io.WriteString(w, "ok")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(control.Close)

	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		if strings.Contains(command, "kill -0") {
			return sshOK
		}
		return sshOut("/home/kandev")
	})
	server.forwardTo(strings.TrimPrefix(control.URL, "http://"))
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &ExecutorCreateRequest{
				InstanceID: "instance-1",
				AuthToken:  "persisted-token",
				Metadata:   cloneSSHMetadata(metadata),
			}
			errs[i] = exec.ResumeRemoteInstance(context.Background(), req)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("ResumeRemoteInstance[%d]: %v", i, err)
		}
	}

	exec.mu.Lock()
	count := len(exec.sessions)
	exec.mu.Unlock()
	if count != 1 {
		t.Fatalf("tracked sessions = %d, want exactly one to survive the race", count)
	}
	// goleak's TestMain verifies the losing caller's dialed client and
	// forwarder were actually closed rather than orphaned.
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.4
func TestSSHExecutorResetTrackedManagedBrokerResumeLosesRaceReturnsSuccess(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	preflightCalls, stopCalls := 0, 0
	brokerPreflight := func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
		preflightCalls++
		return nil
	}
	stopRemote := func(context.Context, *ssh.Client, string, int) error {
		stopCalls++
		return nil
	}
	// state is deliberately not tracked in exec.sessions, simulating another
	// concurrent disposal having already removed it before this call's
	// reading (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.4's "the one that
	// does not remove it finds no tracked session").
	state := &sshSessionState{client: &ssh.Client{}}
	req := &ExecutorCreateRequest{InstanceID: "instance-1"}

	err := exec.resetTrackedManagedBrokerResume(context.Background(), req, state, brokerPreflight, stopRemote)
	if err != nil {
		t.Fatalf("resetTrackedManagedBrokerResume: %v", err)
	}
	if preflightCalls != 0 || stopCalls != 0 {
		t.Fatalf("preflight calls = %d, stop calls = %d, want both 0 — a losing disposal must not pay for a remote round trip",
			preflightCalls, stopCalls)
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.15
func TestSSHExecutorResetTrackedManagedBrokerResumeAbandonsAWedgedPreflight(t *testing.T) {
	withSSHRemoteCleanupTimeouts(t, sshCleanupTimeout, sshAgentctlCleanupTimeout, 20*time.Millisecond)

	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	server.setSilent(true)

	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	state := &sshSessionState{client: client, remoteDir: "/remote/session", pid: 4242}
	exec.sessions["instance-1"] = state

	brokerPreflight := func(ctx context.Context, client *ssh.Client, _ *ExecutorCreateRequest, _ SSHRemotePlatform) error {
		_, _, err := runSSHCommand(ctx, client, "true")
		return err
	}
	stopCalls := 0
	stopRemote := func(context.Context, *ssh.Client, string, int) error { stopCalls++; return nil }

	req := &ExecutorCreateRequest{InstanceID: "instance-1"}
	done := make(chan error, 1)
	go func() {
		done <- exec.resetTrackedManagedBrokerResume(context.Background(), req, state, brokerPreflight, stopRemote)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrSSHTransportLost) {
			t.Fatalf("error = %v, want ErrSSHTransportLost", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resetTrackedManagedBrokerResume did not return after its preflight wedged")
	}
	if stopCalls != 0 {
		t.Fatalf("stop calls = %d, want 0 — the reset must not run its remote stop once the preflight abandoned the client", stopCalls)
	}
	if !exec.isTransportLost(state) {
		t.Fatal("a preflight abandoned by its own timeout must mark the client transport-lost")
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.10
func TestSSHExecutorStopInstanceOnAHealthyTransportDoesNotDeclareLoss(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Millisecond, 200*time.Millisecond)

	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	client := server.dial(t)
	state := newTrackedSSHSession(exec, "instance-1", client)

	time.Sleep(20 * time.Millisecond) // let a few healthy probes complete

	if err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		StopReason: "stopped via API",
	}, false); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if exec.isTransportLost(state) {
		t.Fatal("a deliberate stop on a healthy transport must not declare transport loss")
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.11
func TestSSHExecutorStopInstanceSkipsRemoteCommandsWhenTransportUnresponsive(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Millisecond, 200*time.Millisecond) // unresponsive threshold 10ms, well under the 200ms deadline

	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	cleanupCalls, stopCalls := 0, 0
	exec.cleanupScript = func(context.Context, *ssh.Client, string, map[string]interface{}, map[string]string, SSHRemotePlatform, string, string) error {
		cleanupCalls++
		return nil
	}
	exec.stopRemote = func(context.Context, *ssh.Client, string, int) error {
		stopCalls++
		return nil
	}
	client := server.dial(t)
	newTrackedSSHSession(exec, "instance-1", client)

	server.setSilent(true)
	time.Sleep(30 * time.Millisecond) // let silence exceed the 10ms unresponsive threshold, well under the 200ms deadline

	done := make(chan error, 1)
	go func() {
		done <- exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1",
			StopReason: StopReasonTaskDeleted, // triggers the cleanup script
		}, false)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopInstance did not return while the transport was unresponsive — a skip must have been missed")
	}
	if cleanupCalls != 0 || stopCalls != 0 {
		t.Fatalf("cleanup calls = %d, stop calls = %d, want both skipped on an unresponsive transport", cleanupCalls, stopCalls)
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8
func TestSSHExecutorStopInstanceAbandonsARemoteCommandThatWedgesAfterTheReading(t *testing.T) {
	withSSHKeepaliveTuning(t, 0, 0) // no watchdog: exercises the backstop alone, not the reading-based skip
	withSSHRemoteCleanupTimeouts(t, 20*time.Millisecond, 20*time.Millisecond, sshBrokerPreflightTimeout)

	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	client := server.dial(t)
	server.setSilent(true) // the transport wedges; with no watchdog, the reading alone can't know that (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8's last sentence)

	state := &sshSessionState{client: client, remoteDir: "/remote/session", remoteTaskDir: "/remote/task", pid: 4242}
	exec.sessions["instance-1"] = state
	exec.cleanupScript = func(ctx context.Context, client *ssh.Client, _ string, _ map[string]interface{}, _ map[string]string, _ SSHRemotePlatform, _ string, _ string) error {
		_, _, err := runSSHCommand(ctx, client, "true")
		return err
	}
	exec.stopRemote = func(ctx context.Context, client *ssh.Client, _ string, _ int) error {
		_, _, err := runSSHCommand(ctx, client, "true")
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1",
			StopReason: StopReasonTaskDeleted,
		}, false)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopInstance did not return after its remote command wedged — the backstop must have been missed")
	}
	if !exec.isTransportLost(state) {
		t.Fatal("a command abandoned by its own timeout must mark the client transport-lost")
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.13
func TestSSHExecutorCloseDoesNotWaitForTheNextProbe(t *testing.T) {
	withSSHKeepaliveTuning(t, 500*time.Millisecond, 5*time.Second)

	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	client := server.dial(t)
	newTrackedSSHSession(exec, "instance-1", client)

	time.Sleep(50 * time.Millisecond) // let the first probe complete

	start := time.Now()
	if err := exec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 500*time.Millisecond {
		t.Fatalf("Close took %s, want well under one probe interval (500ms)", elapsed)
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.7
func TestSSHExecutorGetRemoteStatusReportsTransportLoss(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.sessions["i"] = &sshSessionState{
		target:        &SSHTarget{Host: "build.example"},
		pid:           4242,
		transportLost: true,
	}
	status, err := exec.GetRemoteStatus(context.Background(), &ExecutorInstance{InstanceID: "i"})
	if err != nil {
		t.Fatalf("GetRemoteStatus: %v", err)
	}
	if status.State != sshStatusDisconnected {
		t.Fatalf("State = %q, want %q", status.State, sshStatusDisconnected)
	}
	if !strings.Contains(status.ErrorMessage, "transport lost") {
		t.Fatalf("ErrorMessage = %q, want it to identify transport loss", status.ErrorMessage)
	}
}
