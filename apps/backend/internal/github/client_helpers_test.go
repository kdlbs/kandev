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
			want:  "type:pr state:open user-review-requested:@me -is:draft",
		},
		{
			name:   "user scope with repo filter",
			scope:  ReviewScopeUser,
			filter: "repo:owner/repo",
			want:   "type:pr state:open user-review-requested:@me -is:draft repo:owner/repo",
		},
		{
			name:  "user_and_teams scope without filter",
			scope: ReviewScopeUserAndTeams,
			want:  "type:pr state:open review-requested:@me -is:draft",
		},
		{
			name:   "user_and_teams scope with org filter",
			scope:  ReviewScopeUserAndTeams,
			filter: "org:myorg",
			want:   "type:pr state:open review-requested:@me -is:draft org:myorg",
		},
		{
			name:  "empty scope defaults to user_and_teams",
			scope: "",
			want:  "type:pr state:open review-requested:@me -is:draft",
		},
		{
			name:        "empty customQuery falls through to scope logic",
			scope:       ReviewScopeUserAndTeams,
			customQuery: "",
			want:        "type:pr state:open review-requested:@me -is:draft",
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
	if checks[0].Source != checkSourceCheckRun {
		t.Errorf("expected source %q, got %q", checkSourceCheckRun, checks[0].Source)
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
				Type      string `json:"type"`
			}{Login: "alice", AvatarURL: "https://avatar.example.com/alice", Type: "User"},
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
	if comments[0].CommentType != commentTypeReview {
		t.Errorf("expected comment type %q, got %q", commentTypeReview, comments[0].CommentType)
	}
	if comments[0].AuthorIsBot {
		t.Error("expected non-bot comment")
	}
}

func TestConvertRawIssueComments(t *testing.T) {
	now := time.Now()
	raw := []ghIssueComment{
		{
			ID:        10,
			Body:      "Snyk report",
			CreatedAt: now,
			UpdatedAt: now,
			User: struct {
				Login     string `json:"login"`
				AvatarURL string `json:"avatar_url"`
				Type      string `json:"type"`
			}{
				Login:     "snyk-io",
				AvatarURL: "https://avatar.example.com/snyk",
				Type:      "Bot",
			},
		},
	}

	comments := convertRawIssueComments(raw)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].CommentType != commentTypeIssue {
		t.Errorf("expected comment type %q, got %q", commentTypeIssue, comments[0].CommentType)
	}
	if !comments[0].AuthorIsBot {
		t.Error("expected bot comment")
	}
}

func TestMergeAndSortPRComments(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
	review := []PRComment{
		{ID: 2, CommentType: commentTypeReview, CreatedAt: t2},
		{ID: 3, CommentType: commentTypeReview, CreatedAt: t3},
	}
	issue := []PRComment{
		{ID: 1, CommentType: commentTypeIssue, CreatedAt: t1},
	}

	got := mergeAndSortPRComments(review, issue)
	if len(got) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 {
		t.Errorf("unexpected merge/sort order: %#v", got)
	}
}

func TestConvertRawStatusContextsAndMergeChecks(t *testing.T) {
	conclusion := "success"
	checkRuns := []ghCheckRun{
		{
			Name:       "ci/test",
			Status:     "completed",
			Conclusion: &conclusion,
			HTMLURL:    "https://github.com/owner/repo/runs/1",
		},
	}
	statuses := []ghStatusContext{
		{
			Context:   "ci/test",
			State:     "pending",
			TargetURL: "https://github.com/owner/repo/runs/1",
		},
		{
			Context:   "license/snyk",
			State:     "success",
			TargetURL: "https://app.snyk.io/check/2",
		},
	}

	merged := mergeChecks(convertRawCheckRuns(checkRuns), convertRawStatusContexts(statuses))
	if len(merged) != 2 {
		t.Fatalf("expected 2 checks after dedupe, got %d", len(merged))
	}
	if merged[0].Name != "ci/test" || merged[0].Source != checkSourceCheckRun {
		t.Errorf("expected check_run to win dedupe, got %#v", merged[0])
	}
	if merged[1].Name != "license/snyk" || merged[1].Source != checkSourceStatusContext {
		t.Errorf("expected status_context entry, got %#v", merged[1])
	}
	if merged[1].Conclusion != checkStatusSuccess {
		t.Errorf("expected success conclusion, got %q", merged[1].Conclusion)
	}
}

func TestMergeChecksDuplicateCheckRunsByName(t *testing.T) {
	t1 := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 11, 0, 0, 0, time.UTC)

	// Two check-runs with the same name but different URLs (different check suites).
	checkRuns := []CheckRun{
		{Name: "Analyze (go)", Source: checkSourceCheckRun, Status: "in_progress", HTMLURL: "https://github.com/o/r/runs/1", StartedAt: &t1},
		{Name: "Analyze (go)", Source: checkSourceCheckRun, Status: "in_progress", HTMLURL: "https://github.com/o/r/runs/2", StartedAt: &t2},
	}
	merged := mergeChecks(checkRuns, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 check after dedupe, got %d", len(merged))
	}
	if merged[0].HTMLURL != "https://github.com/o/r/runs/2" {
		t.Errorf("expected newer check-run to win, got URL %s", merged[0].HTMLURL)
	}
}

func TestMergeChecksCheckRunWinsOverStatusDifferentURL(t *testing.T) {
	t1 := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	checkRuns := []CheckRun{
		{Name: "ci/test", Source: checkSourceCheckRun, Status: "completed", Conclusion: "success", HTMLURL: "https://github.com/o/r/runs/1", StartedAt: &t1},
	}
	statuses := []CheckRun{
		{Name: "ci/test", Source: checkSourceStatusContext, Status: "in_progress", HTMLURL: "https://other-url.com/status/1", StartedAt: &t1},
	}
	merged := mergeChecks(checkRuns, statuses)
	if len(merged) != 1 {
		t.Fatalf("expected 1 check after dedupe, got %d", len(merged))
	}
	if merged[0].Source != checkSourceCheckRun {
		t.Errorf("expected check_run to win over status_context, got source %s", merged[0].Source)
	}
}

func TestIsNewerCheck(t *testing.T) {
	t1 := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 11, 0, 0, 0, time.UTC)

	if isNewerCheck(CheckRun{StartedAt: &t1}, CheckRun{StartedAt: &t2}) {
		t.Error("older should not be newer")
	}
	if !isNewerCheck(CheckRun{StartedAt: &t2}, CheckRun{StartedAt: &t1}) {
		t.Error("newer should be newer")
	}
	if isNewerCheck(CheckRun{StartedAt: nil}, CheckRun{StartedAt: &t1}) {
		t.Error("nil StartedAt should not be newer")
	}
	if !isNewerCheck(CheckRun{StartedAt: &t1}, CheckRun{StartedAt: nil}) {
		t.Error("non-nil should be newer than nil")
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

func TestCountCheckResults(t *testing.T) {
	tests := []struct {
		name                string
		checks              []CheckRun
		wantTotal, wantPass int
	}{
		{
			name:      "empty",
			wantTotal: 0, wantPass: 0,
		},
		{
			name: "in-progress checks excluded from total",
			checks: []CheckRun{
				{Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
				{Status: "in_progress", Conclusion: ""},
				{Status: "queued", Conclusion: ""},
			},
			wantTotal: 1, wantPass: 1,
		},
		{
			name: "skipped and neutral are excluded",
			checks: []CheckRun{
				{Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
				{Status: checkStatusCompleted, Conclusion: checkConclusionSkipped},
				{Status: checkStatusCompleted, Conclusion: checkConclusionNeutral},
			},
			wantTotal: 1, wantPass: 1,
		},
		{
			name: "failures count toward total but not passing",
			checks: []CheckRun{
				{Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
				{Status: checkStatusCompleted, Conclusion: checkConclusionFail},
				{Status: checkStatusCompleted, Conclusion: checkConclusionTimedOut},
			},
			wantTotal: 3, wantPass: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, passing := countCheckResults(tt.checks)
			if total != tt.wantTotal || passing != tt.wantPass {
				t.Errorf("countCheckResults = (%d, %d), want (%d, %d)",
					total, passing, tt.wantTotal, tt.wantPass)
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

// --- Reviewer dedup helpers (countApprovedReviewers + reduceReviewSummary) ---

func TestCountApprovedReviewers(t *testing.T) {
	t0 := time.Date(2025, 5, 1, 12, 0, 0, 0, time.UTC)
	mk := func(author, state string, offset time.Duration) PRReview {
		return PRReview{Author: author, State: state, CreatedAt: t0.Add(offset)}
	}
	cases := []struct {
		name    string
		reviews []PRReview
		want    int
	}{
		{name: "empty", reviews: []PRReview{}, want: 0},
		{
			name:    "single approval",
			reviews: []PRReview{mk("alice", reviewStateApproved, 0)},
			want:    1,
		},
		{
			name: "dedup same author latest wins",
			reviews: []PRReview{
				mk("alice", reviewStateChangesRequested, 0),
				mk("alice", reviewStateApproved, time.Hour),
			},
			want: 1,
		},
		{
			name: "COMMENTED does not replace prior APPROVED",
			reviews: []PRReview{
				mk("alice", reviewStateApproved, 0),
				mk("alice", "COMMENTED", time.Hour),
			},
			want: 1,
		},
		{
			name:    "PENDING never counts even alone",
			reviews: []PRReview{mk("alice", "PENDING", 0)},
			want:    0,
		},
		{
			name: "two distinct approvers",
			reviews: []PRReview{
				mk("alice", reviewStateApproved, 0),
				mk("bob", reviewStateApproved, time.Hour),
			},
			want: 2,
		},
		{
			name: "approve then change-request reverts",
			reviews: []PRReview{
				mk("alice", reviewStateApproved, 0),
				mk("alice", reviewStateChangesRequested, time.Hour),
			},
			want: 0,
		},
		{
			name: "anonymous reviewers counted separately",
			reviews: []PRReview{
				mk("", reviewStateApproved, 0),
				mk("", reviewStateApproved, time.Hour),
			},
			want: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countApprovedReviewers(tc.reviews); got != tc.want {
				t.Fatalf("countApprovedReviewers = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestReduceReviewSummary(t *testing.T) {
	t0 := time.Date(2025, 5, 1, 12, 0, 0, 0, time.UTC)
	mk := func(author, state string, offset time.Duration) reviewSample {
		return reviewSample{author: author, state: state, at: t0.Add(offset)}
	}
	cases := []struct {
		name string
		in   []reviewSample
		want string
	}{
		{name: "empty", in: nil, want: ""},
		{
			name: "single approval",
			in:   []reviewSample{mk("a", reviewStateApproved, 0)},
			want: computedReviewStateApproved,
		},
		{
			name: "changes requested wins across authors",
			in: []reviewSample{
				mk("a", reviewStateApproved, 0),
				mk("b", reviewStateChangesRequested, time.Hour),
			},
			want: computedReviewStateChangesRequested,
		},
		{
			name: "later approval supersedes earlier changes-requested from same author",
			in: []reviewSample{
				mk("a", reviewStateChangesRequested, 0),
				mk("a", reviewStateApproved, time.Hour),
			},
			want: computedReviewStateApproved,
		},
		{
			name: "only comments returns empty",
			in:   []reviewSample{mk("a", "COMMENTED", 0)},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reduceReviewSummary(tc.in); got != tc.want {
				t.Fatalf("reduceReviewSummary = %q, want %q", got, tc.want)
			}
		})
	}
}
