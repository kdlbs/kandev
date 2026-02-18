package handlers

import (
	"context"
	"fmt"
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

// rawStats holds all raw stats fetched from the repository before DTO conversion.
type rawStats struct {
	globalStats       *models.GlobalStats
	taskStats         []*models.TaskStats
	dailyActivity     []*models.DailyActivity
	completedActivity []*models.CompletedTaskActivity
	agentUsage        []*models.AgentUsage
	repoStats         []*models.RepositoryStats
	gitStats          *models.GitStats
}

func (h *StatsHandlers) httpGetStats(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	rangeKey := c.Query("range")
	start, days := parseStatsRange(rangeKey)

	raw, err := h.fetchStats(c.Request.Context(), workspaceID, start, days)
	if err != nil {
		h.logger.Error("failed to get stats", zap.String("workspace_id", workspaceID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	response := dto.StatsResponse{
		Global: dto.GlobalStatsDTO{
			TotalTasks:           raw.globalStats.TotalTasks,
			CompletedTasks:       raw.globalStats.CompletedTasks,
			InProgressTasks:      raw.globalStats.InProgressTasks,
			TotalSessions:        raw.globalStats.TotalSessions,
			TotalTurns:           raw.globalStats.TotalTurns,
			TotalMessages:        raw.globalStats.TotalMessages,
			TotalUserMessages:    raw.globalStats.TotalUserMessages,
			TotalToolCalls:       raw.globalStats.TotalToolCalls,
			TotalDurationMs:      raw.globalStats.TotalDurationMs,
			AvgTurnsPerTask:      raw.globalStats.AvgTurnsPerTask,
			AvgMessagesPerTask:   raw.globalStats.AvgMessagesPerTask,
			AvgDurationMsPerTask: raw.globalStats.AvgDurationMsPerTask,
		},
		TaskStats:         taskStatsToDTOs(raw.taskStats),
		DailyActivity:     dailyActivityToDTOs(raw.dailyActivity),
		CompletedActivity: completedActivityToDTOs(raw.completedActivity),
		AgentUsage:        agentUsageToDTOs(raw.agentUsage),
		RepositoryStats:   repositoryStatsToDTOs(raw.repoStats),
		GitStats: dto.GitStatsDTO{
			TotalCommits:      raw.gitStats.TotalCommits,
			TotalFilesChanged: raw.gitStats.TotalFilesChanged,
			TotalInsertions:   raw.gitStats.TotalInsertions,
			TotalDeletions:    raw.gitStats.TotalDeletions,
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *StatsHandlers) fetchStats(ctx context.Context, workspaceID string, start *time.Time, days int) (*rawStats, error) {
	globalStats, err := h.repo.GetGlobalStats(ctx, workspaceID, start)
	if err != nil {
		return nil, fmt.Errorf("global stats: %w", err)
	}
	taskStats, err := h.repo.GetTaskStats(ctx, workspaceID, start)
	if err != nil {
		return nil, fmt.Errorf("task stats: %w", err)
	}
	dailyActivity, err := h.repo.GetDailyActivity(ctx, workspaceID, days)
	if err != nil {
		return nil, fmt.Errorf("daily activity: %w", err)
	}
	completedActivity, err := h.repo.GetCompletedTaskActivity(ctx, workspaceID, days)
	if err != nil {
		return nil, fmt.Errorf("completed activity: %w", err)
	}
	agentUsage, err := h.repo.GetAgentUsage(ctx, workspaceID, 5, start)
	if err != nil {
		return nil, fmt.Errorf("agent usage: %w", err)
	}
	repoStats, err := h.repo.GetRepositoryStats(ctx, workspaceID, start)
	if err != nil {
		return nil, fmt.Errorf("repository stats: %w", err)
	}
	gitStats, err := h.repo.GetGitStats(ctx, workspaceID, start)
	if err != nil {
		return nil, fmt.Errorf("git stats: %w", err)
	}
	return &rawStats{
		globalStats:       globalStats,
		taskStats:         taskStats,
		dailyActivity:     dailyActivity,
		completedActivity: completedActivity,
		agentUsage:        agentUsage,
		repoStats:         repoStats,
		gitStats:          gitStats,
	}, nil
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
