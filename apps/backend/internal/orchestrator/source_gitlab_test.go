package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
)

type fakeGitLabWatchService struct {
	reviewReserved bool
	issueReserved  bool
	assigned       []string
	released       []string
	disabled       []string
}

func (f *fakeGitLabWatchService) ReserveReviewMRTask(context.Context, string, int64, string, int, string) (bool, error) {
	return f.reviewReserved, nil
}
func (f *fakeGitLabWatchService) AssignReviewMRTaskID(_ context.Context, watchID string, _ int64, project string, iid int, taskID string) error {
	f.assigned = append(f.assigned, watchID+":"+project+":"+taskID)
	return nil
}
func (f *fakeGitLabWatchService) ReleaseReviewMRTask(_ context.Context, watchID string, _ int64, project string, iid int) error {
	f.released = append(f.released, watchID+":"+project)
	return nil
}
func (f *fakeGitLabWatchService) DisableReviewWatchWithError(_ context.Context, watchID, cause string) error {
	f.disabled = append(f.disabled, watchID+":"+cause)
	return nil
}
func (f *fakeGitLabWatchService) ReserveIssueWatchTask(context.Context, string, int64, string, int, string) (bool, error) {
	return f.issueReserved, nil
}
func (f *fakeGitLabWatchService) AssignIssueWatchTaskID(_ context.Context, watchID string, _ int64, project string, iid int, taskID string) error {
	f.assigned = append(f.assigned, watchID+":"+project+":"+taskID)
	return nil
}
func (f *fakeGitLabWatchService) ReleaseIssueWatchTask(_ context.Context, watchID string, _ int64, project string, iid int) error {
	f.released = append(f.released, watchID+":"+project)
	return nil
}
func (f *fakeGitLabWatchService) DisableIssueWatchWithError(_ context.Context, watchID, cause string) error {
	f.disabled = append(f.disabled, watchID+":"+cause)
	return nil
}

func sampleGitLabReviewEvent() *gitlab.NewReviewMREvent {
	cap := 3
	return &gitlab.NewReviewMREvent{
		ReviewWatchID: "rw-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		AgentProfileID: "agent-1", ExecutorProfileID: "exec-1", Prompt: "Review {{mr.url}}",
		RepositoryID: "repo-1", BaseBranch: "main", MaxInflightTasks: &cap,
		MR: &gitlab.MR{IID: 4, Title: "Fix", ProjectPath: "group/project", WebURL: "https://gitlab/group/project/-/merge_requests/4"},
	}
}

func sampleGitLabIssueEvent() *gitlab.NewIssueEvent {
	return &gitlab.NewIssueEvent{
		IssueWatchID: "iw-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		AgentProfileID: "agent-1", ExecutorProfileID: "exec-1", Prompt: "Handle {{issue.url}}",
		Issue: &gitlab.Issue{IID: 8, Title: "Broken", ProjectPath: "group/project", WebURL: "https://gitlab/group/project/-/issues/8"},
	}
}

func TestGitLabReviewWatcherSourceMapsDispatchData(t *testing.T) {
	fake := &fakeGitLabWatchService{reviewReserved: true}
	source := NewGitLabReviewWatcherSource(fake, nopLogger(t))
	event := sampleGitLabReviewEvent()
	reserved, err := source.Reserve(context.Background(), event)
	if err != nil || !reserved {
		t.Fatalf("reserve=%v err=%v", reserved, err)
	}
	req, err := source.BuildTaskRequest(event)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.WorkspaceID != "ws-1" || req.WorkflowStepID != "step-1" || len(req.Repositories) != 1 {
		t.Fatalf("request = %#v", req)
	}
	if req.Repositories[0].RepositoryID != "repo-1" || req.Repositories[0].BaseBranch != "main" {
		t.Fatalf("repository = %#v", req.Repositories[0])
	}
	if !strings.Contains(req.Description, event.MR.WebURL) || strings.Contains(req.Description, "{{") {
		t.Fatalf("description = %q, want interpolated MR URL", req.Description)
	}
	if source.MaxInflightTasks(event) == nil || *source.MaxInflightTasks(event) != 3 {
		t.Fatalf("max inflight = %v", source.MaxInflightTasks(event))
	}
	if err := source.AttachTaskID(context.Background(), event, "task-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := source.SelfHeal(context.Background(), event, "profile removed"); err != nil {
		t.Fatalf("self heal: %v", err)
	}
	if len(fake.assigned) != 1 || len(fake.disabled) != 1 {
		t.Fatalf("assigned=%v disabled=%v", fake.assigned, fake.disabled)
	}
}

func TestGitLabIssueWatcherSourceReleaseAndMetadata(t *testing.T) {
	fake := &fakeGitLabWatchService{issueReserved: true}
	source := NewGitLabIssueWatcherSource(fake, nopLogger(t))
	event := sampleGitLabIssueEvent()
	req, err := source.BuildTaskRequest(event)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.Title != "[group/project#8] Broken" || req.Metadata[gitlabIssueWatchMetadataKey] != "iw-1" {
		t.Fatalf("request = %#v", req)
	}
	if !strings.Contains(req.Description, event.Issue.WebURL) || strings.Contains(req.Description, "{{") {
		t.Fatalf("description = %q, want interpolated issue URL", req.Description)
	}
	source.Release(context.Background(), event)
	if len(fake.released) != 1 {
		t.Fatalf("released = %v", fake.released)
	}
}

func TestGitLabWatcherSourcesClassifyOwnershipLossAsTerminal(t *testing.T) {
	review := NewGitLabReviewWatcherSource(nil, nopLogger(t))
	issue := NewGitLabIssueWatcherSource(nil, nopLogger(t))
	for name, source := range map[string]interface{ IsTerminalAttachError(error) bool }{
		"review": review,
		"issue":  issue,
	} {
		if !source.IsTerminalAttachError(gitlab.ErrWatchOwnershipLost) {
			t.Fatalf("%s source did not classify ownership loss as terminal", name)
		}
		if source.IsTerminalAttachError(errors.New("temporary attach failure")) {
			t.Fatalf("%s source classified temporary failure as terminal", name)
		}
	}
}
