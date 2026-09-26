package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// ── Fetch/filter/sort helpers (v1 scoping: ADR 0043(a) "global-with-hook") ─

// resolveWorkspaceIDs returns requested unchanged when non-empty (an
// explicit filter always narrows), otherwise every workspace the instance
// holds — this is the "global reads, filters narrow results" v1 scoping
// rule, and the single hook a future per-plugin/per-user workspace
// restriction would replace.
func (h *pluginHost) resolveWorkspaceIDs(ctx context.Context, requested []string) ([]string, error) {
	if len(requested) > 0 {
		return requested, nil
	}
	workspaces, err := h.taskData.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugins: list workspaces: %w", err)
	}
	ids := make([]string, len(workspaces))
	for i, w := range workspaces {
		ids[i] = w.ID
	}
	return ids, nil
}

// fetchTasksForWorkspaces concatenates up to taskFetchPageSize tasks per
// workspace in workspaceIDs. excludeConfig is always true: office config-mode
// tasks (json_extract(metadata, '$.config_mode')) are internal bookkeeping,
// not plugin-visible work items.
func (h *pluginHost) fetchTasksForWorkspaces(ctx context.Context, workspaceIDs []string, includeEphemeral, includeArchived bool) ([]*taskmodels.Task, error) {
	var all []*taskmodels.Task
	for _, workspaceID := range workspaceIDs {
		tasks, err := h.fetchAllTasksForWorkspace(ctx, workspaceID, includeEphemeral, includeArchived)
		if err != nil {
			return nil, err
		}
		all = append(all, tasks...)
	}
	return all, nil
}

// fetchAllTasksForWorkspace loops ListTasksByWorkspace's page/pageSize
// pagination to completion for a single workspace, so a workspace with more
// tasks than one taskFetchPageSize page is never silently truncated (and
// this reader's downstream HasMore/paginate stays accurate). Bounded by
// ListTasksByWorkspace's own returned total, plus a break on an empty page
// as a defensive guard against ever looping forever on an inconsistent total.
func (h *pluginHost) fetchAllTasksForWorkspace(ctx context.Context, workspaceID string, includeEphemeral, includeArchived bool) ([]*taskmodels.Task, error) {
	var out []*taskmodels.Task
	for page := 1; ; page++ {
		tasks, total, err := h.taskData.ListTasksByWorkspace(
			ctx, workspaceID, "", "", "", page, taskFetchPageSize, "",
			includeArchived, includeEphemeral, false, true,
		)
		if err != nil {
			return nil, fmt.Errorf("plugins: list tasks for workspace %q: %w", workspaceID, err)
		}
		if len(tasks) == 0 {
			break
		}
		out = append(out, tasks...)
		if len(out) >= total {
			break
		}
	}
	return out, nil
}

// filterTasks applies TaskFilter's WorkflowIDs/States/ParentID narrowing
// on top of the already-workspace-scoped tasks fetchTasksForWorkspaces
// returned (WorkspaceIDs and IncludeEphemeral are applied earlier, at fetch
// time).
func filterTasks(tasks []*taskmodels.Task, filter pluginsdk.TaskFilter) []*taskmodels.Task {
	if len(filter.WorkflowIDs) == 0 && len(filter.States) == 0 && filter.ParentID == nil {
		return tasks
	}
	workflowSet := toSet(filter.WorkflowIDs)
	stateSet := toSet(filter.States)
	out := make([]*taskmodels.Task, 0, len(tasks))
	for _, t := range tasks {
		if len(workflowSet) > 0 && !workflowSet[t.WorkflowID] {
			continue
		}
		if len(stateSet) > 0 && !stateSet[string(t.State)] {
			continue
		}
		if filter.ParentID != nil && t.ParentID != *filter.ParentID {
			continue
		}
		out = append(out, t)
	}
	return out
}

func toSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// sortTasksNewestFirst orders by CreatedAt descending, tie-broken by ID
// ascending: sort.Slice is unstable, and offset-cursor pagination needs a
// total order across calls — two tasks with equal CreatedAt (a plausible
// seed/batch-import scenario) must land in the same relative position on
// every call, or an offset page can skip or duplicate one across reads.
func sortTasksNewestFirst(tasks []*taskmodels.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func tasksToDTOs(tasks []*taskmodels.Task) []pluginsdk.Task {
	out := make([]pluginsdk.Task, len(tasks))
	for i, t := range tasks {
		out[i] = taskModelToDTO(t)
	}
	return out
}

// fetchSessionsForFilter resolves the task ids to list sessions for (see
// resolveSessionTaskIDs) and lists each task's sessions — a Host data API
// session read is, unavoidably, an N+1 fan-out over the resolved tasks in v1
// (no session listing endpoint spans multiple tasks directly at the service
// layer today).
func (h *pluginHost) fetchSessionsForFilter(ctx context.Context, filter pluginsdk.SessionFilter) ([]*taskmodels.TaskSession, error) {
	taskIDs, err := h.resolveSessionTaskIDs(ctx, filter)
	if err != nil {
		return nil, err
	}

	var sessions []*taskmodels.TaskSession
	for _, taskID := range taskIDs {
		s, err := h.taskData.ListTaskSessions(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("plugins: list sessions for task %q: %w", taskID, err)
		}
		sessions = append(sessions, s...)
	}
	return sessions, nil
}

// resolveSessionTaskIDs returns the task ids fetchSessionsForFilter should
// list sessions for. filter.TaskIDs and filter.WorkspaceIDs are ANDed
// together, never one bypassing the other: with both set, a task id whose
// task lives outside the requested workspaces is dropped, so it can't leak
// sessions from a workspace the caller didn't ask for. With only TaskIDs set,
// they're used as-is (unscoped by design — an explicit task id is itself a
// narrowing filter). With neither set (or only WorkspaceIDs), every task
// across resolveWorkspaceIDs(filter.WorkspaceIDs) is enumerated.
// includeEphemeral AND includeArchived when enumerating by workspace: a
// session is still a real session from the Sessions resource's point of view
// whether its task is a quick-chat or has since been archived. CodeStats
// makes the same call at the SQL layer (no task-state filtering), so the two
// session reads stay consistent.
func (h *pluginHost) resolveSessionTaskIDs(ctx context.Context, filter pluginsdk.SessionFilter) ([]string, error) {
	if len(filter.TaskIDs) == 0 {
		workspaceIDs, err := h.resolveWorkspaceIDs(ctx, filter.WorkspaceIDs)
		if err != nil {
			return nil, err
		}
		tasks, err := h.fetchTasksForWorkspaces(ctx, workspaceIDs, true, true)
		if err != nil {
			return nil, err
		}
		taskIDs := make([]string, len(tasks))
		for i, t := range tasks {
			taskIDs[i] = t.ID
		}
		return taskIDs, nil
	}
	if len(filter.WorkspaceIDs) == 0 {
		return filter.TaskIDs, nil
	}
	return h.filterTaskIDsByWorkspace(ctx, filter.TaskIDs, filter.WorkspaceIDs)
}

// filterTaskIDsByWorkspace keeps only the taskIDs whose task's WorkspaceID is
// in workspaceIDs; a taskID that no longer resolves to a task is dropped
// (not an error) — a session read shouldn't fail just because a stale id was
// passed in TaskIDs.
func (h *pluginHost) filterTaskIDsByWorkspace(ctx context.Context, taskIDs, workspaceIDs []string) ([]string, error) {
	allowed := toSet(workspaceIDs)
	out := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		task, err := h.taskData.GetTask(ctx, taskID)
		if err != nil {
			if errors.Is(err, repoerrors.ErrTaskNotFound) {
				continue
			}
			return nil, fmt.Errorf("plugins: get task %q: %w", taskID, err)
		}
		if allowed[task.WorkspaceID] {
			out = append(out, taskID)
		}
	}
	return out, nil
}

func filterSessionsByState(sessions []*taskmodels.TaskSession, states []string) []*taskmodels.TaskSession {
	if len(states) == 0 {
		return sessions
	}
	set := toSet(states)
	out := make([]*taskmodels.TaskSession, 0, len(sessions))
	for _, s := range sessions {
		if set[string(s.State)] {
			out = append(out, s)
		}
	}
	return out
}

// sortSessionsNewestFirst mirrors sortTasksNewestFirst's ID tie-break, for
// the same offset-cursor pagination stability reason.
func sortSessionsNewestFirst(sessions []*taskmodels.TaskSession) {
	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].StartedAt.Equal(sessions[j].StartedAt) {
			return sessions[i].StartedAt.After(sessions[j].StartedAt)
		}
		return sessions[i].ID < sessions[j].ID
	})
}
