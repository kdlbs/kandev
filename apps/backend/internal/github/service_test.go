package github

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGetPR_NilClient(t *testing.T) {
	svc := &Service{client: nil}
	_, err := svc.GetPR(context.Background(), "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error when client is nil")
	}
	if !strings.Contains(err.Error(), "github client not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPRFeedback_NilClient(t *testing.T) {
	svc := &Service{client: nil}
	_, err := svc.GetPRFeedback(context.Background(), "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error when client is nil")
	}
	if !strings.Contains(err.Error(), "github client not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

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

func TestCountPendingRequestedReviewers(t *testing.T) {
	tests := []struct {
		name string
		pr   *PR
		want int
	}{
		{"nil pr", nil, 0},
		{"none", &PR{}, 0},
		{
			"with requested reviewers",
			&PR{
				RequestedReviewers: []RequestedReviewer{
					{Login: "alice", Type: reviewerTypeUser},
					{Login: "core-team", Type: reviewerTypeTeam},
				},
			},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countPendingRequestedReviewers(tt.pr)
			if got != tt.want {
				t.Errorf("countPendingRequestedReviewers() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDeriveReviewSyncState(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		pr        *PR
		reviews   []PRReview
		wantState string
		wantCount int
	}{
		{
			name:      "uses requested reviewers before review-derived pending",
			pr:        &PR{RequestedReviewers: []RequestedReviewer{{Login: "alice", Type: reviewerTypeUser}}},
			reviews:   []PRReview{{Author: "alice", State: reviewStateCommented, CreatedAt: t1}},
			wantState: computedReviewStatePending,
			wantCount: 1,
		},
		{
			name:      "fallback to review-derived pending when no requests",
			pr:        &PR{},
			reviews:   []PRReview{{Author: "alice", State: reviewStateCommented, CreatedAt: t1}},
			wantState: computedReviewStatePending,
			wantCount: 1,
		},
		{
			name:      "no submitted reviews but pending requests yields pending state",
			pr:        &PR{RequestedReviewers: []RequestedReviewer{{Login: "core", Type: reviewerTypeTeam}}},
			reviews:   nil,
			wantState: computedReviewStatePending,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotCount := deriveReviewSyncState(tt.pr, tt.reviews)
			if gotState != tt.wantState {
				t.Errorf("deriveReviewSyncState() state = %q, want %q", gotState, tt.wantState)
			}
			if gotCount != tt.wantCount {
				t.Errorf("deriveReviewSyncState() pending count = %d, want %d", gotCount, tt.wantCount)
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

func TestTriggerPRSync_SyncsExistingWatch(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up a PR in mock client.
	mockClient.AddPR(&PR{
		Number:     10,
		Title:      "Feature PR",
		State:      "open",
		HeadSHA:    "abc123",
		HeadBranch: "feat/x",
		RepoOwner:  "org",
		RepoName:   "repo",
	})

	// Create a PR watch and TaskPR in DB.
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "org",
		Repo:      "repo",
		PRNumber:  10,
		Branch:    "feat/x",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	tp := &TaskPR{
		TaskID:   "t1",
		Owner:    "org",
		Repo:     "repo",
		PRNumber: 10,
		PRTitle:  "Feature PR",
		State:    "open",
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatal(err)
	}

	// Trigger sync.
	result, err := svc.TriggerPRSync(ctx, "t1")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil TaskPR")
	}
	if result.LastSyncedAt == nil {
		t.Error("expected LastSyncedAt to be set after sync")
	}
}

func TestTriggerPRSync_DetectsPR(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up a PR findable by branch.
	mockClient.AddPR(&PR{
		Number:     20,
		Title:      "New PR",
		State:      "open",
		HeadBranch: "feat/y",
		RepoOwner:  "org",
		RepoName:   "repo",
	})

	// Create a watch with pr_number=0 (still searching).
	watch := &PRWatch{
		SessionID: "s2",
		TaskID:    "t2",
		Owner:     "org",
		Repo:      "repo",
		PRNumber:  0,
		Branch:    "feat/y",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}

	result, err := svc.TriggerPRSync(ctx, "t2")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result == nil {
		t.Fatal("expected TaskPR after detection")
	}
	if result.PRNumber != 20 {
		t.Errorf("expected PR #20, got #%d", result.PRNumber)
	}
}

func TestTriggerPRSync_NoWatch(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	ctx := context.Background()

	result, err := svc.TriggerPRSync(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil TaskPR for task without watch, got %+v", result)
	}
}
