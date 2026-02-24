package github

import (
	"testing"
	"time"
)

func TestConvertPatPR(t *testing.T) {
	raw := &patPR{
		Number:    10,
		Title:     "Feature Y",
		HTMLURL:   "https://github.com/org/repo/pull/10",
		State:     "open",
		Draft:     false,
		Additions: 200,
		Deletions: 30,
		CreatedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
		User: struct {
			Login string `json:"login"`
		}{Login: "bob"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "feature-y", SHA: "deadbeef1234"},
		Base: struct {
			Ref string `json:"ref"`
		}{Ref: "main"},
	}

	pr := convertPatPR(raw, "org", "repo")

	if pr.Number != 10 {
		t.Errorf("number = %d, want 10", pr.Number)
	}
	if pr.State != "open" {
		t.Errorf("state = %q, want open", pr.State)
	}
	if pr.AuthorLogin != "bob" {
		t.Errorf("author = %q, want bob", pr.AuthorLogin)
	}
	if pr.HeadBranch != "feature-y" {
		t.Errorf("head = %q, want feature-y", pr.HeadBranch)
	}
	if pr.HeadSHA != "deadbeef1234" {
		t.Errorf("head_sha = %q, want deadbeef1234", pr.HeadSHA)
	}
	if pr.Mergeable {
		t.Error("expected mergeable = false when nil")
	}
	if pr.MergedAt != nil {
		t.Error("expected nil MergedAt")
	}
}

func TestConvertPatPR_Merged(t *testing.T) {
	mergedAt := "2025-03-05T10:00:00Z"
	raw := &patPR{
		Number:   5,
		State:    "closed",
		MergedAt: &mergedAt,
		User: struct {
			Login string `json:"login"`
		}{Login: "alice"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "fix"},
		Base: struct {
			Ref string `json:"ref"`
		}{Ref: "main"},
	}

	pr := convertPatPR(raw, "org", "repo")

	if pr.State != prStateMerged {
		t.Errorf("state = %q, want merged", pr.State)
	}
	if pr.MergedAt == nil {
		t.Fatal("expected non-nil MergedAt")
	}
}

func TestConvertPatPR_Mergeable(t *testing.T) {
	mergeable := true
	raw := &patPR{
		Number:    1,
		State:     "open",
		Mergeable: &mergeable,
		User: struct {
			Login string `json:"login"`
		}{Login: "alice"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "b"},
		Base: struct {
			Ref string `json:"ref"`
		}{Ref: "main"},
	}

	pr := convertPatPR(raw, "o", "r")
	if !pr.Mergeable {
		t.Error("expected mergeable = true")
	}
}
