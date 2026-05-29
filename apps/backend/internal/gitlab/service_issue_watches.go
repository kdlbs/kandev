package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// CreateIssueWatch persists a new issue watch.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if !IsValidCleanupPolicy(req.CleanupPolicy) {
		return nil, fmt.Errorf("invalid cleanup_policy: %q", req.CleanupPolicy)
	}
	iw := &IssueWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Projects:            normalizeProjectFilters(req.Projects),
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		Labels:              req.Labels,
		CustomQuery:         req.CustomQuery,
		Enabled:             true,
		PollIntervalSeconds: clampPollInterval(req.PollIntervalSeconds),
		CleanupPolicy:       NormalizeCleanupPolicy(req.CleanupPolicy),
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	if err := store.CreateIssueWatch(ctx, iw); err != nil {
		return nil, fmt.Errorf("create issue watch: %w", err)
	}
	go s.initialIssueCheck(context.Background(), iw)
	return iw, nil
}

func (s *Service) initialIssueCheck(ctx context.Context, watch *IssueWatch) {
	issues, err := s.CheckIssueWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial gitlab issue check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
	}
	for _, issue := range issues {
		s.publishNewIssueEvent(ctx, watch, issue)
	}
}

// GetIssueWatch returns an issue watch by id.
func (s *Service) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetIssueWatch(ctx, id)
}

// ListIssueWatches lists issue watches in a workspace.
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListIssueWatches(ctx, workspaceID)
}

// ListAllIssueWatches returns every issue watch.
func (s *Service) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListAllIssueWatches(ctx)
}

// UpdateIssueWatch applies a partial update.
func (s *Service) UpdateIssueWatch(ctx context.Context, id string, req *UpdateIssueWatchRequest) error {
	if req == nil {
		return fmt.Errorf("nil request")
	}
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	iw, err := store.GetIssueWatch(ctx, id)
	if err != nil {
		return err
	}
	if iw == nil {
		return fmt.Errorf("%w: issue watch %s", ErrWatchNotFound, id)
	}
	applyIssueWatchPatch(iw, req)
	if req.CleanupPolicy != nil && !IsValidCleanupPolicy(*req.CleanupPolicy) {
		return fmt.Errorf("invalid cleanup_policy: %q", *req.CleanupPolicy)
	}
	return store.UpdateIssueWatch(ctx, iw)
}

//nolint:dupl // see applyReviewWatchPatch — same shape, different domain.
func applyIssueWatchPatch(iw *IssueWatch, req *UpdateIssueWatchRequest) {
	if req.WorkflowID != nil {
		iw.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		iw.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Projects != nil {
		iw.Projects = normalizeProjectFilters(*req.Projects)
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
		iw.PollIntervalSeconds = clampPollInterval(*req.PollIntervalSeconds)
	}
	if req.CleanupPolicy != nil {
		iw.CleanupPolicy = NormalizeCleanupPolicy(*req.CleanupPolicy)
	}
}

// DeleteIssueWatch removes an issue watch and best-effort reaps tasks.
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	s.mu.RLock()
	deleter := s.taskDeleter
	s.mu.RUnlock()
	if deleter != nil {
		tasks, err := store.ListIssueWatchTasksByWatch(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list issue tasks for pre-delete sweep",
				zap.String("watch_id", id), zap.Error(err))
		} else {
			s.sweepIssueWatchTasksOnDelete(ctx, id, tasks, deleter)
		}
	}
	return store.DeleteIssueWatch(ctx, id)
}

func (s *Service) sweepIssueWatchTasksOnDelete(ctx context.Context, watchID string, tasks []*IssueWatchTask, deleter TaskDeleter) {
	for _, t := range tasks {
		if t.TaskID == "" {
			continue
		}
		if err := deleter.DeleteTask(ctx, t.TaskID); err != nil {
			s.logger.Warn("failed to delete issue task during watch cleanup",
				zap.String("watch_id", watchID),
				zap.String("task_id", t.TaskID),
				zap.Error(err))
		}
	}
}

// CheckIssueWatch polls a single watch and returns new issues.
//
//nolint:dupl // see CheckReviewWatch — same shape, different domain types.
func (s *Service) CheckIssueWatch(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	if watch == nil {
		return nil, fmt.Errorf("watch is nil")
	}
	if !watch.Enabled {
		return nil, nil
	}
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	issues, err := s.fetchIssues(ctx, watch)
	if err != nil {
		return nil, err
	}
	out := make([]*Issue, 0, len(issues))
	for _, issue := range issues {
		exists, err := store.HasIssueWatchTask(ctx, watch.ID, issue.ProjectPath, issue.IID)
		if err != nil {
			s.logger.Error("check issue watch dedup", zap.Error(err))
			continue
		}
		if !exists {
			out = append(out, issue)
		}
	}
	now := time.Now().UTC()
	if err := store.RecordIssueWatchPoll(ctx, watch.ID, now); err != nil {
		s.logger.Warn("record issue watch poll", zap.String("watch_id", watch.ID), zap.Error(err))
	}
	return out, nil
}

func (s *Service) fetchIssues(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	client := s.Client()
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve gitlab username: %w", err)
	}
	// When a custom_query is set, labels need to be folded into it (the
	// client's buildIssueSearchQuery returns customQuery verbatim and
	// ignores the auxiliary `filter` arg). Otherwise build a default
	// "assigned to me" filter and append labels there.
	filter := ""
	customQuery := watch.CustomQuery
	switch {
	case customQuery != "":
		if len(watch.Labels) > 0 {
			customQuery = appendLabelsToQuery(customQuery, watch.Labels)
		}
	default:
		filter = "assignee_username=" + url.QueryEscape(username)
		if len(watch.Labels) > 0 {
			filter += "&labels=" + url.QueryEscape(strings.Join(watch.Labels, ","))
		}
	}
	issues, err := client.ListIssues(ctx, filter, customQuery)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	if len(watch.Projects) == 0 {
		return issues, nil
	}
	allowed := make(map[string]bool, len(watch.Projects))
	for _, p := range watch.Projects {
		allowed[strings.ToLower(strings.TrimSpace(p.Path))] = true
	}
	out := issues[:0]
	for _, i := range issues {
		if allowed[strings.ToLower(i.ProjectPath)] {
			out = append(out, i)
		}
	}
	return out, nil
}

// TriggerIssueWatch runs the watch once on demand.
func (s *Service) TriggerIssueWatch(ctx context.Context, id string) ([]*Issue, error) {
	iw, err := s.GetIssueWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if iw == nil {
		return nil, fmt.Errorf("%w: issue watch %s", ErrWatchNotFound, id)
	}
	found, err := s.CheckIssueWatch(ctx, iw)
	if err != nil {
		return nil, err
	}
	for _, issue := range found {
		s.publishNewIssueEvent(ctx, iw, issue)
	}
	return found, nil
}

// TriggerIssueWatchAll runs every enabled watch.
func (s *Service) TriggerIssueWatchAll(ctx context.Context) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	watches, err := store.ListEnabledIssueWatches(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, iw := range watches {
		found, err := s.CheckIssueWatch(ctx, iw)
		if err != nil {
			s.logger.Warn("trigger issue watch all", zap.String("watch_id", iw.ID), zap.Error(err))
			continue
		}
		for _, issue := range found {
			s.publishNewIssueEvent(ctx, iw, issue)
		}
		total += len(found)
	}
	return total, nil
}
