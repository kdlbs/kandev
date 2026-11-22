package github

import (
	"context"
	"fmt"
	executoridentity "github.com/kandev/kandev/internal/executors/identity"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// --- Issue Watch service methods ---

// CreateIssueWatch creates a new issue watch and triggers an initial poll.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	return s.createIssueWatch(ctx, req)
}

func (s *Service) CreateIssueWatchForWorkspace(
	ctx context.Context, req *CreateIssueWatchRequest,
) (*IssueWatch, error) {
	if req == nil || strings.TrimSpace(req.WorkspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if err := s.authorizeWorkspaceAccess(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, req.WorkspaceID, "", "")
	if err != nil {
		return nil, err
	}
	ctx = withGitHubOperationFence(ctx, s, req.WorkspaceID, resolved)
	return s.createIssueWatch(ctx, req)
}

func (s *Service) createIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	if req == nil || strings.TrimSpace(req.WorkspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if err := s.authorizeWorkspaceAccess(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	ctx, host, unlock, err := s.prepareWatchCreation(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer unlock()
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
		GitHubHost:          host,
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

	if _, err := s.executorActorContext(ctx, executoridentity.CreatorUserID(ctx), iw.ExecutorProfileID, iw.WorkspaceID); err != nil {
		return nil, err
	}
	if err := s.store.CreateIssueWatch(ctx, iw); err != nil {
		return nil, fmt.Errorf("create issue watch: %w", err)
	}
	// Poll in the background off a *copy*: the watch we return is JSON-encoded
	// by the caller while the check writes LastPolledAt, and sharing the pointer
	// races on that field. The goroutine persists its own timestamp, and the
	// returned watch has legitimately not been polled yet.
	initial := *iw
	go s.initialIssueCheck(context.Background(), &initial)
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
	watch, err := s.store.GetIssueWatch(ctx, id)
	if err != nil || watch == nil {
		return watch, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, watch.WorkspaceID); err != nil {
		return nil, err
	}
	return watch, nil
}

// ListIssueWatches returns all issue watches for a workspace.
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
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
	if err := s.authorizeWorkspaceAccess(ctx, iw.WorkspaceID); err != nil {
		return err
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

	if _, err := s.executorActorContext(ctx, iw.CreatedByUserID, iw.ExecutorProfileID, iw.WorkspaceID); err != nil {
		return err
	}
	return s.store.UpdateIssueWatch(ctx, iw)
}

// DeleteIssueWatch deletes an issue watch and best-effort reaps any tasks
// it owned (mirrors DeleteReviewWatch — list errors log Warn and let the
// watch delete proceed).
//
//nolint:nestif // mirrors DeleteReviewWatch shape
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	watch, err := s.store.GetIssueWatch(ctx, id)
	if err != nil {
		return err
	}
	if watch != nil {
		if err := s.authorizeWorkspaceAccess(ctx, watch.WorkspaceID); err != nil {
			return err
		}
	}
	if s.taskDeleter != nil {
		taskIDs, err := s.store.ListIssueWatchTaskIDsByWatch(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list issue tasks for pre-delete sweep",
				zap.String("watch_id", id), zap.Error(err))
		} else {
			for _, taskID := range taskIDs {
				if taskID == "" {
					continue
				}
				if err := s.taskDeleter.DeleteTask(ctx, taskID); err != nil &&
					!isTaskNotFound(err) {
					s.logger.Warn("failed to delete issue task during watch cleanup",
						zap.String("watch_id", id),
						zap.String("task_id", taskID),
						zap.Error(err))
				}
			}
		}
	}
	return s.store.DeleteIssueWatch(ctx, id)
}

// CheckIssueWatch checks for new issues matching the watch and returns ones not yet tracked.
func (s *Service) CheckIssueWatch(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	if watch == nil || strings.TrimSpace(watch.WorkspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	var admissionErr error
	ctx, admissionErr = s.executorActorContext(ctx, watch.CreatedByUserID, watch.ExecutorProfileID, watch.WorkspaceID)
	if admissionErr != nil {
		return nil, admissionErr
	}

	if err := s.validateIssueWatchHost(ctx, watch); err != nil {
		return nil, err
	}
	ctx = withExpectedGitHubHost(ctx, defaultAuthFlowHost(watch.GitHubHost))
	resolved, err := s.resolveAutomationClient(ctx, watch.WorkspaceID, "", "")
	if err != nil {
		return nil, err
	}

	if resolved.host().String() != defaultAuthFlowHost(watch.GitHubHost) {
		return nil, ErrGitHubHostMismatch
	}
	ctx = withGitHubOperationFence(ctx, s, watch.WorkspaceID, resolved)
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
	issues, complete, err := s.fetchIssues(ctx, resolved.Client, &fetchWatch, scopedCacheKey(resolved.CacheScope, workspaceSearchScopeKey(settings)))
	if err != nil {
		return nil, err
	}
	if err := s.requireCurrentHost(ctx, watch.WorkspaceID, resolved); err != nil {
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
			snapshot := *issue
			snapshot.operationFence = &githubOperationFence{service: s, workspaceID: watch.WorkspaceID, resolved: resolved}
			newIssues = append(newIssues, &snapshot)
		}
	}

	if !complete {
		return newIssues, nil
	}
	ctx, unlock, err := s.lockResourceMutation(ctx, watch.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	now := time.Now().UTC()
	watch.LastPolledAt = &now
	_ = s.store.UpdateIssueWatchLastPolledAt(ctx, watch.ID, now)

	return newIssues, nil
}

// fetchIssues fetches issues based on the watch configuration.
func (s *Service) fetchIssues(ctx context.Context, client Client, watch *IssueWatch, scope string) ([]*Issue, bool, error) {
	hasRepos := len(watch.Repos) > 0

	if !hasRepos {
		filter := s.buildIssueFilter(watch)
		items, err := client.ListIssues(ctx, filter, watch.CustomQuery)
		return items, err == nil, err
	}

	return s.fetchIssuesWithRepoFilter(ctx, client, watch, scope)
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
func (s *Service) fetchIssuesWithRepoFilter(ctx context.Context, client Client, watch *IssueWatch, scope string) ([]*Issue, bool, error) {
	cursor := s.beginWatchFilters(ctx, watchPollKey{kind: "issue", workspace: watch.WorkspaceID, id: watch.ID}, scope, s.buildIssueFilter(watch)+"\n"+watch.CustomQuery, watch.Repos)
	if cursor == nil {
		return nil, false, nil
	}
	defer cursor.mu.Unlock()

	var allIssues []*Issue
	seen := make(map[string]bool)

	labelFilter := s.buildIssueFilter(watch)

	for i := cursor.next; i < len(watch.Repos); i++ {
		repo := watch.Repos[i]
		qualifier := repoFilterToQualifier(repo)
		filter := qualifier
		if labelFilter != "" {
			filter += " " + labelFilter
		}

		var issues []*Issue
		var err error
		if watch.CustomQuery != "" {
			query := watch.CustomQuery + " " + qualifier
			issues, err = client.ListIssues(ctx, "", query)
		} else {
			issues, err = client.ListIssues(ctx, filter, "")
		}
		if watchReadDeferred(err) {
			return allIssues, false, nil
		}
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		cursor.next = i + 1
		if err != nil {
			if IsConnectivityError(err) {
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
	cursor.next = 0
	return allIssues, true, nil
}

// ReserveIssueWatchTask atomically claims a dedup slot.
func (s *Service) ReserveIssueWatchTask(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int, issueURL string) (bool, error) {
	watch, err := s.store.GetIssueWatch(ctx, watchID)
	if err != nil {
		return false, err
	}
	if watch == nil {
		return false, ErrGitHubHostMismatch
	}
	lock := s.workspaceConnectionMutationLock(watch.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()
	if err := s.validateWatchReservation(ctx, watch.WorkspaceID, watch.GitHubHost); err != nil {
		return false, err
	}
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
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return 0, err
	}
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
	host, generation, err := s.watchEventIdentity(ctx, watch.WorkspaceID, watch.GitHubHost, issue.operationFence)
	if err != nil {
		return
	}
	event := bus.NewEvent(events.GitHubNewIssue, "github", &NewIssueEvent{
		GitHubHost: host, CredentialGeneration: generation,
		CreatedByUserID:   watch.CreatedByUserID,
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
