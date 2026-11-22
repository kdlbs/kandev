package watcher

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestACPSessionEventDataDecodesNativeConversationReplacement(t *testing.T) {
	w := &Watcher{logger: logger.Default()}
	var data ACPSessionEventData
	err := w.parseEventData(lifecycle.ACPSessionCreatedPayload{
		SessionID:                          "session",
		ACPSessionID:                       "replacement-native",
		DeliveryHarnessGeneration:          2,
		DeliveryReplacesNativeConversation: true,
	}, &data)
	if err != nil {
		t.Fatal(err)
	}
	if !data.DeliveryReplacesNativeConversation || data.DeliveryHarnessGeneration != 2 {
		t.Fatalf("decoded = %+v, want replacement generation 2", data)
	}
}
