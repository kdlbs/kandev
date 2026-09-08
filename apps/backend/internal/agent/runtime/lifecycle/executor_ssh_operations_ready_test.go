package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestAwaitRemoteAgentctlReadyTimeoutReturnsLivePid covers the orphan leak a
// ready-timeout used to cause: the remote process never logged "bound
// successfully" (or "failed to bind") within the budget, but every liveness
// probe up to the deadline said it was still running. The caller needs the
// real pid back to tear it down; returning 0 here made the caller's "if
// pid > 0" teardown a no-op and leaked the process.
func TestAwaitRemoteAgentctlReadyTimeoutReturnsLivePid(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "kill -0"):
			return sshOK // always alive
		case strings.Contains(command, "cat ") || strings.Contains(command, "tail "):
			return sshOut("agentctl: still starting up\n")
		default:
			return sshOK
		}
	})

	pid, err := awaitRemoteAgentctlReady(
		context.Background(), server.dial(t), "/remote/session", 45000, 4242,
		50*time.Millisecond, 10*time.Millisecond, newTestLogger(),
	)
	if err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("error = %v, want a ready-timeout error", err)
	}
	if pid != 4242 {
		t.Fatalf("pid = %d, want the live pid 4242 preserved for teardown", pid)
	}
}

func TestAwaitRemoteAgentctlReadyPreservesPidWhenLivenessIsUnknown(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		if strings.Contains(command, "kill -0") {
			return sshFail("Operation not permitted")
		}
		return sshOut("agentctl: still starting up\n")
	})

	pid, err := awaitRemoteAgentctlReady(
		context.Background(), server.dial(t), "/remote/session", 45000, 4242,
		50*time.Millisecond, 10*time.Millisecond, newTestLogger(),
	)
	if err == nil || !strings.Contains(err.Error(), "readiness probe failed") {
		t.Fatalf("error = %v, want an unknown-liveness probe error", err)
	}
	if pid != 4242 {
		t.Fatalf("pid = %d, want the pid preserved for teardown", pid)
	}
}

func TestRetryRemoteAgentctlPortPreservesPidOnTerminalError(t *testing.T) {
	wantErr := errors.New("ssh: agentctl did not become ready within 30s")
	port, pid, err := retryRemoteAgentctlPort(
		func() int { return 45000 },
		func(int) (int, error) { return 4242, wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if port != 45000 || pid != 4242 {
		t.Fatalf("port/pid = %d/%d, want 45000/4242 preserved so the caller can tear down the live process", port, pid)
	}
}

func TestRetryRemoteAgentctlPortZeroesPidAfterExhaustingPortAttempts(t *testing.T) {
	port, pid, err := retryRemoteAgentctlPort(
		func() int { return 45000 },
		func(int) (int, error) { return 0, errSSHAgentctlPortInUse },
	)
	if !errors.Is(err, errSSHAgentctlPortInUse) {
		t.Fatalf("error = %v, want errSSHAgentctlPortInUse", err)
	}
	if port != 0 || pid != 0 {
		t.Fatalf("port/pid = %d/%d, want 0/0 — every attempt reported no live process", port, pid)
	}
}
