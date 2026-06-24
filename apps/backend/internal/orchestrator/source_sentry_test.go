package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/sentry"
)

type fakeSentryService struct {
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

func (f *fakeSentryService) DisableIssueWatchWithError(_ context.Context, watchID, cause string) error {
	f.gotDisable = append(f.gotDisable, watchID+":"+cause)
	return f.disableErr
}

func sampleSentryEvent() *sentry.NewSentryIssueEvent {
	return &sentry.NewSentryIssueEvent{
		IssueWatchID:      "watch-1",
		InstanceID:        "inst-1",
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
	if req.Metadata["sentry_instance_id"] != "inst-1" {
		t.Errorf("missing sentry_instance_id metadata")
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

func TestSentrySource_BuildTaskRequest_TruncatesLongTitle(t *testing.T) {
	src := &SentryWatcherSource{}
	evt := sampleSentryEvent()
	// Sentry titles come from error messages and can be arbitrarily long.
	evt.Issue.Title = strings.Repeat("é", 500) // multibyte to catch byte-slicing bugs
	req, err := src.BuildTaskRequest(evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The title portion must be bounded at the producer and stay valid UTF-8.
	if !utf8.ValidString(req.Title) {
		t.Errorf("title is not valid UTF-8 after truncation: %q", req.Title)
	}
	if !strings.HasSuffix(req.Title, "…") {
		t.Errorf("expected truncation ellipsis, got %q", req.Title)
	}
	if got := utf8.RuneCountInString(req.Title); got > 100 {
		t.Errorf("title not bounded: %d runes", got)
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

func TestSentrySource_AgentProfileID(t *testing.T) {
	src := &SentryWatcherSource{}
	if got := src.AgentProfileID(sampleSentryEvent()); got != "agent-1" {
		t.Fatalf("AgentProfileID = %q, want agent-1", got)
	}
	if got := src.AgentProfileID("not an event"); got != "" {
		t.Errorf("AgentProfileID for wrong type = %q, want empty", got)
	}
}

func TestSentrySource_SelfHeal_Passthrough(t *testing.T) {
	svc := &fakeSentryService{}
	src := &SentryWatcherSource{service: svc}
	if err := src.SelfHeal(context.Background(), sampleSentryEvent(), "agent profile deleted"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.gotDisable) != 1 || svc.gotDisable[0] != "watch-1:agent profile deleted" {
		t.Fatalf("unexpected disable args: %v", svc.gotDisable)
	}
}

func TestSentrySource_SelfHeal_NilServiceNoop(t *testing.T) {
	src := &SentryWatcherSource{service: nil}
	if err := src.SelfHeal(context.Background(), sampleSentryEvent(), "cause"); err != nil {
		t.Fatalf("expected nil service to no-op, got %v", err)
	}
}

func TestSentrySource_SelfHeal_WrongType(t *testing.T) {
	svc := &fakeSentryService{}
	src := &SentryWatcherSource{service: svc}
	if err := src.SelfHeal(context.Background(), "not an event", "cause"); err != nil {
		t.Fatalf("expected wrong type to no-op, got %v", err)
	}
	if len(svc.gotDisable) != 0 {
		t.Fatalf("expected no disable call for wrong type, got %v", svc.gotDisable)
	}
}
