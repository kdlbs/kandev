package lifecycle

import (
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNativeResumeProofRejectsConcurrentStreamProgress(t *testing.T) {
	before := &agentctl.DeliveryStatus{StreamID: "stream", Stream: &journal.Stream{StreamID: "stream", HighWater: 5, Acknowledged: 4, FirstRetained: 3, Bytes: 100}}
	for _, condition := range []string{"unchanged", "late terminal", "acknowledged", "pruned", "bytes increased", "bytes decreased", "unresolved", "owner", "stream owner", "sealed", "missing stream"} {
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
				after.Stream.Bytes -= 10
			case "bytes increased":
				after.Stream.Bytes += 10
			case "bytes decreased":
				after.Stream.Bytes -= 10
			case "unresolved":
				after.Unresolved = true
			case "owner":
				after.StreamID = "foreign"
			case "stream owner":
				after.Stream.SessionID = "foreign"
			case "sealed":
				after.Stream.Sealed = true
			case "missing stream":
				after.Stream = nil
			}
			require.Equal(t, condition == "unchanged", sameNativeResumeProofStatus(before, &after))
			retryable := condition == "late terminal" || condition == "acknowledged" || condition == "pruned"
			require.Equal(t, retryable, nativeResumeStreamAdvanced(before, &after))
		})
	}
}
