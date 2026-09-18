package backendapp

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/common/redaction"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"sort"
	"unicode/utf8"
)

const workspaceDirectoryRepository = "repository"

func (a *taskCreatorAdapter) AssistantWorkspaceDirectory(ctx context.Context, workspace string) ([]shared.WorkspaceDirectoryEntry, error) {
	if _, err := a.WorkspaceGrantAccess(ctx, workspace, false); err != nil {
		return nil, err
	}
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspace, false)
	if err != nil {
		return nil, err
	}
	rows := []shared.WorkspaceDirectoryEntry{}
	visible := map[string]bool{}
	for _, w := range workflows {
		visible[w.ID] = true
		rows = append(rows, shared.WorkspaceDirectoryEntry{ID: w.ID, Kind: "workflow", Name: workspaceExportText(w.Name, 200)})
	}
	repositories, err := a.taskSvc.ListRepositories(ctx, workspace)
	if err != nil {
		return nil, err
	}
	for _, r := range repositories {
		rows = append(rows, shared.WorkspaceDirectoryEntry{ID: r.ID, Kind: workspaceDirectoryRepository, Name: workspaceExportText(r.Name, 200)})
	}
	if a.workflow != nil {
		steps, err := a.workflow.ListStepsByWorkspaceID(ctx, workspace)
		if err != nil {
			return nil, err
		}
		for _, step := range steps {
			if visible[step.WorkflowID] {
				rows = append(rows, shared.WorkspaceDirectoryEntry{ID: step.ID, Kind: "workflow_step", Name: workspaceExportText(step.Name, 200), WorkflowID: step.WorkflowID, EntryAllowed: step.IsStartStep || step.AllowManualMove})
			}
		}
	}
	return rows, nil
}

func (a *taskCreatorAdapter) AssistantWorkspaceTask(ctx context.Context, workspace, taskID string, includeResult bool) (shared.WorkspaceTaskView, error) {
	view := shared.WorkspaceTaskView{}
	if _, err := a.WorkspaceGrantAccess(ctx, workspace, false); err != nil {
		return view, err
	}
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspace || task.IsEphemeral || task.IsFromOffice {
		return view, fmt.Errorf("delivery task unavailable")
	}
	view.Task = workspaceTaskSummary(task)
	if !includeResult {
		return view, nil
	}
	sessions, err := a.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return view, err
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID < sessions[j].ID })
	view.HasMore = len(sessions) > 8
	for _, s := range sessions[:min(len(sessions), 8)] {
		messages, more, err := a.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{TaskSessionID: s.ID, Limit: 10, AuthorType: string(models.MessageAuthorAgent), Sort: workspaceResultSortDescending})
		if err != nil {
			return view, err
		}
		view.HasMore = view.HasMore || more
		for _, m := range messages {
			if m.Type != models.MessageTypeMessage || m.AuthorType != models.MessageAuthorAgent || m.RequestsInput {
				continue
			}
			view.Results = append(view.Results, shared.WorkspaceTaskResult{ID: m.ID, SessionID: s.ID, ProfileID: s.AgentProfileID, State: string(s.State), Content: workspaceExportText(m.Content, 2000), Truncated: len(m.Content) > 2000})
			break
		}
	}
	return view, nil
}

func (a *taskCreatorAdapter) AssistantWorkspaceTasks(ctx context.Context, workspace string, page, limit int) ([]shared.WorkspaceTaskSummary, bool, error) {
	if _, err := a.WorkspaceGrantAccess(ctx, workspace, false); err != nil {
		return nil, false, err
	}
	if page < 1 || page > 100000 || limit < 1 || limit > 100 {
		return nil, false, fmt.Errorf("invalid task page")
	}
	tasks, total, err := a.taskSvc.ListTasksByWorkspace(ctx, workspace, "", "", "", page, limit, "updated_at_desc", false, false, false, true)
	if err != nil {
		return nil, false, err
	}
	rows := []shared.WorkspaceTaskSummary{}
	for _, task := range tasks {
		if !task.IsEphemeral && !task.IsFromOffice {
			rows = append(rows, workspaceTaskSummary(task))
		}
	}
	return rows, page*limit < total, nil
}

func workspaceTaskSummary(task *models.Task) shared.WorkspaceTaskSummary {
	return shared.WorkspaceTaskSummary{ID: task.ID, WorkspaceID: task.WorkspaceID, Title: workspaceExportText(task.Title, 300), State: string(task.State), WorkflowID: task.WorkflowID, WorkflowStepID: task.WorkflowStepID}
}

func workspaceExportText(value string, limit int) string {
	value = redaction.NewRedactor().String(value)
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}
