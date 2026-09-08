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
// The pidfile leg has the same discipline: <sessionDir>/agentctl.pid must
// name the pid about to be signalled, and a pidfile that cannot be read or
// disagrees is an error rather than a mismatch verdict — the caller reclaims
// the session directory on a mismatch, and that directory may belong to a
// live process.
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

	// A pidfile that names a different pid means the session directory
	// belongs to a launch other than the one this row describes — most
	// plausibly a newer, live one. Identity is unproven, so the verdict is an
	// error: a plain "not ours" would authorise the caller to remove that
	// live launch's directory.
	t.Run("command line matches but pidfile names a different pid leaves identity unproven", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
			sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshOut("9999")},
		).handle)

		ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
		if err == nil || ours {
			t.Fatalf("verify = (%v, %v), want (false, error)", ours, err)
		}
		if !strings.Contains(err.Error(), "names pid 9999") {
			t.Fatalf("error = %v, want the disagreeing pid in the detail", err)
		}
	})

	// The pid is alive and the argv already matches this row's agentctl, so
	// an unreadable pidfile cannot be read as absence. Reporting "not ours"
	// here would leave a live agentctl running while deleting the very
	// pidfile and log that identify it.
	t.Run("unreadable pidfile leaves identity unproven", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			result sshExecResult
		}{
			{name: "missing file", result: sshFail("No such file or directory")},
			{name: "channel fault under load", result: sshFail("ssh: unable to open channel")},
			{name: "non-numeric content", result: sshOut("not-a-pid")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				server := newFakeSSHServer(t, newSSHScriptedHandler(t,
					sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
					sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: tc.result},
				).handle)

				ours, err := verifyRemoteAgentctlIdentity(context.Background(), server.dial(t), 4242, "/remote/session", "/remote/task")
				if err == nil || ours {
					t.Fatalf("verify = (%v, %v), want (false, error)", ours, err)
				}
			})
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
