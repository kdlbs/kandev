package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/analytics/dto"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// allTimeActivityDays is the number of days shown in the daily activity heatmap for the "all" range.
const allTimeActivityDays = 365
const taskStatsLimit = 200
const agentUsageLimit = 5

type StatsHandlers struct {
	repo   repository.Repository
	logger *logger.Logger
}

func NewStatsHandlers(repo repository.Repository, log *logger.Logger) *StatsHandlers {
	return &StatsHandlers{
		repo:   repo,
		logger: log.WithFields(zap.String("component", "analytics-stats-handlers")),
	}
}

func RegisterStatsRoutes(router *gin.Engine, repo repository.Repository, log *logger.Logger) {
	handlers := NewStatsHandlers(repo, log)
	handlers.registerHTTP(router)
}

func (h *StatsHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workspaces/:id/stats/global", h.httpGetGlobalStats)
	api.GET("/workspaces/:id/stats/tasks", h.httpGetTaskStats)
	api.GET("/workspaces/:id/stats/daily-activity", h.httpGetDailyActivity)
	api.GET("/workspaces/:id/stats/completed-activity", h.httpGetCompletedActivity)
	api.GET("/workspaces/:id/stats/agent-usage", h.httpGetAgentUsage)
	api.GET("/workspaces/:id/stats/repositories", h.httpGetRepositoryStats)
	api.GET("/workspaces/:id/stats/git", h.httpGetGitStats)
}

// taskStatsResponse pairs the workload list with a paging flag.
type taskStatsResponse struct {
	TaskStats        []dto.TaskStatsDTO `json:"task_stats"`
	TaskStatsHasMore bool               `json:"task_stats_has_more"`
}

func (h *StatsHandlers) httpGetGlobalStats(c *gin.Context) {
	workspaceID, start, _, ok := h.parseRequest(c)
	if !ok {
		return
	}
	stats, err := h.repo.GetGlobalStats(c.Request.Context(), workspaceID, start)
	if err != nil {
		h.fail(c, workspaceID, "global stats", err)
		return
	}
	c.JSON(http.StatusOK, dto.GlobalStatsDTO{
		TotalTasks:           stats.TotalTasks,
		CompletedTasks:       stats.CompletedTasks,
		InProgressTasks:      stats.InProgressTasks,
		TotalSessions:        stats.TotalSessions,
		TotalTurns:           stats.TotalTurns,
		TotalMessages:        stats.TotalMessages,
		TotalUserMessages:    stats.TotalUserMessages,
		TotalToolCalls:       stats.TotalToolCalls,
		TotalDurationMs:      stats.TotalDurationMs,
		AvgTurnsPerTask:      stats.AvgTurnsPerTask,
		AvgMessagesPerTask:   stats.AvgMessagesPerTask,
		AvgDurationMsPerTask: stats.AvgDurationMsPerTask,
	})
}

func (h *StatsHandlers) httpGetTaskStats(c *gin.Context) {
	workspaceID, start, _, ok := h.parseRequest(c)
	if !ok {
		return
	}
	// Probe one row past the page size so we can distinguish "exactly N rows"
	// from "N rows visible, more after" without a separate COUNT query.
	stats, err := h.repo.GetTaskStats(c.Request.Context(), workspaceID, start, taskStatsLimit+1)
	if err != nil {
		h.fail(c, workspaceID, "task stats", err)
		return
	}
	hasMore := len(stats) > taskStatsLimit
	if hasMore {
		stats = stats[:taskStatsLimit]
	}
	c.JSON(http.StatusOK, taskStatsResponse{
		TaskStats:        taskStatsToDTOs(stats),
		TaskStatsHasMore: hasMore,
	})
}

func (h *StatsHandlers) httpGetDailyActivity(c *gin.Context) {
	workspaceID, _, days, ok := h.parseRequest(c)
	if !ok {
		return
	}
	activity, err := h.repo.GetDailyActivity(c.Request.Context(), workspaceID, days)
	if err != nil {
		h.fail(c, workspaceID, "daily activity", err)
		return
	}
	c.JSON(http.StatusOK, dailyActivityToDTOs(activity))
}

func (h *StatsHandlers) httpGetCompletedActivity(c *gin.Context) {
	workspaceID, _, days, ok := h.parseRequest(c)
	if !ok {
		return
	}
	activity, err := h.repo.GetCompletedTaskActivity(c.Request.Context(), workspaceID, days)
	if err != nil {
		h.fail(c, workspaceID, "completed activity", err)
		return
	}
	c.JSON(http.StatusOK, completedActivityToDTOs(activity))
}

func (h *StatsHandlers) httpGetAgentUsage(c *gin.Context) {
	workspaceID, start, _, ok := h.parseRequest(c)
	if !ok {
		return
	}
	usage, err := h.repo.GetAgentUsage(c.Request.Context(), workspaceID, agentUsageLimit, start)
	if err != nil {
		h.fail(c, workspaceID, "agent usage", err)
		return
	}
	c.JSON(http.StatusOK, agentUsageToDTOs(usage))
}

func (h *StatsHandlers) httpGetRepositoryStats(c *gin.Context) {
	workspaceID, start, _, ok := h.parseRequest(c)
	if !ok {
		return
	}
	stats, err := h.repo.GetRepositoryStats(c.Request.Context(), workspaceID, start)
	if err != nil {
		h.fail(c, workspaceID, "repository stats", err)
		return
	}
	c.JSON(http.StatusOK, repositoryStatsToDTOs(stats))
}

func (h *StatsHandlers) httpGetGitStats(c *gin.Context) {
	workspaceID, start, _, ok := h.parseRequest(c)
	if !ok {
		return
	}
	stats, err := h.repo.GetGitStats(c.Request.Context(), workspaceID, start)
	if err != nil {
		h.fail(c, workspaceID, "git stats", err)
		return
	}
	c.JSON(http.StatusOK, dto.GitStatsDTO{
		TotalCommits:      stats.TotalCommits,
		TotalFilesChanged: stats.TotalFilesChanged,
		TotalInsertions:   stats.TotalInsertions,
		TotalDeletions:    stats.TotalDeletions,
	})
}

// parseRequest extracts the workspace ID and resolves the range query.
// Returns ok=false after writing a 400 response on a missing workspace id.
func (h *StatsHandlers) parseRequest(c *gin.Context) (string, *time.Time, int, bool) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return "", nil, 0, false
	}
	start, days := parseStatsRange(c.Query("range"))
	return workspaceID, start, days, true
}

func (h *StatsHandlers) fail(c *gin.Context, workspaceID, section string, err error) {
	h.logger.Error("failed to get "+section, zap.String("workspace_id", workspaceID), zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get " + section})
}

func taskStatsToDTOs(taskStats []*models.TaskStats) []dto.TaskStatsDTO {
	result := make([]dto.TaskStatsDTO, 0, len(taskStats))
	for _, ts := range taskStats {
		taskDTO := dto.TaskStatsDTO{
			TaskID:           ts.TaskID,
			TaskTitle:        ts.TaskTitle,
			WorkspaceID:      ts.WorkspaceID,
			WorkflowID:       ts.WorkflowID,
			State:            ts.State,
			SessionCount:     ts.SessionCount,
			TurnCount:        ts.TurnCount,
			MessageCount:     ts.MessageCount,
			UserMessageCount: ts.UserMessageCount,
			ToolCallCount:    ts.ToolCallCount,
			TotalDurationMs:  ts.TotalDurationMs,
			ActiveDurationMs: ts.ActiveDurationMs,
			ElapsedSpanMs:    ts.ElapsedSpanMs,
			CreatedAt:        ts.CreatedAt.UTC().Format(time.RFC3339),
		}
		if ts.CompletedAt != nil {
			formatted := ts.CompletedAt.UTC().Format(time.RFC3339)
			taskDTO.CompletedAt = &formatted
		}
		result = append(result, taskDTO)
	}
	return result
}

func dailyActivityToDTOs(items []*models.DailyActivity) []dto.DailyActivityDTO {
	result := make([]dto.DailyActivityDTO, 0, len(items))
	for _, da := range items {
		result = append(result, dto.DailyActivityDTO{
			Date:         da.Date,
			TurnCount:    da.TurnCount,
			MessageCount: da.MessageCount,
			TaskCount:    da.TaskCount,
		})
	}
	return result
}

func completedActivityToDTOs(items []*models.CompletedTaskActivity) []dto.CompletedTaskActivityDTO {
	result := make([]dto.CompletedTaskActivityDTO, 0, len(items))
	for _, ca := range items {
		result = append(result, dto.CompletedTaskActivityDTO{
			Date:           ca.Date,
			CompletedTasks: ca.CompletedTasks,
		})
	}
	return result
}

func agentUsageToDTOs(items []*models.AgentUsage) []dto.AgentUsageDTO {
	result := make([]dto.AgentUsageDTO, 0, len(items))
	for _, au := range items {
		result = append(result, dto.AgentUsageDTO{
			AgentProfileID:   au.AgentProfileID,
			AgentProfileName: au.AgentProfileName,
			AgentModel:       au.AgentModel,
			SessionCount:     au.SessionCount,
			TurnCount:        au.TurnCount,
			TotalDurationMs:  au.TotalDurationMs,
		})
	}
	return result
}

func repositoryStatsToDTOs(items []*models.RepositoryStats) []dto.RepositoryStatsDTO {
	result := make([]dto.RepositoryStatsDTO, 0, len(items))
	for _, rs := range items {
		result = append(result, dto.RepositoryStatsDTO{
			RepositoryID:      rs.RepositoryID,
			RepositoryName:    rs.RepositoryName,
			TotalTasks:        rs.TotalTasks,
			CompletedTasks:    rs.CompletedTasks,
			InProgressTasks:   rs.InProgressTasks,
			SessionCount:      rs.SessionCount,
			TurnCount:         rs.TurnCount,
			MessageCount:      rs.MessageCount,
			UserMessageCount:  rs.UserMessageCount,
			ToolCallCount:     rs.ToolCallCount,
			TotalDurationMs:   rs.TotalDurationMs,
			TotalCommits:      rs.TotalCommits,
			TotalFilesChanged: rs.TotalFilesChanged,
			TotalInsertions:   rs.TotalInsertions,
			TotalDeletions:    rs.TotalDeletions,
		})
	}
	return result
}

func parseStatsRange(rangeKey string) (*time.Time, int) {
	now := time.Now().UTC()
	switch rangeKey {
	case "week":
		start := now.AddDate(0, 0, -7)
		return &start, 7
	case "month":
		start := now.AddDate(0, 0, -30)
		return &start, 30
	case "all":
		return nil, allTimeActivityDays
	default:
		start := now.AddDate(0, 0, -30)
		return &start, 30
	}
}
