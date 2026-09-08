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
func TestVerifyRemoteAgentctlIdentity(t *testing.T) {
	t.Run("matching command line reports identity", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
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

	t.Run("completed remote probe reports absence as no identity", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshFail("no such process")
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
