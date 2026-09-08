package lifecycle

import (
	"context"
	"strings"
	"testing"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.10
func TestProbeRemoteAgentctlLiveness(t *testing.T) {
	t.Run("live process", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "kill -0 4242", result: sshOK},
		).handle)

		alive, err := probeRemoteAgentctlLiveness(context.Background(), server.dial(t), 4242)
		if err != nil || !alive {
			t.Fatalf("probe = (%v, %v), want (true, nil)", alive, err)
		}
	})

	t.Run("completed remote probe reports absence", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshFail("no such process")
		})

		alive, err := probeRemoteAgentctlLiveness(context.Background(), server.dial(t), 4242)
		if err != nil || alive {
			t.Fatalf("probe = (%v, %v), want (false, nil)", alive, err)
		}
	})

	t.Run("permission failure leaves liveness unknown", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshFail("kill: 4242: Operation not permitted")
		})

		alive, err := probeRemoteAgentctlLiveness(context.Background(), server.dial(t), 4242)
		if err == nil || alive {
			t.Fatalf("probe = (%v, %v), want (false, error)", alive, err)
		}
		if !strings.Contains(err.Error(), "Operation not permitted") {
			t.Fatalf("error = %v, want permission detail", err)
		}
	})

	t.Run("closed SSH connection leaves liveness unknown", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		client := server.dial(t)
		if err := client.Close(); err != nil {
			t.Fatalf("close client: %v", err)
		}

		alive, err := probeRemoteAgentctlLiveness(context.Background(), client, 4242)
		if err == nil || alive {
			t.Fatalf("probe = (%v, %v), want (false, error)", alive, err)
		}
	})
}

// TestVerifyRemoteAgentctlIdentity covers R2-F1: verifyRemoteAgentctlIdentity
// must mirror probeRemoteAgentctlLiveness's discipline for a nonzero `ps`
// exit — only a confirmed-absent process is a safe "not ours", any other
// probe failure (missing/unsupported ps, a permission fault, an SSH-level
// fault) must be reported as an error rather than folded into "no match".
//
// It also covers R3-F2: the `ps` argv match alone only proves "some agentctl
// for this taskDir" — sibling sessions of the same task share taskDir in
// their launch argv — so identity additionally requires the per-session
// pidfile the launch wrapper writes at <sessionDir>/agentctl.pid to name the
// same pid.
func TestVerifyRemoteAgentctlIdentity(t *testing.T) {
	t.Run("matching command line and pidfile reports identity", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
			sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshOut("4242")},
		).handle)

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
		if err != nil || !ours {
			t.Fatalf("verify = (%v, %v), want (true, nil)", ours, err)
		}
	})

	t.Run("non-matching command line reports no identity", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/other-task")},
		).handle)

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
		if err != nil || ours {
			t.Fatalf("verify = (%v, %v), want (false, nil)", ours, err)
		}
	})

	// R3-F2: a stale row's pid was recycled by a live sibling session's
	// agentctl on the same taskDir — the argv matches, but this row's own
	// sessionDir pidfile still names the pid its own (now-dead) launch
	// recorded, which differs from the pid actually being probed.
	t.Run("command line matches but pidfile names a different pid reports no identity", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
			sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshOut("9999")},
		).handle)

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
		if err != nil || ours {
			t.Fatalf("verify = (%v, %v), want (false, nil)", ours, err)
		}
	})

	t.Run("missing pidfile reports no identity", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
			sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshFail("No such file or directory")},
		).handle)

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
		if err != nil || ours {
			t.Fatalf("verify = (%v, %v), want (false, nil)", ours, err)
		}
	})

	// R3-F1: real `ps -p <absent-pid> -o command=` exits non-zero with both
	// stdout and stderr empty — it never writes a "no such process" message
	// the way `kill -0` does. sshFail("") reproduces that shape; a fixture
	// using sshFail("no such process") here would fabricate a shape `ps`
	// never actually produces and let the dead-pid case regress silently.
	t.Run("completed remote probe with empty stderr reports absence as no identity", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshFail("")
		})

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
		if err != nil || ours {
			t.Fatalf("verify = (%v, %v), want (false, nil)", ours, err)
		}
	})

	t.Run("unsupported ps command leaves identity unknown", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshFail("ps: unrecognized option '-o'")
		})

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
		if err == nil || ours {
			t.Fatalf("verify = (%v, %v), want (false, error)", ours, err)
		}
	})

	t.Run("closed SSH connection leaves identity unknown", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		client := server.dial(t)
		if err := client.Close(); err != nil {
			t.Fatalf("close client: %v", err)
		}

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), client, 4242, "/remote/session", "/remote/task")
		if err == nil || ours {
			t.Fatalf("verify = (%v, %v), want (false, error)", ours, err)
		}
	})
}
