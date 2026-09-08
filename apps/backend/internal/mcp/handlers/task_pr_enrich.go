package handlers

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskPRInfo is the decoupled view of a task<->PR association consumed by the
// MCP task-listing handlers. The cmd wiring adapts *github.Service.TaskPR into
// this shape so this package does not depend on internal/github.
type TaskPRInfo struct {
	RepositoryID string
	Number       int
	URL          string
	Title        string
	State        string // open, closed, merged
	Draft        *bool
	BaseRef      string
	HeadRef      string
	MergedAt     *time.Time
	ClosedAt     *time.Time
}

// TaskMRInfo is the decoupled view of a GitLab task<->MR association consumed
// by the MCP task-listing handlers.
type TaskMRInfo struct {
	RepositoryID string
	Number       int
	URL          string
	Title        string
	State        string
	Draft        bool
	BaseRef      string
	HeadRef      string
	MergedAt     *time.Time
	ClosedAt     *time.Time
}

// TaskPRLister returns PR associations grouped by task ID. Backed by
// *github.Service in production; left nil in contexts without GitHub, in which
// case PRs are simply omitted from task responses.
type TaskPRLister interface {
	ListTaskPRsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]TaskPRInfo, error)
}

// TaskMRLister returns MR associations grouped by task ID. Backed by
// *gitlab.Service in production; left nil in contexts without GitLab.
type TaskMRLister interface {
	ListTaskMRsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]TaskMRInfo, error)
}

// SetTaskPRLister wires the optional PR lister used to enrich task-listing
// responses with associated pull requests.
func (h *Handlers) SetTaskPRLister(l TaskPRLister) {
	h.taskPRLister = l
}

// SetTaskMRLister wires the optional MR lister used to enrich task-listing
// responses with associated merge requests.
func (h *Handlers) SetTaskMRLister(l TaskMRLister) {
	h.taskMRLister = l
}

type taskChangeRequestsByID struct {
	prs     map[string][]v1.TaskPRSummary
	changes map[string][]v1.TaskChangeRequestSummary
}

func (h *Handlers) changeRequestsByTaskID(ctx context.Context, taskIDs []string) taskChangeRequestsByID {
	out := taskChangeRequestsByID{
		prs:     map[string][]v1.TaskPRSummary{},
		changes: map[string][]v1.TaskChangeRequestSummary{},
	}
	if len(taskIDs) == 0 {
		return out
	}
	if h.taskPRLister != nil {
		h.addPRSummaries(ctx, taskIDs, out)
	}
	if h.taskMRLister != nil {
		h.addMRSummaries(ctx, taskIDs, out)
	}
	return out
}

func (h *Handlers) addPRSummaries(ctx context.Context, taskIDs []string, out taskChangeRequestsByID) {
	byTask, err := h.taskPRLister.ListTaskPRsByTaskIDs(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to list task PRs for MCP response", zap.Error(err))
		return
	}
	for taskID, prs := range byTask {
		for _, pr := range prs {
			out.prs[taskID] = append(out.prs[taskID], v1.TaskPRSummary{
				Number:   pr.Number,
				URL:      pr.URL,
				Title:    pr.Title,
				State:    pr.State,
				MergedAt: pr.MergedAt,
			})
			out.changes[taskID] = append(out.changes[taskID], v1.TaskChangeRequestSummary{
				Provider:     "github",
				RepositoryID: pr.RepositoryID,
				Number:       pr.Number,
				URL:          pr.URL,
				Title:        pr.Title,
				State:        pr.State,
				Draft:        pr.Draft,
				BaseRef:      pr.BaseRef,
				HeadRef:      pr.HeadRef,
				MergedAt:     pr.MergedAt,
				ClosedAt:     pr.ClosedAt,
			})
		}
	}
}

func (h *Handlers) addMRSummaries(ctx context.Context, taskIDs []string, out taskChangeRequestsByID) {
	byTask, err := h.taskMRLister.ListTaskMRsByTaskIDs(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to list task MRs for MCP response", zap.Error(err))
		return
	}
	for taskID, mrs := range byTask {
		for _, mr := range mrs {
			draft := mr.Draft
			out.changes[taskID] = append(out.changes[taskID], v1.TaskChangeRequestSummary{
				Provider:     "gitlab",
				RepositoryID: mr.RepositoryID,
				Number:       mr.Number,
				URL:          mr.URL,
				Title:        mr.Title,
				State:        mr.State,
				Draft:        &draft,
				BaseRef:      mr.BaseRef,
				HeadRef:      mr.HeadRef,
				MergedAt:     mr.MergedAt,
				ClosedAt:     mr.ClosedAt,
			})
		}
	}
}

// enrichTasksWithPRs populates dto.TaskDTO.PRs for compatibility and
// dto.TaskDTO.ChangeRequests with provider-neutral GitHub/GitLab links.
func (h *Handlers) enrichTasksWithPRs(ctx context.Context, tasks []dto.TaskDTO) {
	if (h.taskPRLister == nil && h.taskMRLister == nil) || len(tasks) == 0 {
		return
	}
	ids := make([]string, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}
	byTask := h.changeRequestsByTaskID(ctx, ids)
	for i := range tasks {
		if prs := byTask.prs[tasks[i].ID]; len(prs) > 0 {
			tasks[i].PRs = prs
		}
		if changes := byTask.changes[tasks[i].ID]; len(changes) > 0 {
			tasks[i].ChangeRequests = changes
		}
	}
}

// enrichRelatedTasksWithPRs populates related-task PR compatibility data and
// provider-neutral GitHub/GitLab change requests in one batched lookup.
func (h *Handlers) enrichRelatedTasksWithPRs(ctx context.Context, related *service.RelatedTasks) {
	if (h.taskPRLister == nil && h.taskMRLister == nil) || related == nil {
		return
	}
	nodes := []*service.RelatedTask{&related.Task, related.Parent}
	nodes = append(nodes, related.Children...)
	nodes = append(nodes, related.Siblings...)
	nodes = append(nodes, related.Blockers...)
	nodes = append(nodes, related.BlockedBy...)

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
	byTask := h.changeRequestsByTaskID(ctx, ids)
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if prs := byTask.prs[n.ID]; len(prs) > 0 {
			n.PRs = prs
		}
		if changes := byTask.changes[n.ID]; len(changes) > 0 {
			n.ChangeRequests = changes
		}
	}
}
