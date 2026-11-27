package api

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestUnresolvedDeliveryResponseBlocksOnlyIndependentPrompts(t *testing.T) {
	msg := &ws.Message{ID: "request-1", Action: "agent.prompt"}
	capability := journal.StorageCapability{Durable: true, Unresolved: true}

	if response := unresolvedDeliveryResponse(msg, capability, false); response == nil {
		t.Fatal("ordinary prompt must be blocked while durable delivery is unresolved")
	}

	// Steering is admitted as part of the already-running generation. The
	// transport must not turn its predecessor's in-flight submission into a
	// false recovery barrier.
	if response := unresolvedDeliveryResponse(msg, capability, true); response != nil {
		t.Fatalf("steer must bypass the unresolved-delivery barrier, got %v", response)
	}
}
