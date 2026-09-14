package gitlab

import (
	"errors"
	"strings"
	"testing"
)

func TestServiceMergeMRValidatesMethodAndUpdatesMR(t *testing.T) {
	client := NewMockClient("https://gitlab.example")
	client.SeedMR("acme/widget", &MR{IID: 4, Title: "Feature", State: mrStateOpen})
	service := NewService(client.Host(), client, "mock", nil, newTestLogger(t))

	mr, err := service.MergeMR(t.Context(), "acme/widget", 4, "squash", "combined changes")
	if err != nil {
		t.Fatalf("MergeMR() error = %v", err)
	}
	if mr.State != gitlabStateMerged || mr.MergedAt == nil {
		t.Fatalf("MR = %#v, want merged state and timestamp", mr)
	}

	stored, err := client.GetMR(t.Context(), "acme/widget", 4)
	if err != nil {
		t.Fatalf("GetMR() error = %v", err)
	}
	if stored.State != gitlabStateMerged {
		t.Fatalf("stored state = %q, want %q", stored.State, gitlabStateMerged)
	}
}

func TestServiceMergeMRRejectsUnsupportedMethods(t *testing.T) {
	service := NewService(DefaultHost, NewMockClient(DefaultHost), "mock", nil, newTestLogger(t))
	tests := []struct {
		method string
		want   string
	}{
		{method: "rebase_merge", want: "does not allow rebase merge"},
		{method: "ff", want: "does not allow fast-forward merge"},
		{method: "octopus", want: "unknown merge method"},
	}
	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			_, err := service.MergeMR(t.Context(), "acme/widget", 4, tc.method, "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("MergeMR() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestServiceClientActionsReturnErrNoClient(t *testing.T) {
	service := NewService(DefaultHost, nil, AuthMethodNone, nil, newTestLogger(t))
	tests := []struct {
		name string
		call func() error
	}{
		{name: "merge", call: func() error { _, err := service.MergeMR(t.Context(), "acme/widget", 1, "merge", ""); return err }},
		{name: "merge methods", call: func() error { _, err := service.GetProjectMergeMethods(t.Context(), "acme/widget"); return err }},
		{name: "approve", call: func() error { return service.SubmitMRApproval(t.Context(), "acme/widget", 1) }},
		{name: "unapprove", call: func() error { return service.SubmitMRUnapproval(t.Context(), "acme/widget", 1) }},
		{name: "labels", call: func() error { return service.SetMRLabels(t.Context(), "acme/widget", 1, nil) }},
		{name: "assignees", call: func() error { return service.SetMRAssignees(t.Context(), "acme/widget", 1, nil) }},
		{name: "files", call: func() error { _, err := service.GetMRFiles(t.Context(), "acme/widget", 1); return err }},
		{name: "commits", call: func() error { _, err := service.GetMRCommits(t.Context(), "acme/widget", 1); return err }},
		{name: "projects", call: func() error { _, err := service.ListUserProjects(t.Context()); return err }},
		{name: "search projects", call: func() error { _, err := service.SearchProjects(t.Context(), "widget", 10); return err }},
		{name: "branches", call: func() error { _, err := service.ListProjectBranches(t.Context(), "acme/widget"); return err }},
		{name: "search MRs", call: func() error { _, err := service.SearchUserMRs(t.Context(), "", ""); return err }},
		{name: "search MRs paged", call: func() error { _, err := service.SearchUserMRsPaged(t.Context(), "", "", 1, 10); return err }},
		{name: "search issues", call: func() error { _, err := service.SearchUserIssues(t.Context(), "", "", ""); return err }},
		{name: "search issues paged", call: func() error { _, err := service.SearchUserIssuesPaged(t.Context(), "", "", "", 1, 10); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, ErrNoClient) {
				t.Fatalf("error = %v, want ErrNoClient", err)
			}
		})
	}
}

func TestServiceMockClientSearchAndActions(t *testing.T) {
	client := NewMockClient(DefaultHost)
	client.SeedProjectMembers("acme/widget", []ProjectMember{{ID: 7, Username: "alice", Name: "Alice"}})
	client.SeedBranches("acme/widget", []RepoBranch{{Name: "main"}})
	client.SeedMR("acme/widget", &MR{IID: 2, Title: "Feature", State: mrStateOpen})
	service := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))

	if err := service.SetMRLabels(t.Context(), "acme/widget", 2, []string{"backend"}); err != nil {
		t.Fatalf("SetMRLabels() error = %v", err)
	}
	if err := service.SetMRAssignees(t.Context(), "acme/widget", 2, []int{7}); err != nil {
		t.Fatalf("SetMRAssignees() error = %v", err)
	}
	mr, err := client.GetMR(t.Context(), "acme/widget", 2)
	if err != nil {
		t.Fatalf("GetMR() error = %v", err)
	}
	if len(mr.Labels) != 1 || mr.Labels[0] != "backend" || len(mr.Assignees) != 1 || mr.Assignees[0].Username != "alice" {
		t.Fatalf("MR labels/assignees = %#v/%#v, want backend/alice", mr.Labels, mr.Assignees)
	}

	branches, err := service.ListProjectBranches(t.Context(), "acme/widget")
	if err != nil || len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("ListProjectBranches() = (%#v, %v), want main", branches, err)
	}
	projects, err := service.SearchProjects(t.Context(), "sample", 5)
	if err != nil || len(projects) != 1 || projects[0].PathWithNamespace != "kandev/sample" {
		t.Fatalf("SearchProjects() = (%#v, %v), want kandev/sample", projects, err)
	}
	missing, err := service.SearchProjects(t.Context(), "absent", 5)
	if err != nil || len(missing) != 0 {
		t.Fatalf("SearchProjects(absent) = (%#v, %v), want empty", missing, err)
	}
}
