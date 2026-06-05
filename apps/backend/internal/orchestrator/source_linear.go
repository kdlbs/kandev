package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/task/models"
)

// LinearWatcherSource adapts the Linear integration onto the WatcherSource
// pipeline. Wraps the existing LinearService (already used by the orchestrator
// for dedup) and the package-local interpolateLinearPrompt so behaviour is
// preserved byte-for-byte versus the old createLinearIssueTask helper.
type LinearWatcherSource struct {
	service LinearService
	logger  *logger.Logger
}

// NewLinearWatcherSource constructs a source bound to the orchestrator's
// Linear service handle. logger may be nil — methods that log will no-op.
func NewLinearWatcherSource(svc LinearService, log *logger.Logger) *LinearWatcherSource {
	return &LinearWatcherSource{service: svc, logger: log}
}

func (s *LinearWatcherSource) Name() string { return "linear" }

func (s *LinearWatcherSource) Reserve(ctx context.Context, evt any) (bool, error) {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil || e.Issue == nil {
		return false, errors.New("linear source: event payload missing or wrong type")
	}
	// Match today's reserveLinearIssue: nil service is "fail open".
	if s.service == nil {
		return true, nil
	}
	return s.service.ReserveIssueWatchTask(ctx, e.IssueWatchID, e.Issue.Identifier, e.Issue.URL)
}

func (s *LinearWatcherSource) Release(ctx context.Context, evt any) {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil || e.Issue == nil || s.service == nil {
		return
	}
	if err := s.service.ReleaseIssueWatchTask(ctx, e.IssueWatchID, e.Issue.Identifier); err != nil && s.logger != nil {
		s.logger.Warn("linear source: release failed",
			zap.String("identifier", e.Issue.Identifier), zap.Error(err))
	}
}

func (s *LinearWatcherSource) BuildTaskRequest(evt any) (*IssueTaskRequest, error) {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil || e.Issue == nil {
		return nil, errors.New("linear source: event payload missing or wrong type")
	}
	return &IssueTaskRequest{
		WorkspaceID:    e.WorkspaceID,
		WorkflowID:     e.WorkflowID,
		WorkflowStepID: e.WorkflowStepID,
		Title:          fmt.Sprintf("[%s] %s", e.Issue.Identifier, e.Issue.Title),
		Description:    interpolateLinearPrompt(e.Prompt, e.Issue),
		Metadata: map[string]interface{}{
			"linear_issue_watch_id":         e.IssueWatchID,
			"linear_issue_identifier":       e.Issue.Identifier,
			"linear_issue_url":              e.Issue.URL,
			"linear_state":                  e.Issue.StateName,
			"linear_assignee":               e.Issue.AssigneeName,
			models.MetaKeyAgentProfileID:    e.AgentProfileID,
			models.MetaKeyExecutorProfileID: e.ExecutorProfileID,
		},
	}, nil
}

func (s *LinearWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil || e.Issue == nil || s.service == nil {
		return nil
	}
	return s.service.AssignIssueWatchTaskID(ctx, e.IssueWatchID, e.Issue.Identifier, taskID)
}

func (s *LinearWatcherSource) WatchID(evt any) string {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil {
		return ""
	}
	return e.IssueWatchID
}

func (s *LinearWatcherSource) MaxInflightTasks(evt any) *int {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil {
		return nil
	}
	return e.MaxInflightTasks
}

func (s *LinearWatcherSource) AutoStartParams(evt any) AutoStartParams {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil {
		return AutoStartParams{}
	}
	return AutoStartParams{
		AgentProfileID:    e.AgentProfileID,
		ExecutorProfileID: e.ExecutorProfileID,
		WorkflowStepID:    e.WorkflowStepID,
	}
}

// AgentProfileID returns the watcher's bound profile id, or "" when the
// event payload is malformed. Pre-flight uses "" as the skip-check signal.
func (s *LinearWatcherSource) AgentProfileID(evt any) string {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil {
		return ""
	}
	return e.AgentProfileID
}

// SelfHeal disables the linear_issue_watches row that produced this event.
// Nil-safe: with no LinearService wired the call is silently dropped — same
// pattern as Reserve / Release.
func (s *LinearWatcherSource) SelfHeal(ctx context.Context, evt any, cause string) error {
	e, ok := evt.(*linear.NewLinearIssueEvent)
	if !ok || e == nil || s.service == nil {
		return nil
	}
	return s.service.DisableIssueWatchWithError(ctx, e.IssueWatchID, cause)
}
