package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sentry"
)

type fakeSentryService struct {
	reserveOK  bool
	reserveErr error
	assignErr  error
	releaseErr error
	gotReserve []string
	gotAssign  []string
	gotRelease []string
}

func (f *fakeSentryService) ReserveIssueWatchTask(_ context.Context, watchID, id, _ string) (bool, error) {
	f.gotReserve = append(f.gotReserve, watchID+":"+id)
	return f.reserveOK, f.reserveErr
}

func (f *fakeSentryService) AssignIssueWatchTaskID(_ context.Context, watchID, id, taskID string) error {
	f.gotAssign = append(f.gotAssign, watchID+":"+id+":"+taskID)
	return f.assignErr
}

func (f *fakeSentryService) ReleaseIssueWatchTask(_ context.Context, watchID, id string) error {
	f.gotRelease = append(f.gotRelease, watchID+":"+id)
	return f.releaseErr
}

func sampleSentryEvent() *sentry.NewSentryIssueEvent {
	return &sentry.NewSentryIssueEvent{
		IssueWatchID:      "watch-1",
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Prompt:            "Investigate {{issue.short_id}}: {{issue.title}}",
		Issue: &sentry.SentryIssue{
			ID:           "100",
			ShortID:      "PROJ-1",
			Title:        "Boom",
			Permalink:    "https://sentry.io/issues/PROJ-1",
			ProjectSlug:  "frontend",
			ProjectName:  "Frontend",
			Level:        "error",
			Status:       "unresolved",
			Culprit:      "render.tsx",
			AssigneeName: "Alice",
			Count:        "42",
			UserCount:    7,
		},
	}
}

func TestSentrySource_Name(t *testing.T) {
	src := &SentryWatcherSource{}
	if src.Name() != "sentry" {
		t.Fatalf("expected name=sentry, got %q", src.Name())
	}
}

func TestSentrySource_Reserve_Passthrough(t *testing.T) {
	svc := &fakeSentryService{reserveOK: true}
	src := &SentryWatcherSource{service: svc}
	ok, err := src.Reserve(context.Background(), sampleSentryEvent())
	if err != nil || !ok {
		t.Fatalf("expected reserve ok, got ok=%v err=%v", ok, err)
	}
	if len(svc.gotReserve) != 1 || svc.gotReserve[0] != "watch-1:PROJ-1" {
		t.Fatalf("unexpected reserve args: %v", svc.gotReserve)
	}
}

func TestSentrySource_Reserve_NilServiceFailOpen(t *testing.T) {
	src := &SentryWatcherSource{service: nil}
	ok, err := src.Reserve(context.Background(), sampleSentryEvent())
	if err != nil || !ok {
		t.Fatalf("expected nil service to fail open, got ok=%v err=%v", ok, err)
	}
}

func TestSentrySource_Reserve_Error(t *testing.T) {
	svc := &fakeSentryService{reserveErr: errors.New("boom")}
	src := &SentryWatcherSource{service: svc}
	ok, err := src.Reserve(context.Background(), sampleSentryEvent())
	if ok {
		t.Fatal("expected reserve to fail")
	}
	if err == nil {
		t.Fatal("expected reserve error to surface")
	}
}

func TestSentrySource_Reserve_WrongType(t *testing.T) {
	src := &SentryWatcherSource{}
	if _, err := src.Reserve(context.Background(), "not an event"); err == nil {
		t.Fatal("expected error for wrong event type")
	}
}

func TestSentrySource_BuildTaskRequest(t *testing.T) {
	src := &SentryWatcherSource{}
	req, err := src.BuildTaskRequest(sampleSentryEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantTitle := "[ERROR] PROJ-1 — Boom"
	if req.Title != wantTitle {
		t.Errorf("title = %q, want %q", req.Title, wantTitle)
	}
	if req.WorkspaceID != "ws-1" || req.WorkflowID != "wf-1" || req.WorkflowStepID != "step-1" {
		t.Errorf("workflow fields wrong: %+v", req)
	}
	if !strings.Contains(req.Description, "PROJ-1") || !strings.Contains(req.Description, "Boom") {
		t.Errorf("prompt interpolation wrong: %q", req.Description)
	}
	if req.Metadata["sentry_issue_watch_id"] != "watch-1" {
		t.Errorf("missing sentry_issue_watch_id metadata")
	}
	if req.Metadata["sentry_issue_short_id"] != "PROJ-1" {
		t.Errorf("missing sentry_issue_short_id metadata")
	}
	if req.Metadata["sentry_issue_level"] != "error" {
		t.Errorf("missing sentry_issue_level metadata")
	}
	if req.Metadata["agent_profile_id"] != "agent-1" {
		t.Errorf("missing agent_profile_id metadata")
	}
	if req.Metadata["executor_profile_id"] != "exec-1" {
		t.Errorf("missing executor_profile_id metadata")
	}
}

func TestSentrySource_BuildTaskRequest_WrongType(t *testing.T) {
	src := &SentryWatcherSource{}
	if _, err := src.BuildTaskRequest("not an event"); err == nil {
		t.Fatal("expected error for wrong event type")
	}
}

func TestSentrySource_AttachTaskID(t *testing.T) {
	svc := &fakeSentryService{}
	src := &SentryWatcherSource{service: svc}
	if err := src.AttachTaskID(context.Background(), sampleSentryEvent(), "task-9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.gotAssign) != 1 || svc.gotAssign[0] != "watch-1:PROJ-1:task-9" {
		t.Fatalf("unexpected assign args: %v", svc.gotAssign)
	}
}

func TestSentrySource_Release(t *testing.T) {
	svc := &fakeSentryService{}
	src := &SentryWatcherSource{service: svc}
	src.Release(context.Background(), sampleSentryEvent())
	if len(svc.gotRelease) != 1 || svc.gotRelease[0] != "watch-1:PROJ-1" {
		t.Fatalf("unexpected release args: %v", svc.gotRelease)
	}
}

func TestSentrySource_Release_ErrorIsLoggedNotPropagated(t *testing.T) {
	svc := &fakeSentryService{releaseErr: errors.New("dedup store down")}
	src := &SentryWatcherSource{service: svc, logger: nopLogger(t)}
	src.Release(context.Background(), sampleSentryEvent())
	if len(svc.gotRelease) != 1 {
		t.Fatalf("expected release call to be attempted, got %d", len(svc.gotRelease))
	}
}

func TestSentrySource_AutoStartParams(t *testing.T) {
	src := &SentryWatcherSource{}
	p := src.AutoStartParams(sampleSentryEvent())
	if p.AgentProfileID != "agent-1" || p.ExecutorProfileID != "exec-1" {
		t.Fatalf("unexpected auto-start params: %+v", p)
	}
	if p.WorkflowStepID != "step-1" {
		t.Errorf("step id wrong: %q", p.WorkflowStepID)
	}
}
