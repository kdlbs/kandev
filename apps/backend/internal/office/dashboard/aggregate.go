package dashboard

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// WorkspaceLister returns the workspaces visible to the calling identity.
// Implemented by the task service's identity-scoped ListWorkspaces, so the
// aggregate never lists a workspace the caller cannot reach.
type WorkspaceLister interface {
	ListWorkspaces(ctx context.Context) ([]*taskmodels.Workspace, error)
}

// ErrWorkspaceAggregateUnavailable is returned when the aggregate endpoint is
// reached without a wired workspace lister.
var ErrWorkspaceAggregateUnavailable = errors.New("multi-workspace aggregate is not configured")

// aggregateActivityLimit bounds the merged recent-activity feed returned with
// the aggregate, regardless of workspace count.
const aggregateActivityLimit = 20

// WorkspaceAggregateEntry is one workspace row in the multi-workspace
// aggregate overview (GET /workspaces/aggregate).
type WorkspaceAggregateEntry struct {
	WorkspaceID      string `json:"workspace_id"`
	Name             string `json:"name"`
	TaskCount        int    `json:"task_count"`
	OpenTasks        int    `json:"open_tasks"`
	InProgressTasks  int    `json:"in_progress_tasks"`
	BlockedTasks     int    `json:"blocked_tasks"`
	DoneTasks        int    `json:"done_tasks"`
	PendingApprovals int    `json:"pending_approvals"`
	AgentCount       int    `json:"agent_count"`
	RunningAgents    int    `json:"running_agents"`
}

// WorkspaceAggregateResponse is the read-only multi-workspace overview: one
// entry per visible workspace plus a merged recent-activity feed.
type WorkspaceAggregateResponse struct {
	Workspaces     []WorkspaceAggregateEntry `json:"workspaces"`
	RecentActivity []*models.ActivityEntry   `json:"recent_activity"`
}

// workspaceAgentCounts holds the per-workspace agent totals for the aggregate.
type workspaceAgentCounts struct {
	total   int
	running int
}

// GetWorkspacesAggregate builds the read-only Office overview from the
// identity-scoped workspace list, using batched repository reads per dimension.
// Counts default to zero for a workspace with no matching rows; the activity
// feed is merged across the selected Office workspaces, newest first.
func (s *DashboardService) GetWorkspacesAggregate(ctx context.Context) (*WorkspaceAggregateResponse, error) {
	if s.workspaceLister == nil {
		return nil, ErrWorkspaceAggregateUnavailable
	}
	workspaces, err := s.workspaceLister.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}

	ordered, ids := officeAggregateWorkspaces(workspaces)

	breakdowns, err := s.repo.QueryWorkspaceTaskBreakdowns(ctx, ids)
	if err != nil {
		return nil, err
	}
	approvals, err := s.repo.CountPendingApprovalsByWorkspaces(ctx, ids)
	if err != nil {
		return nil, err
	}
	activity, err := s.repo.ListActivityEntriesForWorkspaces(ctx, ids, aggregateActivityLimit)
	if err != nil {
		return nil, err
	}
	activityByWorkspace := make(map[string][]*models.ActivityEntry)
	for _, entry := range activity {
		if entry != nil {
			activityByWorkspace[entry.WorkspaceID] = append(activityByWorkspace[entry.WorkspaceID], entry)
		}
	}
	for workspaceID, entries := range activityByWorkspace {
		s.enrichActivityLabels(ctx, workspaceID, entries, nil)
	}
	agentCounts, err := s.aggregateAgentCounts(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]WorkspaceAggregateEntry, 0, len(ordered))
	for _, w := range ordered {
		bd := breakdowns[w.ID]
		agents := agentCounts[w.ID]
		entries = append(entries, WorkspaceAggregateEntry{
			WorkspaceID:      w.ID,
			Name:             w.Name,
			TaskCount:        bd.Open + bd.InProgress + bd.Blocked + bd.Done,
			OpenTasks:        bd.Open,
			InProgressTasks:  bd.InProgress,
			BlockedTasks:     bd.Blocked,
			DoneTasks:        bd.Done,
			PendingApprovals: approvals[w.ID],
			AgentCount:       agents.total,
			RunningAgents:    agents.running,
		})
	}
	return &WorkspaceAggregateResponse{Workspaces: entries, RecentActivity: activity}, nil
}

func officeAggregateWorkspaces(workspaces []*taskmodels.Workspace) ([]*taskmodels.Workspace, []string) {
	ids := make([]string, 0, len(workspaces))
	ordered := make([]*taskmodels.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil || workspace.ID == "" || workspace.OfficeWorkflowID == "" {
			continue
		}
		ids = append(ids, workspace.ID)
		ordered = append(ordered, workspace)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered, ids
}

// aggregateAgentCounts totals and running-agents per workspace from the shared
// agent reader (empty workspace id = across all workspaces). A nil agent
// reader or a read failure yields an empty map, so every count stays zero.
func (s *DashboardService) aggregateAgentCounts(ctx context.Context) (map[string]workspaceAgentCounts, error) {
	out := map[string]workspaceAgentCounts{}
	if s.agents == nil {
		return out, nil
	}
	agents, err := s.agents.ListAgentInstances(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a == nil || a.WorkspaceID == "" {
			continue
		}
		c := out[a.WorkspaceID]
		c.total++
		if a.Status == models.AgentStatusWorking {
			c.running++
		}
		out[a.WorkspaceID] = c
	}
	return out, nil
}

// getWorkspacesAggregate serves GET /workspaces/aggregate. It is a read-only
// overview of every Office workspace the caller can reach; the workspace list
// is already identity-scoped by the lister, so this handler does no
// per-workspace ownership loop.
func (h *Handler) getWorkspacesAggregate(c *gin.Context) {
	resp, err := h.svc.GetWorkspacesAggregate(c.Request.Context())
	if err != nil {
		if errors.Is(err, ErrWorkspaceAggregateUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
