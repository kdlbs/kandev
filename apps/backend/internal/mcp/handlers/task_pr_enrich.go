package handlers

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskPRInfo is the decoupled view of a task↔PR association consumed by the
// MCP task-listing handlers. The cmd wiring adapts *github.Service.TaskPR into
// this shape so this package does not depend on internal/github.
type TaskPRInfo struct {
	Number   int
	URL      string
	Title    string
	State    string // open, closed, merged
	MergedAt *time.Time
}

// TaskPRLister returns PR associations grouped by task ID. Backed by
// *github.Service in production; left nil in contexts without GitHub, in which
// case PRs are simply omitted from task responses.
type TaskPRLister interface {
	ListTaskPRsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]TaskPRInfo, error)
}

// SetTaskPRLister wires the optional PR lister used to enrich task-listing
// responses with associated pull requests.
func (h *Handlers) SetTaskPRLister(l TaskPRLister) {
	h.taskPRLister = l
}

// prsByTaskID looks up PR associations for the given task IDs via the wired
// lister, mapping each to the shared v1.TaskPRSummary shape. Returns nil when
// the lister is unset, there are no IDs, or the lookup fails (logged) — callers
// treat a nil map as "no PRs to attach".
func (h *Handlers) prsByTaskID(ctx context.Context, taskIDs []string) map[string][]v1.TaskPRSummary {
	if h.taskPRLister == nil || len(taskIDs) == 0 {
		return nil
	}
	byTask, err := h.taskPRLister.ListTaskPRsByTaskIDs(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to list task PRs for MCP response", zap.Error(err))
		return nil
	}
	out := make(map[string][]v1.TaskPRSummary, len(byTask))
	for taskID, prs := range byTask {
		if len(prs) == 0 {
			continue
		}
		summaries := make([]v1.TaskPRSummary, 0, len(prs))
		for _, pr := range prs {
			summaries = append(summaries, v1.TaskPRSummary{
				Number:   pr.Number,
				URL:      pr.URL,
				Title:    pr.Title,
				State:    pr.State,
				MergedAt: pr.MergedAt,
			})
		}
		out[taskID] = summaries
	}
	return out
}

// enrichTasksWithPRs populates dto.TaskDTO.PRs for each task from the wired
// TaskPRLister. No-op when the lister is unset or there are no tasks.
func (h *Handlers) enrichTasksWithPRs(ctx context.Context, tasks []dto.TaskDTO) {
	if h.taskPRLister == nil || len(tasks) == 0 {
		return
	}
	ids := make([]string, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}
	byTask := h.prsByTaskID(ctx, ids)
	for i := range tasks {
		if prs := byTask[tasks[i].ID]; len(prs) > 0 {
			tasks[i].PRs = prs
		}
	}
}

// enrichRelatedTasksWithPRs populates RelatedTask.PRs across every relation
// surface (the task itself, parent, children, siblings, blockers, blocked-by)
// in a single batched lookup. No-op when the lister is unset.
func (h *Handlers) enrichRelatedTasksWithPRs(ctx context.Context, related *service.RelatedTasks) {
	if h.taskPRLister == nil || related == nil {
		return
	}
	nodes := []*service.RelatedTask{&related.Task, related.Parent}
	nodes = append(nodes, related.Children...)
	nodes = append(nodes, related.Siblings...)
	nodes = append(nodes, related.Blockers...)
	nodes = append(nodes, related.BlockedBy...)

	// A task can appear in more than one relation group (e.g. both a sibling
	// and a blocker), so dedup IDs before the lookup to avoid sending the same
	// value twice to the lister's WHERE id IN (...) query.
	ids := make([]string, 0, len(nodes))
	seen := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if _, dup := seen[n.ID]; dup {
			continue
		}
		seen[n.ID] = struct{}{}
		ids = append(ids, n.ID)
	}
	byTask := h.prsByTaskID(ctx, ids)
	if byTask == nil {
		return
	}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if prs := byTask[n.ID]; len(prs) > 0 {
			n.PRs = prs
		}
	}
}
