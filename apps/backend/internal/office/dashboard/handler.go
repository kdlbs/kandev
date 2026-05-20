package dashboard

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// labelFetcher is the subset of the label repository used by the dashboard handler.
type labelFetcher interface {
	ListLabelsForTask(ctx context.Context, taskID string) ([]*sqlite.Label, error)
	ListLabelsForTasks(ctx context.Context, taskIDs []string) (map[string][]*sqlite.Label, error)
}

// Handler provides HTTP handlers for dashboard, inbox, activity, run,
// task search, git, and meta routes.
type Handler struct {
	svc          *DashboardService
	labels       labelFetcher
	gitMgr       *configloader.GitManager
	runDetail    RunDetailRepo
	agentSummary AgentSummaryRepository
	logger       *logger.Logger
}

// NewHandler creates a new dashboard Handler.
// gitMgr may be nil if git operations are not configured.
// labelRepo may be nil; when nil, tasks are returned with empty label slices.
// labelRepo doubles as the RunDetailRepo and AgentSummaryRepository
// when it satisfies those interfaces (the production *sqlite.Repository
// does); a fake repo that does not implement them causes the
// corresponding endpoints to respond 503.
func NewHandler(svc *DashboardService, labelRepo labelFetcher, gitMgr *configloader.GitManager, log *logger.Logger) *Handler {
	h := &Handler{
		svc:    svc,
		labels: labelRepo,
		gitMgr: gitMgr,
		logger: log.WithFields(zap.String("component", "office-dashboard-handler")),
	}
	if r, ok := labelRepo.(RunDetailRepo); ok {
		h.runDetail = r
	}
	if r, ok := labelRepo.(AgentSummaryRepository); ok {
		h.agentSummary = r
	}
	return h
}

// RegisterRoutes registers all dashboard-related routes on the given router group.
func RegisterRoutes(api *gin.RouterGroup, svc *DashboardService, labelRepo labelFetcher, gitMgr *configloader.GitManager, log *logger.Logger) {
	h := NewHandler(svc, labelRepo, gitMgr, log)

	api.GET("/meta", h.getMeta)
	api.GET("/workspaces/:wsId/dashboard", h.getDashboard)
	api.GET("/workspaces/:wsId/live-runs", h.getLiveRuns)
	api.GET("/workspaces/:wsId/agent-summaries", h.getAgentSummaries)
	api.GET("/agents/:id/summary", h.getAgentSummary)
	api.GET("/agents/:id/runs", h.listAgentRuns)
	api.GET("/agents/:id/runs/:runId", h.getAgentRunDetail)
	api.GET("/workspaces/:wsId/inbox", h.getInbox)
	api.POST("/inbox/dismiss", h.dismissInboxItem)
	api.GET("/workspaces/:wsId/activity", h.listActivity)
	api.GET("/workspaces/:wsId/runs", h.listRuns)
	api.GET("/workspaces/:wsId/tasks/search", h.searchTasks)
	api.GET("/workspaces/:wsId/tasks", h.listTasks)
	api.GET("/workspaces/:wsId/settings", h.getWorkspaceSettings)
	api.PATCH("/workspaces/:wsId/settings", h.updateWorkspaceSettings)
	api.GET("/tasks/:id", h.getTask)
	api.PATCH("/tasks/:id", h.updateTask)
	api.GET("/tasks/:id/comments", h.listComments)
	api.POST("/tasks/:id/comments", h.createComment)
	api.POST("/tasks/:id/blockers", h.addTaskBlocker)
	api.DELETE("/tasks/:id/blockers/:blockerId", h.removeTaskBlocker)
	api.GET("/tasks/:id/reviewers", h.listTaskReviewers)
	api.POST("/tasks/:id/reviewers", h.addTaskReviewer)
	api.DELETE("/tasks/:id/reviewers/:agentId", h.removeTaskReviewer)
	api.GET("/tasks/:id/approvers", h.listTaskApprovers)
	api.POST("/tasks/:id/approvers", h.addTaskApprover)
	api.DELETE("/tasks/:id/approvers/:agentId", h.removeTaskApprover)
	api.POST("/tasks/:id/approve", h.approveTask)
	api.POST("/tasks/:id/request-changes", h.requestTaskChanges)
	api.GET("/tasks/:id/decisions", h.listTaskDecisions)
	api.POST("/workspaces/:wsId/git/clone", h.gitClone)
	api.POST("/workspaces/:wsId/git/pull", h.gitPull)
	api.POST("/workspaces/:wsId/git/push", h.gitPush)
	api.GET("/workspaces/:wsId/git/status", h.gitStatus)

	// Provider routing endpoints (office-provider-routing).
	api.GET("/workspaces/:wsId/routing", h.getWorkspaceRouting)
	api.PUT("/workspaces/:wsId/routing", h.updateWorkspaceRouting)
	api.POST("/workspaces/:wsId/routing/retry", h.retryWorkspaceProvider)
	api.GET("/workspaces/:wsId/routing/health", h.listWorkspaceRoutingHealth)
	api.GET("/workspaces/:wsId/routing/preview", h.getWorkspaceRoutingPreview)
	api.GET("/runs/:id/attempts", h.listRunAttempts)
	api.GET("/agents/:id/route", h.getAgentRoute)
}

// -- Dashboard --

func (h *Handler) getDashboard(c *gin.Context) {
	wsID := c.Param("wsId")
	ctx := c.Request.Context()

	// Fetch dashboard aggregate and per-agent summaries in parallel
	// (Stream A + C of office optimization). Each call internally batches
	// its sub-queries; running them concurrently halves the wall time.
	var (
		data      *models.DashboardData
		summaries []AgentSummary
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		d, err := h.svc.GetDashboardData(gctx, wsID)
		if err != nil {
			return err
		}
		data = d
		return nil
	})
	g.Go(func() error {
		s, err := h.svc.GetAgentSummaries(gctx, wsID)
		if err != nil {
			return err
		}
		summaries = s
		return nil
	})
	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if summaries == nil {
		summaries = []AgentSummary{}
	}

	runActivity := make([]RunActivityDay, len(data.RunActivity))
	for i, d := range data.RunActivity {
		runActivity[i] = RunActivityDay{
			Date:      d.Date,
			Succeeded: d.Succeeded,
			Failed:    d.Failed,
			Other:     d.Other,
		}
	}

	recentTasks := make([]RecentTaskDTO, len(data.RecentTasks))
	for i, t := range data.RecentTasks {
		recentTasks[i] = RecentTaskDTO{
			ID:                     t.ID,
			Identifier:             t.Identifier,
			Title:                  t.Title,
			Status:                 dbStateToOfficeStatus(t.Status),
			AssigneeAgentProfileID: t.AssigneeAgentProfileID,
			UpdatedAt:              t.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, DashboardResponse{
		AgentCount:         data.AgentCount,
		RunningCount:       data.RunningCount,
		PausedCount:        data.PausedCount,
		ErrorCount:         data.ErrorCount,
		MonthSpendSubcents: data.MonthSpendSubcents,
		PendingApprovals:   data.PendingApprovals,
		RecentActivity:     data.RecentActivity,
		TaskCount:          data.TaskCount,
		SkillCount:         data.SkillCount,
		RoutineCount:       data.RoutineCount,
		RunActivity:        runActivity,
		TaskBreakdown: TaskBreakdown{
			Open:       data.TaskBreakdown.Open,
			InProgress: data.TaskBreakdown.InProgress,
			Blocked:    data.TaskBreakdown.Blocked,
			Done:       data.TaskBreakdown.Done,
		},
		RecentTasks:    recentTasks,
		AgentSummaries: summaries,
	})
}

// -- Live runs --

func (h *Handler) getLiveRuns(c *gin.Context) {
	limit := 4
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	runs, err := h.svc.GetLiveRuns(c.Request.Context(), c.Param("wsId"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dtos := make([]LiveRunDTO, len(runs))
	for i, r := range runs {
		dtos[i] = LiveRunDTO(r)
	}
	c.JSON(http.StatusOK, LiveRunsResponse{Runs: dtos})
}

// -- Agent summaries --

// getAgentSummaries returns one card-shaped summary per workspace agent.
// Empty workspaces respond with `{"agents": []}` rather than null.
func (h *Handler) getAgentSummaries(c *gin.Context) {
	summaries, err := h.svc.GetAgentSummaries(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if summaries == nil {
		summaries = []AgentSummary{}
	}
	c.JSON(http.StatusOK, AgentSummariesResponse{Agents: summaries})
}

// -- Agent dashboard summary --

// getAgentSummary serves the agent dashboard's single-shot data
// payload at GET /agents/:id/summary?days=14. The window is clamped
// in [1, 90]; values outside the range are silently capped.
func (h *Handler) getAgentSummary(c *gin.Context) {
	if h.agentSummary == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent summary not configured"})
		return
	}
	agentID := c.Param("id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id required"})
		return
	}
	days := 14
	if v := c.Query("days"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			days = parsed
		}
	}
	summary, err := GetAgentSummary(c.Request.Context(), h.agentSummary, agentID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// -- Agent runs (paginated) + run detail --

// listAgentRuns serves GET /agents/:id/runs?cursor=&limit=. Default
// limit is 25, capped at 100. Cursor is the requested_at of the
// last row in the previous page (RFC3339); empty cursor returns
// page 1.
func (h *Handler) listAgentRuns(c *gin.Context) {
	if h.runDetail == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "run detail not configured"})
		return
	}
	agentID := c.Param("id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id required"})
		return
	}
	limit := 25
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	cursor := c.Query("cursor")
	cursorID := c.Query("cursor_id")
	resp, err := ListAgentRunsPaged(c.Request.Context(), h.runDetail, agentID, cursor, cursorID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// getAgentRunDetail serves GET /agents/:id/runs/:runId. Returns the
// per-run aggregate (header + cost rollup + session + invocation +
// tasks_touched + events). 404 when the run id is unknown or when
// the run belongs to a different agent.
func (h *Handler) getAgentRunDetail(c *gin.Context) {
	if h.runDetail == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "run detail not configured"})
		return
	}
	agentID := c.Param("id")
	runID := c.Param("runId")
	if agentID == "" || runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id and run id required"})
		return
	}
	resp, err := GetRunDetail(c.Request.Context(), h.runDetail, agentID, runID)
	if err != nil {
		switch {
		case errors.Is(err, ErrRunNotFound), errors.Is(err, ErrRunAgentMismatch):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Inbox, Activity, and Runs list handlers live in handler_inbox.go.

// -- Task search --

func (h *Handler) searchTasks(c *gin.Context) {
	wsID := c.Param("wsId")
	query := c.Query("q")

	limit := 50
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Empty query means "no text filter" — return the workspace's tasks
	// up to the limit. This matches what pickers (parent, blockers, …)
	// expect when populating their candidate lists, and avoids forcing
	// every caller to know two endpoints (search + list).
	var results []*sqlite.TaskSearchResult
	var err error
	if query == "" {
		results, err = h.svc.ListTasks(c.Request.Context(), wsID, includeSystemFromQuery(c))
		if len(results) > limit {
			results = results[:limit]
		}
	} else {
		results, err = h.svc.SearchTasks(c.Request.Context(), wsID, query, limit)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tasks := make([]*TaskSearchResultDTO, len(results))
	for i, r := range results {
		tasks[i] = &TaskSearchResultDTO{
			ID:                     r.ID,
			WorkspaceID:            r.WorkspaceID,
			Identifier:             r.Identifier,
			Title:                  r.Title,
			Description:            r.Description,
			Status:                 dbStateToOfficeStatus(r.Status),
			Priority:               normaliseTaskPriority(r.Priority),
			ParentID:               r.ParentID,
			ProjectID:              r.ProjectID,
			AssigneeAgentProfileID: r.AssigneeAgentProfileID,
			Labels:                 r.Labels,
			CreatedAt:              r.CreatedAt,
			UpdatedAt:              r.UpdatedAt,
		}
	}
	c.JSON(http.StatusOK, TaskSearchResponse{Tasks: tasks})
}

// -- Tasks --

func (h *Handler) listTasks(c *gin.Context) {
	ctx := c.Request.Context()
	wsID := c.Param("wsId")

	// Stream E (office optimization): when any filter / sort / pagination
	// query param is present, route through the keyset-paginated path.
	// Without query params, fall back to the legacy unbounded listing so
	// existing callers (kanban board, full task tree) keep working.
	if hasTaskFilterParams(c) {
		h.listTasksFiltered(c, wsID)
		return
	}

	tasks, err := h.svc.ListTasks(ctx, wsID, includeSystemFromQuery(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dtos := h.attachLabelsToTasks(ctx, tasks)
	c.JSON(http.StatusOK, TaskListResponse{Tasks: dtos})
}

// hasTaskFilterParams returns true when the request carries any of the
// Stream E task list query params (filter / sort / pagination).
func hasTaskFilterParams(c *gin.Context) bool {
	q := c.Request.URL.Query()
	for _, k := range []string{"status", "priority", "assignee", "project", "sort", "limit", "cursor"} {
		if q.Get(k) != "" {
			return true
		}
	}
	return false
}

// includeSystemFromQuery reads the `include_system` query param and
// returns true only for an explicit "true"/"1". Default false hides
// kandev-managed system tasks (coordination, future routines) from
// the Office Tasks UI.
func includeSystemFromQuery(c *gin.Context) bool {
	v := c.Query("include_system")
	return v == "true" || v == "1"
}

// listTasksFiltered handles the paginated/filtered task list. Returns the
// same JSON shape as the legacy handler, plus next_cursor / next_id when
// more pages are available.
func (h *Handler) listTasksFiltered(c *gin.Context, wsID string) {
	ctx := c.Request.Context()
	q := c.Request.URL.Query()
	opts := sqlite.ListTasksOptions{
		Status:        q["status"],
		Priority:      q["priority"],
		AssigneeID:    q.Get("assignee"),
		ProjectID:     q.Get("project"),
		CursorValue:   q.Get("cursor"),
		CursorID:      q.Get("cursor_id"),
		SortDesc:      true,
		IncludeSystem: includeSystemFromQuery(c),
	}
	if sortRaw := q.Get("sort"); sortRaw != "" {
		opts.SortField = sqlite.TaskListSortField(sortRaw)
	}
	if order := q.Get("order"); order == "asc" {
		opts.SortDesc = false
	}
	if lim := q.Get("limit"); lim != "" {
		if n, err := strconv.Atoi(lim); err == nil {
			opts.Limit = n
		}
	}
	page, err := h.svc.ListTasksFiltered(ctx, wsID, opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dtos := h.attachLabelsToTasks(ctx, page.Tasks)
	c.JSON(http.StatusOK, gin.H{
		"tasks":       dtos,
		"next_cursor": page.NextCursor,
		"next_id":     page.NextID,
	})
}

// attachLabelsToTasks runs a single batched ListLabelsForTasks call for
// the supplied rows and projects them into TaskDTOs.
func (h *Handler) attachLabelsToTasks(ctx context.Context, tasks []*sqlite.TaskRow) []*TaskDTO {
	labelMap := map[string][]*sqlite.Label{}
	if h.labels != nil && len(tasks) > 0 {
		taskIDs := make([]string, len(tasks))
		for i, t := range tasks {
			taskIDs[i] = t.ID
		}
		labelMap, _ = h.labels.ListLabelsForTasks(ctx, taskIDs)
	}
	dtos := make([]*TaskDTO, len(tasks))
	for i, t := range tasks {
		dtos[i] = taskRowToDTO(t, labelMap[t.ID])
	}
	return dtos
}

func (h *Handler) getTask(c *gin.Context) {
	ctx := c.Request.Context()
	task, err := h.svc.GetTask(ctx, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	var lbls []*sqlite.Label
	if h.labels != nil {
		lbls, _ = h.labels.ListLabelsForTask(ctx, task.ID)
	}
	dto := taskRowToDTO(task, lbls)
	if parts, perr := h.svc.ListAllTaskParticipants(ctx, task.ID); perr == nil {
		dto.Reviewers, dto.Approvers = splitParticipantsByRole(parts)
	}
	// Populate the main task's BlockedBy so the picker UI and tests can
	// observe blocker mutations. Children's BlockedBy is set further down.
	if blockerMap, berr := h.svc.ListBlockersForChildren(ctx, []string{task.ID}); berr == nil {
		if blockers, ok := blockerMap[task.ID]; ok && len(blockers) > 0 {
			dto.BlockedBy = blockers
		}
	}
	h.attachChildren(ctx, task.ID, dto)
	h.attachDecisions(ctx, c, task.ID, dto)

	statusChanges, _ := h.svc.ListStatusChanges(ctx, task.WorkspaceID, task.ID)
	c.JSON(http.StatusOK, TaskResponse{Task: dto, Timeline: buildStatusTimeline(statusChanges)})
}

func (h *Handler) attachChildren(ctx context.Context, taskID string, dto *TaskDTO) {
	children, err := h.svc.ListChildTasks(ctx, taskID)
	if err != nil || len(children) == 0 {
		return
	}
	childIDs := make([]string, len(children))
	for i, child := range children {
		childIDs[i] = child.ID
	}
	blockerMap, _ := h.svc.ListBlockersForChildren(ctx, childIDs)
	dto.Children = make([]*TaskDTO, len(children))
	for i, child := range children {
		childDTO := taskRowToDTO(child, nil)
		if blockers, ok := blockerMap[child.ID]; ok && len(blockers) > 0 {
			childDTO.BlockedBy = blockers
		}
		dto.Children[i] = childDTO
	}
}

func (h *Handler) attachDecisions(ctx context.Context, c *gin.Context, taskID string, dto *TaskDTO) {
	decisions, err := h.svc.ListTaskDecisions(ctx, taskID)
	if err != nil {
		return
	}
	dto.Decisions = make([]*DecisionDTO, len(decisions))
	for i := range decisions {
		dto.Decisions[i] = h.decisionToDTO(c, &decisions[i])
	}
}

func buildStatusTimeline(changes []TimelineEvent) []TimelineEventDTO {
	timeline := make([]TimelineEventDTO, len(changes))
	for i, ev := range changes {
		timeline[i] = TimelineEventDTO{
			Type: "status_change",
			From: ev.From,
			To:   ev.To,
			At:   ev.At,
		}
	}
	return timeline
}

func taskRowToDTO(r *sqlite.TaskRow, lbls []*sqlite.Label) *TaskDTO {
	labels := make([]LabelDTO, len(lbls))
	for i, l := range lbls {
		labels[i] = LabelDTO{Name: l.Name, Color: l.Color}
	}
	return &TaskDTO{
		ID:                     r.ID,
		WorkspaceID:            r.WorkspaceID,
		Identifier:             r.Identifier,
		Title:                  r.Title,
		Description:            r.Description,
		Status:                 dbStateToOfficeStatus(r.Status),
		Priority:               normaliseTaskPriority(r.Priority),
		ParentID:               r.ParentID,
		ProjectID:              r.ProjectID,
		AssigneeAgentProfileID: r.AssigneeAgentProfileID,
		Labels:                 labels,
		Reviewers:              []string{},
		Approvers:              []string{},
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
		IsSystem:               r.IsSystem,
	}
}

// normaliseTaskPriority guards against legacy/empty values seeping through.
// All persisted rows after migration carry one of the four enum strings, but
// callers that bypass the writer (older test fixtures, raw SQL inserts) may
// still return empty strings; default those to "medium" for safety.
func normaliseTaskPriority(p string) string {
	switch p {
	case "critical", "high", "medium", "low":
		return p
	default:
		return "medium"
	}
}

// UpdateTaskRequest is the request body for PATCH /tasks/:id.
type UpdateTaskRequest struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
	// AssigneeAgentProfileID is a pointer so callers can distinguish "field
	// omitted" (nil → no-op) from "explicitly clear assignee" (empty
	// string → clear). The picker UI sends "" when the user picks
	// "No assignee".
	AssigneeAgentProfileID *string `json:"assignee_agent_profile_id"`
	// Priority is one of "critical" | "high" | "medium" | "low" when set.
	Priority *string `json:"priority,omitempty"`
	// ProjectID empty string clears the project; otherwise must be a project
	// in the same workspace.
	ProjectID *string `json:"project_id,omitempty"`
	// ParentID empty string clears the parent; otherwise must be a different
	// task than the one being updated.
	ParentID *string `json:"parent_id,omitempty"`
	// Reopen=true is shorthand for status→todo on a closed task; the
	// reactivity pipeline emits task_reopened (or task_reopened_via_comment
	// if a comment is also attached).
	Reopen bool `json:"reopen,omitempty"`
	// Resume=true means the user wants the agent to pick the task back up
	// with explicit follow-up intent. A non-empty Comment is REQUIRED so
	// the agent has context. Emits task_reopened_via_comment.
	Resume bool `json:"resume,omitempty"`
}

// hasAnyField returns true when at least one mutation field is set, so the
// handler can short-circuit empty bodies with a 400.
func (req *UpdateTaskRequest) hasAnyField() bool {
	if req.Status != "" || req.Comment != "" {
		return true
	}
	if req.AssigneeAgentProfileID != nil ||
		req.Priority != nil || req.ProjectID != nil || req.ParentID != nil {
		return true
	}
	return req.Reopen || req.Resume
}

func (h *Handler) updateTask(c *gin.Context) {
	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.hasAnyField() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no mutation fields supplied"})
		return
	}
	if req.Resume && req.Comment == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resume requires a comment"})
		return
	}
	if req.Status == "" && (req.Reopen || req.Resume) {
		req.Status = statusTODOLowercase
	}

	taskID := c.Param("id")
	actorAgentID := agentIDFromCtx(c)

	if !h.applyTaskMutations(c, taskID, actorAgentID, &req) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// applyTaskMutations runs each mutation in the canonical order and reports
// whether all writes succeeded. When a mutation fails the handler responds
// with the appropriate status code and applyTaskMutations returns false.
//
// Order: assignee → priority → project → parent → status.
// Status is last because it triggers the reactivity pipeline.
func (h *Handler) applyTaskMutations(c *gin.Context, taskID, actorAgentID string, req *UpdateTaskRequest) bool {
	ctx := c.Request.Context()
	if req.AssigneeAgentProfileID != nil {
		if err := h.svc.SetTaskAssigneeAsAgent(ctx, actorAgentID, taskID, *req.AssigneeAgentProfileID); err != nil {
			respondMutationError(c, err)
			return false
		}
	}
	if req.Priority != nil {
		if err := h.svc.UpdateTaskPriority(ctx, taskID, *req.Priority); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return false
		}
	}
	if req.ProjectID != nil {
		if err := h.svc.UpdateTaskProjectID(ctx, taskID, *req.ProjectID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return false
		}
	}
	if req.ParentID != nil {
		if err := h.svc.UpdateTaskParentID(ctx, taskID, *req.ParentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return false
		}
	}
	if req.Status != "" {
		err := h.svc.UpdateTaskStatus(ctx, TaskStatusUpdateRequest{
			TaskID:       taskID,
			NewStatus:    req.Status,
			Comment:      req.Comment,
			ActorAgentID: actorAgentID,
			ReopenIntent: req.Reopen || req.Resume,
			ResumeIntent: req.Resume,
		})
		if err != nil {
			h.respondStatusUpdateError(c, err)
			return false
		}
	}
	return true
}

// respondStatusUpdateError translates UpdateTaskStatus errors into HTTP
// responses. ApprovalsPendingError → 409 with a body listing pending
// approvers (resolved to {agent_profile_id, name}) and the redirected
// status. Everything else → 400.
func (h *Handler) respondStatusUpdateError(c *gin.Context, err error) {
	var pending *ApprovalsPendingError
	if errors.As(err, &pending) {
		c.JSON(http.StatusConflict, gin.H{
			"error":             err.Error(),
			"pending_approvers": h.svc.resolvePendingApprovers(c.Request.Context(), pending.Pending),
			"status":            statusInReviewLowercase,
		})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

// respondMutationError translates a mutation error into the matching HTTP
// response. Forbidden bubbles up as 403; everything else as 500.
func respondMutationError(c *gin.Context, err error) {
	if errors.Is(err, shared.ErrForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// agentIDFromCtx returns the agent instance ID from an authenticated request,
// or "" if the caller is not an authenticated agent.
func agentIDFromCtx(c *gin.Context) string {
	val, ok := c.Get("agent_caller")
	if !ok {
		return ""
	}
	agent, ok := val.(*models.AgentInstance)
	if !ok || agent == nil {
		return ""
	}
	return agent.ID
}

// Comment handlers live in handler_comments.go.
