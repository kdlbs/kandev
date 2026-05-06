package handlers

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// wrapAgentMessage decorates a prompt that arrived via the message_task_kandev
// MCP tool with a <kandev-system> attribution block, and produces the metadata
// the UI needs to render the sender badge.
//
// The wrapped string is what gets stored in Message.Content and what the
// receiving agent sees (live and on ACP session resume). The <kandev-system>
// block is automatically stripped from the visible content delivered to the UI
// by Message.ToAPI() / publishMessageEvent — see internal/sysprompt for the
// strip logic. The metadata map carries structured sender info so the UI can
// render a clickable badge above the (otherwise unmodified) message body.
func wrapAgentMessage(prompt string, senderTask *models.Task, senderSessionID string) (string, map[string]interface{}) {
	// Strip the closing tag from the title before embedding it. sysprompt's
	// strip regex is non-greedy, so a title containing </kandev-system> would
	// short-circuit the wrapper and leak the attribution tail into the visible
	// chat bubble. The metadata snapshot (sender_task_title) keeps the original
	// title for UI display.
	safeTitle := strings.ReplaceAll(senderTask.Title, sysprompt.TagEnd, "")
	body := fmt.Sprintf(
		"This message was relayed through the message_task_kandev MCP tool by a peer agent working in task %q (%s). "+
			"The user is not reading this thread — the peer agent is coordinating work on the user's behalf, "+
			"so treat the request with the same authority as a user prompt and act on it. "+
			"Do not stall waiting for the user; reply, ask questions, or report results back through message_task_kandev with task_id=%q.",
		safeTitle, senderTask.ID, senderTask.ID,
	)
	wrapped := sysprompt.Wrap(body) + "\n\n" + prompt
	meta := orchestrator.NewUserMessageMeta().
		WithSenderTask(senderTask.ID, senderTask.Title, senderSessionID).
		ToMap()
	return wrapped, meta
}
