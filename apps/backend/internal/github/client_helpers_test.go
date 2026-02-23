package github

import (
	"testing"
	"time"
)

func TestBuildReviewSearchQuery(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		filter      string
		customQuery string
		want        string
	}{
		{
			name:        "customQuery overrides everything",
			scope:       ReviewScopeUser,
			filter:      "repo:owner/name",
			customQuery: "type:pr is:open assignee:@me",
			want:        "type:pr is:open assignee:@me",
		},
		{
			name:  "user scope without filter",
			scope: ReviewScopeUser,
			want:  "type:pr state:open user-review-requested:@me",
		},
		{
			name:   "user scope with repo filter",
			scope:  ReviewScopeUser,
			filter: "repo:owner/repo",
			want:   "type:pr state:open user-review-requested:@me repo:owner/repo",
		},
		{
			name:  "user_and_teams scope without filter",
			scope: ReviewScopeUserAndTeams,
			want:  "type:pr state:open review-requested:@me",
		},
		{
			name:   "user_and_teams scope with org filter",
			scope:  ReviewScopeUserAndTeams,
			filter: "org:myorg",
			want:   "type:pr state:open review-requested:@me org:myorg",
		},
		{
			name:  "empty scope defaults to user_and_teams",
			scope: "",
			want:  "type:pr state:open review-requested:@me",
		},
		{
			name:        "empty customQuery falls through to scope logic",
			scope:       ReviewScopeUserAndTeams,
			customQuery: "",
			want:        "type:pr state:open review-requested:@me",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildReviewSearchQuery(tt.scope, tt.filter, tt.customQuery)
			if got != tt.want {
				t.Errorf("buildReviewSearchQuery(%q, %q, %q) = %q, want %q",
					tt.scope, tt.filter, tt.customQuery, got, tt.want)
			}
		})
	}
}

func TestConvertRawCheckRuns(t *testing.T) {
	conclusion := "success"
	summary := "All tests passed"
	startedAt := "2025-01-15T10:00:00Z"
	completedAt := "2025-01-15T10:05:00Z"

	raw := []ghCheckRun{
		{
			Name:        "ci/test",
			Status:      "completed",
			Conclusion:  &conclusion,
			HTMLURL:     "https://github.com/owner/repo/runs/1",
			StartedAt:   startedAt,
			CompletedAt: completedAt,
			Output: struct {
				Title   *string `json:"title"`
				Summary *string `json:"summary"`
			}{Summary: &summary},
		},
		{
			Name:       "ci/lint",
			Status:     "in_progress",
			Conclusion: nil,
			HTMLURL:    "https://github.com/owner/repo/runs/2",
		},
	}

	checks := convertRawCheckRuns(raw)

	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}

	// Check first (completed)
	if checks[0].Name != "ci/test" {
		t.Errorf("expected name ci/test, got %s", checks[0].Name)
	}
	if checks[0].Conclusion != "success" {
		t.Errorf("expected conclusion success, got %s", checks[0].Conclusion)
	}
	if checks[0].Output != "All tests passed" {
		t.Errorf("expected output 'All tests passed', got %s", checks[0].Output)
	}
	if checks[0].StartedAt == nil {
		t.Error("expected non-nil StartedAt")
	}

	// Check second (in progress, nil conclusion)
	if checks[1].Conclusion != "" {
		t.Errorf("expected empty conclusion, got %s", checks[1].Conclusion)
	}
	if checks[1].StartedAt != nil {
		t.Error("expected nil StartedAt for empty string")
	}
}

func TestConvertRawCheckRunsEmpty(t *testing.T) {
	checks := convertRawCheckRuns(nil)
	if len(checks) != 0 {
		t.Errorf("expected empty slice, got %d", len(checks))
	}
}

func TestConvertRawComments(t *testing.T) {
	now := time.Now()
	raw := []ghComment{
		{
			ID:        1,
			Path:      "main.go",
			Line:      42,
			Side:      "RIGHT",
			Body:      "Looks good",
			CreatedAt: now,
			UpdatedAt: now,
			User: struct {
				Login     string `json:"login"`
				AvatarURL string `json:"avatar_url"`
			}{Login: "alice", AvatarURL: "https://avatar.example.com/alice"},
		},
	}

	comments := convertRawComments(raw)

	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Author != "alice" {
		t.Errorf("expected author alice, got %s", comments[0].Author)
	}
	if comments[0].Path != "main.go" {
		t.Errorf("expected path main.go, got %s", comments[0].Path)
	}
	if comments[0].Line != 42 {
		t.Errorf("expected line 42, got %d", comments[0].Line)
	}
}

func TestConvertSearchItemToPR(t *testing.T) {
	now := time.Now()
	pr := convertSearchItemToPR(
		42, "Fix bug", "https://github.com/owner/repo/pull/42", "open",
		"alice", "https://api.github.com/repos/myorg/myrepo", false,
		now, now,
	)

	if pr.Number != 42 {
		t.Errorf("expected number 42, got %d", pr.Number)
	}
	if pr.Title != "Fix bug" {
		t.Errorf("expected title 'Fix bug', got %s", pr.Title)
	}
	if pr.RepoOwner != "myorg" {
		t.Errorf("expected owner myorg, got %s", pr.RepoOwner)
	}
	if pr.RepoName != "myrepo" {
		t.Errorf("expected repo myrepo, got %s", pr.RepoName)
	}
}

func TestLatestReviewByAuthor(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	reviews := []PRReview{
		{Author: "alice", State: "COMMENTED", CreatedAt: t1},
		{Author: "alice", State: "APPROVED", CreatedAt: t2},
		{Author: "bob", State: "CHANGES_REQUESTED", CreatedAt: t1},
	}

	latest := latestReviewByAuthor(reviews)

	if len(latest) != 2 {
		t.Fatalf("expected 2 authors, got %d", len(latest))
	}
	if latest["alice"].State != "APPROVED" {
		t.Errorf("expected alice's latest to be APPROVED, got %s", latest["alice"].State)
	}
	if latest["bob"].State != "CHANGES_REQUESTED" {
		t.Errorf("expected bob's latest to be CHANGES_REQUESTED, got %s", latest["bob"].State)
	}
}

func TestLatestReviewByAuthorEmpty(t *testing.T) {
	latest := latestReviewByAuthor(nil)
	if len(latest) != 0 {
		t.Errorf("expected empty map, got %d entries", len(latest))
	}
}

func TestHasFailingChecks(t *testing.T) {
	tests := []struct {
		name   string
		checks []CheckRun
		want   bool
	}{
		{"empty", nil, false},
		{"all passing", []CheckRun{
			{Status: "completed", Conclusion: "success"},
		}, false},
		{"one failing", []CheckRun{
			{Status: "completed", Conclusion: "success"},
			{Status: "completed", Conclusion: "failure"},
		}, true},
		{"in progress", []CheckRun{
			{Status: "in_progress", Conclusion: ""},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasFailingChecks(tt.checks)
			if got != tt.want {
				t.Errorf("hasFailingChecks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasChangesRequested(t *testing.T) {
	tests := []struct {
		name    string
		reviews []PRReview
		want    bool
	}{
		{"empty", nil, false},
		{"approved", []PRReview{{State: "APPROVED"}}, false},
		{"changes requested", []PRReview{
			{State: "APPROVED"},
			{State: "CHANGES_REQUESTED"},
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasChangesRequested(tt.reviews)
			if got != tt.want {
				t.Errorf("hasChangesRequested() = %v, want %v", got, tt.want)
			}
		})
	}
}
