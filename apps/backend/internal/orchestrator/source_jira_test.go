package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/jira"
)

type fakeJiraService struct {
	reserveOK  bool
	reserveErr error
	assignErr  error
	releaseErr error
	gotReserve []string
	gotAssign  []string
	gotRelease []string
}

func (f *fakeJiraService) ReserveIssueWatchTask(_ context.Context, watchID, key, _ string) (bool, error) {
	f.gotReserve = append(f.gotReserve, watchID+":"+key)
	return f.reserveOK, f.reserveErr
}

func (f *fakeJiraService) AssignIssueWatchTaskID(_ context.Context, watchID, key, taskID string) error {
	f.gotAssign = append(f.gotAssign, watchID+":"+key+":"+taskID)
	return f.assignErr
}

func (f *fakeJiraService) ReleaseIssueWatchTask(_ context.Context, watchID, key string) error {
	f.gotRelease = append(f.gotRelease, watchID+":"+key)
	return f.releaseErr
}

func sampleJiraEvent() *jira.NewJiraIssueEvent {
	return &jira.NewJiraIssueEvent{
		IssueWatchID:      "watch-1",
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Prompt:            "Pick up {{issue.key}}: {{issue.summary}}",
		Issue: &jira.JiraTicket{
			Key:          "PROJ-1",
			Summary:      "Bug",
			URL:          "https://example.atlassian.net/browse/PROJ-1",
			StatusName:   "Open",
			AssigneeName: "Alice",
			Priority:     "High",
			IssueType:    "Bug",
			ReporterName: "Bob",
			ProjectKey:   "PROJ",
			Description:  "details",
		},
	}
}

func TestJiraSource_Name(t *testing.T) {
	src := &JiraWatcherSource{}
	if src.Name() != "jira" {
		t.Fatalf("expected name=jira, got %q", src.Name())
	}
}

func TestJiraSource_Reserve_Passthrough(t *testing.T) {
	svc := &fakeJiraService{reserveOK: true}
	src := &JiraWatcherSource{service: svc}
	ok, err := src.Reserve(context.Background(), sampleJiraEvent())
	if err != nil || !ok {
		t.Fatalf("expected reserve ok, got ok=%v err=%v", ok, err)
	}
	if len(svc.gotReserve) != 1 || svc.gotReserve[0] != "watch-1:PROJ-1" {
		t.Fatalf("unexpected reserve args: %v", svc.gotReserve)
	}
}

func TestJiraSource_Reserve_NilServiceFailOpen(t *testing.T) {
	src := &JiraWatcherSource{service: nil}
	ok, err := src.Reserve(context.Background(), sampleJiraEvent())
	if err != nil || !ok {
		t.Fatalf("expected nil service to fail open, got ok=%v err=%v", ok, err)
	}
}

func TestJiraSource_Reserve_Error(t *testing.T) {
	svc := &fakeJiraService{reserveErr: errors.New("boom")}
	src := &JiraWatcherSource{service: svc}
	if _, err := src.Reserve(context.Background(), sampleJiraEvent()); err == nil {
		t.Fatal("expected reserve error to surface")
	}
}

func TestJiraSource_BuildTaskRequest(t *testing.T) {
	src := &JiraWatcherSource{}
	req, err := src.BuildTaskRequest(sampleJiraEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Title != "[PROJ-1] Bug" {
		t.Errorf("title = %q", req.Title)
	}
	if req.Description != "Pick up PROJ-1: Bug" {
		t.Errorf("prompt interpolation wrong: %q", req.Description)
	}
	if req.Metadata["jira_issue_key"] != "PROJ-1" {
		t.Errorf("missing jira_issue_key metadata")
	}
	if req.Metadata["agent_profile_id"] != "agent-1" {
		t.Errorf("missing agent_profile_id metadata")
	}
}

func TestJiraSource_BuildTaskRequest_WrongType(t *testing.T) {
	src := &JiraWatcherSource{}
	if _, err := src.BuildTaskRequest("nope"); err == nil {
		t.Fatal("expected error for wrong event type")
	}
}

func TestJiraSource_AttachTaskID(t *testing.T) {
	svc := &fakeJiraService{}
	src := &JiraWatcherSource{service: svc}
	if err := src.AttachTaskID(context.Background(), sampleJiraEvent(), "task-9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.gotAssign) != 1 || svc.gotAssign[0] != "watch-1:PROJ-1:task-9" {
		t.Fatalf("unexpected assign args: %v", svc.gotAssign)
	}
}

func TestJiraSource_Release(t *testing.T) {
	svc := &fakeJiraService{}
	src := &JiraWatcherSource{service: svc}
	src.Release(context.Background(), sampleJiraEvent())
	if len(svc.gotRelease) != 1 || svc.gotRelease[0] != "watch-1:PROJ-1" {
		t.Fatalf("unexpected release args: %v", svc.gotRelease)
	}
}

func TestJiraSource_Release_ErrorIsLoggedNotPropagated(t *testing.T) {
	svc := &fakeJiraService{releaseErr: errors.New("dedup store down")}
	src := &JiraWatcherSource{service: svc, logger: nopLogger(t)}
	// Release returns no value: failure must be swallowed and logged.
	src.Release(context.Background(), sampleJiraEvent())
	if len(svc.gotRelease) != 1 {
		t.Fatalf("expected release call to be attempted, got %d", len(svc.gotRelease))
	}
}

func TestJiraSource_AutoStartParams(t *testing.T) {
	src := &JiraWatcherSource{}
	p := src.AutoStartParams(sampleJiraEvent())
	if p.AgentProfileID != "agent-1" || p.ExecutorProfileID != "exec-1" {
		t.Fatalf("unexpected auto-start params: %+v", p)
	}
	if p.WorkflowStepID != "step-1" {
		t.Errorf("step id wrong: %q", p.WorkflowStepID)
	}
	if p.Prompt != "Pick up PROJ-1: Bug" {
		t.Errorf("prompt should be interpolated, got %q", p.Prompt)
	}
}
