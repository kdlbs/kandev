package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// ReconcileComparisonTarget matches one provider change to the exact
// task-repository attachment whose provider identity and live checkout branch
// are both equal to the change head. Ambiguous and stale-fork matches are
// deliberate no-ops.
func (s *Service) ReconcileComparisonTarget(
	ctx context.Context,
	taskID string,
	candidate models.ComparisonTargetCandidate,
) (*models.ComparisonTargetReconciliation, error) {
	return s.reconcileComparisonTarget(ctx, taskID, candidate, true)
}

// ReconcileComparisonTargetFromSync refreshes an existing provider change
// without allowing an older historical payload to replace a newer target.
// Explicit association and retarget events use ReconcileComparisonTarget.
func (s *Service) ReconcileComparisonTargetFromSync(
	ctx context.Context,
	taskID string,
	candidate models.ComparisonTargetCandidate,
) (*models.ComparisonTargetReconciliation, error) {
	return s.reconcileComparisonTarget(ctx, taskID, candidate, false)
}

func (s *Service) reconcileComparisonTarget(
	ctx context.Context,
	taskID string,
	candidate models.ComparisonTargetCandidate,
	allowReplacement bool,
) (*models.ComparisonTargetReconciliation, error) {
	target, err := candidate.Build()
	if err != nil {
		return nil, err
	}
	taskRepos, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task repositories for comparison target: %w", err)
	}
	activeSessions, err := s.activeSessionsForComparison(ctx, taskID)
	if err != nil {
		return nil, err
	}
	matches := s.matchingComparisonTaskRepositories(ctx, taskRepos, activeSessions, candidate)
	if len(matches) == 0 {
		return &models.ComparisonTargetReconciliation{Status: models.ComparisonTargetNoMatch}, nil
	}
	if len(matches) > 1 {
		return &models.ComparisonTargetReconciliation{Status: models.ComparisonTargetAmbiguous}, nil
	}

	matched := matches[0]
	attachedIdentity, ok := s.taskRepositoryComparisonIdentity(ctx, matched)
	if !ok {
		return &models.ComparisonTargetReconciliation{Status: models.ComparisonTargetNoMatch}, nil
	}
	current, hasCurrent, err := models.LoadComparisonTarget(matched.Metadata)
	if err != nil {
		return nil, fmt.Errorf("load current comparison target: %w", err)
	}
	if hasCurrent && !allowReplacement && !current.ChangeIdentityEqual(target) {
		return &models.ComparisonTargetReconciliation{Status: models.ComparisonTargetNoMatch}, nil
	}
	return s.persistMatchedComparisonTarget(ctx, taskID, matched, attachedIdentity, target)
}

func (s *Service) persistMatchedComparisonTarget(
	ctx context.Context,
	taskID string,
	matched *models.TaskRepository,
	attachedIdentity models.ComparisonTargetRepository,
	target models.ComparisonTarget,
) (*models.ComparisonTargetReconciliation, error) {
	if target.IsSameRepository(attachedIdentity) {
		_, changed, err := s.taskRepos.UpdateTaskRepositoryComparisonTarget(ctx, matched.ID, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("clear same-repository comparison target: %w", err)
		}
		if changed {
			s.applyComparisonTargetSideEffects(context.WithoutCancel(ctx), taskID, matched.RepositoryID, matched.BaseBranch)
		}
		return &models.ComparisonTargetReconciliation{
			Status:           models.ComparisonTargetSameRepository,
			TaskRepositoryID: matched.ID,
		}, nil
	}
	_, changed, err := s.taskRepos.UpdateTaskRepositoryComparisonTarget(ctx, matched.ID, &target, nil)
	if err != nil {
		return nil, fmt.Errorf("persist comparison target: %w", err)
	}
	if changed {
		s.applyComparisonTargetSideEffects(context.WithoutCancel(ctx), taskID, matched.RepositoryID, target.TargetBranch)
	}
	return &models.ComparisonTargetReconciliation{
		Status:           models.ComparisonTargetMatched,
		TaskRepositoryID: matched.ID,
		Target:           &target,
	}, nil
}

func (s *Service) matchingComparisonTaskRepositories(
	ctx context.Context,
	taskRepos []*models.TaskRepository,
	activeSessions []*models.TaskSession,
	candidate models.ComparisonTargetCandidate,
) []*models.TaskRepository {
	matches := make([]*models.TaskRepository, 0, 1)
	for _, taskRepo := range taskRepos {
		if taskRepo == nil || !s.taskRepositoryMatchesHead(ctx, taskRepo, candidate.HeadRepository, candidate.Provider) {
			continue
		}
		if comparisonBranchMatches(taskRepo, activeSessions, candidate.HeadBranch) {
			matches = append(matches, taskRepo)
		}
	}
	return matches
}

func (s *Service) activeSessionsForComparison(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if s.sessions == nil {
		return nil, nil
	}
	sessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list active sessions for comparison target: %w", err)
	}
	return sessions, nil
}

func (s *Service) taskRepositoryMatchesHead(
	ctx context.Context,
	taskRepo *models.TaskRepository,
	head models.ComparisonTargetRepository,
	provider string,
) bool {
	if s.repoEntities == nil {
		return false
	}
	repository, err := s.repoEntities.GetRepository(ctx, taskRepo.RepositoryID)
	if err != nil || repository == nil || repository.Provider == "" || !strings.EqualFold(repository.Provider, provider) {
		return false
	}
	identity, ok := repositoryComparisonIdentity(repository)
	return ok && models.ComparisonTargetRepositoriesEqual(identity, head)
}

func (s *Service) taskRepositoryComparisonIdentity(ctx context.Context, taskRepo *models.TaskRepository) (models.ComparisonTargetRepository, bool) {
	if s.repoEntities == nil {
		return models.ComparisonTargetRepository{}, false
	}
	repository, err := s.repoEntities.GetRepository(ctx, taskRepo.RepositoryID)
	if err != nil || repository == nil {
		return models.ComparisonTargetRepository{}, false
	}
	return repositoryComparisonIdentity(repository)
}

func repositoryComparisonIdentity(repository *models.Repository) (models.ComparisonTargetRepository, bool) {
	host := comparisonRepositoryHost(repository.ProviderHost, repository.RemoteURL)
	path := strings.Trim(strings.TrimSuffix(repository.ProviderOwner+"/"+repository.ProviderName, "/"), "/")
	if path == "" {
		path = comparisonRepositoryPath(repository.RemoteURL)
	}
	if host == "" || path == "" {
		return models.ComparisonTargetRepository{}, false
	}
	return models.ComparisonTargetRepository{
		Host:       host,
		Path:       path,
		ProviderID: repository.ProviderRepoID,
		RemoteURL:  "https://" + host + "/" + strings.TrimSuffix(path, ".git") + ".git",
	}, true
}

func comparisonRepositoryHost(providerHost, remoteURL string) string {
	raw := strings.TrimSpace(providerHost)
	if raw == "" {
		raw = remoteURL
	}
	if !strings.Contains(raw, "://") && !strings.Contains(raw, "@") {
		raw = "https://" + raw
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		return strings.ToLower(parsed.Host)
	}
	if at := strings.LastIndex(raw, "@"); at >= 0 {
		raw = raw[at+1:]
	}
	if colon := strings.Index(raw, ":"); colon >= 0 {
		raw = raw[:colon]
	}
	return strings.ToLower(strings.Trim(raw, "/"))
}

func comparisonRepositoryPath(remoteURL string) string {
	raw := strings.TrimSpace(remoteURL)
	if at := strings.LastIndex(raw, ":"); strings.HasPrefix(raw, "git@") && at >= 0 {
		raw = raw[at+1:]
	} else if parsed, err := url.Parse(raw); err == nil {
		raw = parsed.Path
	}
	return strings.Trim(strings.TrimSuffix(raw, ".git"), "/")
}

func comparisonBranchMatches(taskRepo *models.TaskRepository, sessions []*models.TaskSession, branch string) bool {
	branches, hasLiveEvidence := liveComparisonBranches(taskRepo.RepositoryID, sessions)
	if !hasLiveEvidence {
		if len(sessions) > 0 {
			return false
		}
		branches = append(branches, taskRepo.CheckoutBranch)
	}
	for _, candidate := range branches {
		normalized, err := models.NormalizeComparisonBranch(strings.TrimSpace(candidate))
		if err == nil && normalized == branch {
			return true
		}
	}
	return false
}

func liveComparisonBranches(repositoryID string, sessions []*models.TaskSession) ([]string, bool) {
	branches := make([]string, 0)
	hasLiveEvidence := false
	for _, session := range sessions {
		if session == nil {
			continue
		}
		for _, worktree := range session.Worktrees {
			if worktree == nil || worktree.RepositoryID != repositoryID {
				continue
			}
			hasLiveEvidence = true
			branches = append(branches, worktree.WorktreeBranch)
		}
	}
	return branches, hasLiveEvidence
}
