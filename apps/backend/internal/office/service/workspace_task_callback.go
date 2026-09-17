package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// WorkspaceTaskCallback is a small persisted notification, not a worker transcript.
// TaskID on the run still points to the coordinator's conversation.
type WorkspaceTaskCallback struct {
	TaskID      string `json:"task_id"`
	WorkspaceID string `json:"workspace_id"`
	Title       string `json:"title"`
	State       string `json:"state"`
}

func (s *Service) queueWorkspaceTaskCallback(ctx context.Context, chiefID, conversationID string, task *sqlite.TaskExecutionFields, event TaskUpdatedData) error {
	if task.ID == conversationID || (event.State != "" && event.State != task.State) {
		return nil
	}
	revision := task.UpdatedAt.Format(time.RFC3339Nano)
	if stamp, err := time.Parse(time.RFC3339Nano, event.UpdatedAt); err == nil {
		revision = stamp.Format(time.RFC3339Nano)
	}
	callback := WorkspaceTaskCallback{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Title: clipConversationText(task.Title, 300), State: task.State}
	payload := mustJSON(map[string]any{conversationTaskIDKey: conversationID, "callback": callback})
	key := fmt.Sprintf("workspace-task-callback:%s:%s:%s:%s", chiefID, task.ID, task.State, revision)
	// TaskMoved and TaskStateChanged can be published concurrently for one
	// committed transition. Serialize the queue's check-and-insert in this host.
	s.workspaceCallbackMu.Lock()
	defer s.workspaceCallbackMu.Unlock()
	return s.QueueRun(ctx, chiefID, RunReasonWorkspaceTaskCallback, payload, key)
}

func enrichWorkspaceTaskCallback(pc *PromptContext, payload string) {
	var data struct {
		Callback *WorkspaceTaskCallback `json:"callback"`
	}
	if json.Unmarshal([]byte(payload), &data) == nil {
		pc.WorkspaceTaskCallback = data.Callback
	}
}

func buildWorkspaceTaskCallbackPrompt(pc *PromptContext) string {
	callback := pc.WorkspaceTaskCallback
	if callback == nil {
		return "A task callback is missing its task identity. Report the callback error in this conversation; do not guess which task finished."
	}
	return fmt.Sprintf(`A coordinated task has an update. This callback concerns only this task, not all workspace tasks.
Task: %s (%s)
Reported state: %s
Task link: /t/%s?workspaceId=%s

Read the current state and bounded worker result with kandev task inspect --id %s. Then post a concise user-facing update in this conversation (%s): what finished, the important result, and any next action or review needed, with the task link. Your final reply is the chat update. Do not post it on the delivery task instead.
REVIEW means ready for review, not fully completed. FAILED, BLOCKED and WAITING_FOR_INPUT are problems or requests for input, not success. If the task has moved on since this callback, report its current state accurately. Read the latest user messages in this conversation before asking for input. If the user already answered the worker's question, acknowledge that answer and use the existing task/session; do not repeat the question. If a structured gate requires a direct human response, explain that specific limitation and link to the task. Do not imply other tasks are done. Do not restart or duplicate work just to produce this update.`, callback.Title, callback.TaskID, callback.State, callback.TaskID, callback.WorkspaceID, callback.TaskID, pc.TaskID)
}
