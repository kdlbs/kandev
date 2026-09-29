package backendapp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type pluginTaskReaderFake struct{ task *taskmodels.Task }

func (f pluginTaskReaderFake) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return f.task, nil
}

type pluginJiraSourceFake struct {
	link        *jira.TaskIssueLink
	linkErr     error
	ticket      *jira.JiraTicket
	ticketErr   error
	commentErr  error
	comments    int
	lastBody    string
	lastIssue   string
	transitions []string
}

func (f *pluginJiraSourceFake) GetTaskIssueLink(context.Context, string) (*jira.TaskIssueLink, error) {
	if f.linkErr != nil {
		return nil, f.linkErr
	}
	if f.link == nil {
		return nil, jira.ErrSourceLinkNotFound
	}
	copy := *f.link
	return &copy, nil
}

func (f *pluginJiraSourceFake) GetTicketForWorkspace(context.Context, string, string) (*jira.JiraTicket, error) {
	if f.ticketErr != nil {
		return nil, f.ticketErr
	}
	copy := *f.ticket
	copy.Transitions = append([]jira.JiraTransition(nil), f.ticket.Transitions...)
	return &copy, nil
}

func (f *pluginJiraSourceFake) AddCommentForWorkspace(_ context.Context, _, issue, body string) (string, error) {
	f.comments++
	f.lastIssue, f.lastBody = issue, body
	return "jira-comment-9", f.commentErr
}

func (f *pluginJiraSourceFake) DoTransitionForWorkspace(_ context.Context, _, issue, transition string) error {
	f.transitions = append(f.transitions, issue+":"+transition)
	return nil
}

type pluginLinearSourceFake struct {
	link        *linear.TaskIssueLink
	linkErr     error
	issue       *linear.LinearIssue
	issueErr    error
	commentID   string
	commentArgs []string
	states      []string
}

func (f *pluginLinearSourceFake) GetTaskIssueLink(context.Context, string) (*linear.TaskIssueLink, error) {
	if f.linkErr != nil {
		return nil, f.linkErr
	}
	if f.link == nil {
		return nil, linear.ErrSourceLinkNotFound
	}
	copy := *f.link
	return &copy, nil
}

func (f *pluginLinearSourceFake) GetIssueForWorkspace(context.Context, string, string) (*linear.LinearIssue, error) {
	if f.issueErr != nil {
		return nil, f.issueErr
	}
	copy := *f.issue
	return &copy, nil
}

func (f *pluginLinearSourceFake) AddCommentForWorkspace(_ context.Context, _, issueID, body string) (string, error) {
	f.commentArgs = append(f.commentArgs, issueID+":"+body)
	return f.commentID, nil
}

func (f *pluginLinearSourceFake) SetIssueStateForWorkspace(_ context.Context, _, issueID, stateID string) error {
	f.states = append(f.states, issueID+":"+stateID)
	return nil
}

func TestPluginSourceIssueControllerUsesExactTaskLink(t *testing.T) {
	ctx := context.Background()
	task := &taskmodels.Task{
		ID: "task-1", WorkspaceID: "workspace-1", Title: "ENG-42: release", UpdatedAt: time.Date(2026, 9, 26, 11, 0, 0, 0, time.UTC),
		Metadata: map[string]interface{}{"jira_issue_key": "ENG-42", "jira_issue_url": "https://jira.example/browse/ENG-42"},
	}
	jiraSource := &pluginJiraSourceFake{
		link: &jira.TaskIssueLink{WorkspaceID: "workspace-1", IssueKey: "ENG-42", IssueURL: "https://jira.example/browse/ENG-42"},
		ticket: &jira.JiraTicket{ID: "jira-id-42", Key: "ENG-42", Summary: "Release", StatusID: "open", StatusName: "Open", Updated: "2026-09-26T11:02:00Z",
			Transitions: []jira.JiraTransition{{ID: "transition-1", ToStatusName: "In Review"}}},
	}
	controller := pluginSourceIssueController{tasks: pluginTaskReaderFake{task}, jira: jiraSource}
	capabilities, err := controller.GetCapabilities(ctx, "workspace-1", task.ID)
	if err != nil || capabilities == nil || capabilities.Provider != "jira" || capabilities.SourceID != "jira-id-42" ||
		capabilities.Identifier != "ENG-42" || capabilities.TaskResourceVersion != task.UpdatedAt.UTC().Format(time.RFC3339Nano) ||
		len(capabilities.Transitions) != 1 || capabilities.Transitions[0].TargetID != "transition-1" {
		t.Fatalf("Jira capabilities = %+v err=%v", capabilities, err)
	}
	started := false
	commentID, err := controller.ApplyWriteback(ctx, plugins.SourceIssueWritebackInput{
		WorkspaceID: "workspace-1", TaskID: task.ID, Provider: "jira", SourceID: capabilities.SourceID,
		Operation: "comment", ExpectedTaskResourceVersion: capabilities.TaskResourceVersion,
		ExpectedSourceResourceVersion: capabilities.ResourceVersion, Body: "ready",
	}, func() error { started = true; return nil })
	if err != nil || commentID != "jira-comment-9" || !started || jiraSource.comments != 1 || jiraSource.lastIssue != "ENG-42" || jiraSource.lastBody != "ready" {
		t.Fatalf("Jira comment = id:%q started:%v calls:%d issue:%q body:%q err:%v", commentID, started, jiraSource.comments, jiraSource.lastIssue, jiraSource.lastBody, err)
	}
	if _, err := controller.ApplyWriteback(ctx, plugins.SourceIssueWritebackInput{
		WorkspaceID: "workspace-foreign", TaskID: task.ID, Provider: "jira", SourceID: capabilities.SourceID,
	}, func() error { t.Fatal("foreign workspace reached provider send"); return nil }); err == nil {
		t.Fatal("foreign workspace source link was accepted")
	}
}

func TestPluginSourceIssueControllerRejectsStaleLinkAndVersion(t *testing.T) {
	ctx := context.Background()
	task := &taskmodels.Task{
		ID: "task-2", WorkspaceID: "workspace-2", UpdatedAt: time.Date(2026, 9, 26, 11, 0, 0, 0, time.UTC),
		Metadata: map[string]interface{}{"jira_issue_key": "ENG-7", "jira_issue_url": "https://jira.example/browse/ENG-7"},
	}
	staleLink := &pluginJiraSourceFake{link: &jira.TaskIssueLink{
		WorkspaceID: "workspace-2", IssueKey: "ENG-8", IssueURL: "https://jira.example/browse/ENG-8",
	}}
	controller := pluginSourceIssueController{tasks: pluginTaskReaderFake{task}, jira: staleLink}
	if _, err := controller.GetCapabilities(ctx, task.WorkspaceID, task.ID); err == nil {
		t.Fatal("stale metadata/link association was accepted")
	} else {
		var sourceErr *plugins.SourceIssueError
		if !errors.As(err, &sourceErr) || sourceErr.Kind != plugins.SourceIssueErrorStale {
			t.Fatalf("stale metadata/link error = %v, want stale", err)
		}
	}

	task.Metadata["jira_issue_key"] = "ENG-7"
	task.Metadata["jira_issue_url"] = "https://jira.example/browse/ENG-7"
	staleLink.link = &jira.TaskIssueLink{WorkspaceID: task.WorkspaceID, IssueKey: "ENG-7", IssueURL: "https://jira.example/browse/ENG-7"}
	staleLink.ticket = &jira.JiraTicket{ID: "jira-id-7", Key: "ENG-7", StatusID: "open", Updated: "2026-09-26T11:02:00Z"}
	capabilities, err := controller.GetCapabilities(ctx, task.WorkspaceID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	staleLink.ticket.Updated = "2026-09-26T11:03:00Z"
	started := false
	_, err = controller.ApplyWriteback(ctx, plugins.SourceIssueWritebackInput{
		WorkspaceID: task.WorkspaceID, TaskID: task.ID, Provider: "jira", SourceID: capabilities.SourceID,
		Operation: "comment", ExpectedTaskResourceVersion: capabilities.TaskResourceVersion,
		ExpectedSourceResourceVersion: capabilities.ResourceVersion,
	}, func() error { started = true; return nil })
	var sourceErr *plugins.SourceIssueError
	if !errors.As(err, &sourceErr) || sourceErr.Kind != plugins.SourceIssueErrorStale || started || staleLink.comments != 0 {
		t.Fatalf("changed provider version = err:%v started:%v comments:%d, want stale before send", err, started, staleLink.comments)
	}
}

func TestPluginSourceIssueControllerMapsLinearStatesAndRateLimits(t *testing.T) {
	ctx := context.Background()
	task := &taskmodels.Task{
		ID: "task-linear", WorkspaceID: "workspace-linear", UpdatedAt: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC),
		Metadata: map[string]interface{}{"linear_issue_identifier": "ENG-5", "linear_issue_url": "https://linear.app/acme/issue/ENG-5"},
	}
	linearSource := &pluginLinearSourceFake{
		link: &linear.TaskIssueLink{WorkspaceID: task.WorkspaceID, IssueIdentifier: "ENG-5", IssueURL: "https://linear.app/acme/issue/ENG-5"},
		issue: &linear.LinearIssue{ID: "linear-uuid-5", Identifier: "ENG-5", Title: "Fix release", StateID: "state-open", StateName: "Open", Updated: "2026-09-26T12:01:00Z",
			States: []linear.LinearWorkflowState{{ID: "state-done", Name: "Done"}}},
	}
	controller := pluginSourceIssueController{tasks: pluginTaskReaderFake{task}, linear: linearSource}
	capabilities, err := controller.GetCapabilities(ctx, task.WorkspaceID, task.ID)
	if err != nil || capabilities.Provider != "linear" || capabilities.SourceID != "linear-uuid-5" || len(capabilities.Transitions) != 1 {
		t.Fatalf("Linear capabilities = %+v err=%v", capabilities, err)
	}
	started := false
	if _, err := controller.ApplyWriteback(ctx, plugins.SourceIssueWritebackInput{
		WorkspaceID: task.WorkspaceID, TaskID: task.ID, Provider: "linear", SourceID: capabilities.SourceID,
		Operation: "transition", ExpectedTaskResourceVersion: capabilities.TaskResourceVersion,
		ExpectedSourceResourceVersion: capabilities.ResourceVersion, TargetID: "state-done",
	}, func() error { started = true; return nil }); err != nil || !started || len(linearSource.states) != 1 || linearSource.states[0] != "linear-uuid-5:state-done" {
		t.Fatalf("Linear transition = states:%v started:%v err:%v", linearSource.states, started, err)
	}
	if err := pluginJiraWriteError(&jira.APIError{StatusCode: http.StatusTooManyRequests}); err == nil {
		t.Fatal("expected typed Jira rate-limit error")
	} else {
		var sourceErr *plugins.SourceIssueError
		if !errors.As(err, &sourceErr) || sourceErr.Kind != plugins.SourceIssueErrorRateLimited {
			t.Fatalf("Jira 429 error = %v, want rate limited", err)
		}
	}
}

func TestPluginSourceIssueControllerRejectsAmbiguousAndUnsupportedWrites(t *testing.T) {
	task := &taskmodels.Task{
		ID: "task-ambiguous", WorkspaceID: "workspace-ambiguous",
		Metadata: map[string]interface{}{"jira_issue_key": "ENG-42", "jira_issue_url": "https://jira.example/browse/ENG-42"},
	}
	controller := pluginSourceIssueController{
		tasks: pluginTaskReaderFake{task},
		jira:  &pluginJiraSourceFake{linkErr: jira.ErrSourceLinkAmbiguous},
	}
	if _, err := controller.GetCapabilities(context.Background(), task.WorkspaceID, task.ID); err == nil {
		t.Fatal("ambiguous Jira source links were accepted")
	} else {
		var sourceErr *plugins.SourceIssueError
		if !errors.As(err, &sourceErr) || sourceErr.Kind != plugins.SourceIssueErrorStale {
			t.Fatalf("ambiguous source link error = %v, want stale", err)
		}
	}
	started := false
	_, err := controller.ApplyWriteback(context.Background(), plugins.SourceIssueWritebackInput{
		WorkspaceID: task.WorkspaceID, TaskID: task.ID, Operation: "delete",
	}, func() error { started = true; return nil })
	var sourceErr *plugins.SourceIssueError
	if !errors.As(err, &sourceErr) || sourceErr.Kind != plugins.SourceIssueErrorUnsupported || started {
		t.Fatalf("unsupported write = err:%v started:%v, want unsupported before send", err, started)
	}
}
