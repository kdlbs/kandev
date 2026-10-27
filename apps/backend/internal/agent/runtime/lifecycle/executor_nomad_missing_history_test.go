package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Neither a missing local allocation checkpoint nor an empty server history
// proves that a submitted (or unknown-registration) job never ran.
func TestNomadPinnedMissingHistoryRetainsUnknownRuntime(t *testing.T) {
	for _, submitted := range []bool{false, true} {
		for _, via := range []string{"drain", "resume", "receipt"} {
			name := via + "/unknown-registration"
			if submitted {
				name = via + "/registered"
			}
			t.Run(name, func(t *testing.T) {
				server := &pinnedUnplacedServer{}
				c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
				c.state.BootstrapPath = c.state.AllocationRoot + "/job-1/bootstrap-" + strings.Repeat("a", 64) + ".sh"
				c.state.AgentctlPath = c.state.AllocationRoot + "/agentctl"
				c.state.NativeJobPolicyVersion = 1
				c.state.NativeJobSpecSHA256 = strings.Repeat("b", 64)
				if submitted {
					c.state.JobCreateIndex, c.state.JobDefinitionIndex = 5, 6
				} else {
					c.state.Phase = "intent"
				}
				if via != "drain" {
					c.state.Phase = nomadPhaseCleanupPending
				}
				if err := c.state.Validate(); err != nil {
					t.Fatalf("invalid fixture: %v", err)
				}
				if via == "receipt" {
					if err := c.recordEmptyDrainReceipt(context.Background()); err != nil {
						t.Fatal(err)
					}
					*checkpoints = nil
				}
				var err error
				if via == "drain" {
					err = c.drainAndSettle(context.Background())
				} else {
					err = c.resume(context.Background())
				}
				if !errors.Is(err, ErrNomadCleanupRetained) {
					t.Fatalf("cleanup error=%v, releases=%d quarantine=%d; want retained unknown runtime", err, *releases, server.quarantineCalls)
				}
				assertPinnedRowRetained(t, server, *checkpoints, *releases)
				for _, state := range *checkpoints {
					if via != "receipt" && state.DrainReceiptDigest != "" {
						t.Fatal("fabricated a drain receipt without process-absence evidence")
					}
				}
			})
		}
	}
}
