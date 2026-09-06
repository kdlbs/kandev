package runtime

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TaskFilteredLister lists one workspace's tasks, ordered and paginated.
// Satisfied by internal/office/dashboard.DashboardService, which already
// implements this exact signature for the dashboard's own filtered list.
type TaskFilteredLister interface {
	ListTasksFiltered(
		ctx context.Context, workspaceID string, opts sqlite.ListTasksOptions,
	) (*sqlite.ListTasksFilteredResult, error)
}

// TaskListItem is one row in a runtime board-read response.
type TaskListItem struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	ParentID    string `json:"parent_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	AssigneeID  string `json:"assignee_agent_profile_id,omitempty"`
	Labels      string `json:"labels,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	IsSystem    bool   `json:"is_system,omitempty"`
}

// listTasks handles GET /runtime/tasks (REQ-OFFICE-COORDINATOR-AUTHORITY-001,
// -002): board read scoped to the run token's workspace claim, never a
// caller-supplied one. Evaluation order is capability, workspace claim,
// parameter validation, then the query — so a run lacking the capability
// learns nothing about which parameters would have been accepted.
func (h *Handler) listTasks(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	if !runCtx.Capabilities.Allows(CapabilityListTasks) {
		h.respondRuntimeError(c, runCtx, "list_tasks", "workspace", runCtx.WorkspaceID, ErrCapabilityDenied)
		return
	}
	if strings.TrimSpace(runCtx.WorkspaceID) == "" {
		h.respondRuntimeError(c, runCtx, "list_tasks", "workspace", runCtx.WorkspaceID, ErrWorkspaceOutOfScope)
		return
	}
	opts, err := parseListTasksQuery(c.Request.URL.Query())
	if err != nil {
		h.respondRuntimeError(c, runCtx, "list_tasks", "workspace", runCtx.WorkspaceID, err)
		return
	}
	if h.taskLister == nil {
		h.respondRuntimeError(c, runCtx, "list_tasks", "workspace", runCtx.WorkspaceID, ErrRuntimeDependencyMissing)
		return
	}
	page, err := h.taskLister.ListTasksFiltered(c.Request.Context(), runCtx.WorkspaceID, opts)
	if err != nil {
		h.respondRuntimeError(c, runCtx, "list_tasks", "workspace", runCtx.WorkspaceID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "list_tasks", "workspace", runCtx.WorkspaceID)
	c.JSON(http.StatusOK, gin.H{
		"tasks":       toTaskListItems(page.Tasks),
		"next_cursor": page.NextCursor,
		"next_id":     page.NextID,
	})
}

func toTaskListItems(rows []*sqlite.TaskRow) []TaskListItem {
	items := make([]TaskListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, TaskListItem{
			ID:          r.ID,
			WorkspaceID: r.WorkspaceID,
			Identifier:  r.Identifier,
			Title:       r.Title,
			Description: r.Description,
			Status:      r.Status,
			Priority:    r.Priority,
			ParentID:    r.ParentID,
			ProjectID:   r.ProjectID,
			AssigneeID:  r.AssigneeAgentProfileID,
			Labels:      r.Labels,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			IsSystem:    r.IsSystem,
		})
	}
	return items
}

// parseListTasksQuery validates and maps the board-read query string per
// docs/specs/office/system-design/taskless-coordinator-authority-02.md
// #request-and-response-contract. Every rule is strict on purpose: an
// empty value means absent, enumerated values match byte-for-byte with no
// case folding or trimming, and repeating a single-valued parameter is a
// refusal rather than "last one wins".
func parseListTasksQuery(query url.Values) (sqlite.ListTasksOptions, error) {
	opts := sqlite.ListTasksOptions{
		Status:   repeatableListParam(query, "status"),
		Priority: repeatableListParam(query, "priority"),
		SortDesc: true,
	}
	if err := parseSingleValuedFilters(query, &opts); err != nil {
		return sqlite.ListTasksOptions{}, err
	}
	if err := parseSortAndOrder(query, &opts); err != nil {
		return sqlite.ListTasksOptions{}, err
	}
	if err := parseLimit(query, &opts); err != nil {
		return sqlite.ListTasksOptions{}, err
	}
	if err := parseCursor(query, &opts); err != nil {
		return sqlite.ListTasksOptions{}, err
	}
	if err := parseIncludeSystem(query, &opts); err != nil {
		return sqlite.ListTasksOptions{}, err
	}
	return opts, nil
}

func parseSingleValuedFilters(query url.Values, opts *sqlite.ListTasksOptions) error {
	assignee, _, err := singleListParam(query, "assignee")
	if err != nil {
		return err
	}
	opts.AssigneeID = assignee

	project, _, err := singleListParam(query, "project")
	if err != nil {
		return err
	}
	opts.ProjectID = project
	return nil
}

func parseSortAndOrder(query url.Values, opts *sqlite.ListTasksOptions) error {
	sortRaw, sortPresent, err := singleListParam(query, "sort")
	if err != nil {
		return err
	}
	if sortPresent {
		switch sortRaw {
		case string(sqlite.TaskSortUpdatedAt), string(sqlite.TaskSortCreatedAt), string(sqlite.TaskSortPriority):
			opts.SortField = sqlite.TaskListSortField(sortRaw)
		default:
			return ErrInvalidListParams
		}
	}

	orderRaw, orderPresent, err := singleListParam(query, "order")
	if err != nil {
		return err
	}
	if orderPresent {
		switch orderRaw {
		case "asc":
			opts.SortDesc = false
		case "desc":
			opts.SortDesc = true
		default:
			return ErrInvalidListParams
		}
	}
	return nil
}

func parseLimit(query url.Values, opts *sqlite.ListTasksOptions) error {
	limitRaw, present, err := singleListParam(query, "limit")
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	n, convErr := strconv.Atoi(limitRaw)
	if convErr != nil {
		return ErrInvalidListParams
	}
	opts.Limit = n
	return nil
}

func parseCursor(query url.Values, opts *sqlite.ListTasksOptions) error {
	cursorValue, cursorPresent, err := singleListParam(query, "cursor")
	if err != nil {
		return err
	}
	cursorID, cursorIDPresent, err := singleListParam(query, "cursor_id")
	if err != nil {
		return err
	}
	if cursorPresent != cursorIDPresent {
		return ErrInvalidListParams
	}
	opts.CursorValue = cursorValue
	opts.CursorID = cursorID
	return nil
}

func parseIncludeSystem(query url.Values, opts *sqlite.ListTasksOptions) error {
	raw, present, err := singleListParam(query, "include_system")
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	switch raw {
	case "true":
		opts.IncludeSystem = true
	case "false":
		opts.IncludeSystem = false
	default:
		return ErrInvalidListParams
	}
	return nil
}

// singleListParam reads a single-valued query parameter. A repeat of the
// key is ErrInvalidListParams; a present-but-empty value is treated as
// absent (present=false) per the "empty means absent" normalization rule.
func singleListParam(query url.Values, key string) (value string, present bool, err error) {
	values := query[key]
	switch len(values) {
	case 0:
		return "", false, nil
	case 1:
		if values[0] == "" {
			return "", false, nil
		}
		return values[0], true, nil
	default:
		return "", false, ErrInvalidListParams
	}
}

// repeatableListParam reads a repeatable query parameter, dropping any
// individually-empty values ("empty means absent" applies per value).
func repeatableListParam(query url.Values, key string) []string {
	values := query[key]
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
