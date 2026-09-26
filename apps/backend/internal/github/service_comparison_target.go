package github

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// ComparisonTargetObserver is the narrow provider-to-task boundary for
// comparison reconciliation. GitHub owns authoritative PR identity; the task
// service owns attachment matching and durable metadata.
type ComparisonTargetObserver interface {
	ReconcileComparisonTarget(ctx context.Context, taskID string, candidate taskmodels.ComparisonTargetCandidate) (*taskmodels.ComparisonTargetReconciliation, error)
	ReconcileComparisonTargetFromSync(ctx context.Context, taskID string, candidate taskmodels.ComparisonTargetCandidate) (*taskmodels.ComparisonTargetReconciliation, error)
	RemoveComparisonTargetForChange(ctx context.Context, taskID, repositoryID, provider, kind string, number int) error
}

// SetComparisonTargetObserver wires the task-service reconciler after both
// services are constructed, avoiding a package import cycle.
func (s *Service) SetComparisonTargetObserver(observer ComparisonTargetObserver) {
	s.comparisonTargetObserver = observer
}

func (s *Service) reconcileComparisonTarget(ctx context.Context, taskID string, pr *PR) {
	s.reconcileComparisonTargetMode(ctx, taskID, pr, true)
}

func (s *Service) reconcileComparisonTargetFromSync(ctx context.Context, taskID string, pr *PR) {
	s.reconcileComparisonTargetMode(ctx, taskID, pr, false)
}

func (s *Service) reconcileComparisonTargetMode(ctx context.Context, taskID string, pr *PR, allowReplacement bool) {
	if s.comparisonTargetObserver == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	candidate, ok := githubComparisonTargetCandidate(pr)
	if !ok {
		if s.logger != nil {
			s.logger.Debug("comparison target reconciliation skipped: PR identity incomplete",
				zap.String("task_id", taskID),
				zap.Int("pr_number", prNumber(pr)))
		}
		return
	}
	var result *taskmodels.ComparisonTargetReconciliation
	var err error
	if allowReplacement {
		result, err = s.comparisonTargetObserver.ReconcileComparisonTarget(ctx, taskID, candidate)
	} else {
		result, err = s.comparisonTargetObserver.ReconcileComparisonTargetFromSync(ctx, taskID, candidate)
	}
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("comparison target reconciliation failed",
				zap.String("task_id", taskID),
				zap.Int("pr_number", candidate.Number),
				zap.Error(err))
		}
		return
	}
	if s.logger != nil && result != nil {
		s.logger.Info("comparison target reconciliation completed",
			zap.String("task_id", taskID),
			zap.Int("pr_number", candidate.Number),
			zap.String("outcome", string(result.Status)),
			zap.String("target", displayComparisonTarget(candidate)))
	}
}

func githubComparisonTargetCandidate(pr *PR) (taskmodels.ComparisonTargetCandidate, bool) {
	if pr == nil || pr.Number <= 0 {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	headOwner := strings.TrimSpace(pr.HeadRepoOwner)
	headName := strings.TrimSpace(pr.HeadRepoName)
	targetOwner := strings.TrimSpace(pr.BaseRepoOwner)
	targetName := strings.TrimSpace(pr.BaseRepoName)
	if targetOwner == "" {
		targetOwner = strings.TrimSpace(pr.RepoOwner)
	}
	if targetName == "" {
		targetName = strings.TrimSpace(pr.RepoName)
	}
	if !validGitHubPathPart(headOwner) || !validGitHubPathPart(headName) ||
		!validGitHubPathPart(targetOwner) || !validGitHubPathPart(targetName) {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	headBranch, err := taskmodels.NormalizeComparisonBranch(strings.TrimSpace(pr.HeadBranch))
	if err != nil {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	targetBranch, err := taskmodels.NormalizeComparisonBranch(strings.TrimSpace(pr.BaseBranch))
	if err != nil {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	return taskmodels.ComparisonTargetCandidate{
		Provider:     taskmodels.ComparisonTargetProviderGitHub,
		Kind:         taskmodels.ComparisonTargetKindPullRequest,
		Number:       pr.Number,
		HeadBranch:   headBranch,
		TargetBranch: targetBranch,
		HeadRepository: taskmodels.ComparisonTargetRepository{
			Host:       "github.com",
			Path:       headOwner + "/" + headName,
			ProviderID: githubHeadProviderID(pr),
			RemoteURL:  fmt.Sprintf("https://github.com/%s/%s.git", headOwner, headName),
		},
		TargetRepository: taskmodels.ComparisonTargetRepository{
			Host:       "github.com",
			Path:       targetOwner + "/" + targetName,
			ProviderID: positiveID(pr.BaseRepoID),
			RemoteURL:  fmt.Sprintf("https://github.com/%s/%s.git", targetOwner, targetName),
		},
	}, true
}

func displayComparisonTarget(candidate taskmodels.ComparisonTargetCandidate) string {
	return candidate.TargetRepository.Path + ":" + candidate.TargetBranch
}

func prNumber(pr *PR) int {
	if pr == nil {
		return 0
	}
	return pr.Number
}
