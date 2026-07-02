package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// --- Issue Watch service methods ---

// CreateIssueWatch creates a new issue watch and triggers an initial poll.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	if req.PollIntervalSeconds <= 0 {
		req.PollIntervalSeconds = defaultWatchPollIntervalSec
	}
	if req.PollIntervalSeconds < minWatchPollIntervalSec {
		req.PollIntervalSeconds = minWatchPollIntervalSec
	}
	repos := req.Repos
	if repos == nil {
		repos = []RepoFilter{}
	}
	labels := req.Labels
	if labels == nil {
		labels = []string{}
	}
	if !IsValidCleanupPolicy(req.CleanupPolicy) {
		return nil, fmt.Errorf("invalid cleanup_policy: %q", req.CleanupPolicy)
	}
	iw := &IssueWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Repos:               repos,
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		Labels:              labels,
		CustomQuery:         req.CustomQuery,
		Enabled:             true,
		PollIntervalSeconds: req.PollIntervalSeconds,
		CleanupPolicy:       NormalizeCleanupPolicy(req.CleanupPolicy),
	}
	if err := s.store.CreateIssueWatch(ctx, iw); err != nil {
		return nil, fmt.Errorf("create issue watch: %w", err)
	}
	go s.initialIssueCheck(context.Background(), iw)
	return iw, nil
}

// initialIssueCheck runs a single poll for a newly created issue watch.
func (s *Service) initialIssueCheck(ctx context.Context, watch *IssueWatch) {
	newIssues, err := s.CheckIssueWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial issue check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
	}
	for _, issue := range newIssues {
		s.publishNewIssueEvent(ctx, watch, issue)
	}
	if len(newIssues) > 0 {
		s.logger.Info("initial issue check found issues",
			zap.String("watch_id", watch.ID),
			zap.Int("new_issues", len(newIssues)))
	}
}

// GetIssueWatch returns a single issue watch by ID.
func (s *Service) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	return s.store.GetIssueWatch(ctx, id)
}

// ListIssueWatches returns all issue watches for a workspace.
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	return s.store.ListIssueWatches(ctx, workspaceID)
}

// ListAllIssueWatches returns every issue watch across all workspaces.
func (s *Service) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.store.ListAllIssueWatches(ctx)
}

// ListEnabledIssueWatches returns the live (enabled = 1) subset. Used by
// the profile-delete dependency check so self-healed (already-disabled)
// watchers do not inflate the count and trigger spurious 409 confirmations.
func (s *Service) ListEnabledIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.store.ListEnabledIssueWatches(ctx)
}

// UpdateIssueWatch updates an issue watch.
//
//nolint:dupl,cyclop // mirrors UpdateReviewWatch — different types, same structure; field-by-field nil-pointer apply
func (s *Service) UpdateIssueWatch(ctx context.Context, id string, req *UpdateIssueWatchRequest) error {
	iw, err := s.store.GetIssueWatch(ctx, id)
	if err != nil {
		return err
	}
	if iw == nil {
		return fmt.Errorf("issue watch not found: %s", id)
	}
	if req.WorkflowID != nil {
		iw.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		iw.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Repos != nil {
		iw.Repos = *req.Repos
	}
	if req.AgentProfileID != nil {
		iw.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		iw.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		iw.Prompt = *req.Prompt
	}
	if req.Labels != nil {
		iw.Labels = *req.Labels
	}
	if req.CustomQuery != nil {
		iw.CustomQuery = *req.CustomQuery
	}
	if req.Enabled != nil {
		iw.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		v := *req.PollIntervalSeconds
		if v <= 0 {
			v = defaultWatchPollIntervalSec
		}
		if v < minWatchPollIntervalSec {
			v = minWatchPollIntervalSec
		}
		iw.PollIntervalSeconds = v
	}
	if req.CleanupPolicy != nil {
		if !IsValidCleanupPolicy(*req.CleanupPolicy) {
			return fmt.Errorf("invalid cleanup_policy: %q", *req.CleanupPolicy)
		}
		iw.CleanupPolicy = NormalizeCleanupPolicy(*req.CleanupPolicy)
	}
	return s.store.UpdateIssueWatch(ctx, iw)
}

// DeleteIssueWatch deletes an issue watch and best-effort reaps any tasks
// it owned (mirrors DeleteReviewWatch — list errors log Warn and let the
// watch delete proceed).
//
//nolint:nestif // mirrors DeleteReviewWatch shape
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	if s.taskDeleter != nil {
		issueTasks, err := s.store.ListIssueWatchTasksByWatch(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list issue tasks for pre-delete sweep",
				zap.String("watch_id", id), zap.Error(err))
		} else {
			for _, it := range issueTasks {
				if it.TaskID == "" {
					continue
				}
				if err := s.taskDeleter.DeleteTask(ctx, it.TaskID); err != nil &&
					!isTaskNotFound(err) {
					s.logger.Warn("failed to delete issue task during watch cleanup",
						zap.String("watch_id", id),
						zap.String("task_id", it.TaskID),
						zap.Error(err))
				}
			}
		}
	}
	return s.store.DeleteIssueWatch(ctx, id)
}

// CheckIssueWatch checks for new issues matching the watch and returns ones not yet tracked.
func (s *Service) CheckIssueWatch(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}

	s.logger.Debug("checking issue watch for new issues",
		zap.String("watch_id", watch.ID),
		zap.Int("repo_filters", len(watch.Repos)),
		zap.String("custom_query", watch.CustomQuery),
		zap.Bool("enabled", watch.Enabled))

	settings, settingsErr := s.GetWorkspaceSettings(ctx, watch.WorkspaceID)
	if settingsErr != nil {
		return nil, settingsErr
	}
	if len(watch.Repos) == 0 && workspaceSettingsHasEmptyScope(settings) {
		return nil, nil
	}
	fetchWatch := *watch
	if len(fetchWatch.Repos) == 0 {
		fetchWatch.Repos = workspaceScopeRepoFilters(settings)
	}
	issues, err := s.fetchIssues(ctx, &fetchWatch)
	if err != nil {
		return nil, err
	}
	issues = filterIssuesByWorkspaceScope(issues, settings)

	var newIssues []*Issue
	for _, issue := range issues {
		exists, checkErr := s.store.HasIssueWatchTask(ctx, watch.ID, issue.RepoOwner, issue.RepoName, issue.Number)
		if checkErr != nil {
			s.logger.Error("failed to check issue watch task", zap.Error(checkErr))
			continue
		}
		if !exists {
			newIssues = append(newIssues, issue)
		}
	}

	now := time.Now().UTC()
	watch.LastPolledAt = &now
	_ = s.store.UpdateIssueWatch(ctx, watch)

	return newIssues, nil
}

// fetchIssues fetches issues based on the watch configuration.
func (s *Service) fetchIssues(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	hasRepos := len(watch.Repos) > 0

	if !hasRepos {
		filter := s.buildIssueFilter(watch)
		return s.client.ListIssues(ctx, filter, watch.CustomQuery)
	}

	return s.fetchIssuesWithRepoFilter(ctx, watch), nil
}

// buildIssueFilter builds the filter qualifier from watch labels. `state:open`
// is included because the watcher is only interested in active issues —
// buildIssueSearchQuery no longer injects it (the /github page presets
// supply their own state qualifiers), so we add it here instead.
func (s *Service) buildIssueFilter(watch *IssueWatch) string {
	parts := []string{"state:open"}
	for _, label := range watch.Labels {
		if strings.ContainsRune(label, ' ') {
			parts = append(parts, `label:"`+label+`"`)
		} else {
			parts = append(parts, "label:"+label)
		}
	}
	return strings.Join(parts, " ")
}

// fetchIssuesWithRepoFilter queries each repo individually and deduplicates.
func (s *Service) fetchIssuesWithRepoFilter(ctx context.Context, watch *IssueWatch) []*Issue {
	var allIssues []*Issue
	seen := make(map[string]bool)

	labelFilter := s.buildIssueFilter(watch)

	for _, repo := range watch.Repos {
		qualifier := repoFilterToQualifier(repo)
		filter := qualifier
		if labelFilter != "" {
			filter += " " + labelFilter
		}

		var issues []*Issue
		var err error
		if watch.CustomQuery != "" {
			query := watch.CustomQuery + " " + qualifier
			issues, err = s.client.ListIssues(ctx, "", query)
		} else {
			issues, err = s.client.ListIssues(ctx, filter, "")
		}
		if err != nil {
			if isConnectivityError(err) {
				s.logger.Warn("failed to list issues (connectivity)",
					zap.String("filter", qualifier), zap.Error(err))
			} else {
				s.logger.Error("failed to list issues",
					zap.String("filter", qualifier), zap.Error(err))
			}
			continue
		}

		for _, issue := range issues {
			key := fmt.Sprintf("%s/%s#%d", issue.RepoOwner, issue.RepoName, issue.Number)
			if !seen[key] {
				seen[key] = true
				allIssues = append(allIssues, issue)
			}
		}
	}
	return allIssues
}

// ReserveIssueWatchTask atomically claims a dedup slot.
func (s *Service) ReserveIssueWatchTask(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int, issueURL string) (bool, error) {
	return s.store.ReserveIssueWatchTask(ctx, watchID, repoOwner, repoName, issueNumber, issueURL)
}

// AssignIssueWatchTaskID attaches a task ID to a previously reserved slot.
func (s *Service) AssignIssueWatchTaskID(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int, taskID string) error {
	return s.store.AssignIssueWatchTaskID(ctx, watchID, repoOwner, repoName, issueNumber, taskID)
}

// ReleaseIssueWatchTask removes a reservation when task creation fails.
func (s *Service) ReleaseIssueWatchTask(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int) error {
	return s.store.ReleaseIssueWatchTask(ctx, watchID, repoOwner, repoName, issueNumber)
}

// DisableIssueWatchWithError is invoked by the orchestrator's self-heal flow
// when the watcher's bound agent profile has been soft-deleted.
func (s *Service) DisableIssueWatchWithError(ctx context.Context, watchID, cause string) error {
	return s.store.DisableIssueWatchWithError(ctx, watchID, cause)
}

// TriggerAllIssueChecks triggers all issue watches for a workspace.
//
//nolint:dupl // mirrors TriggerAllReviewChecks — different types, same structure
func (s *Service) TriggerAllIssueChecks(ctx context.Context, workspaceID string) (int, error) {
	watches, err := s.store.ListIssueWatches(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	enabled := 0
	for _, w := range watches {
		if w.Enabled {
			enabled++
		}
	}
	s.logger.Info("triggering issue checks",
		zap.String("workspace_id", workspaceID),
		zap.Int("total_watches", len(watches)),
		zap.Int("enabled_watches", enabled))

	totalNew := 0
	for _, watch := range watches {
		if !watch.Enabled {
			continue
		}
		newIssues, checkErr := s.CheckIssueWatch(ctx, watch)
		if checkErr != nil {
			s.logger.Error("failed to check issue watch",
				zap.String("watch_id", watch.ID), zap.Error(checkErr))
			continue
		}
		for _, issue := range newIssues {
			s.publishNewIssueEvent(ctx, watch, issue)
		}
		totalNew += len(newIssues)
		if _, cleanErr := s.CleanupClosedIssueTasks(ctx, watch); cleanErr != nil {
			s.logger.Warn("cleanup closed issue tasks failed",
				zap.String("watch_id", watch.ID), zap.Error(cleanErr))
		}
	}
	s.logger.Info("issue checks completed",
		zap.String("workspace_id", workspaceID),
		zap.Int("new_issues_found", totalNew))
	return totalNew, nil
}

func (s *Service) publishNewIssueEvent(ctx context.Context, watch *IssueWatch, issue *Issue) {
	if s.eventBus == nil {
		return
	}
	event := bus.NewEvent(events.GitHubNewIssue, "github", &NewIssueEvent{
		IssueWatchID:      watch.ID,
		WorkspaceID:       watch.WorkspaceID,
		WorkflowID:        watch.WorkflowID,
		WorkflowStepID:    watch.WorkflowStepID,
		AgentProfileID:    watch.AgentProfileID,
		ExecutorProfileID: watch.ExecutorProfileID,
		Prompt:            watch.Prompt,
		Issue:             issue,
	})
	if err := s.eventBus.Publish(ctx, events.GitHubNewIssue, event); err != nil {
		s.logger.Debug("failed to publish new issue event", zap.Error(err))
	}
}
