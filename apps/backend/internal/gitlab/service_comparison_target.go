package gitlab

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// ComparisonTargetObserver is implemented by the task service. GitLab only
// uses it when the MR payload contains explicit source and target project
// identities; it never infers a fork from the target project URL alone.
type ComparisonTargetObserver interface {
	ReconcileComparisonTarget(ctx context.Context, taskID string, candidate taskmodels.ComparisonTargetCandidate) (*taskmodels.ComparisonTargetReconciliation, error)
	ReconcileComparisonTargetFromSync(ctx context.Context, taskID string, candidate taskmodels.ComparisonTargetCandidate) (*taskmodels.ComparisonTargetReconciliation, error)
	RemoveComparisonTargetForChange(ctx context.Context, taskID, repositoryID, provider, kind string, number int) error
}

func (s *Service) SetComparisonTargetObserver(observer ComparisonTargetObserver) {
	s.comparisonTargetObserver = observer
}

func (s *Service) reconcileComparisonTarget(ctx context.Context, taskID, host string, mr *MR) {
	s.reconcileComparisonTargetMode(ctx, taskID, host, mr, true)
}

func (s *Service) reconcileComparisonTargetFromSync(ctx context.Context, taskID, host string, mr *MR) {
	s.reconcileComparisonTargetMode(ctx, taskID, host, mr, false)
}

func (s *Service) reconcileComparisonTargetMode(ctx context.Context, taskID, host string, mr *MR, allowReplacement bool) {
	if s.comparisonTargetObserver == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	candidate, ok := gitLabComparisonTargetCandidate(host, mr)
	if !ok {
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
			s.logger.Warn("GitLab comparison target reconciliation failed",
				zap.String("task_id", taskID), zap.Int("mr_iid", candidate.Number), zap.Error(err))
		}
		return
	}
	if s.logger != nil && result != nil {
		s.logger.Info("GitLab comparison target reconciliation completed",
			zap.String("task_id", taskID), zap.Int("mr_iid", candidate.Number),
			zap.String("outcome", string(result.Status)),
			zap.String("target", candidate.TargetRepository.Path+":"+candidate.TargetBranch))
	}
}

func gitLabComparisonTargetCandidate(host string, mr *MR) (taskmodels.ComparisonTargetCandidate, bool) {
	if mr == nil || mr.IID <= 0 {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	sourcePath := strings.TrimSpace(mr.SourceProjectPath)
	targetPath := strings.TrimSpace(mr.TargetProjectPath)
	if sourcePath == "" || targetPath == "" || !validGitLabProjectPath(sourcePath) || !validGitLabProjectPath(targetPath) {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	hostOrigin, hostName, err := gitLabContributionHost(host)
	if err != nil {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	headBranch, err := taskmodels.NormalizeComparisonBranch(strings.TrimSpace(mr.HeadBranch))
	if err != nil {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	targetBranch, err := taskmodels.NormalizeComparisonBranch(strings.TrimSpace(mr.BaseBranch))
	if err != nil {
		return taskmodels.ComparisonTargetCandidate{}, false
	}
	return taskmodels.ComparisonTargetCandidate{
		Provider:     taskmodels.ComparisonTargetProviderGitLab,
		Kind:         taskmodels.ComparisonTargetKindMergeRequest,
		Number:       mr.IID,
		HeadBranch:   headBranch,
		TargetBranch: targetBranch,
		HeadRepository: taskmodels.ComparisonTargetRepository{
			Host:       hostName,
			Path:       sourcePath,
			ProviderID: positiveGitLabID(mr.SourceProjectID),
			RemoteURL:  fmt.Sprintf("%s/%s.git", strings.TrimRight(hostOrigin, "/"), sourcePath),
		},
		TargetRepository: taskmodels.ComparisonTargetRepository{
			Host:       hostName,
			Path:       targetPath,
			ProviderID: positiveGitLabID(firstGitLabID(mr.TargetProjectID, mr.ProjectID)),
			RemoteURL:  fmt.Sprintf("%s/%s.git", strings.TrimRight(hostOrigin, "/"), targetPath),
		},
	}, true
}
