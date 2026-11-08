package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// liveReadClient adds canned GraphQL and REST answers to the mock client.
type liveReadClient struct {
	*MockClient
	graphQL map[string]string // query substring -> response
	rest    map[string]string // endpoint -> JSON body
	restErr map[string]int    // endpoint -> HTTP status
}

func (c *liveReadClient) ExecuteGraphQL(_ context.Context, query string, _ map[string]any, out any) error {
	for needle, resp := range c.graphQL {
		if strings.Contains(query, needle) {
			return json.Unmarshal([]byte(resp), out)
		}
	}
	return errors.New("no canned GraphQL response")
}

func (c *liveReadClient) GetRESTJSON(_ context.Context, endpoint string) (json.RawMessage, error) {
	if status, ok := c.restErr[endpoint]; ok {
		return nil, &GitHubAPIError{StatusCode: status, Endpoint: endpoint, Body: "denied"}
	}
	if body, ok := c.rest[endpoint]; ok {
		return json.RawMessage(body), nil
	}
	return nil, &GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: endpoint}
}

func newLiveReadService(t *testing.T, client *liveReadClient) *Service {
	t.Helper()
	_, svc, _, _ := setupPollerTest(t)
	svc.client = client
	configureTestWorkspaceAuth(t, svc, client, testWorkspaceID)
	return svc
}

const livePRResponse = `{"data":{"repo0":{"pr0":{
	"state":"OPEN","title":"T","url":"https://x/7","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN",
	"headRefName":"feat","baseRefName":"main","headRefOid":"head1","author":{"login":"a"},
	"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z",
	"reviews":{"nodes":[{"state":"APPROVED","author":{"login":"b"},"submittedAt":"2026-01-02T00:00:00Z"}]},
	"reviewRequests":{"totalCount":0},
	"reviewThreads":{"nodes":[{"isResolved":false},{"isResolved":true}],"pageInfo":{"hasNextPage":false}},
	"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}}}}}`

func TestReadPRLiveForAutomationUsesGraphQLAndHostBinding(t *testing.T) {
	client := &liveReadClient{MockClient: NewMockClient(), graphQL: map[string]string{"Batch": livePRResponse}}
	svc := newLiveReadService(t, client)
	read, err := svc.ReadPRLiveForAutomation(context.Background(), testWorkspaceID, "github.com", "o", "r", 7)
	if err != nil {
		t.Fatal(err)
	}
	st := read.Status
	if st.PR.HeadSHA != "head1" || st.UnresolvedReviewThreads != 1 || !st.UnresolvedReviewThreadsPopulated ||
		st.ReviewState != "approved" || st.ChecksState != "success" || st.PR.State != "open" {
		t.Errorf("status = %+v pr = %+v", st, st.PR)
	}
	if read.HasMergeConflicts == nil || *read.HasMergeConflicts {
		t.Errorf("HasMergeConflicts = %v, want explicit false", read.HasMergeConflicts)
	}
	if _, err := svc.ReadPRLiveForAutomation(context.Background(), testWorkspaceID, "ghe.example.com", "o", "r", 7); !errors.Is(err, ErrGitHubHostMismatch) {
		t.Errorf("host mismatch error = %v", err)
	}
}

func TestReadPRLiveForAutomationFailureIsError(t *testing.T) {
	client := &liveReadClient{MockClient: NewMockClient(), graphQL: map[string]string{}}
	svc := newLiveReadService(t, client)
	if _, err := svc.ReadPRLiveForAutomation(context.Background(), testWorkspaceID, "", "o", "r", 7); err == nil {
		t.Fatal("a failed provider read must be an error, never a snapshot")
	}
}

func TestMergePRForAutomationOutcome(t *testing.T) {
	for _, tc := range []struct {
		name       string
		outcome    MergeOutcome
		headChange bool
		wantErr    error
		wantOut    MergeOutcome
		wantCommit string
	}{
		{name: "merged", wantOut: MergeOutcomeMerged, wantCommit: "mc1"},
		{name: "queued", outcome: MergeOutcomeQueued, wantOut: MergeOutcomeQueued},
		{name: "head moved", headChange: true, wantErr: ErrMergeHeadMoved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := NewMockClient()
			mock.AddPR(&PR{Number: 7, RepoOwner: "o", RepoName: "r", HeadSHA: "head1", State: "open"})
			if tc.outcome != "" {
				mock.SetMergeOutcome("o", "r", 7, tc.outcome)
			}
			client := &liveReadClient{MockClient: mock, graphQL: map[string]string{
				"mergeCommit": `{"data":{"repository":{"pullRequest":{"mergeCommit":{"oid":"mc1"}}}}}`,
			}}
			svc := newLiveReadService(t, client)
			sha := "head1"
			if tc.headChange {
				sha = "stale"
			}
			res, err := svc.MergePRForAutomationOutcome(context.Background(), testWorkspaceID, "github.com", "o", "r", 7, "squash", sha)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || res.Outcome != tc.wantOut || res.MergeCommit != tc.wantCommit {
				t.Fatalf("result = %+v, err = %v", res, err)
			}
			if got := mock.MergedPRs(); len(got) != 1 || got[0].ExpectedHeadSHA != "head1" || got[0].MergeMethod != "squash" {
				t.Errorf("merge request = %+v", got)
			}
		})
	}
}

func TestMergePRForAutomationOutcomeRequiresExpectedHead(t *testing.T) {
	if _, err := (&Service{}).MergePRForAutomationOutcome(context.Background(), "w", "", "o", "r", 1, "squash", " "); err == nil {
		t.Fatal("expected head SHA is required")
	}
}

func TestClassifyMergeErrorHeadMoved(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"409 head modified", &GitHubAPIError{StatusCode: 409, Body: `{"message":"Head branch was modified. Review the latest changes and try again."}`}, true},
		{"422 head modified", &GitHubAPIError{StatusCode: 422, Body: `Head branch was modified`}, true},
		{"405 not mergeable", &GitHubAPIError{StatusCode: 405, Body: `Pull Request is not mergeable`}, false},
		{"409 other", &GitHubAPIError{StatusCode: 409, Body: `Merge conflict`}, false},
		{"async expected head", newMergeFailureError(mergeAsyncResponseDetails{ExpectedHeadSHA: "abc", Message: "no"}), true},
		{"async head modified", newMergeFailureError(mergeAsyncResponseDetails{Message: "Head branch was modified"}), true},
		{"async other", newMergeFailureError(mergeAsyncResponseDetails{Message: "checks failing"}), false},
		{"transport", errors.New("EOF"), false},
	} {
		if got := errors.Is(classifyMergeError(tc.err), ErrMergeHeadMoved); got != tc.want {
			t.Errorf("%s: head moved = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestReadBranchProtectionForAutomation(t *testing.T) {
	repo := `{"default_branch":"main","permissions":{"admin":true}}`
	classicFull := `{"required_status_checks":{"strict":true},"required_pull_request_reviews":{"required_approving_review_count":2},"required_conversation_resolution":{"enabled":true}}`
	rules := `[{"type":"pull_request","parameters":{"required_approving_review_count":0,"required_review_thread_resolution":true}},{"type":"required_status_checks"}]`
	for _, tc := range []struct {
		name    string
		branch  string
		rest    map[string]string
		restErr map[string]int
		want    BranchProtectionRead
	}{
		{"classic and rules", "", map[string]string{
			"/repos/o/r": repo, "/repos/o/r/branches/main/protection": classicFull, "/repos/o/r/rules/branches/main": rules,
		}, nil, BranchProtectionRead{Classic: ClassicProtectionFound, AdminConfirmed: true,
			ClassicRequire: ProtectionRequirements{Pipeline: true, Approvals: true, Threads: true},
			RulesetsOK:     true, RulesetsRequire: ProtectionRequirements{Pipeline: true, Threads: true}}},
		{"classic absent, empty rules", "dev/x", map[string]string{
			"/repos/o/r": repo, "/repos/o/r/rules/branches/dev%2Fx": `[]`,
		}, nil, BranchProtectionRead{Classic: ClassicProtectionAbsent, AdminConfirmed: true, RulesetsOK: true}},
		{"classic forbidden, rules failed", "main", map[string]string{"/repos/o/r": repo},
			map[string]int{"/repos/o/r/branches/main/protection": 403, "/repos/o/r/rules/branches/main": 500},
			BranchProtectionRead{Classic: ClassicProtectionFailed, AdminConfirmed: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &liveReadClient{MockClient: NewMockClient(), rest: tc.rest, restErr: tc.restErr}
			svc := newLiveReadService(t, client)
			got, err := svc.ReadBranchProtectionForAutomation(context.Background(), testWorkspaceID, "", "o", "r", tc.branch)
			if err != nil || got != tc.want {
				t.Fatalf("got %+v err %v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestReadBranchProtectionRepositoryReadFailureIsError(t *testing.T) {
	client := &liveReadClient{MockClient: NewMockClient(), restErr: map[string]int{"/repos/o/r": 500}}
	svc := newLiveReadService(t, client)
	if _, err := svc.ReadBranchProtectionForAutomation(context.Background(), testWorkspaceID, "", "o", "r", "main"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListBranchCommitsForAutomation(t *testing.T) {
	full := make([]string, 100)
	for i := range full {
		full[i] = `{"sha":"s","commit":{"message":"m"}}`
	}
	client := &liveReadClient{MockClient: NewMockClient(), rest: map[string]string{
		"/repos/o/r/commits?sha=main&per_page=100&page=1": `[` + strings.Join(full, ",") + `]`,
		"/repos/o/r/commits?sha=main&per_page=100&page=2": `[{"sha":"c2","commit":{"message":"This reverts commit x"}}]`,
	}}
	svc := newLiveReadService(t, client)
	commits, more, err := svc.ListBranchCommitsForAutomation(context.Background(), testWorkspaceID, "", "o", "r", "main", 1)
	if err != nil || len(commits) != 100 || !more {
		t.Fatalf("page 1: n=%d more=%v err=%v", len(commits), more, err)
	}
	commits, more, err = svc.ListBranchCommitsForAutomation(context.Background(), testWorkspaceID, "", "o", "r", "main", 2)
	if err != nil || len(commits) != 1 || more || commits[0].Message != "This reverts commit x" {
		t.Fatalf("page 2: %+v more=%v err=%v", commits, more, err)
	}
}

func TestClassifyMergeErrorRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"405", &GitHubAPIError{StatusCode: 405, Body: "Pull Request is not mergeable"}, true},
		{"409 conflict", &GitHubAPIError{StatusCode: 409, Body: "Merge conflict"}, true},
		{"422", &GitHubAPIError{StatusCode: 422, Body: "Validation failed"}, true},
		{"async failed", newMergeFailureError(mergeAsyncResponseDetails{Message: "checks failing"}), true},
		{"head moved is not a plain rejection", &GitHubAPIError{StatusCode: 409, Body: "Head branch was modified"}, false},
		{"rate limited 403", &GitHubAPIError{StatusCode: 403, Body: "secondary rate limit"}, false},
		{"server error", &GitHubAPIError{StatusCode: 502, Body: "bad gateway"}, false},
		{"transport", errors.New("EOF"), false},
	} {
		if got := errors.Is(classifyMergeError(tc.err), ErrMergeRejected); got != tc.want {
			t.Errorf("%s: rejected = %v, want %v", tc.name, got, tc.want)
		}
	}
}
