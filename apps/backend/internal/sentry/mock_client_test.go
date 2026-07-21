package sentry

import (
	"context"
	"net/http"
	"testing"
)

// mockInstance returns a Client view of the shared mock bound to instanceID.
func mockInstance(m *MockClient, instanceID string) Client {
	return MockClientFactory(m)(&SentryConfig{ID: instanceID}, "")
}

func TestMockClient_DefaultsToSuccessfulAuth(t *testing.T) {
	m := NewMockClient()
	res, err := mockInstance(m, "inst-1").TestAuth(context.Background())
	if err != nil || !res.OK {
		t.Fatalf("expected OK=true by default, got %+v err=%v", res, err)
	}
}

func TestMockClient_SearchFiltersByProject(t *testing.T) {
	m := NewMockClient()
	m.AddIssue("inst-1", &SentryIssue{ShortID: "FE-1", Title: "Boom", Level: "error", Status: "unresolved", ProjectSlug: "frontend"})
	m.AddIssue("inst-1", &SentryIssue{ShortID: "BE-1", Title: "Crash", Level: "fatal", Status: "unresolved", ProjectSlug: "backend"})

	res, err := mockInstance(m, "inst-1").SearchIssues(context.Background(), SearchFilter{
		OrgSlug: "acme", ProjectSlug: "frontend",
	}, "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Issues) != 1 || res.Issues[0].ShortID != "FE-1" {
		t.Errorf("expected only FE-1, got %+v", res.Issues)
	}
}

func TestMockClient_SearchFiltersByLevelStatusQuery(t *testing.T) {
	m := NewMockClient()
	m.AddIssue("inst-1", &SentryIssue{ShortID: "A-1", Title: "Login failed", Level: "error", Status: "unresolved", ProjectSlug: "fe"})
	m.AddIssue("inst-1", &SentryIssue{ShortID: "A-2", Title: "Signup failed", Level: "warning", Status: "unresolved", ProjectSlug: "fe"})
	m.AddIssue("inst-1", &SentryIssue{ShortID: "A-3", Title: "Login broken", Level: "error", Status: "resolved", ProjectSlug: "fe"})

	client := mockInstance(m, "inst-1")
	cases := []struct {
		name   string
		filter SearchFilter
		want   []string
	}{
		{"levels", SearchFilter{OrgSlug: "acme", Levels: []string{"error"}}, []string{"A-1", "A-3"}},
		{"statuses", SearchFilter{OrgSlug: "acme", Statuses: []string{"resolved"}}, []string{"A-3"}},
		{"query", SearchFilter{OrgSlug: "acme", Query: "Login"}, []string{"A-1", "A-3"}},
		{"combined", SearchFilter{OrgSlug: "acme", Levels: []string{"error"}, Statuses: []string{"unresolved"}}, []string{"A-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := client.SearchIssues(context.Background(), tc.filter, "")
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			got := make([]string, 0, len(res.Issues))
			for _, i := range res.Issues {
				got = append(got, i.ShortID)
			}
			if !sameStrings(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMockClient_GetIssueByShortIDOrNumeric(t *testing.T) {
	m := NewMockClient()
	m.AddIssue("inst-1", &SentryIssue{ID: "99", ShortID: "PROJ-1", Title: "x"})
	client := mockInstance(m, "inst-1")
	if _, err := client.GetIssue(context.Background(), "PROJ-1"); err != nil {
		t.Errorf("short id lookup failed: %v", err)
	}
	if _, err := client.GetIssue(context.Background(), "99"); err != nil {
		t.Errorf("numeric id lookup failed: %v", err)
	}
	if _, err := client.GetIssue(context.Background(), "missing"); err == nil {
		t.Error("expected error on missing id")
	}
}

func TestMockClient_GetIssueErrorOverride(t *testing.T) {
	m := NewMockClient()
	m.AddIssue("inst-1", &SentryIssue{ShortID: "PROJ-1"})
	m.SetGetIssueError("inst-1", &APIError{StatusCode: http.StatusInternalServerError, Message: "boom"})
	if _, err := mockInstance(m, "inst-1").GetIssue(context.Background(), "PROJ-1"); err == nil {
		t.Error("expected forced error")
	}
}

func TestMockClient_Reset(t *testing.T) {
	m := NewMockClient()
	m.AddIssue("inst-1", &SentryIssue{ShortID: "PROJ-1"})
	m.SetProjects("inst-1", []SentryProject{{Slug: "fe"}})
	m.Reset()
	client := mockInstance(m, "inst-1")
	res, _ := client.SearchIssues(context.Background(), SearchFilter{OrgSlug: "x"}, "")
	if len(res.Issues) != 0 {
		t.Error("expected issues cleared")
	}
	projects, _ := client.ListProjects(context.Background())
	if len(projects) != 0 {
		t.Error("expected projects cleared")
	}
}

// TestMockClient_InstanceIsolation proves two instances hold independent data.
func TestMockClient_InstanceIsolation(t *testing.T) {
	m := NewMockClient()
	m.AddIssue("inst-a", &SentryIssue{ShortID: "A-1", Title: "from A", ProjectSlug: "fe"})
	m.AddIssue("inst-b", &SentryIssue{ShortID: "B-1", Title: "from B", ProjectSlug: "fe"})

	aRes, _ := mockInstance(m, "inst-a").SearchIssues(context.Background(), SearchFilter{OrgSlug: "acme"}, "")
	bRes, _ := mockInstance(m, "inst-b").SearchIssues(context.Background(), SearchFilter{OrgSlug: "acme"}, "")
	if len(aRes.Issues) != 1 || aRes.Issues[0].ShortID != "A-1" {
		t.Errorf("instance A leaked/missing data: %+v", aRes.Issues)
	}
	if len(bRes.Issues) != 1 || bRes.Issues[0].ShortID != "B-1" {
		t.Errorf("instance B leaked/missing data: %+v", bRes.Issues)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
