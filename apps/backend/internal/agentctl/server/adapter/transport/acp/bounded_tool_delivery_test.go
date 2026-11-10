package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestBoundedToolACPEmission(t *testing.T) {
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "locked"}[locked], func(t *testing.T) {
			payload := streams.NewGeneric("arbitrary-tool", nil)
			original := strings.Repeat("x", 1048605)
			payload.Generic().Output = original
			a := &Adapter{updatesCh: make(chan AgentEvent, 1), lifetimeCtx: context.Background()}
			event := AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool", ToolStatus: "complete", NormalizedPayload: payload}
			if locked {
				a.mu.Lock()
				ok := a.sendUpdateLocked(event)
				a.mu.Unlock()
				require.True(t, ok)
			} else {
				a.sendUpdateNow(event)
			}
			retained := <-a.updatesCh
			encoded, err := json.Marshal(retained)
			require.NoError(t, err)
			require.Less(t, len(encoded), 1<<20)
			require.Equal(t, true, retained.NormalizedPayload.Generic().Output.(map[string]any)["truncated"])
			require.Equal(t, original, payload.Generic().Output)
		})
	}
}
