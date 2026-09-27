package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

const sidebarTaskRequestLimit = 256 * 1024

type sidebarTaskPageResponse struct {
	QueryKey          string                         `json:"query_key"`
	Page              int                            `json:"page"`
	PageSize          int                            `json:"page_size"`
	TotalEntries      int                            `json:"total_entries"`
	TotalTasks        int                            `json:"total_tasks"`
	TotalVisibleTasks int                            `json:"total_visible_tasks"`
	HasPrevious       bool                           `json:"has_previous"`
	HasNext           bool                           `json:"has_next"`
	Entries           []sidebarTaskPageEntryResponse `json:"entries"`
}

type sidebarTaskPageEntryResponse struct {
	Kind             string       `json:"kind"`
	TaskID           string       `json:"task_id,omitempty"`
	Task             *dto.TaskDTO `json:"task,omitempty"`
	GroupKey         string       `json:"group_key,omitempty"`
	GroupLabel       string       `json:"group_label,omitempty"`
	WorkflowName     string       `json:"workflow_name,omitempty"`
	StepName         string       `json:"workflow_step_name,omitempty"`
	StepColor        string       `json:"workflow_step_color,omitempty"`
	Depth            int          `json:"depth,omitempty"`
	ParentID         string       `json:"parent_id,omitempty"`
	ParentTitle      string       `json:"parent_title,omitempty"`
	Continuation     bool         `json:"continuation,omitempty"`
	MatchingCount    int          `json:"matching_count,omitempty"`
	WIPQueuePosition int          `json:"wip_queue_position,omitempty"`
	WIPQueueTotal    int          `json:"wip_queue_total,omitempty"`
	SubtaskCount     int          `json:"subtask_count,omitempty"`
}

func (h *TaskHandlers) httpQuerySidebarTasks(c *gin.Context) {
	query, malformed, validationErr := decodeSidebarTaskQuery(c)
	if malformed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sidebar query"})
		return
	}
	if validationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
		return
	}

	var prefs models.SidebarTaskViewPreferences
	if h.sidebarSettingsReader != nil {
		settings, err := h.sidebarSettingsReader.GetUserSettings(c.Request.Context())
		if err != nil {
			h.logger.Error("failed to read sidebar task preferences", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "request failed"})
			return
		}
		if settings != nil {
			prefs = models.SidebarTaskViewPreferences{
				PinnedTaskIDs:          settings.SidebarTaskPrefs.PinnedTaskIDs,
				OrderedTaskIDs:         settings.SidebarTaskPrefs.OrderedTaskIDs,
				SubtaskOrderByParentID: settings.SidebarTaskPrefs.SubtaskOrderByParentID,
			}
		}
	}

	page, err := h.service.QuerySidebarTaskPage(c.Request.Context(), c.Param("id"), query, prefs)
	if err != nil {
		if errors.Is(err, service.ErrSidebarTaskViewUnavailable) {
			h.logger.Error("sidebar task query repository unavailable", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "request failed"})
			return
		}
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}
	response, err := h.sidebarTaskPageResponse(c, page)
	if err != nil {
		h.logger.Error("failed to enrich sidebar task page", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "request failed"})
		return
	}
	c.JSON(http.StatusOK, response)
}

func decodeSidebarTaskQuery(c *gin.Context) (models.SidebarTaskViewQuery, bool, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, sidebarTaskRequestLimit)
	var query models.SidebarTaskViewQuery
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&query); err != nil {
		return query, true, nil
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return query, true, nil
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = models.MaxSidebarTaskPageSize
	}
	if query.Sort.Key == "" {
		query.Sort = models.SidebarTaskViewSort{Key: "updatedAt", Direction: "desc"}
	}
	if query.Group == "" {
		query.Group = "none"
	}
	if query.Locale == "" {
		query.Locale = "en"
	}
	return query, false, query.Validate()
}

func (h *TaskHandlers) sidebarTaskPageResponse(c *gin.Context, page *models.SidebarTaskPageResult) (sidebarTaskPageResponse, error) {
	tasks, err := h.toTaskDTOsWithSessionInfo(c.Request.Context(), page.Tasks)
	if err != nil {
		return sidebarTaskPageResponse{}, err
	}
	tasksByID := make(map[string]*dto.TaskDTO, len(tasks))
	for index := range tasks {
		tasksByID[tasks[index].ID] = &tasks[index]
	}
	entries := make([]sidebarTaskPageEntryResponse, 0, len(page.Entries))
	for _, entry := range page.Entries {
		entries = append(entries, sidebarTaskPageEntryResponse{
			Kind: entry.Kind, TaskID: entry.TaskID, Task: tasksByID[entry.TaskID],
			GroupKey: entry.GroupKey, GroupLabel: entry.GroupLabel, Depth: entry.Depth,
			WorkflowName: entry.WorkflowName, StepName: entry.StepName, StepColor: entry.StepColor,
			ParentID: entry.ParentID, ParentTitle: entry.ParentTitle,
			Continuation: entry.Continuation, MatchingCount: entry.MatchingCount,
			WIPQueuePosition: entry.WIPQueuePosition, WIPQueueTotal: entry.WIPQueueTotal,
			SubtaskCount: entry.SubtaskCount,
		})
	}
	return sidebarTaskPageResponse{
		QueryKey: page.QueryKey, Page: page.Page, PageSize: page.PageSize,
		TotalEntries: page.TotalEntries, TotalTasks: page.TotalTasks,
		TotalVisibleTasks: page.TotalVisibleTasks, HasPrevious: page.HasPrevious,
		HasNext: page.HasNext, Entries: entries,
	}, nil
}
