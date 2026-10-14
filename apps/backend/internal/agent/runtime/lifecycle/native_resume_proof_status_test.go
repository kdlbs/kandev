package lifecycle

import (
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNativeResumeProofRejectsConcurrentStreamProgress(t *testing.T) {
	before := &agentctl.DeliveryStatus{StreamID: "stream", Stream: &journal.Stream{StreamID: "stream", HighWater: 5, Acknowledged: 5, FirstRetained: 6}}
	for _, condition := range []string{"unchanged", "late terminal", "acknowledged", "pruned", "unresolved", "owner", "missing stream"} {
		t.Run(condition, func(t *testing.T) {
			after := *before
			stream := *before.Stream
			after.Stream = &stream
			switch condition {
			case "late terminal":
				after.Stream.HighWater++
			case "acknowledged":
				after.Stream.Acknowledged++
			case "pruned":
				after.Stream.FirstRetained++
			case "unresolved":
				after.Unresolved = true
			case "owner":
				after.StreamID = "foreign"
			case "missing stream":
				after.Stream = nil
			}
			require.Equal(t, condition == "unchanged", sameNativeResumeProofStatus(before, &after))
		})
	}
}
