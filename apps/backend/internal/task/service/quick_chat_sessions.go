package service

import (
	"context"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// Quick-chat tab kinds. A workspace config chat is a quick chat whose task
// carries the config_mode metadata flag; everything else is a regular chat.
const (
	QuickChatKindChat   = "chat"
	QuickChatKindConfig = "config"

	// quickChatDefaultTitle is the placeholder title used when a quick chat is
	// started without a name. It is not surfaced as a tab label.
	quickChatDefaultTitle = "Quick Chat"

	quickChatListPageSize = 1000
)

// QuickChatSession is one restorable quick-chat tab: the ephemeral task's
// primary session plus the metadata the UI needs to render its tab.
type QuickChatSession struct {
	SessionID      string
	TaskID         string
	WorkspaceID    string
	Kind           string
	Name           string
	AgentProfileID string
	Session        *models.TaskSession

	createdAt time.Time
}

// ListQuickChatSessions returns the workspace's restorable quick-chat sessions,
// oldest created first. It is the single source of truth for the quick
// chat tab strip: the boot payload and the runtime resync endpoint both read it,
// so a client that reloads and a client that resyncs converge on the same list.
//
// Quick-chat names are user-authored text, so the workspace check belongs here
// rather than in the callers: the resync endpoint takes its workspace ID
// straight from the URL. Today listQuickChatTasks also happens to authorize on
// the way through ListTasksByWorkspace, but that is an implementation detail of
// how this list is assembled, not a guarantee this function should lean on.
func (s *Service) ListQuickChatSessions(ctx context.Context, workspaceID string) ([]QuickChatSession, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	tasks, err := s.listQuickChatTasks(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return []QuickChatSession{}, nil
	}
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			ids = append(ids, task.ID)
		}
	}
	primaryByTask, err := s.GetPrimarySessionInfoForTasks(ctx, ids)
	if err != nil {
		return nil, err
	}

	items := make([]QuickChatSession, 0, len(tasks))
	for _, task := range tasks {
		if !IsRestorableQuickChatTask(task) {
			continue
		}
		primary := primaryByTask[task.ID]
		if primary == nil || primary.ID == "" {
			continue
		}
		items = append(items, newQuickChatSession(task, primary))
	}
	sortQuickChatSessions(items)
	return items, nil
}

func newQuickChatSession(task *models.Task, primary *models.TaskSession) QuickChatSession {
	item := QuickChatSession{
		SessionID:      primary.ID,
		TaskID:         task.ID,
		WorkspaceID:    task.WorkspaceID,
		Kind:           quickChatSessionKind(task),
		AgentProfileID: quickChatAgentProfileID(task, primary),
		Session:        primary,
		createdAt:      task.CreatedAt,
	}
	if task.Title != "" && task.Title != quickChatDefaultTitle {
		item.Name = task.Title
	}
	return item
}

// sortQuickChatSessions orders tabs by creation time and then task ID. Activity
// updates task data but must not move a tab in the presentation baseline.
func sortQuickChatSessions(items []QuickChatSession) {
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].createdAt.Equal(items[j].createdAt) {
			return items[i].createdAt.Before(items[j].createdAt)
		}
		return items[i].TaskID < items[j].TaskID
	})
}

func (s *Service) listQuickChatTasks(ctx context.Context, workspaceID string) ([]*models.Task, error) {
	var all []*models.Task
	for page := 1; ; page++ {
		tasks, total, err := s.ListTasksByWorkspace(
			ctx, workspaceID, "", "", "", page, quickChatListPageSize, "", false, false, true, false,
		)
		if err != nil {
			return nil, err
		}
		all = append(all, tasks...)
		if len(tasks) == 0 || len(all) >= total {
			return all, nil
		}
	}
}

// IsRestorableQuickChatTask reports whether a task backs a quick-chat tab.
// Workflow-bound and automation-run tasks are ephemeral for other reasons and
// must never surface in the tab strip.
func IsRestorableQuickChatTask(task *models.Task) bool {
	return task != nil &&
		task.IsEphemeral &&
		task.WorkflowID == "" &&
		task.Origin != models.TaskOriginAutomationRun
}

func quickChatSessionKind(task *models.Task) string {
	if task != nil {
		if configMode, ok := task.Metadata["config_mode"].(bool); ok && configMode {
			return QuickChatKindConfig
		}
	}
	return QuickChatKindChat
}

func quickChatAgentProfileID(task *models.Task, primary *models.TaskSession) string {
	if task != nil {
		if value, ok := task.Metadata[models.MetaKeyAgentProfileID].(string); ok {
			return value
		}
	}
	if primary != nil {
		return primary.AgentProfileID
	}
	return ""
}
