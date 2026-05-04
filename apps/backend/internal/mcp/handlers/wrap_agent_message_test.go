package handlers

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

func TestWrapAgentMessage_BasicShape(t *testing.T) {
	sender := &models.Task{ID: "task-uuid-123", Title: "Fix login bug"}

	wrapped, meta := wrapAgentMessage("please review my changes", sender, "session-uuid-456")

	// Must be wrapped in <kandev-system> tags so the existing strip pipeline
	// (Message.ToAPI / publishMessageEvent) hides the attribution block from the UI.
	if !sysprompt.HasSystemContent(wrapped) {
		t.Fatal("expected wrapped content to contain <kandev-system> tags")
	}

	// The original prompt body must remain present and intact.
	if !strings.Contains(wrapped, "please review my changes") {
		t.Errorf("wrapped content lost original prompt: %q", wrapped)
	}

	// Sender attribution must reference the task by title and ID so the
	// receiving agent can both identify the sender and reply via MCP.
	if !strings.Contains(wrapped, "Fix login bug") {
		t.Errorf("wrapped content missing sender title: %q", wrapped)
	}
	if !strings.Contains(wrapped, "task-uuid-123") {
		t.Errorf("wrapped content missing sender id: %q", wrapped)
	}

	// Stripping the <kandev-system> block should leave only the original prompt.
	stripped := sysprompt.StripSystemContent(wrapped)
	if stripped != "please review my changes" {
		t.Errorf("after strip, expected original prompt, got %q", stripped)
	}

	// Metadata must surface structured sender keys for the UI badge.
	if meta["sender_task_id"] != "task-uuid-123" {
		t.Errorf("meta missing sender_task_id: %v", meta)
	}
	if meta["sender_task_title"] != "Fix login bug" {
		t.Errorf("meta missing sender_task_title: %v", meta)
	}
	if meta["sender_session_id"] != "session-uuid-456" {
		t.Errorf("meta missing sender_session_id: %v", meta)
	}
}

func TestWrapAgentMessage_MultilinePrompt(t *testing.T) {
	sender := &models.Task{ID: "t-id", Title: "Task title"}
	prompt := "line one\nline two\n\nparagraph two"

	wrapped, _ := wrapAgentMessage(prompt, sender, "")
	stripped := sysprompt.StripSystemContent(wrapped)
	if stripped != prompt {
		t.Errorf("multiline prompt was mangled by wrap+strip:\nwant %q\n got %q", prompt, stripped)
	}
}

func TestWrapAgentMessage_PromptContainingKandevSystemTag(t *testing.T) {
	// If the user prompt itself contains a literal <kandev-system> block, it will
	// be stripped along with the attribution block. This is a known limitation
	// of the existing strip pipeline (used by every other <kandev-system> caller
	// in the codebase) and is documented here so future readers understand the
	// trade-off rather than treat it as a bug.
	sender := &models.Task{ID: "t-id", Title: "Task title"}
	prompt := "before <kandev-system>fake</kandev-system> after"

	wrapped, _ := wrapAgentMessage(prompt, sender, "")
	stripped := sysprompt.StripSystemContent(wrapped)
	// The strip regex removes BOTH <kandev-system> blocks (ours + the embedded one).
	// "before " and " after" survive (with the embedded block removed).
	if !strings.Contains(stripped, "before") || !strings.Contains(stripped, "after") {
		t.Errorf("expected outer 'before'/'after' to survive, got %q", stripped)
	}
}

func TestWrapAgentMessage_EmptySessionIDOmitted(t *testing.T) {
	sender := &models.Task{ID: "t-id", Title: "Task title"}
	_, meta := wrapAgentMessage("hi", sender, "")
	if _, ok := meta["sender_session_id"]; ok {
		t.Errorf("sender_session_id should be omitted when empty, got %v", meta)
	}
}
