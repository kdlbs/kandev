package sentry

import (
	"context"
	"net/http"
	"testing"
)

func TestMockClient_DefaultsToSuccessfulAuth(t *testing.T) {
	m := NewMockClient()
	res, err := m.TestAuth(context.Background())
	if err != nil || !res.OK {
		t.Fatalf("expected OK=true by default, got %+v err=%v", res, err)
	}
}

func TestMockClient_SearchFiltersByProject(t *testing.T) {
	m := NewMockClient()
	m.AddIssue(&SentryIssue{ShortID: "FE-1", Title: "Boom", Level: "error", Status: "unresolved", ProjectSlug: "frontend"})
	m.AddIssue(&SentryIssue{ShortID: "BE-1", Title: "Crash", Level: "fatal", Status: "unresolved", ProjectSlug: "backend"})

	res, err := m.SearchIssues(context.Background(), SearchFilter{
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
	m.AddIssue(&SentryIssue{ShortID: "A-1", Title: "Login failed", Level: "error", Status: "unresolved", ProjectSlug: "fe"})
	m.AddIssue(&SentryIssue{ShortID: "A-2", Title: "Signup failed", Level: "warning", Status: "unresolved", ProjectSlug: "fe"})
	m.AddIssue(&SentryIssue{ShortID: "A-3", Title: "Login broken", Level: "error", Status: "resolved", ProjectSlug: "fe"})

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
			res, err := m.SearchIssues(context.Background(), tc.filter, "")
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
	m.AddIssue(&SentryIssue{ID: "99", ShortID: "PROJ-1", Title: "x"})
	if _, err := m.GetIssue(context.Background(), "PROJ-1"); err != nil {
		t.Errorf("short id lookup failed: %v", err)
	}
	if _, err := m.GetIssue(context.Background(), "99"); err != nil {
		t.Errorf("numeric id lookup failed: %v", err)
	}
	if _, err := m.GetIssue(context.Background(), "missing"); err == nil {
		t.Error("expected error on missing id")
	}
}

func TestMockClient_GetIssueErrorOverride(t *testing.T) {
	m := NewMockClient()
	m.AddIssue(&SentryIssue{ShortID: "PROJ-1"})
	m.SetGetIssueError(&APIError{StatusCode: http.StatusInternalServerError, Message: "boom"})
	if _, err := m.GetIssue(context.Background(), "PROJ-1"); err == nil {
		t.Error("expected forced error")
	}
}

func TestMockClient_Reset(t *testing.T) {
	m := NewMockClient()
	m.AddIssue(&SentryIssue{ShortID: "PROJ-1"})
	m.SetProjects([]SentryProject{{Slug: "fe"}})
	m.Reset()
	res, _ := m.SearchIssues(context.Background(), SearchFilter{OrgSlug: "x"}, "")
	if len(res.Issues) != 0 {
		t.Error("expected issues cleared")
	}
	projects, _ := m.ListProjects(context.Background())
	if len(projects) != 0 {
		t.Error("expected projects cleared")
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
