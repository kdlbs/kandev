package lifecycle

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestRetryAgentctlHandshakeRetriesRejectedHandshakes covers the launch-time
// fix for the terminal "ssh: agentctl handshake returned 403" failure: a
// handshake rejected by whatever agentctl is currently listening on the
// picked port must not kill the launch outright. retryAgentctlHandshake tears
// the failed attempt down and tries again with a fresh instance.
func TestRetryAgentctlHandshakeRetriesRejectedHandshakes(t *testing.T) {
	var attempts int
	var torndown []int // ports handed to teardown, in order

	attempt := func() (port, pid int, token string, err error) {
		attempts++
		if attempts < 3 {
			return 5000 + attempts, 100 + attempts, "", fmt.Errorf("%w: status 403", errSSHAgentctlHandshakeRejected)
		}
		return 5003, 103, "token-3", nil
	}
	teardown := func(port, pid int) {
		torndown = append(torndown, port)
	}

	port, pid, token, err := retryAgentctlHandshake(attempt, teardown)
	if err != nil {
		t.Fatalf("retryAgentctlHandshake: %v", err)
	}
	if port != 5003 || pid != 103 || token != "token-3" {
		t.Fatalf("result = port %d pid %d token %q, want 5003/103/token-3", port, pid, token)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(torndown) != 2 || torndown[0] != 5001 || torndown[1] != 5002 {
		t.Fatalf("torndown = %v, want the two rejected instances torn down before the retry", torndown)
	}
}

// TestRetryAgentctlHandshakeDoesNotRetryOtherFailures covers every other
// failure shape (bind exhaustion inside start, transport failure, malformed
// handshake response): the launch must fail on the first attempt, exactly as
// before this change, and must still tear down an agentctl that reached a
// non-zero pid before failing.
func TestRetryAgentctlHandshakeDoesNotRetryOtherFailures(t *testing.T) {
	t.Run("start failure never started an instance, so nothing to tear down", func(t *testing.T) {
		wantErr := errors.New("remote launch failed")
		var teardownCalls int
		attempt := func() (int, int, string, error) {
			return 0, 0, "", wantErr
		}
		teardown := func(int, int) { teardownCalls++ }

		_, _, _, err := retryAgentctlHandshake(attempt, teardown)
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if teardownCalls != 0 {
			t.Fatalf("teardown calls = %d, want 0 (agentctl never started)", teardownCalls)
		}
	})

	t.Run("a started agentctl is torn down even for a non-rejection handshake failure", func(t *testing.T) {
		wantErr := errors.New("ssh: agentctl handshake returned no token")
		var torndownPort, torndownPID int
		attempt := func() (int, int, string, error) {
			return 5001, 101, "", wantErr
		}
		teardown := func(port, pid int) { torndownPort, torndownPID = port, pid }

		_, _, _, err := retryAgentctlHandshake(attempt, teardown)
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if torndownPort != 5001 || torndownPID != 101 {
			t.Fatalf("teardown = (%d, %d), want (5001, 101)", torndownPort, torndownPID)
		}
	})
}

// TestRetryAgentctlHandshakeExhaustsAttemptsWithDiagnosis covers the case a
// stale listener never goes away within the retry budget: the launch must
// still fail, but the returned error carries the last attempt's diagnosis
// (the sentinel plus whatever the attempt closure attached, e.g. port/pid/log
// tail) instead of a bare "handshake returned 403".
func TestRetryAgentctlHandshakeExhaustsAttemptsWithDiagnosis(t *testing.T) {
	var attempts int
	var torndown int
	attempt := func() (int, int, string, error) {
		attempts++
		return 6000 + attempts, 200 + attempts, "", fmt.Errorf(
			"%w (port %d, pid %d); log:\nHTTP server bound successfully",
			errSSHAgentctlHandshakeRejected, 6000+attempts, 200+attempts,
		)
	}
	teardown := func(int, int) { torndown++ }

	_, _, _, err := retryAgentctlHandshake(attempt, teardown)
	if err == nil {
		t.Fatal("expected an error after exhausting every attempt")
	}
	if !errors.Is(err, errSSHAgentctlHandshakeRejected) {
		t.Fatalf("error = %v, want it to still wrap errSSHAgentctlHandshakeRejected", err)
	}
	if attempts != sshAgentctlHandshakeAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, sshAgentctlHandshakeAttempts)
	}
	if torndown != sshAgentctlHandshakeAttempts {
		t.Fatalf("torndown = %d, want every exhausted attempt torn down", torndown)
	}
	if got := err.Error(); !strings.Contains(got, "log:") || !strings.Contains(got, "pid") {
		t.Fatalf("error = %q, want the last attempt's diagnosis preserved", got)
	}
}
