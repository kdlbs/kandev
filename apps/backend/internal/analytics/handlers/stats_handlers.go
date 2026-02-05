package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/analytics/dto"
	"github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

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
	api.GET("/workspaces/:id/stats", h.httpGetStats)
}

func (h *StatsHandlers) httpGetStats(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	rangeKey := c.Query("range")
	start, days := parseStatsRange(rangeKey)

	globalStats, err := h.repo.GetGlobalStats(c.Request.Context(), workspaceID, start)
	if err != nil {
		h.logger.Error("failed to get global stats", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	taskStats, err := h.repo.GetTaskStats(c.Request.Context(), workspaceID, start)
	if err != nil {
		h.logger.Error("failed to get task stats", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	// Get daily activity for the selected range
	dailyActivity, err := h.repo.GetDailyActivity(c.Request.Context(), workspaceID, days)
	if err != nil {
		h.logger.Error("failed to get daily activity", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	// Get completed task activity for the selected range
	completedActivity, err := h.repo.GetCompletedTaskActivity(c.Request.Context(), workspaceID, days)
	if err != nil {
		h.logger.Error("failed to get completed task activity", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	// Convert to DTOs
	taskStatsDTOs := make([]dto.TaskStatsDTO, 0, len(taskStats))
	for _, ts := range taskStats {
		taskDTO := dto.TaskStatsDTO{
			TaskID:           ts.TaskID,
			TaskTitle:        ts.TaskTitle,
			WorkspaceID:      ts.WorkspaceID,
			BoardID:          ts.BoardID,
			State:            ts.State,
			SessionCount:     ts.SessionCount,
			TurnCount:        ts.TurnCount,
			MessageCount:     ts.MessageCount,
			UserMessageCount: ts.UserMessageCount,
			ToolCallCount:    ts.ToolCallCount,
			TotalDurationMs:  ts.TotalDurationMs,
			CreatedAt:        ts.CreatedAt.UTC().Format(time.RFC3339),
		}
		if ts.CompletedAt != nil {
			formatted := ts.CompletedAt.UTC().Format(time.RFC3339)
			taskDTO.CompletedAt = &formatted
		}
		taskStatsDTOs = append(taskStatsDTOs, taskDTO)
	}

	dailyActivityDTOs := make([]dto.DailyActivityDTO, 0, len(dailyActivity))
	for _, da := range dailyActivity {
		dailyActivityDTOs = append(dailyActivityDTOs, dto.DailyActivityDTO{
			Date:         da.Date,
			TurnCount:    da.TurnCount,
			MessageCount: da.MessageCount,
			TaskCount:    da.TaskCount,
		})
	}

	completedActivityDTOs := make([]dto.CompletedTaskActivityDTO, 0, len(completedActivity))
	for _, ca := range completedActivity {
		completedActivityDTOs = append(completedActivityDTOs, dto.CompletedTaskActivityDTO{
			Date:           ca.Date,
			CompletedTasks: ca.CompletedTasks,
		})
	}

	// Get agent usage (top 5)
	agentUsage, err := h.repo.GetAgentUsage(c.Request.Context(), workspaceID, 5, start)
	if err != nil {
		h.logger.Error("failed to get agent usage", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	agentUsageDTOs := make([]dto.AgentUsageDTO, 0, len(agentUsage))
	for _, au := range agentUsage {
		agentUsageDTOs = append(agentUsageDTOs, dto.AgentUsageDTO{
			AgentProfileID:   au.AgentProfileID,
			AgentProfileName: au.AgentProfileName,
			AgentModel:       au.AgentModel,
			SessionCount:     au.SessionCount,
			TurnCount:        au.TurnCount,
			TotalDurationMs:  au.TotalDurationMs,
		})
	}

	// Get repository stats
	repoStats, err := h.repo.GetRepositoryStats(c.Request.Context(), workspaceID, start)
	if err != nil {
		h.logger.Error("failed to get repository stats", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	repoStatsDTOs := make([]dto.RepositoryStatsDTO, 0, len(repoStats))
	for _, rs := range repoStats {
		repoStatsDTOs = append(repoStatsDTOs, dto.RepositoryStatsDTO{
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

	// Get git stats
	gitStats, err := h.repo.GetGitStats(c.Request.Context(), workspaceID, start)
	if err != nil {
		h.logger.Error("failed to get git stats", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	response := dto.StatsResponse{
		Global: dto.GlobalStatsDTO{
			TotalTasks:           globalStats.TotalTasks,
			CompletedTasks:       globalStats.CompletedTasks,
			InProgressTasks:      globalStats.InProgressTasks,
			TotalSessions:        globalStats.TotalSessions,
			TotalTurns:           globalStats.TotalTurns,
			TotalMessages:        globalStats.TotalMessages,
			TotalUserMessages:    globalStats.TotalUserMessages,
			TotalToolCalls:       globalStats.TotalToolCalls,
			TotalDurationMs:      globalStats.TotalDurationMs,
			AvgTurnsPerTask:      globalStats.AvgTurnsPerTask,
			AvgMessagesPerTask:   globalStats.AvgMessagesPerTask,
			AvgDurationMsPerTask: globalStats.AvgDurationMsPerTask,
		},
		TaskStats:         taskStatsDTOs,
		DailyActivity:     dailyActivityDTOs,
		CompletedActivity: completedActivityDTOs,
		AgentUsage:        agentUsageDTOs,
		RepositoryStats:   repoStatsDTOs,
		GitStats: dto.GitStatsDTO{
			TotalCommits:      gitStats.TotalCommits,
			TotalFilesChanged: gitStats.TotalFilesChanged,
			TotalInsertions:   gitStats.TotalInsertions,
			TotalDeletions:    gitStats.TotalDeletions,
		},
	}

	c.JSON(http.StatusOK, response)
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
	default:
		start := now.AddDate(0, 0, -30)
		return &start, 30
	}
}
