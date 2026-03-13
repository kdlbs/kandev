package models

import (
	"testing"
	"time"
)

func TestMessageToAPI_StripsOnlySystemContent(t *testing.T) {
	msg := &Message{
		ID:            "msg-1",
		TaskSessionID: "session-1",
		TaskID:        "task-1",
		TurnID:        "turn-1",
		AuthorType:    MessageAuthorUser,
		Content:       "<kandev-system>PLAN MODE ACTIVE</kandev-system>\n\nCommit the changes, push and create a draft PR.",
		Type:          MessageTypeMessage,
		CreatedAt:     time.Now().UTC(),
	}

	apiMsg := msg.ToAPI()

	if apiMsg.Content != "Commit the changes, push and create a draft PR." {
		t.Fatalf("expected visible content to keep workflow prompt, got %q", apiMsg.Content)
	}
	if apiMsg.RawContent != msg.Content {
		t.Fatalf("expected raw_content to preserve original content, got %q", apiMsg.RawContent)
	}
	if apiMsg.Metadata["has_hidden_prompts"] != true {
		t.Fatalf("expected has_hidden_prompts metadata, got %v", apiMsg.Metadata["has_hidden_prompts"])
	}
}

func TestMessageToAPI_KeepsWorkflowPromptVisibleWithoutSystemTags(t *testing.T) {
	msg := &Message{
		ID:            "msg-1",
		TaskSessionID: "session-1",
		TaskID:        "task-1",
		TurnID:        "turn-1",
		AuthorType:    MessageAuthorUser,
		Content:       "Commit the changes, push and create a draft PR.",
		Type:          MessageTypeMessage,
		CreatedAt:     time.Now().UTC(),
	}

	apiMsg := msg.ToAPI()

	if apiMsg.Content != msg.Content {
		t.Fatalf("expected visible content %q, got %q", msg.Content, apiMsg.Content)
	}
	if apiMsg.RawContent != "" {
		t.Fatalf("expected empty raw_content when there is no hidden prompt, got %q", apiMsg.RawContent)
	}
}
