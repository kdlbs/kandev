package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestAzureDevOpsWorkItemWatchEventDispatchesThroughIssueCreator(t *testing.T) {
	creator := &channelIssueTaskCreator{requests: make(chan *IssueTaskRequest, 1)}
	svc := &Service{logger: nopLogger(t)}
	svc.SetIssueTaskCreator(creator)
	svc.SetAzureDevOpsService(nil)
	evt := &azuredevops.WorkItemWatchEvent{
		WatchID: "watch-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Prompt: "Investigate {{work_item.title}}", ProjectID: "Platform", WorkItem: azuredevops.WorkItem{ID: 101, Title: "Fix build", WebURL: "https://dev.azure.com/acme/wi/101"},
	}
	if err := svc.handleAzureDevOpsWorkItemWatchMatch(context.Background(), &bus.Event{Data: evt}); err != nil {
		t.Fatalf("handle event: %v", err)
	}
	select {
	case req := <-creator.requests:
		if req.Title != "[Platform#101] Fix build" || req.Description != "Investigate Fix build" {
			t.Fatalf("request = %+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("Azure work-item watch event was not dispatched")
	}
}

func TestAzureDevOpsPullRequestWatchEventDispatchesThroughIssueCreator(t *testing.T) {
	creator := &channelIssueTaskCreator{requests: make(chan *IssueTaskRequest, 1)}
	svc := &Service{logger: nopLogger(t)}
	svc.SetIssueTaskCreator(creator)
	svc.SetAzureDevOpsService(nil)
	evt := &azuredevops.PullRequestWatchEvent{
		WatchID: "watch-pr", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Prompt: "Review {{pull_request.title}}", ProjectID: "Platform", PullRequest: azuredevops.PullRequest{ID: 42, Title: "Ship it", WebURL: "https://dev.azure.com/acme/pr/42"},
	}
	if err := svc.handleAzureDevOpsPullRequestWatchMatch(context.Background(), &bus.Event{Data: evt}); err != nil {
		t.Fatalf("handle event: %v", err)
	}
	select {
	case req := <-creator.requests:
		if req.Title != "[Platform!42] Ship it" || req.Description != "Review Ship it" {
			t.Fatalf("request = %+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("Azure pull-request watch event was not dispatched")
	}
}
