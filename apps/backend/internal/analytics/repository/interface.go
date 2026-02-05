package repository

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

// Repository defines the interface for analytics/statistics operations.
type Repository interface {
	GetTaskStats(ctx context.Context, workspaceID string, start *time.Time) ([]*models.TaskStats, error)
	GetGlobalStats(ctx context.Context, workspaceID string, start *time.Time) (*models.GlobalStats, error)
	GetDailyActivity(ctx context.Context, workspaceID string, days int) ([]*models.DailyActivity, error)
	GetCompletedTaskActivity(ctx context.Context, workspaceID string, days int) ([]*models.CompletedTaskActivity, error)
	GetAgentUsage(ctx context.Context, workspaceID string, limit int, start *time.Time) ([]*models.AgentUsage, error)
	GetRepositoryStats(ctx context.Context, workspaceID string, start *time.Time) ([]*models.RepositoryStats, error)
	GetGitStats(ctx context.Context, workspaceID string, start *time.Time) (*models.GitStats, error)
}
