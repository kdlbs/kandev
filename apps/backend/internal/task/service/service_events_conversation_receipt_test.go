package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestProjectConversationReceiptDoesNotExposeOrMutateSourceEntities(t *testing.T) {
	message := &models.Message{
		ID:      "message-1",
		Content: "<kandev-system>private prompt</kandev-system>visible",
		Metadata: map[string]any{
			"normalized": map[string]any{
				"shell_exec": map[string]any{
					"output": map[string]any{"stdout": "private shell output"},
				},
			},
		},
	}
	turn := &models.Turn{ID: "turn-1", Metadata: map[string]any{
		models.TurnMetaKeyRuntimeConfigSnapshot: models.TurnRuntimeConfigSnapshot{Model: "mock-fast"},
		models.TurnMetaKeyPromptDispatchPending: true,
		"internal":                              "private turn state",
	}}
	receipt := &models.ConversationMutationReceipt{
		SessionID: "session-1",
		Operations: []models.ConversationMutationOperation{
			{Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityMessage, Message: message},
			{Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityTurn, Turn: turn},
		},
	}

	projected := projectConversationReceipt(receipt)
	if projected == receipt || projected.Operations[0].Message == message || projected.Operations[1].Turn == turn {
		t.Fatal("receipt projection reused source-owned entities")
	}
	if got := projected.Operations[0].Message.Content; got != "visible" {
		t.Fatalf("projected message content = %q, want visible content", got)
	}
	projectedMetadata := projected.Operations[1].Turn.Metadata
	if projectedMetadata[models.TurnMetaKeyRuntimeConfigSnapshot] == nil {
		t.Fatal("projected turn dropped public runtime metadata")
	}
	if _, ok := projectedMetadata[models.TurnMetaKeyPromptDispatchPending]; ok {
		t.Fatal("projected turn retained prompt-dispatch metadata")
	}
	if _, ok := projectedMetadata["internal"]; ok {
		t.Fatal("projected turn retained private metadata")
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal projected receipt: %v", err)
	}
	if strings.Contains(string(encoded), "private prompt") || strings.Contains(string(encoded), "private shell output") {
		t.Fatalf("projected receipt contains private source data: %s", encoded)
	}
	if message.Content != "<kandev-system>private prompt</kandev-system>visible" {
		t.Fatal("receipt projection mutated source message content")
	}
	if turn.Metadata["internal"] != "private turn state" || turn.Metadata[models.TurnMetaKeyPromptDispatchPending] != true {
		t.Fatal("receipt projection mutated source turn metadata")
	}
}
