package azuredevops

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type WatchTriggerResult struct {
	Matched int `json:"matched"`
}

// Reservation methods are intentionally exposed on Service so the
// orchestrator can keep provider-specific deduplication behind its watcher
// source interface. They do not accept a workspace because the reservation
// key is already scoped by the watch ID and generation; authorization happens
// before the event is published.
func (s *Service) ReserveWorkItemWatchTask(ctx context.Context, watchID string, generation int64, projectID string, workItemID int, workItemURL string) (bool, error) {
	return s.store.ReserveWorkItemWatchTask(ctx, watchID, generation, projectID, workItemID, workItemURL)
}

func (s *Service) AssignWorkItemWatchTaskID(ctx context.Context, watchID string, generation int64, projectID string, workItemID int, taskID string) error {
	return s.store.AssignWorkItemWatchTaskID(ctx, watchID, generation, projectID, workItemID, taskID)
}

func (s *Service) ReleaseWorkItemWatchTask(ctx context.Context, watchID string, generation int64, projectID string, workItemID int) error {
	return s.store.ReleaseWorkItemWatchTask(ctx, watchID, generation, projectID, workItemID)
}

func (s *Service) DisableWorkItemWatchWithError(ctx context.Context, watchID, cause string) error {
	return s.store.DisableWorkItemWatchWithError(ctx, watchID, cause)
}

func (s *Service) ReservePullRequestWatchTask(ctx context.Context, watchID string, generation int64, projectID, azureRepositoryID string, pullRequestID int, pullRequestURL string) (bool, error) {
	return s.store.ReservePullRequestWatchTask(ctx, watchID, generation, projectID, azureRepositoryID, pullRequestID, pullRequestURL)
}

func (s *Service) AssignPullRequestWatchTaskID(ctx context.Context, watchID string, generation int64, projectID, azureRepositoryID string, pullRequestID int, taskID string) error {
	return s.store.AssignPullRequestWatchTaskID(ctx, watchID, generation, projectID, azureRepositoryID, pullRequestID, taskID)
}

func (s *Service) ReleasePullRequestWatchTask(ctx context.Context, watchID string, generation int64, projectID, azureRepositoryID string, pullRequestID int) error {
	return s.store.ReleasePullRequestWatchTask(ctx, watchID, generation, projectID, azureRepositoryID, pullRequestID)
}

func (s *Service) DisablePullRequestWatchWithError(ctx context.Context, watchID, cause string) error {
	return s.store.DisablePullRequestWatchWithError(ctx, watchID, cause)
}

func (s *Service) TriggerWorkItemWatch(ctx context.Context, workspaceID, id string) (*WatchTriggerResult, error) {
	watch, err := s.authorizedWorkItemWatch(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		_ = s.store.SetWorkItemWatchError(ctx, id, err.Error(), time.Now().UTC())
		return nil, err
	}
	result, err := client.QueryWIQL(ctx, watch.ProjectID, watch.WIQL, 100)
	if err != nil {
		_ = s.store.SetWorkItemWatchError(ctx, id, err.Error(), time.Now().UTC())
		return nil, err
	}
	matched := 0
	publishFailed := false
	for _, item := range result.Items {
		reserved, reserveErr := s.store.ReserveWorkItemWatchTask(ctx, watch.ID, watch.Generation, watch.ProjectID, item.ID, item.WebURL)
		if reserveErr != nil {
			_ = s.store.SetWorkItemWatchError(ctx, id, reserveErr.Error(), time.Now().UTC())
			return nil, reserveErr
		}
		if !reserved {
			continue
		}
		if publishErr := s.publishWorkItemWatchEvent(ctx, watch, item); publishErr != nil {
			publishFailed = true
			_ = s.store.ReleaseWorkItemWatchTask(ctx, watch.ID, watch.Generation, watch.ProjectID, item.ID)
			_ = s.store.SetWorkItemWatchError(ctx, id, publishErr.Error(), time.Now().UTC())
			continue
		}
		matched++
	}
	if !publishFailed {
		if err := s.store.ClearWorkItemWatchError(ctx, id, time.Now().UTC()); err != nil {
			return nil, err
		}
	} else if err := s.store.SetWorkItemWatchError(ctx, id, "one or more watcher matches could not be published", time.Now().UTC()); err != nil {
		return nil, err
	}
	return &WatchTriggerResult{Matched: matched}, nil
}

func (s *Service) TriggerPullRequestWatch(ctx context.Context, workspaceID, id string) (*WatchTriggerResult, error) {
	watch, err := s.authorizedPullRequestWatch(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		_ = s.store.SetPullRequestWatchError(ctx, id, err.Error(), time.Now().UTC())
		return nil, err
	}
	result, err := client.ListPullRequests(ctx, PullRequestFilter{
		ProjectID: watch.ProjectID, RepositoryID: watch.AzureRepositoryID,
		Status: watch.Status, CreatorID: watch.CreatorID, ReviewerID: watch.ReviewerID,
		Top: 100,
	})
	if err != nil {
		_ = s.store.SetPullRequestWatchError(ctx, id, err.Error(), time.Now().UTC())
		return nil, err
	}
	matched := 0
	publishFailed := false
	for _, pullRequest := range result.Items {
		reserved, reserveErr := s.store.ReservePullRequestWatchTask(ctx, watch.ID, watch.Generation, watch.ProjectID, pullRequest.RepositoryID, pullRequest.ID, pullRequest.WebURL)
		if reserveErr != nil {
			_ = s.store.SetPullRequestWatchError(ctx, id, reserveErr.Error(), time.Now().UTC())
			return nil, reserveErr
		}
		if !reserved {
			continue
		}
		if publishErr := s.publishPullRequestWatchEvent(ctx, watch, pullRequest); publishErr != nil {
			publishFailed = true
			_ = s.store.ReleasePullRequestWatchTask(ctx, watch.ID, watch.Generation, watch.ProjectID, pullRequest.RepositoryID, pullRequest.ID)
			_ = s.store.SetPullRequestWatchError(ctx, id, publishErr.Error(), time.Now().UTC())
			continue
		}
		matched++
	}
	if !publishFailed {
		if err := s.store.ClearPullRequestWatchError(ctx, id, time.Now().UTC()); err != nil {
			return nil, err
		}
	} else if err := s.store.SetPullRequestWatchError(ctx, id, "one or more watcher matches could not be published", time.Now().UTC()); err != nil {
		return nil, err
	}
	return &WatchTriggerResult{Matched: matched}, nil
}

func (s *Service) publishWorkItemWatchEvent(ctx context.Context, watch *WorkItemWatch, item WorkItem) error {
	if s.eventBus == nil {
		return nil
	}
	event := &WorkItemWatchEvent{
		WatchID: watch.ID, WatchGeneration: watch.Generation, WorkspaceID: watch.WorkspaceID,
		WorkflowID: watch.WorkflowID, WorkflowStepID: watch.WorkflowStepID,
		RepositoryID: watch.RepositoryID, BaseBranch: watch.BaseBranch,
		AgentProfileID: watch.AgentProfileID, ExecutorProfileID: watch.ExecutorProfileID,
		Prompt: watch.Prompt, CleanupPolicy: watch.CleanupPolicy,
		MaxInflightTasks: watch.MaxInflightTasks, ProjectID: watch.ProjectID, WorkItem: item,
		ReservationClaimed: true,
	}
	if err := s.eventBus.Publish(ctx, events.AzureDevOpsWorkItemWatchMatch, bus.NewEvent(events.AzureDevOpsWorkItemWatchMatch, "azuredevops", event)); err != nil {
		s.log.Warn("azure devops: publish work-item watch event failed")
		return err
	}
	return nil
}

func (s *Service) publishPullRequestWatchEvent(ctx context.Context, watch *PullRequestWatch, pullRequest PullRequest) error {
	if s.eventBus == nil {
		return nil
	}
	event := &PullRequestWatchEvent{
		WatchID: watch.ID, WatchGeneration: watch.Generation, WorkspaceID: watch.WorkspaceID,
		WorkflowID: watch.WorkflowID, WorkflowStepID: watch.WorkflowStepID,
		RepositoryID: watch.RepositoryID, BaseBranch: watch.BaseBranch,
		AgentProfileID: watch.AgentProfileID, ExecutorProfileID: watch.ExecutorProfileID,
		Prompt: watch.Prompt, CleanupPolicy: watch.CleanupPolicy,
		MaxInflightTasks: watch.MaxInflightTasks, ProjectID: watch.ProjectID,
		AzureRepositoryID: pullRequest.RepositoryID, PullRequest: pullRequest,
		ReservationClaimed: true,
	}
	if err := s.eventBus.Publish(ctx, events.AzureDevOpsPullRequestWatchMatch, bus.NewEvent(events.AzureDevOpsPullRequestWatchMatch, "azuredevops", event)); err != nil {
		s.log.Warn("azure devops: publish pull-request watch event failed")
		return err
	}
	return nil
}

func (s *Service) CreateWorkItemWatch(ctx context.Context, req *CreateWorkItemWatchRequest) (*WorkItemWatch, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidConfig)
	}
	if err := validateWorkspaceID(req.WorkspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	if err := s.validateWatchDependencies(ctx, req.WorkspaceID, req.WorkflowID, req.WorkflowStepID, req.AgentProfileID, req.ExecutorProfileID); err != nil {
		return nil, err
	}
	repositoryID, baseBranch, err := s.resolveWatchRepository(ctx, req.WorkspaceID, req.RepositoryID, req.BaseBranch)
	if err != nil {
		return nil, err
	}
	watch := &WorkItemWatch{
		WorkspaceID: req.WorkspaceID, WorkflowID: req.WorkflowID, WorkflowStepID: req.WorkflowStepID,
		ProjectID: req.ProjectID, WIQL: req.WIQL, RepositoryID: repositoryID,
		BaseBranch: baseBranch, AgentProfileID: req.AgentProfileID,
		ExecutorProfileID: req.ExecutorProfileID, Prompt: req.Prompt,
		PollIntervalSeconds: req.PollIntervalSeconds, CleanupPolicy: req.CleanupPolicy,
		MaxInflightTasks: req.MaxInflightTasks,
	}
	if err := s.store.CreateWorkItemWatch(ctx, watch); err != nil {
		return nil, err
	}
	s.triggerInitialWorkItemWatch(watch)
	return watch, nil
}

func (s *Service) ListWorkItemWatches(ctx context.Context, workspaceID string) ([]*WorkItemWatch, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListWorkItemWatches(ctx, workspaceID)
}

func (s *Service) UpdateWorkItemWatch(ctx context.Context, workspaceID, id string, req *UpdateWorkItemWatchRequest) (*WorkItemWatch, error) {
	watch, err := s.authorizedWorkItemWatch(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	if req != nil {
		applyWorkItemWatchUpdate(watch, req)
	}
	if err := s.validateWatchDependencies(ctx, watch.WorkspaceID, watch.WorkflowID, watch.WorkflowStepID, watch.AgentProfileID, watch.ExecutorProfileID); err != nil {
		return nil, err
	}
	watch.RepositoryID, watch.BaseBranch, err = s.resolveWatchRepository(ctx, watch.WorkspaceID, watch.RepositoryID, watch.BaseBranch)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateWorkItemWatch(ctx, watch); err != nil {
		return nil, err
	}
	return watch, nil
}

func (s *Service) DeleteWorkItemWatch(ctx context.Context, workspaceID, id string) error {
	if _, err := s.authorizedWorkItemWatch(ctx, workspaceID, id); err != nil {
		return err
	}
	if err := s.cleanupWatchTaskTrees(ctx, id, s.store.ListWorkItemWatchTaskIDs); err != nil {
		return err
	}
	return s.store.DeleteWorkItemWatch(ctx, id)
}

func (s *Service) PreviewResetWorkItemWatch(ctx context.Context, workspaceID, id string) (int, error) {
	if _, err := s.authorizedWorkItemWatch(ctx, workspaceID, id); err != nil {
		return 0, err
	}
	ids, err := s.store.ListWorkItemWatchTaskIDs(ctx, id)
	return len(ids), err
}

func (s *Service) ResetWorkItemWatch(ctx context.Context, workspaceID, id string) (*WatchResetResult, error) {
	if _, err := s.authorizedWorkItemWatch(ctx, workspaceID, id); err != nil {
		return nil, err
	}
	reset, err := s.store.BeginWorkItemWatchReset(ctx, id)
	if err != nil {
		return nil, err
	}
	s.cleanupTaskIDs(ctx, id, reset.TaskIDs)
	if err := s.store.FinishWorkItemWatchReset(ctx, id, reset.Generation); err != nil {
		return nil, err
	}
	return reset, nil
}

func (s *Service) CreatePullRequestWatch(ctx context.Context, req *CreatePullRequestWatchRequest) (*PullRequestWatch, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidConfig)
	}
	if err := validateWorkspaceID(req.WorkspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	if err := s.validateWatchDependencies(ctx, req.WorkspaceID, req.WorkflowID, req.WorkflowStepID, req.AgentProfileID, req.ExecutorProfileID); err != nil {
		return nil, err
	}
	repositoryID, baseBranch, err := s.resolveWatchRepository(ctx, req.WorkspaceID, req.RepositoryID, req.BaseBranch)
	if err != nil {
		return nil, err
	}
	watch := &PullRequestWatch{
		WorkspaceID: req.WorkspaceID, WorkflowID: req.WorkflowID, WorkflowStepID: req.WorkflowStepID,
		ProjectID: req.ProjectID, AzureRepositoryID: req.AzureRepositoryID, Status: req.Status,
		CreatorID: req.CreatorID, ReviewerID: req.ReviewerID, RepositoryID: repositoryID,
		BaseBranch: baseBranch, AgentProfileID: req.AgentProfileID,
		ExecutorProfileID: req.ExecutorProfileID, Prompt: req.Prompt,
		PollIntervalSeconds: req.PollIntervalSeconds, CleanupPolicy: req.CleanupPolicy,
		MaxInflightTasks: req.MaxInflightTasks,
	}
	if err := s.store.CreatePullRequestWatch(ctx, watch); err != nil {
		return nil, err
	}
	s.triggerInitialPullRequestWatch(watch)
	return watch, nil
}

func (s *Service) triggerInitialWorkItemWatch(watch *WorkItemWatch) {
	if watch == nil {
		return
	}
	go func() {
		if _, err := s.TriggerWorkItemWatch(context.Background(), watch.WorkspaceID, watch.ID); err != nil {
			s.log.Warn("azure devops watcher: initial work-item check failed", zap.String("watch_id", watch.ID), zap.Error(err))
		}
	}()
}

func (s *Service) triggerInitialPullRequestWatch(watch *PullRequestWatch) {
	if watch == nil {
		return
	}
	go func() {
		if _, err := s.TriggerPullRequestWatch(context.Background(), watch.WorkspaceID, watch.ID); err != nil {
			s.log.Warn("azure devops watcher: initial pull-request check failed", zap.String("watch_id", watch.ID), zap.Error(err))
		}
	}()
}

func (s *Service) ListPullRequestWatches(ctx context.Context, workspaceID string) ([]*PullRequestWatch, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListPullRequestWatches(ctx, workspaceID)
}

func (s *Service) UpdatePullRequestWatch(ctx context.Context, workspaceID, id string, req *UpdatePullRequestWatchRequest) (*PullRequestWatch, error) {
	watch, err := s.authorizedPullRequestWatch(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	if req != nil {
		applyPullRequestWatchUpdate(watch, req)
	}
	if err := s.validateWatchDependencies(ctx, watch.WorkspaceID, watch.WorkflowID, watch.WorkflowStepID, watch.AgentProfileID, watch.ExecutorProfileID); err != nil {
		return nil, err
	}
	watch.RepositoryID, watch.BaseBranch, err = s.resolveWatchRepository(ctx, watch.WorkspaceID, watch.RepositoryID, watch.BaseBranch)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdatePullRequestWatch(ctx, watch); err != nil {
		return nil, err
	}
	return watch, nil
}

func (s *Service) DeletePullRequestWatch(ctx context.Context, workspaceID, id string) error {
	if _, err := s.authorizedPullRequestWatch(ctx, workspaceID, id); err != nil {
		return err
	}
	if err := s.cleanupWatchTaskTrees(ctx, id, s.store.ListPullRequestWatchTaskIDs); err != nil {
		return err
	}
	return s.store.DeletePullRequestWatch(ctx, id)
}

func (s *Service) PreviewResetPullRequestWatch(ctx context.Context, workspaceID, id string) (int, error) {
	if _, err := s.authorizedPullRequestWatch(ctx, workspaceID, id); err != nil {
		return 0, err
	}
	ids, err := s.store.ListPullRequestWatchTaskIDs(ctx, id)
	return len(ids), err
}

func (s *Service) ResetPullRequestWatch(ctx context.Context, workspaceID, id string) (*WatchResetResult, error) {
	if _, err := s.authorizedPullRequestWatch(ctx, workspaceID, id); err != nil {
		return nil, err
	}
	reset, err := s.store.BeginPullRequestWatchReset(ctx, id)
	if err != nil {
		return nil, err
	}
	s.cleanupTaskIDs(ctx, id, reset.TaskIDs)
	if err := s.store.FinishPullRequestWatchReset(ctx, id, reset.Generation); err != nil {
		return nil, err
	}
	return reset, nil
}

func (s *Service) cleanupWatchTaskTrees(ctx context.Context, watchID string, list func(context.Context, string) ([]string, error)) error {
	ids, err := list(ctx, watchID)
	if err != nil {
		return err
	}
	s.cleanupTaskIDs(ctx, watchID, ids)
	return nil
}

func (s *Service) cleanupTaskIDs(ctx context.Context, watchID string, ids []string) {
	deleter := s.getCascadeTaskDeleter()
	if deleter == nil {
		return
	}
	for _, taskID := range ids {
		if taskID == "" {
			continue
		}
		if _, err := deleter.DeleteTaskTree(ctx, taskID, true); err != nil {
			s.log.Warn("azure devops watcher: task cleanup failed", zap.String("watch_id", watchID), zap.String("task_id", taskID), zap.Error(err))
		}
	}
}

func (s *Service) authorizedWorkItemWatch(ctx context.Context, workspaceID, id string) (*WorkItemWatch, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	watch, err := s.store.GetWorkItemWatch(ctx, id)
	if err != nil || watch == nil || watch.WorkspaceID != workspaceID || watch.Deleting {
		return nil, ErrWatchNotFound
	}
	return watch, nil
}

func (s *Service) authorizedPullRequestWatch(ctx context.Context, workspaceID, id string) (*PullRequestWatch, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	watch, err := s.store.GetPullRequestWatch(ctx, id)
	if err != nil || watch == nil || watch.WorkspaceID != workspaceID || watch.Deleting {
		return nil, ErrWatchNotFound
	}
	return watch, nil
}

func applyWorkItemWatchUpdate(watch *WorkItemWatch, req *UpdateWorkItemWatchRequest) {
	if req.WorkflowID != nil {
		watch.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		watch.WorkflowStepID = *req.WorkflowStepID
	}
	if req.ProjectID != nil {
		watch.ProjectID = *req.ProjectID
	}
	if req.WIQL != nil {
		watch.WIQL = *req.WIQL
	}
	if req.RepositoryID != nil {
		watch.RepositoryID = *req.RepositoryID
	}
	if req.BaseBranch != nil {
		watch.BaseBranch = *req.BaseBranch
	}
	if req.AgentProfileID != nil {
		watch.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		watch.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		watch.Prompt = *req.Prompt
	}
	if req.Enabled != nil {
		watch.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		watch.PollIntervalSeconds = *req.PollIntervalSeconds
	}
	if req.CleanupPolicy != nil {
		watch.CleanupPolicy = *req.CleanupPolicy
	}
	if req.MaxInflightTasks != nil {
		if *req.MaxInflightTasks == 0 {
			watch.MaxInflightTasks = nil
		} else {
			watch.MaxInflightTasks = req.MaxInflightTasks
		}
	}
}

func applyPullRequestWatchUpdate(watch *PullRequestWatch, req *UpdatePullRequestWatchRequest) {
	applyPullRequestWatchFilters(watch, req)
	if req.WorkflowID != nil {
		watch.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		watch.WorkflowStepID = *req.WorkflowStepID
	}
	if req.ProjectID != nil {
		watch.ProjectID = *req.ProjectID
	}
	if req.RepositoryID != nil {
		watch.RepositoryID = *req.RepositoryID
	}
	if req.BaseBranch != nil {
		watch.BaseBranch = *req.BaseBranch
	}
	if req.AgentProfileID != nil {
		watch.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		watch.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		watch.Prompt = *req.Prompt
	}
	applyWatchRuntimeUpdate(
		&watch.Enabled,
		&watch.PollIntervalSeconds,
		&watch.CleanupPolicy,
		&watch.MaxInflightTasks,
		req.Enabled,
		req.PollIntervalSeconds,
		req.CleanupPolicy,
		req.MaxInflightTasks,
	)
}

func applyPullRequestWatchFilters(watch *PullRequestWatch, req *UpdatePullRequestWatchRequest) {
	if req.AzureRepositoryID != nil {
		watch.AzureRepositoryID = *req.AzureRepositoryID
	}
	if req.Status != nil {
		watch.Status = *req.Status
	}
	if req.CreatorID != nil {
		watch.CreatorID = *req.CreatorID
	}
	if req.ReviewerID != nil {
		watch.ReviewerID = *req.ReviewerID
	}
}

func applyWatchRuntimeUpdate(
	enabled *bool,
	pollInterval *int,
	cleanupPolicy *string,
	maxInflight **int,
	requestedEnabled *bool,
	requestedPollInterval *int,
	requestedCleanupPolicy *string,
	requestedMaxInflight *int,
) {
	if requestedEnabled != nil {
		*enabled = *requestedEnabled
	}
	if requestedPollInterval != nil {
		*pollInterval = *requestedPollInterval
	}
	if requestedCleanupPolicy != nil {
		*cleanupPolicy = *requestedCleanupPolicy
	}
	if requestedMaxInflight != nil {
		if *requestedMaxInflight == 0 {
			*maxInflight = nil
		} else {
			*maxInflight = requestedMaxInflight
		}
	}
}
