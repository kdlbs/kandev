package main

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
)

func TestResponseRetryScenarioEmitsAbandonedResetReplacementSequence(t *testing.T) {
	emitter, updates := newTestEmitter()

	emitPredefinedScenario(emitter, "response-retry")

	got := updates.getUpdates()
	if len(got) != 4 {
		t.Fatalf("update count = %d, want abandoned thought, abandoned answer, reset, replacement: %+v", len(got), got)
	}
	thought := got[0].notification.Update.AgentThoughtChunk
	abandoned := got[1].notification.Update.AgentMessageChunk
	reset := got[2].notification.Update.SessionInfoUpdate
	replacement := got[3].notification.Update.AgentMessageChunk
	if thought == nil || getThoughtContent(got[0]) != "Abandoned response attempt reasoning." {
		t.Fatalf("first update = %+v, want abandoned reasoning", got[0])
	}
	if abandoned == nil || getTextContent(got[1]) != "Abandoned response attempt answer." {
		t.Fatalf("second update = %+v, want abandoned answer", got[1])
	}
	if reset == nil {
		t.Fatalf("third update = %+v, want response-attempt reset", got[2])
	}
	mockMeta, _ := reset.Meta["kandevMock"].(map[string]any)
	if resetValue, _ := mockMeta["responseAttemptReset"].(bool); !resetValue {
		t.Fatalf("reset metadata = %+v, want controlled mock marker", reset.Meta)
	}
	if replacement == nil || getTextContent(got[3]) != "Replacement response after provider retry." {
		t.Fatalf("fourth update = %+v, want replacement answer", got[3])
	}

	ids := []*acp.MessageId{thought.MessageId, abandoned.MessageId, replacement.MessageId}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == nil {
			t.Fatal("response-retry content update omitted its protocol message ID")
		}
		messageID := string(*id)
		if _, err := uuid.Parse(messageID); err != nil {
			t.Fatalf("protocol message ID %q is not a UUID: %v", *id, err)
		}
		if _, exists := seen[messageID]; exists {
			t.Fatalf("protocol message ID %q was reused across attempts or content types", *id)
		}
		seen[messageID] = struct{}{}
	}
}
