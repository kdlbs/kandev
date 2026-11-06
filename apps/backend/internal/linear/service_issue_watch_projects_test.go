package linear

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func newProjectWatchFixture(t *testing.T, prompt string, issues []LinearIssue) (*svcFixture, *IssueWatch) {
	t.Helper()
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfigForWorkspace(ctx, "ws-1", &SetConfigRequest{
		AuthMethod: AuthMethodAPIKey, Secret: "lin_api",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	f.client.withSearchResults(issues)
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{ProjectIDs: []string{"p1", "p2"}},
		Prompt: prompt,
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	return f, w
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.2
func TestService_IssueWatch_ProjectIDsFilter(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{ProjectIDs: []string{" p1 ", "", "p2"}},
	})
	if err != nil {
		t.Fatalf("projectIds alone must satisfy the filter rule: %v", err)
	}
	if !reflect.DeepEqual(w.Filter.ProjectIDs, []string{"p1", "p2"}) {
		t.Fatalf("projectIds not normalized: %#v", w.Filter.ProjectIDs)
	}
	stored, err := f.store.GetIssueWatch(ctx, w.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !reflect.DeepEqual(stored.Filter.ProjectIDs, []string{"p1", "p2"}) {
		t.Fatalf("projectIds not persisted: %#v", stored.Filter.ProjectIDs)
	}

	_, err = f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{ProjectIDs: []string{" ", ""}},
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("blank projectIds must leave the filter empty, got %v", err)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.2
func TestService_CheckIssueWatch_AttachesProjectDescriptions(t *testing.T) {
	f, w := newProjectWatchFixture(t, "Brief: {{issue.project.description}}", []LinearIssue{
		{Identifier: "ENG-1", URL: "u1", ProjectID: "p1"},
		{Identifier: "ENG-2", URL: "u2", ProjectID: "p2"},
		{Identifier: "ENG-3", URL: "u3", ProjectID: "p1"},
		{Identifier: "ENG-4", URL: "u4"},
	})
	var lookups [][]string
	f.client.projectContentsFn = func(ids []string) (map[string]string, error) {
		cp := append([]string(nil), ids...)
		sort.Strings(cp)
		lookups = append(lookups, cp)
		return map[string]string{"p1": "brief one"}, nil
	}
	got, err := f.svc.CheckIssueWatch(context.Background(), w)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !reflect.DeepEqual(lookups, [][]string{{"p1", "p2"}}) {
		t.Fatalf("lookups=%v, want one lookup of each distinct project", lookups)
	}
	descriptions := map[string]string{}
	for _, issue := range got {
		descriptions[issue.Identifier] = issue.ProjectDescription
	}
	want := map[string]string{"ENG-1": "brief one", "ENG-2": "", "ENG-3": "brief one", "ENG-4": ""}
	if !reflect.DeepEqual(descriptions, want) {
		t.Fatalf("descriptions=%v, want %v", descriptions, want)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.2, AC-INTEGRATIONS-LINEAR-PROJECTS-002.4
func TestService_CheckIssueWatch_SkipsProjectLookupWithoutPlaceholder(t *testing.T) {
	f, w := newProjectWatchFixture(t, "{{issue.project.name}}: {{issue.title}}", []LinearIssue{
		{Identifier: "ENG-1", URL: "u1", ProjectID: "p1"},
	})
	f.client.projectContentsFn = func([]string) (map[string]string, error) {
		t.Fatal("a prompt without the description placeholder must not fetch project content")
		return nil, nil
	}
	got, err := f.svc.CheckIssueWatch(context.Background(), w)
	if err != nil || len(got) != 1 {
		t.Fatalf("check: got %d issues, err %v", len(got), err)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.2
func TestService_CheckIssueWatch_ProjectLookupFailureFailsPoll(t *testing.T) {
	f, w := newProjectWatchFixture(t, "{{issue.project.description}}", []LinearIssue{
		{Identifier: "ENG-1", URL: "u1", ProjectID: "p1"},
	})
	upstream := &APIError{StatusCode: 500, Message: "boom"}
	f.client.projectContentsFn = func([]string) (map[string]string, error) {
		return nil, upstream
	}
	got, err := f.svc.CheckIssueWatch(context.Background(), w)
	if got != nil || !errors.Is(err, upstream) {
		t.Fatalf("got %v, err %v; want no issues and the upstream error", got, err)
	}
}
