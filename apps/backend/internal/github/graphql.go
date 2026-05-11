package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// GraphQLExecutor runs a single GraphQL query against the GitHub API. Both
// PATClient (direct HTTP) and GHClient (`gh api graphql`) implement it so the
// batched poller works regardless of auth method.
type GraphQLExecutor interface {
	ExecuteGraphQL(ctx context.Context, query string, variables map[string]any, out any) error
}

// graphQLPRBatchAlias is the per-PR alias prefix; the index makes aliases
// unique within a single repository block.
const graphQLPRBatchAlias = "pr"

// graphQLBatchChunkSize bounds the number of aliased fields per request to
// stay well under GitHub's per-query node-count limit (500). 50 PRs × ~10
// fields = ~500 nodes, leaving headroom.
const graphQLBatchChunkSize = 50

// reviewNode is one PR review entry from the batched GraphQL query.
type reviewNode struct {
	State  string `json:"state"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	SubmittedAt time.Time `json:"submittedAt"`
}

// graphQLPRRef is one entry in a batched PR-status request.
type graphQLPRRef struct {
	Owner  string
	Repo   string
	Number int
}

// graphQLBranchRef is one entry in a batched branch-lookup request.
type graphQLBranchRef struct {
	Owner  string
	Repo   string
	Branch string
}

// chunkedRefs splits refs into chunks of at most graphQLBatchChunkSize so
// callers can keep individual GraphQL queries under the node-count limit.
func chunkedRefs[T any](refs []T) [][]T {
	if len(refs) == 0 {
		return nil
	}
	out := make([][]T, 0, (len(refs)+graphQLBatchChunkSize-1)/graphQLBatchChunkSize)
	for i := 0; i < len(refs); i += graphQLBatchChunkSize {
		end := i + graphQLBatchChunkSize
		if end > len(refs) {
			end = len(refs)
		}
		out = append(out, refs[i:end])
	}
	return out
}

// batchedPRResult is the decoded shape of one aliased pullRequest block.
type batchedPRResult struct {
	State       string `json:"state"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	IsDraft     bool   `json:"isDraft"`
	Mergeable   string `json:"mergeable"`
	MergeStatus string `json:"mergeStateStatus"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	HeadRefOid  string `json:"headRefOid"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	MergedAt  string    `json:"mergedAt"`
	ClosedAt  string    `json:"closedAt"`
	Reviews   struct {
		Nodes []reviewNode `json:"nodes"`
	} `json:"reviews"`
	ReviewRequests struct {
		TotalCount int `json:"totalCount"`
	} `json:"reviewRequests"`
	ReviewThreads struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			IsResolved bool `json:"isResolved"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// graphQLError mirrors a single entry in a GraphQL response's "errors" array.
type graphQLError struct {
	Message string   `json:"message"`
	Type    string   `json:"type"`
	Path    []string `json:"path"`
}

// graphQLErrorsToErr returns a non-nil error when the GraphQL response carries
// a top-level "errors" array. The first message is included so logs are
// actionable; the count is appended when there are multiple.
func graphQLErrorsToErr(errs []graphQLError) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return fmt.Errorf("graphql error: %s", errs[0].Message)
	}
	return fmt.Errorf("graphql error: %s (and %d more)", errs[0].Message, len(errs)-1)
}

// graphQLRateLimit mirrors GitHub's top-level rateLimit field, the most
// accurate source of GraphQL quota since limits are point-cost based.
type graphQLRateLimit struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"resetAt"`
	Cost      int       `json:"cost"`
}

// buildBatchedPRQuery groups refs by (owner, repo) and emits one aliased
// repository block per group, with one aliased pullRequest per number inside.
// The shape mirrors gh pr view fields used by the existing converter so we
// can reuse the conversion logic without reshaping callers.
func buildBatchedPRQuery(refs []graphQLPRRef) (string, map[string]any) {
	// Group refs by (owner, repo) and assign deterministic indices so aliases
	// stay stable across runs (helpful for tests and snapshot debugging).
	type group struct {
		owner   string
		repo    string
		numbers []int
	}
	byKey := map[string]*group{}
	keys := []string{}
	for _, r := range refs {
		key := r.Owner + "/" + r.Repo
		g, ok := byKey[key]
		if !ok {
			g = &group{owner: r.Owner, repo: r.Repo}
			byKey[key] = g
			keys = append(keys, key)
		}
		g.numbers = append(g.numbers, r.Number)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("query Batch { ")
	for repoIdx, key := range keys {
		g := byKey[key]
		fmt.Fprintf(&b, `repo%d: repository(owner: %q, name: %q) { `, repoIdx, g.owner, g.repo)
		for prIdx, n := range g.numbers {
			fmt.Fprintf(&b, `%s%d: pullRequest(number: %d) { %s } `, graphQLPRBatchAlias, prIdx, n, prFieldsBlock())
		}
		b.WriteString(`} `)
	}
	b.WriteString(`rateLimit { limit remaining resetAt cost } `)
	b.WriteString(`}`)
	return b.String(), nil
}

// prFieldsBlock is the GraphQL field selection used in every batched PR
// query. Kept as a constant to make snapshot assertions stable and to keep
// the batched and single-PR paths returning the same data.
func prFieldsBlock() string {
	return `state title url isDraft mergeable mergeStateStatus ` +
		`headRefName baseRefName headRefOid additions deletions ` +
		`author { login } createdAt updatedAt mergedAt closedAt ` +
		`reviews(last: 100) { nodes { state author { login } submittedAt } } ` +
		`reviewRequests(first: 0) { totalCount } ` +
		`reviewThreads(first: 100) { totalCount nodes { isResolved } } ` +
		`commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }`
}

// buildBatchedBranchQuery emits one aliased associatedPullRequests block per
// (owner, repo, branch) ref. Used to batch the "find PR by branch" path.
func buildBatchedBranchQuery(refs []graphQLBranchRef) (string, map[string]any) {
	var b strings.Builder
	b.WriteString("query Branches { ")
	for i, r := range refs {
		fmt.Fprintf(&b, `b%d: repository(owner: %q, name: %q) { ref(qualifiedName: %q) { associatedPullRequests(first: 1, states: OPEN) { nodes { number %s } } } } `,
			i, r.Owner, r.Repo, "refs/heads/"+r.Branch, prFieldsBlock())
	}
	b.WriteString(`rateLimit { limit remaining resetAt cost } `)
	b.WriteString(`}`)
	return b.String(), nil
}

// convertBatchedPRResult turns a batched GraphQL row into the (PR, PRStatus)
// pair the existing poller code uses.
func convertBatchedPRResult(raw *batchedPRResult, owner, repo string, number int) *PRStatus {
	state := strings.ToLower(raw.State)
	if raw.MergedAt != "" {
		state = prStateMerged
	}
	pr := &PR{
		Number:         number,
		Title:          raw.Title,
		URL:            raw.URL,
		HTMLURL:        raw.URL,
		State:          state,
		HeadBranch:     raw.HeadRefName,
		HeadSHA:        raw.HeadRefOid,
		BaseBranch:     raw.BaseRefName,
		AuthorLogin:    raw.Author.Login,
		RepoOwner:      owner,
		RepoName:       repo,
		Draft:          raw.IsDraft,
		Mergeable:      raw.Mergeable == "MERGEABLE",
		MergeableState: strings.ToLower(raw.MergeStatus),
		Additions:      raw.Additions,
		Deletions:      raw.Deletions,
		CreatedAt:      raw.CreatedAt,
		UpdatedAt:      raw.UpdatedAt,
		MergedAt:       parseTimePtr(raw.MergedAt),
		ClosedAt:       parseTimePtr(raw.ClosedAt),
	}

	reviewState := summarizeReviewState(raw.Reviews.Nodes)
	checksState := ""
	if len(raw.Commits.Nodes) > 0 && raw.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		checksState = strings.ToLower(raw.Commits.Nodes[0].Commit.StatusCheckRollup.State)
	}
	// Count unresolved threads from the fetched nodes. When the page is
	// capped (totalCount > nodes), fall back to totalCount for the
	// resolved-vs-unresolved estimate so the popover doesn't undercount on
	// busy PRs. The fallback is conservative: we have no way to tell from
	// the truncated page how many of the un-fetched threads are resolved,
	// so attribute the unseen tail to "unresolved" — the popover's value
	// is meant to be actionable, not exact, and over-reporting is safer
	// than silently hiding open feedback.
	unresolved := 0
	for _, t := range raw.ReviewThreads.Nodes {
		if !t.IsResolved {
			unresolved++
		}
	}
	if total := raw.ReviewThreads.TotalCount; total > len(raw.ReviewThreads.Nodes) {
		unresolved += total - len(raw.ReviewThreads.Nodes)
	}
	return &PRStatus{
		PR:                               pr,
		ReviewState:                      reviewState,
		ChecksState:                      checksState,
		MergeableState:                   pr.MergeableState,
		ReviewCount:                      countApprovedReviewerNodes(raw.Reviews.Nodes),
		PendingReviewCount:               raw.ReviewRequests.TotalCount,
		ReviewCountsPopulated:            true,
		UnresolvedReviewThreads:          unresolved,
		UnresolvedReviewThreadsPopulated: true,
	}
}

// reviewNodesToSamples converts the GraphQL reviewNode shape to the shared
// reviewSample slice used by the dedup helpers in client_helpers.go.
func reviewNodesToSamples(nodes []reviewNode) []reviewSample {
	samples := make([]reviewSample, len(nodes))
	for i, n := range nodes {
		samples[i] = reviewSample{author: n.Author.Login, state: n.State, at: n.SubmittedAt}
	}
	return samples
}

// countApprovedReviewerNodes returns the number of distinct authors whose
// latest review state is APPROVED, on the GraphQL reviewNode shape. Thin
// adapter over countApprovedAuthors so REST and GraphQL agree on what
// "Approved (N)" means.
func countApprovedReviewerNodes(nodes []reviewNode) int {
	return countApprovedAuthors(reviewNodesToSamples(nodes))
}

// summarizeReviewState collapses the review history to a single
// "approved"/"changes_requested"/"" value. Per-reviewer dedup: each
// reviewer's most-recent binding review wins, so a CHANGES_REQUESTED
// followed by APPROVED from the same author resolves to APPROVED.
// CHANGES_REQUESTED beats APPROVED across distinct reviewers.
func summarizeReviewState(nodes []reviewNode) string {
	return reduceReviewSummary(reviewNodesToSamples(nodes))
}

// PATClient.ExecuteGraphQL satisfies GraphQLExecutor by POSTing to /graphql.
func (c *PATClient) ExecuteGraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return fmt.Errorf("marshal graphql: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubAPIBase+"/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setGitHubHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graphql request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.recordRateHeaders(resp, "/graphql")

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode >= 400 {
		c.maybeMarkRateExhaustedFromBody("/graphql", resp.StatusCode, respBody)
		return &GitHubAPIError{StatusCode: resp.StatusCode, Endpoint: "/graphql", Body: string(respBody)}
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode graphql response: %w", err)
	}
	recordGraphQLRateFromPayload(c.rateTracker, respBody)
	return nil
}

// recordGraphQLRateFromPayload extracts data.rateLimit from a GraphQL
// response body and feeds it to the tracker. The GraphQL rate-limit field is
// point-cost based and more accurate than HTTP headers.
func recordGraphQLRateFromPayload(tracker *RateTracker, body []byte) {
	if tracker == nil {
		return
	}
	var probe struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return
	}
	raw, ok := probe.Data["rateLimit"]
	if !ok || len(raw) == 0 {
		return
	}
	var rl graphQLRateLimit
	if err := json.Unmarshal(raw, &rl); err != nil {
		return
	}
	tracker.Record(RateSnapshot{
		Resource:  ResourceGraphQL,
		Limit:     rl.Limit,
		Remaining: rl.Remaining,
		ResetAt:   rl.ResetAt,
		UpdatedAt: time.Now().UTC(),
	})
}

// GHClient.ExecuteGraphQL satisfies GraphQLExecutor via `gh api graphql -f query=...`.
// Variables are passed as -F field_name=value entries; they are ignored when
// the query has no $variables.
func (c *GHClient) ExecuteGraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range variables {
		args = append(args, "-F", fmt.Sprintf("%s=%v", k, v))
	}
	stdout, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("gh graphql: %w", err)
	}
	if err := json.Unmarshal([]byte(stdout), out); err != nil {
		return fmt.Errorf("decode graphql response: %w", err)
	}
	recordGraphQLRateFromPayload(c.rateTracker, []byte(stdout))
	return nil
}

// noopGraphQLExecutorErr is returned when the active client is the noop
// fallback (no auth). Caller paths must handle this gracefully — typically
// by skipping the batched call entirely.
var errGraphQLUnsupported = fmt.Errorf("github client does not support GraphQL")

// graphQLExecutorFor returns an executor for the given client, or
// errGraphQLUnsupported when the client is nil/noop.
func graphQLExecutorFor(client Client) (GraphQLExecutor, error) {
	if exec, ok := client.(GraphQLExecutor); ok && client != nil {
		return exec, nil
	}
	return nil, errGraphQLUnsupported
}

// runBatchedPRQuery executes the batched query in chunks and merges the
// results into a single map keyed by prStatusCacheKey(owner, repo, number).
func runBatchedPRQuery(ctx context.Context, exec GraphQLExecutor, refs []graphQLPRRef) (map[string]*PRStatus, error) {
	if exec == nil || len(refs) == 0 {
		return nil, nil
	}
	result := make(map[string]*PRStatus, len(refs))
	for _, chunk := range chunkedRefs(refs) {
		query, vars := buildBatchedPRQuery(chunk)
		var resp struct {
			Data   map[string]json.RawMessage `json:"data"`
			Errors []graphQLError             `json:"errors"`
		}
		if err := exec.ExecuteGraphQL(ctx, query, vars, &resp); err != nil {
			return nil, err
		}
		// GitHub returns HTTP 200 with a top-level "errors" array for partial
		// auth failures, schema mismatches, or per-alias errors. Surface them
		// as an error so the caller falls back to per-watch checks instead of
		// silently absorbing the failure.
		if err := graphQLErrorsToErr(resp.Errors); err != nil {
			return nil, err
		}
		if err := decodeBatchedPRChunk(chunk, resp.Data, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// decodeBatchedPRChunk maps the aliased response back to the input refs and
// fills in the result map. Refs whose alias is missing or null are skipped
// (e.g. PR was deleted upstream); the next poller tick will retry.
func decodeBatchedPRChunk(refs []graphQLPRRef, data map[string]json.RawMessage, result map[string]*PRStatus) error {
	type group struct {
		idx    int
		owner  string
		repo   string
		prRefs []graphQLPRRef
	}
	groups := []*group{}
	byKey := map[string]*group{}
	for _, r := range refs {
		k := r.Owner + "/" + r.Repo
		g, ok := byKey[k]
		if !ok {
			g = &group{idx: len(groups), owner: r.Owner, repo: r.Repo}
			byKey[k] = g
			groups = append(groups, g)
		}
		g.prRefs = append(g.prRefs, r)
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Reassign group indices in sorted order to match buildBatchedPRQuery.
	for i, k := range keys {
		byKey[k].idx = i
	}
	for _, k := range keys {
		g := byKey[k]
		raw, ok := data[fmt.Sprintf("repo%d", g.idx)]
		if !ok || len(raw) == 0 {
			continue
		}
		repoBlock := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &repoBlock); err != nil {
			return fmt.Errorf("decode repo block: %w", err)
		}
		for prIdx, ref := range g.prRefs {
			alias := fmt.Sprintf("%s%d", graphQLPRBatchAlias, prIdx)
			rawPR, ok := repoBlock[alias]
			if !ok || len(rawPR) == 0 || string(rawPR) == "null" {
				continue
			}
			var raw batchedPRResult
			if err := json.Unmarshal(rawPR, &raw); err != nil {
				return fmt.Errorf("decode pr alias %s: %w", alias, err)
			}
			result[prStatusCacheKey(ref.Owner, ref.Repo, ref.Number)] = convertBatchedPRResult(&raw, ref.Owner, ref.Repo, ref.Number)
		}
	}
	return nil
}

// runBatchedBranchQuery executes the branch-lookup query in chunks and maps
// each branch ref to its first OPEN associated PR (if any). Result keys are
// "owner/repo/branch" so callers can index by their input refs.
func runBatchedBranchQuery(ctx context.Context, exec GraphQLExecutor, refs []graphQLBranchRef) (map[string]*PRStatus, error) {
	if exec == nil || len(refs) == 0 {
		return nil, nil
	}
	result := make(map[string]*PRStatus, len(refs))
	for _, chunk := range chunkedRefs(refs) {
		query, vars := buildBatchedBranchQuery(chunk)
		var resp struct {
			Data   map[string]json.RawMessage `json:"data"`
			Errors []graphQLError             `json:"errors"`
		}
		if err := exec.ExecuteGraphQL(ctx, query, vars, &resp); err != nil {
			return nil, err
		}
		if err := graphQLErrorsToErr(resp.Errors); err != nil {
			return nil, err
		}
		decodeBatchedBranchChunk(chunk, resp.Data, result)
	}
	return result, nil
}

func decodeBatchedBranchChunk(refs []graphQLBranchRef, data map[string]json.RawMessage, result map[string]*PRStatus) {
	for i, ref := range refs {
		raw, ok := data[fmt.Sprintf("b%d", i)]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var inner struct {
			Ref *struct {
				AssociatedPullRequests struct {
					Nodes []struct {
						Number int `json:"number"`
						batchedPRResult
					} `json:"nodes"`
				} `json:"associatedPullRequests"`
			} `json:"ref"`
		}
		if err := json.Unmarshal(raw, &inner); err != nil {
			continue
		}
		if inner.Ref == nil || len(inner.Ref.AssociatedPullRequests.Nodes) == 0 {
			continue
		}
		node := inner.Ref.AssociatedPullRequests.Nodes[0]
		status := convertBatchedPRResult(&node.batchedPRResult, ref.Owner, ref.Repo, node.Number)
		result[graphqlBranchKey(ref.Owner, ref.Repo, ref.Branch)] = status
	}
}

// graphqlBranchKey is the lookup key used in batched-branch result maps.
// Named graphql* to avoid collision with the mock_client.go branchKey type.
func graphqlBranchKey(owner, repo, branch string) string {
	return owner + "/" + repo + "/" + branch
}

// Compile-time assertions that the existing CLI/PAT clients satisfy
// GraphQLExecutor. If a client stops implementing it the build fails here.
var (
	_ GraphQLExecutor = (*PATClient)(nil)
	_ GraphQLExecutor = (*GHClient)(nil)
)
