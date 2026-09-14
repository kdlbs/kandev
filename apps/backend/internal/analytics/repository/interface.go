package repository

import (
	"context"
	"time"

	analyticserrors "github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
)

// ErrAnalyticsBusy marks a bounded analytics operation that could not finish
// within its admission-plus-query budget.
var ErrAnalyticsBusy = analyticserrors.ErrAnalyticsBusy

// NewAnalyticsBusyError wraps the internal timeout cause with the stable
// classification used by HTTP and plugin callers.
func NewAnalyticsBusyError(cause error) error {
	return analyticserrors.NewAnalyticsBusyError(cause)
}

// IsAnalyticsBusy reports whether an analytics operation exhausted its bounded
// budget rather than failing because the caller canceled it.
func IsAnalyticsBusy(err error) bool { return analyticserrors.IsAnalyticsBusy(err) }

// Repository defines the interface for analytics/statistics operations.
type Repository interface {
	GetTaskStats(ctx context.Context, workspaceID string, start *time.Time, limit int) ([]*models.TaskStats, error)
	GetGlobalStats(ctx context.Context, workspaceID string, start *time.Time) (*models.GlobalStats, error)
	GetDailyActivity(ctx context.Context, workspaceID string, days int) ([]*models.DailyActivity, error)
	GetCompletedTaskActivity(ctx context.Context, workspaceID string, days int) ([]*models.CompletedTaskActivity, error)
	GetModelUsage(ctx context.Context, workspaceID string, limit int, start *time.Time) ([]*models.ModelUsage, error)
	GetRepositoryStats(ctx context.Context, workspaceID string, start *time.Time) ([]*models.RepositoryStats, error)
	GetGitStats(ctx context.Context, workspaceID string, start *time.Time) (*models.GitStats, error)
	ListSessionCodeStats(ctx context.Context, filter models.SessionCodeStatsFilter) ([]*models.SessionCodeStats, error)
}
