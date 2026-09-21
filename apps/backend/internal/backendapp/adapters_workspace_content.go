package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/common/redaction"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const workspaceClarificationRequest = "clarification_request"

func (a *taskCreatorAdapter) WorkspaceTaskContent(ctx context.Context, workspaceID, taskID string, query shared.WorkspaceContentQuery) (any, error) {
	if query.Offset < 0 || query.Limit < 0 || query.Limit > 4000 {
		return nil, fmt.Errorf("offset must be nonnegative and limit must be 1 to 4000")
	}
	if query.Limit == 0 {
		query.Limit = 3000
	}
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspaceID || task.IsEphemeral || task.IsFromOffice {
		return nil, fmt.Errorf("delivery task unavailable")
	}
	if query.Source == "description" {
		return workspaceContentPage(task.Description, query), nil
	}
	if query.Source != "" && query.Source != "messages" {
		return nil, fmt.Errorf("unsupported source")
	}
	if query.MessageID != "" {
		return a.workspaceMessageContent(ctx, taskID, query)
	}
	return a.workspaceMessagePage(ctx, taskID, query)
}

func (a *taskCreatorAdapter) workspaceMessageContent(ctx context.Context, taskID string, query shared.WorkspaceContentQuery) (any, error) {
	message, err := a.taskSvc.GetMessage(ctx, query.MessageID)
	if err != nil || message.TaskID != taskID || (query.SessionID != "" && message.TaskSessionID != query.SessionID) {
		return nil, fmt.Errorf("message must belong to this task and session")
	}
	page := workspaceContentPage(workspaceMessageText(message), query)
	page["message_id"], page["session_id"], page["type"] = message.ID, message.TaskSessionID, message.Type
	return page, nil
}

func workspaceContentPage(content string, query shared.WorkspaceContentQuery) map[string]any {
	text := []rune(redaction.NewRedactor().String(content))
	start := min(query.Offset, len(text))
	end := min(start+query.Limit, len(text))
	return map[string]any{"content": string(text[start:end]), "offset": start, "next_offset": end, "has_more": end < len(text), "total_characters": len(text)}
}

func workspaceMessageText(message *models.Message) string {
	text := message.Content
	if message.Type == workspaceClarificationRequest {
		input := map[string]any{}
		for _, key := range []string{"question", "response", "status"} {
			input[key] = message.Metadata[key]
		}
		if raw, err := json.Marshal(input); err == nil {
			text += "\n" + string(raw)
		}
	}
	// Shell failures live in normalized output, not the command title.
	normalized, _ := message.Metadata["normalized"].(map[string]any)
	shell, _ := normalized["shell_exec"].(map[string]any)
	output, _ := shell["output"].(map[string]any)
	for _, key := range []string{"stdout", "stderr"} {
		if value, ok := output[key].(string); ok && value != "" {
			text += "\n" + value
		}
	}
	return text
}

func (a *taskCreatorAdapter) workspaceMessagePage(ctx context.Context, taskID string, query shared.WorkspaceContentQuery) (any, error) {
	sessions, err := a.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt) })
	if query.SessionID == "" && len(sessions) > 0 {
		query.SessionID = sessions[0].ID
	}
	found := false
	for _, session := range sessions {
		if session.ID == query.SessionID {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("session must belong to this task")
	}
	messages, more, err := a.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{TaskSessionID: query.SessionID, Limit: 4, Before: query.Before, Sort: "desc"})
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		text := workspaceMessageText(message)
		rows = append(rows, map[string]any{"id": message.ID, "type": message.Type, "author_type": message.AuthorType, "content": workspaceExportText(text, 1200), "truncated": len(text) > 1200, "requests_input": message.RequestsInput})
	}
	next := ""
	if more && len(messages) > 0 {
		next = messages[len(messages)-1].ID
	}
	return map[string]any{"session_id": query.SessionID, "messages": rows, "has_more": more, "next_before": next}, nil
}
