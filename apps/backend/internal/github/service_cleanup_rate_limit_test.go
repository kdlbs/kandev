package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
)

type cleanupRateLimitClient struct {
	NoopClient
	feedback    map[int]cleanupPRResponse
	issues      map[int]cleanupIssueResponse
	prCalls     []int
	reviewCalls []int
	issueCalls  []int
	user        string
	userErr     error
}

type cleanupGHCLIClient struct {
	*GHClient
	err   error
	calls []int
}

func (c *cleanupGHCLIClient) GetPR(_ context.Context, _, _ string, number int) (*PR, error) {
	c.calls = append(c.calls, number)
	if c.err != nil {
		return nil, c.err
	}
	return &PR{Number: number, State: prStateMerged}, nil
}

type cleanupPRResponse struct {
	state   string
	reviews []PRReview
	err     error
}

type cleanupIssueResponse struct {
	state string
	err   error
}

func (c *cleanupRateLimitClient) GetPR(_ context.Context, owner, repo string, number int) (*PR, error) {
	c.prCalls = append(c.prCalls, number)
	response := c.feedback[number]
	if response.err != nil {
		return nil, response.err
	}
	state := response.state
	if state == "" {
		state = prStateMerged
	}
	return &PR{Number: number, State: state, RepoOwner: owner, RepoName: repo}, nil
}

func (c *cleanupRateLimitClient) ListPRReviews(_ context.Context, _, _ string, number int) ([]PRReview, error) {
	c.reviewCalls = append(c.reviewCalls, number)
	response := c.feedback[number]
	if response.err != nil {
		return nil, response.err
	}
	return response.reviews, nil
}

func (c *cleanupRateLimitClient) GetIssueState(_ context.Context, _, _ string, number int) (string, error) {
	c.issueCalls = append(c.issueCalls, number)
	response := c.issues[number]
	if response.err != nil {
		return "", response.err
	}
	if response.state == "" {
		return "closed", nil
	}
	return response.state, nil
}

func (c *cleanupRateLimitClient) GetAuthenticatedUser(context.Context) (string, error) {
	if c.userErr != nil {
		return "", c.userErr
	}
	if c.user == "" {
		return mockDefaultUser, nil
	}
	return c.user, nil
}

type cleanupAutomationProvider struct {
	client  Client
	tracker *RateTracker
}

func (p cleanupAutomationProvider) ResolveAutomation(
	_ context.Context,
	connection *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return &ResolvedCredential{
		Client:       p.client,
		Capabilities: allTokenCapabilities(),
		RateTracker:  p.tracker,
		Principal: AuthPrincipal{
			Kind:   AuthPrincipalHuman,
			Source: ConnectionSourcePAT,
			Login:  connection.Login,
		},
	}, nil
}

func configureCleanupRateLimitClient(t *testing.T, svc *Service, client Client, tracker *RateTracker) {
	t.Helper()
	configureTestWorkspaceAuth(t, svc, client, "ws-1")
	if tracker != nil {
		svc.resolver.SetAutomationProvider(cleanupAutomationProvider{client: client, tracker: tracker})
	}
}

func createReviewCleanupRows(t *testing.T, store *Store, watch *ReviewWatch, taskIDs ...string) {
	t.Helper()
	ctx := context.Background()
	for i, taskID := range taskIDs {
		if taskID != "" {
			seedTask(t, store, taskID, false)
		}
		if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
			ReviewWatchID: watch.ID,
			RepoOwner:     "acme",
			RepoName:      "widget",
			PRNumber:      i + 1,
			TaskID:        taskID,
		}); err != nil {
			t.Fatalf("CreateReviewPRTask(%d): %v", i+1, err)
		}
	}
}

func createIssueCleanupRows(t *testing.T, store *Store, watch *IssueWatch, taskIDs ...string) {
	t.Helper()
	ctx := context.Background()
	for i, taskID := range taskIDs {
		if taskID != "" {
			seedTask(t, store, taskID, false)
		}
		issueNumber := i + 1
		reserved, err := store.ReserveIssueWatchTask(
			ctx, watch.ID, "acme", "widget", issueNumber,
			fmt.Sprintf("https://github.com/acme/widget/issues/%d", issueNumber),
		)
		if err != nil || !reserved {
			t.Fatalf("ReserveIssueWatchTask(%d): reserved=%v err=%v", issueNumber, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", issueNumber, taskID); err != nil {
				t.Fatalf("AssignIssueWatchTaskID(%d): %v", issueNumber, err)
			}
		}
	}
}

func rateLimitAPIError(status int, body string) error {
	return fmt.Errorf("cleanup fetch: %w", &GitHubAPIError{StatusCode: status, Body: body})
}

func assertCleanupRateLimitError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected cleanup to return a rate-limit error")
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || !isGitHubRateLimitAPIError(apiErr) {
		t.Fatalf("cleanup error = %v, want a classified rate-limit API error", err)
	}
}

func TestCleanupBatchStopsOnRateLimit(t *testing.T) {
	t.Run("review partial count and retry", func(t *testing.T) {
		_, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{feedback: map[int]cleanupPRResponse{
			2: {err: rateLimitAPIError(http.StatusTooManyRequests, "secondary limit")},
		}}
		configureCleanupRateLimitClient(t, svc, client, nil)
		watch := &ReviewWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
		if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
			t.Fatalf("CreateReviewWatch: %v", err)
		}
		createReviewCleanupRows(t, store, watch, "review-one", "review-two", "review-three")
		recorder := &recordingTaskDeleter{}
		svc.SetTaskDeleter(recorder)

		deleted, err := svc.CleanupMergedReviewTasks(context.Background(), watch)
		assertCleanupRateLimitError(t, err)
		if deleted != 1 {
			t.Fatalf("deleted = %d, want partial count 1", deleted)
		}
		if !reflect.DeepEqual(client.prCalls, []int{1, 2}) {
			t.Fatalf("GetPR calls = %v, want [1 2]", client.prCalls)
		}

		client.feedback[2] = cleanupPRResponse{}
		deleted, err = svc.CleanupMergedReviewTasks(context.Background(), watch)
		if err != nil || deleted != 2 {
			t.Fatalf("retry deleted=%d err=%v, want 2 and nil", deleted, err)
		}
	})

	t.Run("issue wrapped forbidden rate limit", func(t *testing.T) {
		_, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{issues: map[int]cleanupIssueResponse{
			1: {err: rateLimitAPIError(http.StatusForbidden, "abuse detection mechanism triggered")},
		}}
		configureCleanupRateLimitClient(t, svc, client, nil)
		watch := &IssueWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
		if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
			t.Fatalf("CreateIssueWatch: %v", err)
		}
		createIssueCleanupRows(t, store, watch, "issue-one", "issue-two")
		svc.SetTaskDeleter(&recordingTaskDeleter{})

		deleted, err := svc.CleanupClosedIssueTasks(context.Background(), watch)
		assertCleanupRateLimitError(t, err)
		if deleted != 0 || !reflect.DeepEqual(client.issueCalls, []int{1}) {
			t.Fatalf("deleted=%d issue calls=%v, want 0 and [1]", deleted, client.issueCalls)
		}
	})

	t.Run("empty reservation is admitted and can stop the batch", func(t *testing.T) {
		_, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{feedback: map[int]cleanupPRResponse{
			1: {err: rateLimitAPIError(http.StatusForbidden, "API rate limit exceeded")},
		}}
		configureCleanupRateLimitClient(t, svc, client, nil)
		watch := &ReviewWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
		if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
			t.Fatalf("CreateReviewWatch: %v", err)
		}
		createReviewCleanupRows(t, store, watch, "", "review-after-limit")
		svc.SetTaskDeleter(&recordingTaskDeleter{})

		deleted, err := svc.CleanupMergedReviewTasks(context.Background(), watch)
		assertCleanupRateLimitError(t, err)
		if deleted != 0 || !reflect.DeepEqual(client.prCalls, []int{1}) {
			t.Fatalf("deleted=%d GetPR calls=%v, want 0 and [1]", deleted, client.prCalls)
		}
	})
}

func TestCleanupBatchPreservesOrdinaryErrorsAndStopsOnCancellation(t *testing.T) {
	t.Run("unrelated forbidden continues", func(t *testing.T) {
		_, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{feedback: map[int]cleanupPRResponse{
			1: {err: rateLimitAPIError(http.StatusForbidden, "resource forbidden")},
		}}
		configureCleanupRateLimitClient(t, svc, client, nil)
		watch := &ReviewWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
		if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
			t.Fatalf("CreateReviewWatch: %v", err)
		}
		createReviewCleanupRows(t, store, watch, "forbidden", "healthy")
		recorder := &recordingTaskDeleter{}
		svc.SetTaskDeleter(recorder)

		deleted, err := svc.CleanupMergedReviewTasks(context.Background(), watch)
		if err != nil || deleted != 1 {
			t.Fatalf("deleted=%d err=%v, want 1 and nil", deleted, err)
		}
		if !reflect.DeepEqual(client.prCalls, []int{1, 2}) || !reflect.DeepEqual(recorder.calls, []string{"healthy"}) {
			t.Fatalf("GetPR calls=%v deletes=%v, want [1 2] and [healthy]", client.prCalls, recorder.calls)
		}
	})

	t.Run("authenticated user rate limit propagates", func(t *testing.T) {
		_, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{
			feedback: map[int]cleanupPRResponse{1: {
				state:   "open",
				reviews: []PRReview{{State: "APPROVED", Author: mockDefaultUser}},
			}},
			userErr: rateLimitAPIError(http.StatusTooManyRequests, "rate limit exceeded"),
		}
		configureCleanupRateLimitClient(t, svc, client, nil)
		watch := &ReviewWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
		if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
			t.Fatalf("CreateReviewWatch: %v", err)
		}
		createReviewCleanupRows(t, store, watch, "approved")
		svc.SetTaskDeleter(&recordingTaskDeleter{})

		deleted, err := svc.CleanupMergedReviewTasks(context.Background(), watch)
		assertCleanupRateLimitError(t, err)
		if deleted != 0 {
			t.Fatalf("deleted = %d, want 0", deleted)
		}
	})

	t.Run("cancellation stops before provider call", func(t *testing.T) {
		_, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{}
		configureCleanupRateLimitClient(t, svc, client, nil)
		watch := &ReviewWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
		if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
			t.Fatalf("CreateReviewWatch: %v", err)
		}
		createReviewCleanupRows(t, store, watch, "cancelled")
		svc.SetTaskDeleter(&recordingTaskDeleter{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
		if !errors.Is(err, context.Canceled) || deleted != 0 || len(client.prCalls) != 0 {
			t.Fatalf("deleted=%d err=%v calls=%v, want cancellation before provider call", deleted, err, client.prCalls)
		}
	})
}

func TestCleanupBatchStopsWhenWorkspaceCoreTrackerIsExhausted(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	client := &cleanupRateLimitClient{}
	tracker := NewRateTracker(nil, nil)
	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 0, Limit: 5000,
		ResetAt: time.Now().Add(time.Hour), UpdatedAt: time.Now().UTC(),
	})
	configureCleanupRateLimitClient(t, svc, client, tracker)
	watch := &ReviewWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	createReviewCleanupRows(t, store, watch, "tracker-exhausted")
	svc.SetTaskDeleter(&recordingTaskDeleter{})

	deleted, err := svc.CleanupMergedReviewTasks(context.Background(), watch)
	assertCleanupRateLimitError(t, err)
	if deleted != 0 || len(client.prCalls) != 0 {
		t.Fatalf("deleted=%d GetPR calls=%v, want 0 and no provider call", deleted, client.prCalls)
	}
}

func TestCleanupBatchAdmissionReopensAfterRateLimitReset(t *testing.T) {
	tracker := NewRateTracker(nil, nil)
	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 0, Limit: 5000,
		ResetAt: time.Now().Add(time.Hour), UpdatedAt: time.Now().UTC(),
	})
	tracker.mu.Lock()
	snapshot := tracker.snapshots[ResourceCore]
	snapshot.ResetAt = time.Now().Add(-time.Second)
	tracker.snapshots[ResourceCore] = snapshot
	tracker.mu.Unlock()

	if err := cleanupBatchAdmission(context.Background(), NewMockClient(), tracker, CleanupPolicyAlways); err != nil {
		t.Fatalf("cleanup admission after reset = %v, want nil", err)
	}
}

func TestCleanupBatchStopsForGHCLIWrappedGraphQLRateLimit(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	client := &cleanupGHCLIClient{
		GHClient: NewGHClient(),
		err:      fmt.Errorf("get PR #1: gh pr: exit status 1: GraphQL: API rate limit already exceeded for user"),
	}
	tracker := NewRateTracker(nil, nil)
	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 0, Limit: 5000,
		ResetAt: time.Now().Add(time.Hour), UpdatedAt: time.Now().UTC(),
	})
	configureCleanupRateLimitClient(t, svc, client, tracker)
	watch := &ReviewWatch{WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	createReviewCleanupRows(t, store, watch, "cli-rate", "cli-after")
	svc.SetTaskDeleter(&recordingTaskDeleter{})

	deleted, err := svc.CleanupMergedReviewTasks(context.Background(), watch)
	if err == nil || !cleanupBatchShouldStop(err) {
		t.Fatalf("cleanup error = %v, want a rate-limit stop", err)
	}
	if deleted != 0 || !reflect.DeepEqual(client.calls, []int{1}) {
		t.Fatalf("deleted=%d GetPR calls=%v, want 0 and [1]", deleted, client.calls)
	}

	// Core exhaustion belongs to an unrelated REST bucket and must not block
	// a GH CLI GraphQL cleanup before its provider call.
	client.err = nil
	tracker.Record(RateSnapshot{
		Resource: ResourceGraphQL, Remaining: 0, Limit: 5000,
		ResetAt: time.Now().Add(time.Hour), UpdatedAt: time.Now().UTC(),
	})
	deleted, err = svc.CleanupMergedReviewTasks(context.Background(), watch)
	assertCleanupRateLimitError(t, err)
	if deleted != 0 || !reflect.DeepEqual(client.calls, []int{1}) {
		t.Fatalf("graphql admission deleted=%d GetPR calls=%v, want 0 and [1]", deleted, client.calls)
	}
}
