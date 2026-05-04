package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// stubGraphQLExecutor lets tests inspect/return canned responses without
// touching HTTP or the gh CLI.
type stubGraphQLExecutor struct {
	queries  []string
	response string
	err      error
}

func (s *stubGraphQLExecutor) ExecuteGraphQL(_ context.Context, query string, _ map[string]any, out any) error {
	s.queries = append(s.queries, query)
	if s.err != nil {
		return s.err
	}
	return json.Unmarshal([]byte(s.response), out)
}

func TestBuildBatchedPRQuery_GroupsByRepo(t *testing.T) {
	q, _ := buildBatchedPRQuery([]graphQLPRRef{
		{Owner: "octo", Repo: "alpha", Number: 1},
		{Owner: "octo", Repo: "alpha", Number: 2},
		{Owner: "octo", Repo: "beta", Number: 9},
	})
	if !strings.Contains(q, `repo0: repository(owner: "octo", name: "alpha")`) {
		t.Errorf("expected repo0 alias for octo/alpha, got: %s", q)
	}
	if !strings.Contains(q, `repo1: repository(owner: "octo", name: "beta")`) {
		t.Errorf("expected repo1 alias for octo/beta, got: %s", q)
	}
	if !strings.Contains(q, `pr0: pullRequest(number: 1)`) ||
		!strings.Contains(q, `pr1: pullRequest(number: 2)`) {
		t.Errorf("expected aliased pullRequests inside repo0: %s", q)
	}
	if !strings.Contains(q, "rateLimit") {
		t.Errorf("expected rateLimit field in query")
	}
}

func TestBuildBatchedBranchQuery_AliasesAllBranches(t *testing.T) {
	q, _ := buildBatchedBranchQuery([]graphQLBranchRef{
		{Owner: "o", Repo: "r", Branch: "feat-1"},
		{Owner: "o", Repo: "r", Branch: "feat-2"},
	})
	if !strings.Contains(q, `b0: repository`) || !strings.Contains(q, `b1: repository`) {
		t.Errorf("expected b0/b1 aliases: %s", q)
	}
	if !strings.Contains(q, `qualifiedName: "refs/heads/feat-1"`) {
		t.Errorf("expected qualifiedName for feat-1: %s", q)
	}
}

func TestRunBatchedPRQuery_DecodesAliasesBackToRefs(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{
			"data": {
				"repo0": {
					"pr0": {
						"state": "OPEN", "title": "PR A", "url": "https://x/1",
						"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
						"headRefName": "h1", "baseRefName": "main", "headRefOid": "abc",
						"author": {"login":"alice"}, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
						"reviews": {"nodes": [{"state": "APPROVED"}]},
						"reviewRequests": {"totalCount": 0},
						"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}
					}
				},
				"rateLimit": {"limit":5000, "remaining":4999, "resetAt":"2030-01-01T00:00:00Z", "cost":1}
			}
		}`,
	}
	got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "r", Number: 42},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	status, ok := got[prStatusCacheKey("o", "r", 42)]
	if !ok {
		t.Fatalf("expected status for o/r#42, got keys: %v", keysOf(got))
	}
	if status.ReviewState != "approved" {
		t.Errorf("ReviewState = %q, want approved", status.ReviewState)
	}
	if status.ChecksState != "success" {
		t.Errorf("ChecksState = %q, want success", status.ChecksState)
	}
	if status.PR == nil || status.PR.Title != "PR A" {
		t.Errorf("PR.Title mismatch: %#v", status.PR)
	}
}

func TestRunBatchedPRQuery_PropagatesError(t *testing.T) {
	exec := &stubGraphQLExecutor{err: errors.New("graphql 500")}
	_, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{{Owner: "o", Repo: "r", Number: 1}})
	if err == nil {
		t.Fatalf("expected error to propagate")
	}
}

func TestRunBatchedBranchQuery_DecodesPRNode(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{
			"data": {
				"b0": {
					"ref": {
						"associatedPullRequests": {
							"nodes": [{
								"number": 7,
								"state": "OPEN", "title": "branch PR", "url": "https://x/7",
								"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
								"headRefName": "feat", "baseRefName": "main", "headRefOid": "deadbeef",
								"author": {"login":"alice"},
								"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
								"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
								"commits": {"nodes": []}
							}]
						}
					}
				}
			}
		}`,
	}
	got, err := runBatchedBranchQuery(context.Background(), exec, []graphQLBranchRef{
		{Owner: "o", Repo: "r", Branch: "feat"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	status, ok := got[graphqlBranchKey("o", "r", "feat")]
	if !ok {
		t.Fatalf("expected branch result, got keys: %v", keysOf(got))
	}
	if status.PR == nil || status.PR.Number != 7 {
		t.Errorf("expected PR number 7, got %#v", status.PR)
	}
}

func TestSummarizeReviewState_PrefersChangesRequested(t *testing.T) {
	type node = struct {
		State string `json:"state"`
	}
	if got := summarizeReviewState([]node{{State: "APPROVED"}, {State: "CHANGES_REQUESTED"}}); got != "changes_requested" {
		t.Errorf("got %q", got)
	}
	if got := summarizeReviewState([]node{{State: "APPROVED"}, {State: "COMMENTED"}}); got != "approved" {
		t.Errorf("got %q", got)
	}
	if got := summarizeReviewState([]node{{State: "COMMENTED"}}); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestChunkedRefs_RespectsBatchSize(t *testing.T) {
	refs := make([]graphQLPRRef, graphQLBatchChunkSize+5)
	chunks := chunkedRefs(refs)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks for %d refs, got %d", len(refs), len(chunks))
	}
	if len(chunks[0]) != graphQLBatchChunkSize {
		t.Errorf("first chunk size = %d, want %d", len(chunks[0]), graphQLBatchChunkSize)
	}
	if len(chunks[1]) != 5 {
		t.Errorf("second chunk size = %d, want 5", len(chunks[1]))
	}
}

func keysOf[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestGraphQLExecutorFor_NoopReturnsError(t *testing.T) {
	if _, err := graphQLExecutorFor(&NoopClient{}); err == nil {
		t.Fatalf("expected unsupported error for NoopClient")
	}
	if _, err := graphQLExecutorFor(NewPATClient("token")); err != nil {
		t.Fatalf("PATClient should satisfy GraphQLExecutor: %v", err)
	}
	if _, err := graphQLExecutorFor(NewGHClient()); err != nil {
		t.Fatalf("GHClient should satisfy GraphQLExecutor: %v", err)
	}
}
