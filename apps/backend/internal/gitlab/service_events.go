package gitlab

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// publishMRFeedbackEvent fires when a watched MR's pipeline or approval
// state changes. Used by the UI to surface a toast or refresh the task.
func (s *Service) publishMRFeedbackEvent(ctx context.Context, watch *MRWatch, status *MRStatus) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	ev := &MRFeedbackEvent{
		SessionID:        watch.SessionID,
		TaskID:           watch.TaskID,
		ProjectPath:      watch.ProjectPath,
		MRIID:            watch.MRIID,
		NewPipelineState: status.PipelineState,
		NewApprovalState: status.ApprovalState,
	}
	if err := eb.Publish(ctx, events.GitLabMRFeedback, bus.NewEvent(events.GitLabMRFeedback, eventSource, ev)); err != nil {
		s.logger.Warn("publish gitlab feedback event", zap.Error(err))
	}
}

// publishNewReviewMREvent fires when a review watch finds a new MR.
func (s *Service) publishNewReviewMREvent(ctx context.Context, watch *ReviewWatch, mr *MR) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	ev := &NewReviewMREvent{
		ReviewWatchID:     watch.ID,
		WorkspaceID:       watch.WorkspaceID,
		WorkflowID:        watch.WorkflowID,
		WorkflowStepID:    watch.WorkflowStepID,
		AgentProfileID:    watch.AgentProfileID,
		ExecutorProfileID: watch.ExecutorProfileID,
		Prompt:            watch.Prompt,
		MR:                mr,
	}
	if err := eb.Publish(ctx, events.GitLabNewReviewMR, bus.NewEvent(events.GitLabNewReviewMR, eventSource, ev)); err != nil {
		s.logger.Warn("publish gitlab new review MR event", zap.Error(err))
	}
}

// publishNewIssueEvent fires when an issue watch finds a new issue.
func (s *Service) publishNewIssueEvent(ctx context.Context, watch *IssueWatch, issue *Issue) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	ev := &NewIssueEvent{
		IssueWatchID:      watch.ID,
		WorkspaceID:       watch.WorkspaceID,
		WorkflowID:        watch.WorkflowID,
		WorkflowStepID:    watch.WorkflowStepID,
		AgentProfileID:    watch.AgentProfileID,
		ExecutorProfileID: watch.ExecutorProfileID,
		Prompt:            watch.Prompt,
		Issue:             issue,
	}
	if err := eb.Publish(ctx, events.GitLabNewIssue, bus.NewEvent(events.GitLabNewIssue, eventSource, ev)); err != nil {
		s.logger.Warn("publish gitlab new issue event", zap.Error(err))
	}
}

// publishWatchEvent fires when a watch is created/deleted (UI list refresh).
func (s *Service) publishWatchEvent(ctx context.Context, kind, id, sessionID, taskID string) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	_ = eb.Publish(ctx, events.GitLabWatchEvent, bus.NewEvent(events.GitLabWatchEvent, eventSource, map[string]string{
		"kind":       kind,
		"id":         id,
		"session_id": sessionID,
		"task_id":    taskID,
	}))
}
