package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
)

type channelIssueTaskCreator struct {
	requests chan *IssueTaskRequest
}

func (c *channelIssueTaskCreator) CreateIssueTask(_ context.Context, req *IssueTaskRequest) (*models.Task, error) {
	c.requests <- req
	return &models.Task{ID: "task-gitlab-1", Description: req.Description}, nil
}

func TestGitLabReviewEventDispatchesThroughSharedIssueTaskCreator(t *testing.T) {
	creator := &channelIssueTaskCreator{requests: make(chan *IssueTaskRequest, 1)}
	svc := &Service{logger: nopLogger(t)}
	svc.SetIssueTaskCreator(creator)
	svc.SetGitLabService(nil)

	evt := &gitlab.NewReviewMREvent{
		ReviewWatchID:     "review-watch-1",
		WorkspaceID:       "ws-1",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Prompt:            "Review {{mr.url}}",
		MR: &gitlab.MR{
			IID:         17,
			Title:       "Improve GitLab",
			WebURL:      "https://gitlab.example.com/group/project/-/merge_requests/17",
			ProjectPath: "group/project",
		},
	}
	if err := svc.handleGitLabNewReviewMR(context.Background(), &bus.Event{Data: evt}); err != nil {
		t.Fatalf("handle event: %v", err)
	}

	select {
	case req := <-creator.requests:
		if req.Title != "[group/project!17] Improve GitLab" {
			t.Fatalf("title = %q", req.Title)
		}
		if req.Description != "Review https://gitlab.example.com/group/project/-/merge_requests/17" {
			t.Fatalf("description = %q", req.Description)
		}
		if req.Metadata["gitlab_review_watch_id"] != "review-watch-1" {
			t.Fatalf("watch metadata = %#v", req.Metadata)
		}
	case <-time.After(time.Second):
		t.Fatal("GitLab review event was not dispatched through IssueTaskCreator")
	}
}
