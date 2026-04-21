package github

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
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
		{
			// Regression for the "CI pending" badge bug: GitHub shows
			// "All checks have passed — 1 skipped, 20 successful" but
			// we were returning pending because the skipped runs weren't
			// treated as completed-successful.
			"AllSkipped mixed with success returns success",
			func() []CheckRun {
				checks := make([]CheckRun, 21)
				for i := 0; i < 20; i++ {
					checks[i] = CheckRun{Status: "completed", Conclusion: "success"}
				}
				checks[20] = CheckRun{Status: "completed", Conclusion: "skipped"}
				return checks
			}(),
			"success",
		},
		{
			"only skipped returns empty (no signal)",
			[]CheckRun{
				{Status: "completed", Conclusion: "skipped"},
				{Status: "completed", Conclusion: "skipped"},
			},
			"",
		},
		{
			"only neutral returns empty (no signal)",
			[]CheckRun{
				{Status: "completed", Conclusion: "neutral"},
				{Status: "completed", Conclusion: "neutral"},
			},
			"",
		},
		{
			"unknown future conclusion treated as passing",
			[]CheckRun{
				{Status: "completed", Conclusion: "stale"}, // hypothetical future enum
			},
			"success",
		},
		{
			"neutral conclusion is ignored like skipped",
			[]CheckRun{
				{Status: "completed", Conclusion: "success"},
				{Status: "completed", Conclusion: "neutral"},
			},
			"success",
		},
		{
			"cancelled conclusion counts as failure",
			[]CheckRun{
				{Status: "completed", Conclusion: "success"},
				{Status: "completed", Conclusion: "cancelled"},
			},
			"failure",
		},
		{
			"timed_out conclusion counts as failure",
			[]CheckRun{
				{Status: "completed", Conclusion: "timed_out"},
			},
			"failure",
		},
		{
			"action_required conclusion counts as failure",
			[]CheckRun{
				{Status: "completed", Conclusion: "action_required"},
			},
			"failure",
		},
		{
			"in_progress plus skipped returns pending (skipped ignored)",
			[]CheckRun{
				{Status: "in_progress"},
				{Status: "completed", Conclusion: "skipped"},
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

// --- mockEventBus for SyncTaskPR tests ---

type mockEventBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (m *mockEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *mockEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *mockEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *mockEventBus) Close() {}

func (m *mockEventBus) IsConnected() bool { return true }

func (m *mockEventBus) publishedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// setupSyncTest creates a Service backed by in-memory SQLite and a mock event bus.
func setupSyncTest(t *testing.T) (*Service, *Store, *mockEventBus) {
	t.Helper()

	rawDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	rawDB.SetMaxIdleConns(1)
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eb := &mockEventBus{}
	svc := NewService(nil, "pat", nil, store, eb, log)

	return svc, store, eb
}

func TestSyncTaskPR_PublishesEventOnChange(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:     "t1",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   1,
		PRURL:      "https://github.com/owner/repo/pull/1",
		PRTitle:    "Initial",
		HeadBranch: "feat",
		BaseBranch: "main",
		State:      "open",
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	status := &PRStatus{
		PR: &PR{
			Number:    1,
			Title:     "Updated title",
			State:     "open",
			RepoOwner: "owner",
			RepoName:  "repo",
		},
	}

	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("sync task PR: %v", err)
	}

	if got := eb.publishedCount(); got != 1 {
		t.Errorf("expected 1 event after change, got %d", got)
	}
}

func TestSyncTaskPR_NoEventWhenUnchanged(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:      "t1",
		Owner:       "owner",
		Repo:        "repo",
		PRNumber:    1,
		PRURL:       "https://github.com/owner/repo/pull/1",
		PRTitle:     "Same title",
		HeadBranch:  "feat",
		BaseBranch:  "main",
		State:       "open",
		Additions:   10,
		Deletions:   5,
		ReviewState: "approved",
		ChecksState: "success",
		ReviewCount: 2,
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	status := &PRStatus{
		PR: &PR{
			Number:    1,
			Title:     "Same title",
			State:     "open",
			Additions: 10,
			Deletions: 5,
			RepoOwner: "owner",
			RepoName:  "repo",
		},
		ReviewState: "approved",
		ChecksState: "success",
		ReviewCount: 2,
	}

	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("sync task PR: %v", err)
	}

	if got := eb.publishedCount(); got != 0 {
		t.Errorf("expected 0 events when unchanged, got %d", got)
	}
}

func TestSyncTaskPR_SecondIdenticalSyncNoEvent(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:     "t1",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   1,
		PRURL:      "https://github.com/owner/repo/pull/1",
		PRTitle:    "Original",
		HeadBranch: "feat",
		BaseBranch: "main",
		State:      "open",
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	status := &PRStatus{
		PR: &PR{
			Number:    1,
			Title:     "Changed",
			State:     "open",
			RepoOwner: "owner",
			RepoName:  "repo",
		},
	}

	// First sync: data changed -> event.
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Fatalf("expected 1 event after first sync, got %d", got)
	}

	// Second sync with same data -> no additional event.
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Errorf("expected still 1 event after identical second sync, got %d", got)
	}
}

func TestSyncTaskPR_PublishesEventOnMergeableStateChange(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:         "t1",
		Owner:          "owner",
		Repo:           "repo",
		PRNumber:       1,
		PRURL:          "https://github.com/owner/repo/pull/1",
		PRTitle:        "Same",
		HeadBranch:     "feat",
		BaseBranch:     "main",
		State:          "open",
		Additions:      3,
		Deletions:      1,
		ReviewState:    "approved",
		ChecksState:    "success",
		MergeableState: "blocked",
		ReviewCount:    1,
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Only mergeable_state changes (blocked -> clean); everything else identical.
	status := &PRStatus{
		PR: &PR{
			Number:    1,
			Title:     "Same",
			State:     "open",
			Additions: 3,
			Deletions: 1,
			RepoOwner: "owner",
			RepoName:  "repo",
		},
		ReviewState:    "approved",
		ChecksState:    "success",
		MergeableState: "clean",
		ReviewCount:    1,
	}

	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Errorf("expected 1 event after mergeable_state change, got %d", got)
	}

	stored, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("get task PR: %v", err)
	}
	if stored.MergeableState != "clean" {
		t.Errorf("expected stored mergeable_state=clean, got %q", stored.MergeableState)
	}
}

func TestTimeEqual(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Second)

	tests := []struct {
		name string
		a, b *time.Time
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil", nil, &now, false},
		{"b nil", &now, nil, false},
		{"equal", &now, &now, true},
		{"not equal", &now, &later, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timeEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("timeEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

