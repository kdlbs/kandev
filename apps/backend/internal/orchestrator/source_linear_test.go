package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/linear"
)

type fakeLinearService struct {
	reserveOK  bool
	reserveErr error
	assignErr  error
	releaseErr error
	disableErr error
	gotReserve []string
	gotAssign  []string
	gotRelease []string
	gotDisable []string
}

func (f *fakeLinearService) ReserveIssueWatchTask(_ context.Context, watchID, id, _ string) (bool, error) {
	f.gotReserve = append(f.gotReserve, watchID+":"+id)
	return f.reserveOK, f.reserveErr
}

func (f *fakeLinearService) AssignIssueWatchTaskID(_ context.Context, watchID, id, taskID string) error {
	f.gotAssign = append(f.gotAssign, watchID+":"+id+":"+taskID)
	return f.assignErr
}

func (f *fakeLinearService) ReleaseIssueWatchTask(_ context.Context, watchID, id string) error {
	f.gotRelease = append(f.gotRelease, watchID+":"+id)
	return f.releaseErr
}

func (f *fakeLinearService) DisableIssueWatchWithError(_ context.Context, watchID, cause string) error {
	f.gotDisable = append(f.gotDisable, watchID+":"+cause)
	return f.disableErr
}

func sampleLinearEvent() *linear.NewLinearIssueEvent {
	return &linear.NewLinearIssueEvent{
		IssueWatchID:      "watch-1",
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Prompt:            "Work on {{issue.identifier}}: {{issue.title}}",
		Issue: &linear.LinearIssue{
			Identifier:    "ENG-1",
			Title:         "Bug",
			URL:           "https://linear.app/x/issue/ENG-1",
			StateName:     "Todo",
			AssigneeName:  "Alice",
			CreatorName:   "Bob",
			Description:   "details",
			PriorityLabel: "High",
			TeamKey:       "ENG",
		},
	}
}

func TestLinearSource_Name(t *testing.T) {
	src := &LinearWatcherSource{}
	if src.Name() != "linear" {
		t.Fatalf("expected name=linear, got %q", src.Name())
	}
}

func TestLinearSource_Reserve_Passthrough(t *testing.T) {
	svc := &fakeLinearService{reserveOK: true}
	src := &LinearWatcherSource{service: svc}
	ok, err := src.Reserve(context.Background(), sampleLinearEvent())
	if err != nil || !ok {
		t.Fatalf("expected reserve ok, got ok=%v err=%v", ok, err)
	}
	if len(svc.gotReserve) != 1 || svc.gotReserve[0] != "watch-1:ENG-1" {
		t.Fatalf("unexpected reserve args: %v", svc.gotReserve)
	}
}

func TestLinearSource_Reserve_NilServiceFailOpen(t *testing.T) {
	// Matches today's reserveLinearIssue: when service is unset (boot order
	// corner case) the pipeline proceeds — better to risk a dup than drop.
	src := &LinearWatcherSource{service: nil}
	ok, err := src.Reserve(context.Background(), sampleLinearEvent())
	if err != nil || !ok {
		t.Fatalf("expected nil service to fail open, got ok=%v err=%v", ok, err)
	}
}

func TestLinearSource_Reserve_Error(t *testing.T) {
	svc := &fakeLinearService{reserveErr: errors.New("boom")}
	src := &LinearWatcherSource{service: svc}
	ok, err := src.Reserve(context.Background(), sampleLinearEvent())
	if ok {
		t.Fatal("expected reserve to fail")
	}
	if err == nil {
		t.Fatal("expected reserve error to surface")
	}
}

func TestLinearSource_BuildTaskRequest(t *testing.T) {
	src := &LinearWatcherSource{}
	req, err := src.BuildTaskRequest(sampleLinearEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Title != "[ENG-1] Bug" {
		t.Errorf("title = %q", req.Title)
	}
	if req.WorkspaceID != "ws-1" || req.WorkflowID != "wf-1" || req.WorkflowStepID != "step-1" {
		t.Errorf("workflow fields wrong: %+v", req)
	}
	if req.Description != "Work on ENG-1: Bug" {
		t.Errorf("prompt interpolation wrong: %q", req.Description)
	}
	if req.Metadata["linear_issue_watch_id"] != "watch-1" {
		t.Errorf("missing linear_issue_watch_id metadata")
	}
	if req.Metadata["linear_issue_identifier"] != "ENG-1" {
		t.Errorf("missing linear_issue_identifier metadata")
	}
	if req.Metadata["agent_profile_id"] != "agent-1" {
		t.Errorf("missing agent_profile_id metadata")
	}
	// Unbound event (no RepositoryID) must leave Repositories nil so the launch
	// path keeps the historical repo-less behaviour.
	if req.Repositories != nil {
		t.Errorf("unbound event should yield nil Repositories, got %+v", req.Repositories)
	}
}

func TestLinearSource_BuildTaskRequest_RepositoryBinding(t *testing.T) {
	src := &LinearWatcherSource{}
	evt := sampleLinearEvent()
	evt.RepositoryID = "repo-9"
	evt.BaseBranch = "develop"
	req, err := src.BuildTaskRequest(evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Repositories) != 1 {
		t.Fatalf("expected one repository, got %d", len(req.Repositories))
	}
	if req.Repositories[0].RepositoryID != "repo-9" || req.Repositories[0].BaseBranch != "develop" {
		t.Errorf("repository binding wrong: %+v", req.Repositories[0])
	}
}

func TestLinearSource_BuildTaskRequest_WrongType(t *testing.T) {
	src := &LinearWatcherSource{}
	_, err := src.BuildTaskRequest("not an event")
	if err == nil {
		t.Fatal("expected error for wrong event type")
	}
}

func TestLinearSource_AttachTaskID(t *testing.T) {
	svc := &fakeLinearService{}
	src := &LinearWatcherSource{service: svc}
	if err := src.AttachTaskID(context.Background(), sampleLinearEvent(), "task-9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.gotAssign) != 1 || svc.gotAssign[0] != "watch-1:ENG-1:task-9" {
		t.Fatalf("unexpected assign args: %v", svc.gotAssign)
	}
}

func TestLinearSource_Release(t *testing.T) {
	svc := &fakeLinearService{}
	src := &LinearWatcherSource{service: svc}
	src.Release(context.Background(), sampleLinearEvent())
	if len(svc.gotRelease) != 1 || svc.gotRelease[0] != "watch-1:ENG-1" {
		t.Fatalf("unexpected release args: %v", svc.gotRelease)
	}
}

func TestLinearSource_Release_ErrorIsLoggedNotPropagated(t *testing.T) {
	svc := &fakeLinearService{releaseErr: errors.New("dedup store down")}
	src := &LinearWatcherSource{service: svc, logger: nopLogger(t)}
	// Release returns no value: failure must be swallowed and logged.
	// We only assert it does not panic and that the underlying service
	// was still asked to release.
	src.Release(context.Background(), sampleLinearEvent())
	if len(svc.gotRelease) != 1 {
		t.Fatalf("expected release call to be attempted, got %d", len(svc.gotRelease))
	}
}

func TestLinearSource_AutoStartParams(t *testing.T) {
	src := &LinearWatcherSource{}
	p := src.AutoStartParams(sampleLinearEvent())
	if p.AgentProfileID != "agent-1" || p.ExecutorProfileID != "exec-1" {
		t.Fatalf("unexpected auto-start params: %+v", p)
	}
	if p.WorkflowStepID != "step-1" {
		t.Errorf("step id wrong: %q", p.WorkflowStepID)
	}
}
