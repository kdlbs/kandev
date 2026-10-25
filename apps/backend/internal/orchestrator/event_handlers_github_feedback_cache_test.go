package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
)

type feedbackCacheGitHubService struct {
	*mockGitHubService
	invalidationErr error
}

func (s *feedbackCacheGitHubService) InvalidatePRFeedbackForAutomation(
	_ context.Context, _, _, _ string, _ int,
) error {
	if s.invalidationErr != nil {
		return s.invalidationErr
	}
	s.prFeedback = &github.PRFeedback{
		Comments: []github.PRComment{{ID: 99, Body: "New actionable feedback"}},
	}
	return nil
}

func TestPRFeedbackEventInvalidationFailureDoesNotQueue(t *testing.T) {
	ctx := context.Background()
	svc, pr := setupCIAutomationFreshnessTest(t)
	ghSvc := &feedbackCacheGitHubService{
		invalidationErr: errors.New("credential unavailable"),
		mockGitHubService: &mockGitHubService{
			taskPR: pr,
			ciOptionsResp: &github.TaskCIOptionsResponse{
				TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix {{pr.feedback}}",
			},
			prFeedback: &github.PRFeedback{Comments: []github.PRComment{{ID: 99, Body: "Actionable cached feedback"}}},
		},
	}
	svc.SetGitHubService(ghSvc)
	if err := svc.handlePRFeedback(ctx, &bus.Event{Data: &github.PRFeedbackEvent{
		TaskID: pr.TaskID, Owner: pr.Owner, Repo: pr.Repo, PRNumber: pr.PRNumber,
	}}); err != nil {
		t.Fatal(err)
	}
	svc.ciAutomationWorkers.Wait()
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("queued feedback after credential failure = %+v, want no prompt", status)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.7
func TestPRFeedbackEventReadsNewFeedbackWithoutStatusRefresh(t *testing.T) {
	ctx := context.Background()
	svc, pr := setupCIAutomationFreshnessTest(t)
	ghSvc := &feedbackCacheGitHubService{mockGitHubService: &mockGitHubService{
		taskPR: pr,
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix {{pr.feedback}}",
		},
		prFeedback: &github.PRFeedback{},
	}}
	svc.SetGitHubService(ghSvc)
	if err := svc.handlePRFeedback(ctx, &bus.Event{Data: &github.PRFeedbackEvent{
		TaskID: pr.TaskID, Owner: pr.Owner, Repo: pr.Repo, PRNumber: pr.PRNumber,
	}}); err != nil {
		t.Fatal(err)
	}
	svc.ciAutomationWorkers.Wait()
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "New actionable feedback") {
		t.Fatalf("queued feedback = %+v, want newly observed actionable comment", status)
	}
	if ghSvc.triggerPRSyncAllCalls != 0 {
		t.Fatalf("status refresh calls = %d, want 0", ghSvc.triggerPRSyncAllCalls)
	}
}
