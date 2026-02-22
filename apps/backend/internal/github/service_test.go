package github

import (
	"testing"
	"time"
)

func TestParsePRURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantOwner  string
		wantRepo   string
		wantNumber int
		wantErr    bool
	}{
		{
			name:       "standard URL",
			url:        "https://github.com/owner/repo/pull/123",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 123,
		},
		{
			name:       "trailing slash",
			url:        "https://github.com/owner/repo/pull/456/",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 456,
		},
		{
			name:       "with query params",
			url:        "https://github.com/owner/repo/pull/789?diff=unified",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 789,
		},
		{
			name:       "with fragment",
			url:        "https://github.com/owner/repo/pull/42#discussion_r123",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 42,
		},
		{
			name:       "with query and fragment",
			url:        "https://github.com/owner/repo/pull/10?a=b#frag",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 10,
		},
		{
			name:       "with whitespace",
			url:        "  https://github.com/owner/repo/pull/5  \n",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 5,
		},
		{
			name:    "no pull segment",
			url:     "https://github.com/owner/repo/issues/1",
			wantErr: true,
		},
		{
			name:    "invalid PR number",
			url:     "https://github.com/owner/repo/pull/abc",
			wantErr: true,
		},
		{
			name:    "too short path",
			url:     "/pull/1",
			wantErr: true,
		},
		{
			name:    "empty owner",
			url:     "https://github.com//repo/pull/1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, number, err := parsePRURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if number != tt.wantNumber {
				t.Errorf("number = %d, want %d", number, tt.wantNumber)
			}
		})
	}
}

func TestComputeOverallCheckStatus(t *testing.T) {
	tests := []struct {
		name   string
		checks []CheckRun
		want   string
	}{
		{"empty", nil, ""},
		{
			"all completed success",
			[]CheckRun{
				{Status: "completed", Conclusion: "success"},
				{Status: "completed", Conclusion: "success"},
			},
			"success",
		},
		{
			"one failure",
			[]CheckRun{
				{Status: "completed", Conclusion: "success"},
				{Status: "completed", Conclusion: "failure"},
			},
			"failure",
		},
		{
			"failure takes priority over pending",
			[]CheckRun{
				{Status: "in_progress"},
				{Status: "completed", Conclusion: "failure"},
			},
			"failure",
		},
		{
			"pending when in progress",
			[]CheckRun{
				{Status: "completed", Conclusion: "success"},
				{Status: "in_progress"},
			},
			"pending",
		},
		{
			"all in progress",
			[]CheckRun{
				{Status: "queued"},
				{Status: "in_progress"},
			},
			"pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeOverallCheckStatus(tt.checks)
			if got != tt.want {
				t.Errorf("computeOverallCheckStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeOverallReviewState(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		reviews []PRReview
		want    string
	}{
		{"empty", nil, ""},
		{
			"all approved",
			[]PRReview{
				{Author: "alice", State: "APPROVED", CreatedAt: t1},
				{Author: "bob", State: "APPROVED", CreatedAt: t1},
			},
			"approved",
		},
		{
			"changes requested",
			[]PRReview{
				{Author: "alice", State: "APPROVED", CreatedAt: t1},
				{Author: "bob", State: "CHANGES_REQUESTED", CreatedAt: t1},
			},
			"changes_requested",
		},
		{
			"latest per author wins",
			[]PRReview{
				{Author: "alice", State: "CHANGES_REQUESTED", CreatedAt: t1},
				{Author: "alice", State: "APPROVED", CreatedAt: t2},
			},
			"approved",
		},
		{
			"pending when not all approved",
			[]PRReview{
				{Author: "alice", State: "APPROVED", CreatedAt: t1},
				{Author: "bob", State: "COMMENTED", CreatedAt: t1},
			},
			"pending",
		},
		{
			"changes requested takes priority over pending",
			[]PRReview{
				{Author: "alice", State: "COMMENTED", CreatedAt: t1},
				{Author: "bob", State: "CHANGES_REQUESTED", CreatedAt: t1},
			},
			"changes_requested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeOverallReviewState(tt.reviews)
			if got != tt.want {
				t.Errorf("computeOverallReviewState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCountPendingReviews(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		reviews []PRReview
		want    int
	}{
		{"empty", nil, 0},
		{
			"no pending",
			[]PRReview{
				{Author: "alice", State: "APPROVED", CreatedAt: t1},
			},
			0,
		},
		{
			"one pending",
			[]PRReview{
				{Author: "alice", State: "PENDING", CreatedAt: t1},
			},
			1,
		},
		{
			"commented counts as pending",
			[]PRReview{
				{Author: "alice", State: "COMMENTED", CreatedAt: t1},
			},
			1,
		},
		{
			"latest per author wins",
			[]PRReview{
				{Author: "alice", State: "COMMENTED", CreatedAt: t1},
				{Author: "alice", State: "APPROVED", CreatedAt: t2},
			},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countPendingReviews(tt.reviews)
			if got != tt.want {
				t.Errorf("countPendingReviews() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFindLatestCommentTime(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		comments []PRComment
		want     *time.Time
	}{
		{"empty", nil, nil},
		{
			"single",
			[]PRComment{{UpdatedAt: t1}},
			&t1,
		},
		{
			"multiple",
			[]PRComment{
				{UpdatedAt: t1},
				{UpdatedAt: t2},
				{UpdatedAt: t3},
			},
			&t2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findLatestCommentTime(tt.comments)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil, got nil")
			}
			if !got.Equal(*tt.want) {
				t.Errorf("got %v, want %v", *got, *tt.want)
			}
		})
	}
}
