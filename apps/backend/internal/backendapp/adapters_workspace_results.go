package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator/dispatchcontext"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/share"
	"sort"
	"strings"
)

const (
	workspaceResultStateKey = "state"
	workspaceResultTaskKey  = "task"
	workspaceTasksKey       = "tasks"
	credentialUnavailable   = "unavailable"
	workspaceWorkflowsKey   = "workflows"
)

const (
	workspaceResultContentKey     = "content"
	workspaceResultSortDescending = "desc"
	workspaceRepositoriesKey      = "repositories"
)

// WorkspaceTaskDetails exposes bounded worker results without copying tool transcripts.
func (a *taskCreatorAdapter) WorkspaceTaskDetails(ctx context.Context, workspaceID, taskID string) (any, error) {
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("task must belong to this workspace")
	}
	sessions, err := a.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{workspaceResultTaskKey: workspaceTaskDetailSummary(task), "sessions": workspaceSessionSummaries(sessions)}
	completionErr := a.ValidateAssistantTaskCompletion(ctx, workspaceID, taskID)
	result["completion_ready"] = completionErr == nil
	if completionErr != nil {
		result["completion_blocker"] = completionErr.Error()
	}
	if err := a.attachWorkspaceSessionResults(ctx, result, sessions); err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return result, nil
	}
	return result, a.attachLatestWorkspaceMessages(ctx, result, sessions)
}

func (a *taskCreatorAdapter) attachLatestWorkspaceMessages(ctx context.Context, result map[string]any, sessions []*models.TaskSession) error {
	latest := sessions[0]
	for _, session := range sessions[1:] {
		if session.UpdatedAt.After(latest.UpdatedAt) {
			latest = session
		}
	}
	messages, more, err := a.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{TaskSessionID: latest.ID, Limit: 20, Sort: workspaceResultSortDescending, AuthorType: string(models.MessageAuthorAgent)})
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, 6)
	for _, message := range messages {
		if message.Type != "message" && !message.RequestsInput {
			continue
		}
		if len(rows) == 1 {
			more = true
			break
		}
		row := map[string]any{"id": message.ID, "author_type": message.AuthorType, workspaceResultContentKey: workspaceExportText(message.Content, 4000), "requests_input": message.RequestsInput, "truncated": len(message.Content) > 4000}
		rows = append(rows, row)
	}
	result["messages"], result["has_more"], result[sessionIDPayloadKey] = rows, more, latest.ID
	return nil
}

func (a *taskCreatorAdapter) attachWorkspaceSessionResults(ctx context.Context, result map[string]any, sessions []*models.TaskSession) error {
	ordered := append([]*models.TaskSession(nil), sessions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	result["has_more_sessions"] = len(ordered) > 8
	if len(ordered) > 8 {
		ordered = ordered[:8]
	}
	rows := make([]map[string]any, 0, len(ordered))
	for _, session := range ordered {
		messages, more, err := a.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{TaskSessionID: session.ID, Limit: 10, Sort: workspaceResultSortDescending, AuthorType: string(models.MessageAuthorAgent)})
		if err != nil {
			return err
		}
		excerpts := make([]map[string]any, 0, 2)
		for _, m := range messages {
			if m.Type != models.MessageTypeMessage {
				continue
			}
			if len(excerpts) == 2 {
				more = true
				break
			}
			excerpts = append(excerpts, map[string]any{"id": m.ID, workspaceResultContentKey: workspaceExportText(m.Content, 256), "author_type": m.AuthorType, "truncated": len(m.Content) > 256})
		}
		rows = append(rows, map[string]any{sessionIDPayloadKey: session.ID, "profile_id": session.AgentProfileID, workspaceResultStateKey: session.State, "review_status": session.ReviewStatus, "messages": excerpts, "has_more": more})
	}
	result["session_results"] = rows
	return nil
}

func (a *taskCreatorAdapter) messageWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if a.orch == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	if strings.TrimSpace(command.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	command.Prompt = assistantDelegationPrompt(command.Prompt, command.DelegationReference)
	session, err := a.taskSvc.GetTaskSession(ctx, command.SessionID)
	if err != nil || session.TaskID != task.ID {
		return fmt.Errorf("session must belong to this task")
	}
	if command.Packet != nil {
		if command.Packet.ProfileID != session.AgentProfileID {
			return fmt.Errorf("context account must match the target session")
		}
		ctx = dispatchcontext.WithReference(ctx, command.ContextRef)
	}
	pinned, _ := task.Metadata["orchestration_execution_profile_id"].(string)
	managed, _ := task.Metadata["orchestration_managed"].(bool)
	if !managed && pinned != "" && session.AgentProfileID != pinned {
		return fmt.Errorf("session does not use the task's pinned account")
	}
	if session.State == models.TaskSessionStateRunning {
		_, err = a.orch.SteerTask(ctx, task.ID, session.ID, command.Prompt, "", false, nil)
	} else {
		_, err = a.orch.PromptTask(ctx, task.ID, session.ID, command.Prompt, "", false, nil, false)
	}
	return err
}

func assistantDelegationPrompt(prompt string, ref shared.DelegationReference) string {
	if ref.ObjectiveID == "" {
		return prompt
	}
	var b strings.Builder
	b.WriteString(share.NewRedactor().String(prompt))
	if ref.Packet != nil {
		raw, _ := json.Marshal(ref.Packet)
		fmt.Fprintf(&b, "\n\n<assistant-context>\n%s\n</assistant-context>\n", raw)
	}
	fmt.Fprintf(&b, "\n\nAssistant objective: %s (acceptance revision %d)\nSource comment: %s\nContext reference: %s\n", ref.ObjectiveID, ref.AcceptanceRevision, ref.SourceCommentID, ref.ContextRef)
	for _, c := range ref.Acceptance {
		fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Description)
	}
	b.WriteString("Repository instructions and required design/review handoffs remain authoritative. A workflow entry selection does not waive them. Return evidence and unresolved decisions for these acceptance criteria.")
	return b.String()
}
