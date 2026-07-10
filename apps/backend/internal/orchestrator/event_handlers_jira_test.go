package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/jira"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// mockJiraService records dedup calls so tests can assert on the
// reserve→assign→release contract used by handleNewJiraIssue.
type mockJiraService struct {
	reserveReturn      bool
	reserveErr         error
	reserveCalls       int
	assignCalls        int
	releaseCalls       int
	lastWatchID        string
	lastIssueKey       string
	assignedTaskID     string
	disableCalls       int
	lastDisableWatchID string
	lastDisableCause   string
}

func (m *mockJiraService) ReserveIssueWatchTask(_ context.Context, watchID, issueKey, _ string) (bool, error) {
	m.reserveCalls++
	m.lastWatchID = watchID
	m.lastIssueKey = issueKey
	return m.reserveReturn, m.reserveErr
}

func (m *mockJiraService) AssignIssueWatchTaskID(_ context.Context, _, _ string, taskID string) error {
	m.assignCalls++
	m.assignedTaskID = taskID
	return nil
}

func (m *mockJiraService) ReleaseIssueWatchTask(_ context.Context, _, _ string) error {
	m.releaseCalls++
	return nil
}

func (m *mockJiraService) DisableIssueWatchWithError(_ context.Context, watchID, cause string) error {
	m.disableCalls++
	m.lastDisableWatchID = watchID
	m.lastDisableCause = cause
	return nil
}

func newJiraIssueEvent() *jira.NewJiraIssueEvent {
	return &jira.NewJiraIssueEvent{
		IssueWatchID:   "iw-1",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		Issue: &jira.JiraTicket{
			Key:     "PROJ-42",
			Summary: "Login fails on mobile",
			URL:     "https://acme.atlassian.net/browse/PROJ-42",
		},
	}
}

func setupJiraTaskTest(t *testing.T) *Service {
	t.Helper()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{},
	}
	return createTestService(repo, stepGetter, newMockTaskRepo())
}

// dispatchJiraEvent drives an event through the watcher coordinator
// synchronously using a JiraWatcherSource — mirrors what handleNewJiraIssue
// does, but without the goroutine, so tests can assert on observable side
// effects deterministically.
func dispatchJiraEvent(svc *Service, evt *jira.NewJiraIssueEvent) {
	src := NewJiraWatcherSource(svc.jiraService, svc.logger)
	svc.watcherCoordinator.Dispatch(context.Background(), src, evt)
}

func TestCreateJiraIssueTask_HappyPath(t *testing.T) {
	svc := setupJiraTaskTest(t)
	jiraSvc := &mockJiraService{reserveReturn: true}
	svc.SetJiraService(jiraSvc)
	creator := &countingIssueTaskCreator{taskID: "task-jira-1"}
	svc.SetIssueTaskCreator(creator)

	dispatchJiraEvent(svc, newJiraIssueEvent())

	if jiraSvc.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", jiraSvc.reserveCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateIssueTask call, got %d", creator.calls)
	}
	if jiraSvc.assignCalls != 1 {
		t.Errorf("expected 1 AssignIssueWatchTaskID call, got %d", jiraSvc.assignCalls)
	}
	if jiraSvc.assignedTaskID != "task-jira-1" {
		t.Errorf("expected assigned task id task-jira-1, got %q", jiraSvc.assignedTaskID)
	}
	if jiraSvc.releaseCalls != 0 {
		t.Errorf("expected no Release on happy path, got %d", jiraSvc.releaseCalls)
	}
	if jiraSvc.lastIssueKey != "PROJ-42" {
		t.Errorf("expected reservation keyed on PROJ-42, got %q", jiraSvc.lastIssueKey)
	}
}

func TestCreateJiraIssueTask_SkipsWhenAlreadyReserved(t *testing.T) {
	svc := setupJiraTaskTest(t)
	jiraSvc := &mockJiraService{reserveReturn: false}
	svc.SetJiraService(jiraSvc)
	creator := &countingIssueTaskCreator{}
	svc.SetIssueTaskCreator(creator)

	dispatchJiraEvent(svc, newJiraIssueEvent())

	if creator.calls != 0 {
		t.Errorf("expected CreateIssueTask NOT to be called when reservation is lost, got %d", creator.calls)
	}
	if jiraSvc.releaseCalls != 0 {
		t.Errorf("expected no Release when reservation was never held, got %d", jiraSvc.releaseCalls)
	}
}

func TestCreateJiraIssueTask_ReleasesWhenCreateFails(t *testing.T) {
	svc := setupJiraTaskTest(t)
	jiraSvc := &mockJiraService{reserveReturn: true}
	svc.SetJiraService(jiraSvc)
	creator := &countingIssueTaskCreator{err: errors.New("task creation failed")}
	svc.SetIssueTaskCreator(creator)

	dispatchJiraEvent(svc, newJiraIssueEvent())

	if jiraSvc.assignCalls != 0 {
		t.Errorf("expected no Assign when task creation failed, got %d", jiraSvc.assignCalls)
	}
	if jiraSvc.releaseCalls != 1 {
		t.Errorf("expected Release after task creation failure, got %d", jiraSvc.releaseCalls)
	}
}

// TestSetIssueTaskCreator_RefreshesCoordinatorTaskCreator pins the wiring
// contract that calling SetIssueTaskCreator again must update the coordinator.
// Regression guard: an earlier version of initWatcherCoordinator returned
// early on the second call and silently kept the original creator.
func TestSetIssueTaskCreator_RefreshesCoordinatorTaskCreator(t *testing.T) {
	svc := setupJiraTaskTest(t)
	jiraSvc := &mockJiraService{reserveReturn: true}
	svc.SetJiraService(jiraSvc)

	first := &countingIssueTaskCreator{taskID: "task-first"}
	svc.SetIssueTaskCreator(first)

	second := &countingIssueTaskCreator{taskID: "task-second"}
	svc.SetIssueTaskCreator(second)

	dispatchJiraEvent(svc, newJiraIssueEvent())

	if first.calls != 0 {
		t.Errorf("first creator must not be invoked after being replaced, got %d calls", first.calls)
	}
	if second.calls != 1 {
		t.Errorf("expected the latest creator to be used, got %d calls", second.calls)
	}
}

func TestInterpolateJiraPrompt(t *testing.T) {
	ticket := &jira.JiraTicket{
		Key:          "PROJ-7",
		Summary:      "Login fails on mobile",
		URL:          "https://acme.atlassian.net/browse/PROJ-7",
		StatusName:   "In Progress",
		Priority:     "High",
		IssueType:    "Bug",
		AssigneeName: "Alice",
		ProjectKey:   "PROJ",
	}

	// Empty template falls back to a default that mentions key + URL.
	got := interpolateJiraPrompt("", ticket)
	if !strings.Contains(got, "PROJ-7") || !strings.Contains(got, "https://acme.atlassian.net/browse/PROJ-7") {
		t.Errorf("default template missing key or URL: %q", got)
	}

	// All placeholders.
	got = interpolateJiraPrompt(
		"{{issue.key}} | {{issue.summary}} | {{issue.status}} | {{issue.priority}} | {{issue.assignee}}",
		ticket,
	)
	want := "PROJ-7 | Login fails on mobile | In Progress | High | Alice"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
