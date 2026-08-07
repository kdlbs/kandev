package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// stubGraphQLExecutor lets tests inspect/return canned responses without
// touching HTTP or the gh CLI.
type stubGraphQLExecutor struct {
	queries   []string
	response  string
	responses []string
	err       error
}

func (s *stubGraphQLExecutor) ExecuteGraphQL(_ context.Context, query string, _ map[string]any, out any) error {
	s.queries = append(s.queries, query)
	if s.err != nil {
		return s.err
	}
	response := s.response
	if len(s.responses) > 0 {
		response = s.responses[0]
		s.responses = s.responses[1:]
	}
	return json.Unmarshal([]byte(response), out)
}

func resolvedReviewThreadNodes(count int) []map[string]bool {
	nodes := make([]map[string]bool, count)
	for i := range nodes {
		nodes[i] = map[string]bool{"isResolved": true}
	}
	return nodes
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal GraphQL fixture: %v", err)
	}
	return string(data)
}

func reviewThreadsFixture(
	totalCount int,
	nodes []map[string]bool,
	hasNextPage bool,
	endCursor string,
) map[string]any {
	return map[string]any{
		"totalCount": totalCount,
		"nodes":      nodes,
		"pageInfo":   map[string]any{"hasNextPage": hasNextPage, "endCursor": endCursor},
	}
}

func batchedPRFixture(
	title string,
	url string,
	totalCount int,
	nodes []map[string]bool,
	hasNextPage bool,
	endCursor string,
) map[string]any {
	return map[string]any{
		"state": "OPEN", "title": title, "url": url,
		"headRefName": "feat", "baseRefName": "main", "headRefOid": "abc",
		"author":    map[string]any{"login": "alice"},
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
		"reviews":        map[string]any{"nodes": []any{}},
		"reviewRequests": map[string]any{"totalCount": 0},
		"reviewThreads":  reviewThreadsFixture(totalCount, nodes, hasNextPage, endCursor),
		"commits":        map[string]any{"nodes": []any{}},
	}
}

func batchedPRReviewThreadsResponse(
	t *testing.T,
	totalCount int,
	nodes []map[string]bool,
	hasNextPage bool,
	endCursor string,
) string {
	t.Helper()
	return mustJSON(t, map[string]any{
		"data": map[string]any{
			"repo0": map[string]any{
				"pr0": batchedPRFixture("Busy PR", "https://x/42", totalCount, nodes, hasNextPage, endCursor),
			},
		},
	})
}

func reviewThreadPageResponse(
	t *testing.T,
	nodes []map[string]bool,
	hasNextPage bool,
	endCursor string,
) string {
	t.Helper()
	return reviewThreadPagesResponse(t, reviewThreadsFixture(len(nodes), nodes, hasNextPage, endCursor))
}

func reviewThreadPagesResponse(t *testing.T, pages ...map[string]any) string {
	t.Helper()
	data := make(map[string]any, len(pages))
	for i, page := range pages {
		data[fmt.Sprintf("repo%d", i)] = map[string]any{
			"pr0": map[string]any{"reviewThreads": page},
		}
	}
	return mustJSON(t, map[string]any{
		"data": data,
	})
}

func batchedBranchReviewThreadsResponse(
	t *testing.T,
	totalCount int,
	nodes []map[string]bool,
	hasNextPage bool,
	endCursor string,
) string {
	t.Helper()
	node := batchedPRFixture("Busy branch PR", "https://x/7", totalCount, nodes, hasNextPage, endCursor)
	node["number"] = 7
	return mustJSON(t, map[string]any{
		"data": map[string]any{
			"b0": map[string]any{
				"pullRequests": map[string]any{
					"nodes": []any{node},
				},
			},
		},
	})
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
	if !strings.Contains(q, `pullRequests(first: 2, states: OPEN, headRefName: "feat-1")`) {
		t.Errorf("expected headRefName lookup for feat-1: %s", q)
	}
	if strings.Contains(q, `ref(qualifiedName:`) {
		t.Errorf("branch lookup should not require a base-repo ref: %s", q)
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

func TestRunBatchedPRQuery_PaginatesResolvedReviewThreads(t *testing.T) {
	exec := &stubGraphQLExecutor{responses: []string{
		batchedPRReviewThreadsResponse(t, 102, resolvedReviewThreadNodes(100), true, "cursor-1"),
		reviewThreadPageResponse(t, resolvedReviewThreadNodes(2), false, "cursor-2"),
	}}

	got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "r", Number: 42},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	status := got[prStatusCacheKey("o", "r", 42)]
	if status.UnresolvedReviewThreads != 0 {
		t.Fatalf("UnresolvedReviewThreads = %d, want 0", status.UnresolvedReviewThreads)
	}
}

func TestRunBatchedPRQuery_BatchesReviewThreadContinuations(t *testing.T) {
	exec := &stubGraphQLExecutor{responses: []string{
		mustJSON(t, map[string]any{
			"data": map[string]any{
				"repo0": map[string]any{
					"pr0": batchedPRFixture("PR A", "https://x/a/1", 101, resolvedReviewThreadNodes(100), true, "cursor-a"),
				},
				"repo1": map[string]any{
					"pr0": batchedPRFixture("PR B", "https://x/b/2", 101, resolvedReviewThreadNodes(100), true, "cursor-b"),
				},
			},
		}),
		reviewThreadPagesResponse(t,
			reviewThreadsFixture(1, []map[string]bool{{"isResolved": false}}, false, "cursor-a2"),
			reviewThreadsFixture(1, resolvedReviewThreadNodes(1), false, "cursor-b2"),
		),
	}}

	got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "a", Number: 1},
		{Owner: "o", Repo: "b", Number: 2},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(exec.queries) != 2 {
		t.Fatalf("GraphQL query count = %d, want 2", len(exec.queries))
	}
	if !strings.Contains(exec.queries[0], `pageInfo { hasNextPage endCursor }`) {
		t.Errorf("initial query does not request review-thread pageInfo: %s", exec.queries[0])
	}
	if !strings.Contains(exec.queries[1], `query ReviewThreadPages`) ||
		!strings.Contains(exec.queries[1], `after: "cursor-a"`) ||
		!strings.Contains(exec.queries[1], `after: "cursor-b"`) {
		t.Errorf("follow-up query does not batch both cursors: %s", exec.queries[1])
	}
	if got[prStatusCacheKey("o", "a", 1)].UnresolvedReviewThreads != 1 {
		t.Errorf("o/a#1 unresolved count = %d, want 1", got[prStatusCacheKey("o", "a", 1)].UnresolvedReviewThreads)
	}
	if got[prStatusCacheKey("o", "b", 2)].UnresolvedReviewThreads != 0 {
		t.Errorf("o/b#2 unresolved count = %d, want 0", got[prStatusCacheKey("o", "b", 2)].UnresolvedReviewThreads)
	}
}

func TestRunBatchedPRQuery_BoundsReviewThreadContinuationBatchSize(t *testing.T) {
	initialData := make(map[string]any, 6)
	refs := make([]graphQLPRRef, 6)
	pages := make([]map[string]any, 6)
	for i := range refs {
		repo := fmt.Sprintf("repo-%d", i)
		initialData[fmt.Sprintf("repo%d", i)] = map[string]any{
			"pr0": batchedPRFixture(
				fmt.Sprintf("PR %d", i), fmt.Sprintf("https://x/%d", i),
				101, resolvedReviewThreadNodes(100), true, fmt.Sprintf("cursor-%d", i),
			),
		}
		refs[i] = graphQLPRRef{Owner: "o", Repo: repo, Number: i + 1}
		pages[i] = reviewThreadsFixture(1, resolvedReviewThreadNodes(1), false, "done")
	}

	exec := &stubGraphQLExecutor{responses: []string{
		mustJSON(t, map[string]any{"data": initialData}),
		reviewThreadPagesResponse(t, pages...),
		reviewThreadPageResponse(t, resolvedReviewThreadNodes(1), false, "done"),
	}}

	if _, err := runBatchedPRQuery(context.Background(), exec, refs); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(exec.queries) != 3 {
		t.Fatalf("GraphQL query count = %d, want 3", len(exec.queries))
	}
}

func TestRunBatchedPRQuery_OneReviewThreadPageSkipsContinuation(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: batchedPRReviewThreadsResponse(
			t, 2, []map[string]bool{{"isResolved": true}, {"isResolved": false}}, false, "cursor-1",
		),
	}

	got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "r", Number: 42},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(exec.queries) != 1 {
		t.Fatalf("GraphQL query count = %d, want 1", len(exec.queries))
	}
	if got[prStatusCacheKey("o", "r", 42)].UnresolvedReviewThreads != 1 {
		t.Fatalf("UnresolvedReviewThreads = %d, want 1", got[prStatusCacheKey("o", "r", 42)].UnresolvedReviewThreads)
	}
}

func TestRunBatchedPRQuery_RejectsRepeatedReviewThreadCursor(t *testing.T) {
	exec := &stubGraphQLExecutor{responses: []string{
		batchedPRReviewThreadsResponse(t, 201, resolvedReviewThreadNodes(100), true, "cursor-1"),
		reviewThreadPageResponse(t, resolvedReviewThreadNodes(1), true, "cursor-1"),
		reviewThreadPageResponse(t, resolvedReviewThreadNodes(1), false, "cursor-2"),
	}}

	_, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "r", Number: 42},
	})
	if err == nil {
		t.Fatal("expected repeated review-thread cursor to fail")
	}
	if !strings.Contains(err.Error(), "repeated cursor") {
		t.Fatalf("error = %q, want repeated-cursor failure", err)
	}
}

func TestRunBatchedPRQuery_RejectsPagesBeyondInitialTotalCount(t *testing.T) {
	exec := &stubGraphQLExecutor{responses: []string{
		batchedPRReviewThreadsResponse(t, 101, resolvedReviewThreadNodes(100), true, "cursor-1"),
		reviewThreadPageResponse(t, resolvedReviewThreadNodes(1), true, "cursor-2"),
		reviewThreadPageResponse(t, resolvedReviewThreadNodes(1), false, "cursor-3"),
	}}

	_, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "r", Number: 42},
	})
	if err == nil {
		t.Fatal("expected review-thread pages beyond initial totalCount to fail")
	}
	if !strings.Contains(err.Error(), "page limit") {
		t.Fatalf("error = %q, want page-limit failure", err)
	}
}

func TestRunBatchedPRQuery_PreservesMissingReposOnReviewThreadPaginationFailure(t *testing.T) {
	exec := &stubGraphQLExecutor{responses: []string{
		mustJSON(t, map[string]any{
			"data": map[string]any{
				"repo1": map[string]any{
					"pr0": batchedPRFixture(
						"Busy PR", "https://x/live/42", 101,
						resolvedReviewThreadNodes(100), true, "cursor-1",
					),
				},
			},
			"errors": []map[string]any{{
				"message": "Could not resolve to a Repository with the name 'o/dead'.",
				"type":    "NOT_FOUND",
				"path":    []string{"repo0"},
			}},
		}),
		`{"data": {}}`,
	}}

	got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "dead", Number: 1},
		{Owner: "o", Repo: "live", Number: 42},
	})
	if got != nil {
		t.Fatalf("status map = %#v, want nil after incomplete pagination", got)
	}
	var missingErr *batchedMissingReposErr
	if !errors.As(err, &missingErr) {
		t.Fatalf("error = %v, want batchedMissingReposErr", err)
	}
	if len(missingErr.Repos) != 1 || missingErr.Repos[0] != (repoRef{Owner: "o", Repo: "dead"}) {
		t.Fatalf("missing repos = %#v, want o/dead", missingErr.Repos)
	}
	if missingErr.Inner == nil || !strings.Contains(missingErr.Inner.Error(), "missing repository alias") {
		t.Fatalf("inner error = %v, want pagination failure", missingErr.Inner)
	}
}

func TestRunBatchedPRQuery_RejectsEmptyReviewThreadCursor(t *testing.T) {
	tests := []struct {
		name      string
		responses []string
	}{
		{
			name: "initial page",
			responses: []string{
				batchedPRReviewThreadsResponse(t, 101, resolvedReviewThreadNodes(100), true, ""),
			},
		},
		{
			name: "continuation page",
			responses: []string{
				batchedPRReviewThreadsResponse(t, 201, resolvedReviewThreadNodes(100), true, "cursor-1"),
				reviewThreadPageResponse(t, resolvedReviewThreadNodes(100), true, ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &stubGraphQLExecutor{responses: tt.responses}
			got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
				{Owner: "o", Repo: "r", Number: 42},
			})
			if err == nil || got != nil {
				t.Fatalf("got (%v, %v), want (nil, error)", got, err)
			}
			if !strings.Contains(err.Error(), "empty cursor") {
				t.Fatalf("error = %q, want empty-cursor failure", err)
			}
		})
	}
}

func TestRunBatchedPRQuery_PaginatesMultipleReviewThreadRounds(t *testing.T) {
	secondPage := append(
		[]map[string]bool{{"isResolved": false}},
		resolvedReviewThreadNodes(99)...,
	)
	exec := &stubGraphQLExecutor{responses: []string{
		batchedPRReviewThreadsResponse(t, 202, resolvedReviewThreadNodes(100), true, "cursor-1"),
		reviewThreadPageResponse(t, secondPage, true, "cursor-2"),
		reviewThreadPageResponse(t, []map[string]bool{
			{"isResolved": false},
			{"isResolved": true},
		}, false, "cursor-3"),
	}}

	got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "r", Number: 42},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(exec.queries) != 3 {
		t.Fatalf("GraphQL query count = %d, want 3", len(exec.queries))
	}
	if got[prStatusCacheKey("o", "r", 42)].UnresolvedReviewThreads != 2 {
		t.Fatalf("UnresolvedReviewThreads = %d, want 2", got[prStatusCacheKey("o", "r", 42)].UnresolvedReviewThreads)
	}
}

func TestRunBatchedPRQuery_RejectsNullReviewThreadPage(t *testing.T) {
	exec := &stubGraphQLExecutor{responses: []string{
		batchedPRReviewThreadsResponse(t, 101, resolvedReviewThreadNodes(100), true, "cursor-1"),
		mustJSON(t, map[string]any{
			"data": map[string]any{
				"repo0": map[string]any{
					"pr0": map[string]any{"reviewThreads": nil},
				},
			},
		}),
	}}

	_, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "o", Repo: "r", Number: 42},
	})
	if err == nil {
		t.Fatal("expected null review-thread page to fail")
	}
}

func TestRunBatchedPRQuery_DecodesMultipleReposIntoCorrectKeys(t *testing.T) {
	// Two repos in one batch: check each aliased PR lands in its correct key.
	exec := &stubGraphQLExecutor{
		response: `{
			"data": {
				"repo0": {
					"pr0": {
						"state": "OPEN", "title": "alpha PR", "url": "https://x/a/1",
						"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
						"headRefName": "h1", "baseRefName": "main", "headRefOid": "aaa",
						"author": {"login":"alice"}, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
						"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
						"commits": {"nodes": []}
					}
				},
				"repo1": {
					"pr0": {
						"state": "OPEN", "title": "beta PR", "url": "https://x/b/9",
						"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
						"headRefName": "h2", "baseRefName": "main", "headRefOid": "bbb",
						"author": {"login":"alice"}, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
						"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
						"commits": {"nodes": []}
					}
				}
			}
		}`,
	}
	got, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{
		{Owner: "octo", Repo: "alpha", Number: 1},
		{Owner: "octo", Repo: "beta", Number: 9},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if s, ok := got[prStatusCacheKey("octo", "alpha", 1)]; !ok || s.PR == nil || s.PR.Title != "alpha PR" {
		t.Errorf("alpha#1 not decoded into correct slot: %#v", s)
	}
	if s, ok := got[prStatusCacheKey("octo", "beta", 9)]; !ok || s.PR == nil || s.PR.Title != "beta PR" {
		t.Errorf("beta#9 not decoded into correct slot: %#v", s)
	}
}

func TestRunBatchedPRQuery_PropagatesError(t *testing.T) {
	exec := &stubGraphQLExecutor{err: errors.New("graphql 500")}
	_, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{{Owner: "o", Repo: "r", Number: 1}})
	if err == nil {
		t.Fatalf("expected error to propagate")
	}
}

func TestRunBatchedPRQuery_SurfacesGraphQLErrors(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{"data": {}, "errors": [{"message": "Field 'foo' doesn't exist on type 'Repository'"}]}`,
	}
	_, err := runBatchedPRQuery(context.Background(), exec, []graphQLPRRef{{Owner: "o", Repo: "r", Number: 1}})
	if err == nil {
		t.Fatalf("expected error from non-empty errors array")
	}
	if !strings.Contains(err.Error(), "Field 'foo'") {
		t.Errorf("expected error to include first message, got: %v", err)
	}
}

func TestRunBatchedBranchQuery_SurfacesGraphQLErrors(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{"data": {}, "errors": [{"message": "rate limited"}, {"message": "secondary"}]}`,
	}
	_, err := runBatchedBranchQuery(context.Background(), exec, []graphQLBranchRef{{Owner: "o", Repo: "r", Branch: "main"}})
	if err == nil {
		t.Fatalf("expected error from non-empty errors array")
	}
	if !strings.Contains(err.Error(), "rate limited") || !strings.Contains(err.Error(), "and 1 more") {
		t.Errorf("expected error to include first message + count, got: %v", err)
	}
}

func TestRunBatchedBranchQuery_SurfacesDecodeErrors(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{"data": {"b0": {"pullRequests": {"nodes": [{"number": "bad"}]}}}}`,
	}
	_, err := runBatchedBranchQuery(context.Background(), exec, []graphQLBranchRef{{Owner: "o", Repo: "r", Branch: "feat"}})
	if err == nil {
		t.Fatalf("expected malformed branch node to return an error")
	}
	if !strings.Contains(err.Error(), "decode branch alias b0") {
		t.Errorf("expected error to identify branch alias, got: %v", err)
	}
}

func TestRunBatchedBranchQuery_DecodesPRNode(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{
			"data": {
				"b0": {
					"pullRequests": {
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

func TestRunBatchedBranchQuery_SkipsUnusedReviewThreadPagination(t *testing.T) {
	exec := &stubGraphQLExecutor{response: batchedBranchReviewThreadsResponse(t, 101, resolvedReviewThreadNodes(100), true, "cursor-1")}

	got, err := runBatchedBranchQuery(context.Background(), exec, []graphQLBranchRef{
		{Owner: "o", Repo: "r", Branch: "feat"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	status := got[graphqlBranchKey("o", "r", "feat")]
	if status == nil || status.PR == nil || status.PR.Number != 7 {
		t.Fatalf("branch status = %#v, want discovered PR #7", status)
	}
	if len(exec.queries) != 1 {
		t.Fatalf("GraphQL query count = %d, want 1", len(exec.queries))
	}
}

func TestRunBatchedBranchQuery_EmptyNodesReturnsNoResult(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{
			"data": {
				"b0": {
					"pullRequests": {
						"nodes": []
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
	if len(got) != 0 {
		t.Fatalf("expected no branch result, got %#v", got)
	}
}

func TestRunBatchedBranchQuery_SkipsAmbiguousForkHeads(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{
			"data": {
				"b0": {
					"pullRequests": {
						"nodes": [{
							"number": 7,
							"state": "OPEN", "title": "branch PR A", "url": "https://x/7",
							"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
							"headRefName": "feat", "baseRefName": "main", "headRefOid": "deadbeef",
							"author": {"login":"alice"},
							"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
							"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
							"commits": {"nodes": []}
						}, {
							"number": 8,
							"state": "OPEN", "title": "branch PR B", "url": "https://x/8",
							"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
							"headRefName": "feat", "baseRefName": "main", "headRefOid": "cafebabe",
							"author": {"login":"bob"},
							"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
							"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
							"commits": {"nodes": []}
						}]
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
	if len(got) != 0 {
		t.Fatalf("expected ambiguous fork heads to be skipped, got %#v", got)
	}
}

func TestRunBatchedBranchQuery_SkipsMultipleBranchMatchesEvenWithOwnerMatch(t *testing.T) {
	exec := &stubGraphQLExecutor{
		response: `{
			"data": {
				"b0": {
					"pullRequests": {
						"nodes": [{
							"number": 7,
							"state": "OPEN", "title": "fork PR", "url": "https://x/7",
							"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
							"headRefName": "feat", "baseRefName": "main", "headRefOid": "deadbeef",
							"author": {"login":"alice"},
							"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
							"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
							"commits": {"nodes": []}
						}, {
							"number": 9,
							"state": "OPEN", "title": "base PR", "url": "https://x/9",
							"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
							"headRefName": "feat", "baseRefName": "main", "headRefOid": "cafebabe",
							"author": {"login":"o"},
							"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
							"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
							"commits": {"nodes": []}
						}]
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
	if len(got) != 0 {
		t.Fatalf("expected duplicate branch matches to be skipped, got %#v", got)
	}
}

func TestSummarizeReviewState_PrefersChangesRequested(t *testing.T) {
	mk := func(login, state string, sec int) reviewNode {
		var n reviewNode
		n.Author.Login = login
		n.State = state
		n.SubmittedAt = time.Unix(int64(sec), 0).UTC()
		return n
	}
	// Different reviewers: CHANGES_REQUESTED beats APPROVED.
	if got := summarizeReviewState([]reviewNode{
		mk("alice", "APPROVED", 1),
		mk("bob", "CHANGES_REQUESTED", 2),
	}); got != "changes_requested" {
		t.Errorf("got %q", got)
	}
	// Different reviewers: APPROVED + COMMENTED -> approved.
	if got := summarizeReviewState([]reviewNode{
		mk("alice", "APPROVED", 1),
		mk("bob", "COMMENTED", 2),
	}); got != "approved" {
		t.Errorf("got %q", got)
	}
	// Single COMMENTED -> empty.
	if got := summarizeReviewState([]reviewNode{
		mk("alice", "COMMENTED", 1),
	}); got != "" {
		t.Errorf("got %q", got)
	}
	// Same reviewer: CHANGES_REQUESTED then APPROVED -> approved (Greptile P1).
	if got := summarizeReviewState([]reviewNode{
		mk("alice", "CHANGES_REQUESTED", 1),
		mk("alice", "APPROVED", 2),
	}); got != "approved" {
		t.Errorf("dedup: got %q, want approved", got)
	}
	// Same reviewer: APPROVED then CHANGES_REQUESTED -> changes_requested.
	if got := summarizeReviewState([]reviewNode{
		mk("alice", "APPROVED", 1),
		mk("alice", "CHANGES_REQUESTED", 2),
	}); got != "changes_requested" {
		t.Errorf("dedup reverse: got %q, want changes_requested", got)
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
