package lifecycle

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// fakeTimeoutError is a minimal net.Error whose Timeout() reports true,
// wrapped inside a *net.OpError below to model a timeout that is also
// structurally shaped like a network dial error — @covers
// AC-EXECUTORS-SSH-REACHABILITY-001.27 (timeout must win the ordering, not
// fall through to network).
type fakeTimeoutError struct{}

func (fakeTimeoutError) Error() string   { return "fake timeout" }
func (fakeTimeoutError) Timeout() bool   { return true }
func (fakeTimeoutError) Temporary() bool { return true }

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.27 — classification order
// config, timeout, host_key, auth, network, unknown, first match wins,
// including the two explicit ordering cases.
func TestClassifyDialError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want SSHReachabilityReason
	}{
		{
			name: "a context deadline classifies as timeout",
			err:  fmt.Errorf("ssh: tcp dial x: %w", context.DeadlineExceeded),
			want: SSHReachabilityReasonTimeout,
		},
		{
			name: "a timeout-shaped dial error wins over the network fallback",
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: fakeTimeoutError{}},
			want: SSHReachabilityReasonTimeout,
		},
		{
			name: "a pinned host key mismatch classifies as host_key",
			err:  fmt.Errorf("ssh: handshake with x: %w", &errHostKeyMismatch{Expected: "A", Got: "B"}),
			want: SSHReachabilityReasonHostKey,
		},
		{
			name: "a bastion known_hosts mismatch classifies as host_key",
			err:  fmt.Errorf("ssh: bastion dial: %w", &knownhosts.KeyError{Want: []knownhosts.KnownKey{{}}}),
			want: SSHReachabilityReasonHostKey,
		},
		{
			name: "host_key wins over an auth-shaped message in the same error",
			err: fmt.Errorf("ssh: handshake failed: ssh: unable to authenticate: %w",
				&errHostKeyMismatch{Expected: "A", Got: "B"}),
			want: SSHReachabilityReasonHostKey,
		},
		{
			name: "an authentication failure classifies as auth",
			err:  errors.New("ssh: unable to authenticate, attempted methods [publickey], no supported methods remain"),
			want: SSHReachabilityReasonAuth,
		},
		{
			name: "a connection-refused dial error classifies as network",
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			want: SSHReachabilityReasonNetwork,
		},
		{
			name: "an unrecognized error classifies as unknown",
			err:  errors.New("something unexpected happened"),
			want: SSHReachabilityReasonUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyDialError(tt.err); got != tt.want {
				t.Fatalf("ClassifyDialError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.4, 001.5
func TestProbeSSHHostSuccess(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)

	target := &SSHTarget{
		Host: "127.0.0.1", Port: server.port(), User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: server.hostKeyFingerprint(),
	}

	outcome := ProbeSSHHost(context.Background(), target, 5*time.Second)

	if !outcome.Success {
		t.Fatalf("outcome = %+v, want Success", outcome)
	}
	if outcome.Host != "127.0.0.1" {
		t.Fatalf("Host = %q", outcome.Host)
	}
	if outcome.Reason != "" || outcome.Message != "" {
		t.Fatalf("a successful outcome must carry no reason/message: %+v", outcome)
	}
	if len(server.execCalls()) != 0 {
		t.Fatalf("execCalls = %v, want the probe to open no session channel", server.execCalls())
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.5
func TestProbeSSHHostHostKeyMismatch(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)

	target := &SSHTarget{
		Host: "127.0.0.1", Port: server.port(), User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: "SHA256:not-the-server-key",
	}

	outcome := ProbeSSHHost(context.Background(), target, 5*time.Second)

	if outcome.Success {
		t.Fatalf("outcome = %+v, want failure", outcome)
	}
	if outcome.Reason != SSHReachabilityReasonHostKey {
		t.Fatalf("Reason = %q, want host_key", outcome.Reason)
	}
	if target.PinnedFingerprint != "SHA256:not-the-server-key" {
		t.Fatalf("PinnedFingerprint mutated: %q", target.PinnedFingerprint)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.6
func TestProbeSSHHostEmptyPinDoesNotDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dialed := false
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err == nil {
			dialed = true
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	addr := ln.Addr().(*net.TCPAddr)

	target := &SSHTarget{
		Host: addr.IP.String(), Port: addr.Port, User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: "/does/not/matter",
		PinnedFingerprint: "",
	}

	outcome := ProbeSSHHost(context.Background(), target, 5*time.Second)

	if outcome.Reason != SSHReachabilityReasonConfig {
		t.Fatalf("Reason = %q, want config", outcome.Reason)
	}
	if outcome.Success {
		t.Fatal("outcome must not be a success")
	}
	_ = ln.Close()
	wg.Wait()
	if dialed {
		t.Fatal("ProbeSSHHost dialed TCP despite an empty pinned fingerprint")
	}
}

// newStallHandshakeListener accepts exactly one TCP connection and holds it
// open without ever writing the SSH version banner, so a client's handshake
// blocks on read until its own deadline fires. Used to prove the probe
// bounds the handshake, not only the TCP dial — @covers
// AC-EXECUTORS-SSH-REACHABILITY-001.7.
func newStallHandshakeListener(t *testing.T) *net.TCPAddr {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		<-done
		_ = conn.Close()
	}()
	t.Cleanup(func() {
		close(done)
		_ = ln.Close()
		wg.Wait()
	})
	return ln.Addr().(*net.TCPAddr)
}

func TestProbeSSHHostHandshakeStallIsBoundedByTheDeadline(t *testing.T) {
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)
	addr := newStallHandshakeListener(t)

	target := &SSHTarget{
		Host: addr.IP.String(), Port: addr.Port, User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: "SHA256:irrelevant",
	}

	const probeTimeout = 300 * time.Millisecond
	start := time.Now()
	done := make(chan SSHProbeOutcome, 1)
	go func() { done <- ProbeSSHHost(context.Background(), target, probeTimeout) }()

	select {
	case outcome := <-done:
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			t.Fatalf("probe took %s, want it bounded near the %s deadline", elapsed, probeTimeout)
		}
		if outcome.Reason != SSHReachabilityReasonTimeout {
			t.Fatalf("Reason = %q, want timeout; message=%q", outcome.Reason, outcome.Message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ProbeSSHHost did not return within 5s of a 300ms deadline — handshake is unbounded")
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.29
func TestProbeSSHHostBastionHostKeyMismatchClassifiesAsHostKey(t *testing.T) {
	home := newTestHomeDir(t)
	identity := writeTestIdentityFile(t)

	final := newFakeSSHServer(t, nil)
	bastion := newFakeSSHServer(t, nil)

	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherPriv)
	if err != nil {
		t.Fatalf("other signer: %v", err)
	}
	writeKnownHosts(t, home, fmt.Sprintf("[127.0.0.1]:%d", bastion.port()), otherSigner.PublicKey())

	proxyJump := "jump@127.0.0.1:" + strconv.Itoa(bastion.port())
	target := &SSHTarget{
		Host: "127.0.0.1", Port: final.port(), User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: final.hostKeyFingerprint(),
		ProxyJump:         proxyJump,
	}

	outcome := ProbeSSHHost(context.Background(), target, 5*time.Second)

	if outcome.Reason != SSHReachabilityReasonHostKey {
		t.Fatalf("Reason = %q, want host_key; message=%q", outcome.Reason, outcome.Message)
	}
	if outcome.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want the target host, not the bastion", outcome.Host)
	}
	if !strings.Contains(outcome.Message, "bastion") {
		t.Fatalf("Message = %q, want it to name the bastion", outcome.Message)
	}
}

func TestProbeSSHHostCancelledContextProducesNoObservation(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)

	target := &SSHTarget{
		Host: "127.0.0.1", Port: server.port(), User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: server.hostKeyFingerprint(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome := ProbeSSHHost(ctx, target, 5*time.Second)

	if !outcome.Cancelled {
		t.Fatalf("outcome = %+v, want Cancelled", outcome)
	}
	if outcome.Success {
		t.Fatal("a cancelled probe must not report success")
	}
	if outcome.Reason != "" {
		t.Fatalf("Reason = %q, want no reason for a cancelled probe", outcome.Reason)
	}
}

func TestSSHTargetFromExecutorConfig(t *testing.T) {
	newTestHomeDir(t)

	t.Run("a valid config resolves", func(t *testing.T) {
		target, err := SSHTargetFromExecutorConfig(map[string]string{
			"ssh_host":             "target.internal",
			"ssh_port":             "2222",
			"ssh_user":             "deploy",
			"ssh_identity_source":  "file",
			"ssh_identity_file":    "/keys/id_ed25519",
			"ssh_host_fingerprint": "SHA256:abc",
		})
		if err != nil {
			t.Fatalf("SSHTargetFromExecutorConfig: %v", err)
		}
		if target.Host != "target.internal" || target.Port != 2222 || target.User != "deploy" {
			t.Fatalf("target = %+v", target)
		}
		if target.PinnedFingerprint != "SHA256:abc" {
			t.Fatalf("PinnedFingerprint = %q", target.PinnedFingerprint)
		}
	})

	t.Run("a nil config fails resolution", func(t *testing.T) {
		if _, err := SSHTargetFromExecutorConfig(nil); err == nil {
			t.Fatal("expected an error for a nil config")
		}
	})

	t.Run("an invalid port fails resolution", func(t *testing.T) {
		if _, err := SSHTargetFromExecutorConfig(map[string]string{
			"ssh_host": "target.internal",
			"ssh_port": "not-a-port",
		}); err == nil {
			t.Fatal("expected an error for an invalid port")
		}
	})
}
