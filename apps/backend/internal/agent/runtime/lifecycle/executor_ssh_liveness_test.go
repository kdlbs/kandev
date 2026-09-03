package lifecycle

import (
	"context"
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
