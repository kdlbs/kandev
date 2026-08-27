package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
// keep initial status queries and their decoded response size manageable.
const graphQLBatchChunkSize = 50

// Each continuation can return 100 review-thread nodes. Five continuations
// keep a follow-up query near 500 connection nodes while preserving batching.
const graphQLReviewThreadContinuationChunkSize = 5

// graphQLBranchProbeLimit fetches two matches so branch lookup can detect
// ambiguous fork heads instead of linking the first arbitrary PR.
const graphQLBranchProbeLimit = 2

const graphQLReviewThreadPageSize = 100

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

type graphQLPRRepoGroup struct {
	Owner string
	Repo  string
	Refs  []graphQLPRRef
}

func groupGraphQLPRRefs(refs []graphQLPRRef) []graphQLPRRepoGroup {
	byKey := make(map[string]*graphQLPRRepoGroup)
	keys := make([]string, 0)
	for _, ref := range refs {
		key := ref.Owner + "/" + ref.Repo
		group, ok := byKey[key]
		if !ok {
			group = &graphQLPRRepoGroup{Owner: ref.Owner, Repo: ref.Repo}
			byKey[key] = group
			keys = append(keys, key)
		}
		group.Refs = append(group.Refs, ref)
	}
	sort.Strings(keys)

	groups := make([]graphQLPRRepoGroup, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, *byKey[key])
	}
	return groups
}

type reviewThreadNode struct {
	IsResolved bool `json:"isResolved"`
}

type graphQLPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type reviewThreadConnection struct {
	TotalCount int                `json:"totalCount"`
	Nodes      []reviewThreadNode `json:"nodes"`
	PageInfo   graphQLPageInfo    `json:"pageInfo"`
}

type reviewThreadContinuation struct {
	Ref            graphQLPRRef
	Cursor         string
	SeenCursors    map[string]struct{}
	RemainingPages int
	Status         *PRStatus
}

func newReviewThreadContinuation(
	ref graphQLPRRef,
	connection reviewThreadConnection,
	status *PRStatus,
) (reviewThreadContinuation, error) {
	cursor := connection.PageInfo.EndCursor
	if cursor == "" {
		return reviewThreadContinuation{}, fmt.Errorf(
			"review thread pagination for %s/%s#%d returned an empty cursor",
			ref.Owner, ref.Repo, ref.Number,
		)
	}
	remainingThreads := connection.TotalCount - len(connection.Nodes)
	if remainingThreads <= 0 {
		return reviewThreadContinuation{}, fmt.Errorf(
			"review thread pagination for %s/%s#%d exceeded totalCount %d",
			ref.Owner, ref.Repo, ref.Number, connection.TotalCount,
		)
	}
	return reviewThreadContinuation{
		Ref:            ref,
		Cursor:         cursor,
		SeenCursors:    map[string]struct{}{cursor: {}},
		RemainingPages: (remainingThreads + graphQLReviewThreadPageSize - 1) / graphQLReviewThreadPageSize,
		Status:         status,
	}, nil
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
	return chunkRefs(refs, graphQLBatchChunkSize)
}

func chunkRefs[T any](refs []T, chunkSize int) [][]T {
	if len(refs) == 0 {
		return nil
	}
	out := make([][]T, 0, (len(refs)+chunkSize-1)/chunkSize)
	for i := 0; i < len(refs); i += chunkSize {
		end := i + chunkSize
		if end > len(refs) {
			end = len(refs)
		}
		out = append(out, refs[i:end])
	}
	return out
}

// timelineActor is a GraphQL Actor's login, used by the closed-event
// timeline selection to attribute who closed a PR.
type timelineActor struct {
	Login string `json:"login"`
}

// timelineClosedEventNode is one node of the `timelineItems(itemTypes:
// CLOSED_EVENT)` selection. Actor is a pointer because GitHub can return a
// null actor (e.g. a bot-deleted account), which must leave closure
// attribution unpopulated rather than writing an empty login.
type timelineClosedEventNode struct {
	Actor *timelineActor `json:"actor"`
}

type mergeQueueRemovalEventNode struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	Reason       string    `json:"reason"`
	BeforeCommit *struct {
		OID string `json:"oid"`
	} `json:"beforeCommit"`
}

type batchedMergeQueueEntry struct {
	ID                   string `json:"id"`
	State                string `json:"state"`
	Position             *int   `json:"position"`
	EstimatedTimeToMerge *int   `json:"estimatedTimeToMerge"`
	HeadCommit           *struct {
		OID string `json:"oid"`
	} `json:"headCommit"`
}

// batchedPRResult is the decoded shape of one aliased pullRequest block.
type batchedPRResult struct {
	State string `json:"state"`
	Title string `json:"title"`
	URL   string `json:"url"`
	// IsDraft is a pointer: AC-12a requires distinguishing an upstream
	// response that omits or nulls isDraft from one that genuinely reports
	// false, and a plain bool can't tell the two apart after decode.
	IsDraft         *bool                   `json:"isDraft"`
	Mergeable       string                  `json:"mergeable"`
	MergeStatus     string                  `json:"mergeStateStatus"`
	MergeQueueEntry *batchedMergeQueueEntry `json:"mergeQueueEntry"`
	HeadRefName     string                  `json:"headRefName"`
	BaseRefName     string                  `json:"baseRefName"`
	HeadRefOid      string                  `json:"headRefOid"`
	Additions       int                     `json:"additions"`
	Deletions       int                     `json:"deletions"`
	// ChangedFiles is a pointer for the same reason as IsDraft (AC-12a): 0 is
	// a legitimate observation and must stay distinguishable from absent/null.
	ChangedFiles *int `json:"changedFiles"`
	Author       struct {
		Login string `json:"login"`
	} `json:"author"`
	// MergedBy is a value (not a pointer) because GitHub always serializes
	// the field for a PullRequest, using a zero-value login when there is no
	// merger; the empty-login case is handled explicitly in
	// convertBatchedPRResult rather than relying on a nil check.
	MergedBy struct {
		Login string `json:"login"`
	} `json:"mergedBy"`
	// AutoMergeRequest is a pointer: GitHub returns null when auto-merge was
	// never armed. Any non-nil value means "armed at fetch time" — never
	// "merged by auto-merge" (auto_merge is cleared once it fires).
	AutoMergeRequest *struct {
		EnabledAt string `json:"enabledAt"`
	} `json:"autoMergeRequest"`
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
	ReviewThreads reviewThreadConnection `json:"reviewThreads"`
	Commits       struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	// TimelineItems carries at most the single most-recent CLOSED_EVENT, so
	// closed-by attribution reflects the latest closure even across a
	// reopen-then-close cycle. closedBy has no equivalent on the REST pulls
	// endpoint or the gh CLI's PR field set (AC-09, AC-15) — this GraphQL
	// selection is the only source.
	TimelineItems struct {
		Nodes []timelineClosedEventNode `json:"nodes"`
	} `json:"timelineItems"`
	MergeQueueRemovalEvents struct {
		Nodes []mergeQueueRemovalEventNode `json:"nodes"`
	} `json:"mergeQueueRemovalEvents"`
}

type batchedBranchPRNode struct {
	Number int `json:"number"`
	batchedPRResult
}

// graphQLError mirrors a single entry in a GraphQL response's "errors" array.
type graphQLError struct {
	Message string   `json:"message"`
	Type    string   `json:"type"`
	Path    []string `json:"path"`
}

// repoRef is a minimal (owner, repo) pair used by the batched GraphQL
// helpers to surface which repositories the response flagged as
// unresolvable. Service.SyncWatchesBatched feeds these into the negative
// cache so subsequent polls short-circuit the dead-repo storm.
type repoRef struct {
	Owner string
	Repo  string
}

// batchedMissingReposErr is the typed error runBatchedPRQuery /
// runBatchedBranchQuery returns when the GraphQL response carried one or
// more "Could not resolve to a Repository" entries. Callers use
// errors.As to extract Repos and populate Service.repoErrorCache. When
// Inner is nil, the response contained ONLY resolution failures and
// the partial-decode result returned alongside the error is still safe
// to consume for the repos that did resolve; when Inner is non-nil,
// other (non-resolution) errors were also present and the caller should
// treat the whole batch as failed for those.
type batchedMissingReposErr struct {
	Repos []repoRef
	Inner error
}

func (e *batchedMissingReposErr) Error() string {
	if e.Inner != nil {
		return e.Inner.Error()
	}
	parts := make([]string, len(e.Repos))
	for i, r := range e.Repos {
		parts[i] = r.Owner + "/" + r.Repo
	}
	return "github: repositories not resolvable: " + strings.Join(parts, ", ")
}

// Is lets callers errors.Is(err, ErrRepoNotResolvable) without having to
// type-assert. The sentinel is the canonical "stop hammering this repo"
// signal across the package.
func (e *batchedMissingReposErr) Is(target error) bool {
	return target == ErrRepoNotResolvable
}

// Unwrap exposes the composite non-resolution error (if any) so callers
// can errors.As / errors.Is past the batched-missing wrapper.
func (e *batchedMissingReposErr) Unwrap() error { return e.Inner }

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

// classifyBatchedErrors splits the GraphQL errors[] array into "this
// repo is permanently unresolvable" and "everything else", mapping each
// resolution error's path-prefix alias back to the (owner, repo) it
// referred to via aliasToRepo. The caller (runBatchedPRQuery /
// runBatchedBranchQuery) lifts the resolution misses into the typed
// batchedMissingReposErr; the residual non-resolution errors are
// collapsed via graphQLErrorsToErr exactly as before so the storm-fix
// doesn't change behavior for unrelated GraphQL failures.
func classifyBatchedErrors(errs []graphQLError, aliasToRepo map[string]repoRef) (missing []repoRef, residual []graphQLError) {
	seen := map[string]bool{}
	for _, e := range errs {
		if isRepoNotResolvableErr(errors.New(e.Message)) && len(e.Path) > 0 {
			ref, ok := aliasToRepo[e.Path[0]]
			if ok {
				key := ref.Owner + "/" + ref.Repo
				if !seen[key] {
					seen[key] = true
					missing = append(missing, ref)
				}
				continue
			}
		}
		residual = append(residual, e)
	}
	return missing, residual
}

// aliasMapForPRRefs returns alias -> repoRef for the
// owner/repo-sorted ordering buildBatchedPRQuery applies. A
// resolution-failure error's path[0] is always the outer repository
// alias ("repo0", "repo1", ...).
func aliasMapForPRRefs(refs []graphQLPRRef) map[string]repoRef {
	groups := groupGraphQLPRRefs(refs)
	out := make(map[string]repoRef, len(groups))
	for i, group := range groups {
		out[fmt.Sprintf("repo%d", i)] = repoRef{Owner: group.Owner, Repo: group.Repo}
	}
	return out
}

// aliasMapForBranchRefs returns alias -> repoRef for the input-order
// b0, b1, ... aliases buildBatchedBranchQuery applies.
func aliasMapForBranchRefs(refs []graphQLBranchRef) map[string]repoRef {
	out := make(map[string]repoRef, len(refs))
	for i, r := range refs {
		out[fmt.Sprintf("b%d", i)] = repoRef{Owner: r.Owner, Repo: r.Repo}
	}
	return out
}

// wrapBatchedErrors composes a classifyBatchedErrors result into the
// (result, error) shape runBatchedPRQuery / runBatchedBranchQuery
// already return. Behaviour:
//   - no resolution misses, no residual errors → (nil): caller continues
//   - no resolution misses, residual errors only → return composite err
//     unchanged (pre-storm-fix semantics)
//   - resolution misses present → return *batchedMissingReposErr; Inner
//     is nil iff there were no residuals so callers can keep the
//     partial decode, otherwise Inner carries the composite
func wrapBatchedErrors(missing []repoRef, residual []graphQLError) error {
	residualErr := graphQLErrorsToErr(residual)
	if len(missing) == 0 {
		return residualErr
	}
	return &batchedMissingReposErr{Repos: missing, Inner: residualErr}
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
	var b strings.Builder
	b.WriteString("query Batch { ")
	for repoIdx, group := range groupGraphQLPRRefs(refs) {
		fmt.Fprintf(&b, `repo%d: repository(owner: %q, name: %q) { `, repoIdx, group.Owner, group.Repo)
		for prIdx, ref := range group.Refs {
			fmt.Fprintf(&b, `%s%d: pullRequest(number: %d) { %s } `,
				graphQLPRBatchAlias, prIdx, ref.Number, prFieldsBlock())
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
		`headRefName baseRefName headRefOid additions deletions changedFiles ` +
		`author { login } mergedBy { login } autoMergeRequest { enabledAt } ` +
		`mergeQueueEntry { id state position estimatedTimeToMerge headCommit { oid } } ` +
		`createdAt updatedAt mergedAt closedAt ` +
		`reviews(last: 100) { nodes { state author { login } submittedAt } } ` +
		`reviewRequests(first: 0) { totalCount } ` +
		fmt.Sprintf(`reviewThreads(first: %d) { totalCount nodes { isResolved } pageInfo { hasNextPage endCursor } } `,
			graphQLReviewThreadPageSize) +
		`commits(last: 1) { nodes { commit { statusCheckRollup { state } } } } ` +
		`timelineItems(last: 1, itemTypes: CLOSED_EVENT) { nodes { ... on ClosedEvent { actor { login } } } } ` +
		`mergeQueueRemovalEvents: timelineItems(last: 1, itemTypes: REMOVED_FROM_MERGE_QUEUE_EVENT) { nodes { ... on RemovedFromMergeQueueEvent { id createdAt reason beforeCommit { oid } } } }`
}

// buildBatchedBranchQuery emits one aliased pullRequests(headRefName:) block
// per (owner, repo, branch). Used to batch the "find PR by branch" path.
func buildBatchedBranchQuery(refs []graphQLBranchRef) (string, map[string]any) {
	var b strings.Builder
	b.WriteString("query Branches { ")
	for i, r := range refs {
		fmt.Fprintf(&b, `b%d: repository(owner: %q, name: %q) { pullRequests(first: %d, states: OPEN, headRefName: %q) { nodes { number %s } } } `,
			i, r.Owner, r.Repo, graphQLBranchProbeLimit, r.Branch, prFieldsBlock())
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
	draft, changedFiles := false, 0
	if raw.IsDraft != nil {
		draft = *raw.IsDraft
	}
	if raw.ChangedFiles != nil {
		changedFiles = *raw.ChangedFiles
	}
	pr := &PR{
		Number:               number,
		Title:                raw.Title,
		URL:                  raw.URL,
		HTMLURL:              raw.URL,
		State:                state,
		HeadBranch:           raw.HeadRefName,
		HeadSHA:              raw.HeadRefOid,
		BaseBranch:           raw.BaseRefName,
		AuthorLogin:          raw.Author.Login,
		RepoOwner:            owner,
		RepoName:             repo,
		Draft:                draft,
		IsDraftObserved:      raw.IsDraft != nil,
		Mergeable:            raw.Mergeable == ghMergeableState,
		MergeableState:       strings.ToLower(raw.MergeStatus),
		Additions:            raw.Additions,
		Deletions:            raw.Deletions,
		ChangedFiles:         changedFiles,
		ChangedFilesObserved: raw.ChangedFiles != nil,
		MergedByLogin:        raw.MergedBy.Login,
		AutoMergeEnabled:     raw.AutoMergeRequest != nil,
		CreatedAt:            raw.CreatedAt,
		UpdatedAt:            raw.UpdatedAt,
		MergedAt:             parseTimePtr(raw.MergedAt),
		ClosedAt:             parseTimePtr(raw.ClosedAt),
	}

	reviewState := summarizeReviewState(raw.Reviews.Nodes)
	checksState := ""
	if len(raw.Commits.Nodes) > 0 && raw.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		checksState = normalizeGraphQLCheckRollupState(raw.Commits.Nodes[0].Commit.StatusCheckRollup.State)
	}
	unresolved := 0
	for _, t := range raw.ReviewThreads.Nodes {
		if !t.IsResolved {
			unresolved++
		}
	}
	closedByLogin, closureAttributionPopulated := closedEventActor(raw.TimelineItems.Nodes)
	mergeQueueState, mergeQueuePosition, mergeQueueEstimate, mergeQueueEntryID, mergeQueueEntryHeadSHA := convertMergeQueueEntry(raw.MergeQueueEntry)
	removalID, removalAt, removalReason, removalBeforeSHA := convertMergeQueueRemoval(raw.MergeQueueRemovalEvents.Nodes)
	return &PRStatus{
		PR:                                    pr,
		ReviewState:                           reviewState,
		ChecksState:                           checksState,
		MergeableState:                        pr.MergeableState,
		ReviewCount:                           countApprovedReviewerNodes(raw.Reviews.Nodes),
		PendingReviewCount:                    raw.ReviewRequests.TotalCount,
		ReviewCountsPopulated:                 true,
		UnresolvedReviewThreads:               unresolved,
		UnresolvedReviewThreadsPopulated:      true,
		OutcomeFieldsPopulated:                true,
		ClosedByLogin:                         closedByLogin,
		ClosureAttributionPopulated:           closureAttributionPopulated,
		MergeQueueState:                       mergeQueueState,
		MergeQueuePosition:                    mergeQueuePosition,
		MergeQueueEntryID:                     mergeQueueEntryID,
		MergeQueueEntryHeadSHA:                mergeQueueEntryHeadSHA,
		MergeQueueEstimatedTimeToMergeSeconds: mergeQueueEstimate,
		MergeQueueLastRemovalID:               removalID,
		MergeQueueLastRemovedAt:               removalAt,
		MergeQueueLastRemovalReason:           removalReason,
		MergeQueueLastRemovalBeforeSHA:        removalBeforeSHA,
		mergeQueuePopulated:                   true,
		mergeQueueRecoveryPopulated:           true,
	}
}

func convertMergeQueueEntry(entry *batchedMergeQueueEntry) (string, *int, *int, string, string) {
	if entry == nil {
		return "", nil, nil, "", ""
	}
	headSHA := ""
	if entry.HeadCommit != nil {
		headSHA = entry.HeadCommit.OID
	}
	return normalizeMergeQueueState(entry.State), positiveIntPtr(entry.Position), nonNegativeIntPtr(entry.EstimatedTimeToMerge), entry.ID, headSHA
}

func convertMergeQueueRemoval(nodes []mergeQueueRemovalEventNode) (string, *time.Time, string, string) {
	if len(nodes) == 0 {
		return "", nil, "", ""
	}
	node := nodes[0]
	var beforeSHA string
	if node.BeforeCommit != nil {
		beforeSHA = node.BeforeCommit.OID
	}
	removedAt := node.CreatedAt
	return node.ID, &removedAt, node.Reason, beforeSHA
}

func normalizeMergeQueueState(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func positiveIntPtr(value *int) *int {
	if value == nil || *value <= 0 {
		return nil
	}
	copy := *value
	return &copy
}

func nonNegativeIntPtr(value *int) *int {
	if value == nil || *value < 0 {
		return nil
	}
	copy := *value
	return &copy
}

// closedEventActor extracts the closed-event actor's login from a
// `timelineItems(itemTypes: CLOSED_EVENT)` node list. Returns
// populated=false when there is no node or the node's actor is null (e.g. a
// bot-deleted account) — the caller must not write an empty login as if it
// were an observation (AC-15).
func closedEventActor(nodes []timelineClosedEventNode) (login string, populated bool) {
	if len(nodes) == 0 || nodes[0].Actor == nil || nodes[0].Actor.Login == "" {
		return "", false
	}
	return nodes[0].Actor.Login, true
}

// normalizeGraphQLCheckRollupState converts GitHub's GraphQL status-rollup
// enum to the smaller contract shared by REST and TaskPR persistence.
func normalizeGraphQLCheckRollupState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case checkStatusSuccess:
		return checkStatusSuccess
	case checkConclusionFail, "error":
		return checkConclusionFail
	case "expected", checkStatusPending:
		return checkStatusPending
	default:
		return ""
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
	// Accumulate missing / residual errors across all chunks so a chunk
	// of purely-dead repos doesn't short-circuit the loop and drop the
	// good data in later chunks. Pre-fix, a workspace with >50 watches
	// where the first 50 happened to be dead would silently skip the
	// remaining watches on every poll cycle until the first batch's
	// repos landed in the negative cache.
	var allMissing []repoRef
	var allResidual []graphQLError
	var continuations []reviewThreadContinuation
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
		// auth failures, schema mismatches, or per-alias errors. Split out
		// "Could not resolve to a Repository" entries via classifyBatchedErrors
		// so SyncWatchesBatched can negative-cache the dead repos and keep
		// partial results for the ones that resolved; non-resolution errors
		// still bubble up so the caller falls back to per-watch checks.
		missing, residual := classifyBatchedErrors(resp.Errors, aliasMapForPRRefs(chunk))
		chunkContinuations, err := decodeBatchedPRChunk(chunk, resp.Data, result)
		if err != nil {
			return nil, err
		}
		continuations = append(continuations, chunkContinuations...)
		allMissing = append(allMissing, missing...)
		allResidual = append(allResidual, residual...)
	}
	return finishBatchedQuery(ctx, exec, result, allMissing, allResidual, continuations)
}

func finishBatchedQuery(
	ctx context.Context,
	exec GraphQLExecutor,
	result map[string]*PRStatus,
	missing []repoRef,
	residual []graphQLError,
	continuations []reviewThreadContinuation,
) (map[string]*PRStatus, error) {
	batchErr := wrapBatchedErrors(missing, residual)
	if batchErr != nil && (len(missing) == 0 || len(residual) > 0) {
		return nil, batchErr
	}
	if err := completeReviewThreadContinuations(ctx, exec, continuations); err != nil {
		if len(missing) > 0 {
			return nil, &batchedMissingReposErr{Repos: missing, Inner: err}
		}
		return nil, err
	}
	if batchErr != nil {
		// When the error is purely "missing repos" across all chunks,
		// partial Statuses for the other refs are still in `result`;
		// surface them to the caller alongside the typed error so it
		// can populate the negative cache without dropping the good data.
		return result, batchErr
	}
	return result, nil
}

func buildReviewThreadPageQuery(continuations []reviewThreadContinuation) string {
	var b strings.Builder
	b.WriteString("query ReviewThreadPages { ")
	for i, continuation := range continuations {
		fmt.Fprintf(&b,
			`repo%d: repository(owner: %q, name: %q) { pr0: pullRequest(number: %d) { reviewThreads(first: %d, after: %q) { nodes { isResolved } pageInfo { hasNextPage endCursor } } } } `,
			i, continuation.Ref.Owner, continuation.Ref.Repo, continuation.Ref.Number,
			graphQLReviewThreadPageSize, continuation.Cursor,
		)
	}
	b.WriteString(`rateLimit { limit remaining resetAt cost } }`)
	return b.String()
}

func completeReviewThreadContinuations(
	ctx context.Context,
	exec GraphQLExecutor,
	continuations []reviewThreadContinuation,
) error {
	for len(continuations) > 0 {
		next := make([]reviewThreadContinuation, 0, len(continuations))
		for _, chunk := range chunkRefs(continuations, graphQLReviewThreadContinuationChunkSize) {
			var resp struct {
				Data   map[string]json.RawMessage `json:"data"`
				Errors []graphQLError             `json:"errors"`
			}
			if err := exec.ExecuteGraphQL(ctx, buildReviewThreadPageQuery(chunk), nil, &resp); err != nil {
				return err
			}
			if err := graphQLErrorsToErr(resp.Errors); err != nil {
				return err
			}
			chunkNext, err := applyReviewThreadPageChunk(chunk, resp.Data)
			if err != nil {
				return err
			}
			next = append(next, chunkNext...)
		}
		continuations = next
	}
	return nil
}

func applyReviewThreadPageChunk(
	continuations []reviewThreadContinuation,
	data map[string]json.RawMessage,
) ([]reviewThreadContinuation, error) {
	next := make([]reviewThreadContinuation, 0, len(continuations))
	for i, continuation := range continuations {
		page, err := decodeReviewThreadPage(i, data)
		if err != nil {
			return nil, err
		}
		continuation, hasNext, err := advanceReviewThreadContinuation(continuation, page)
		if err != nil {
			return nil, err
		}
		if hasNext {
			next = append(next, continuation)
		}
	}
	return next, nil
}

func decodeReviewThreadPage(index int, data map[string]json.RawMessage) (reviewThreadConnection, error) {
	repoAlias := fmt.Sprintf("repo%d", index)
	rawRepo, ok := data[repoAlias]
	if !ok || isNullGraphQLValue(rawRepo) {
		return reviewThreadConnection{}, fmt.Errorf("review thread response missing repository alias %s", repoAlias)
	}
	var repoBlock map[string]json.RawMessage
	if err := json.Unmarshal(rawRepo, &repoBlock); err != nil {
		return reviewThreadConnection{}, fmt.Errorf("decode review thread repo alias %s: %w", repoAlias, err)
	}
	rawPR, ok := repoBlock["pr0"]
	if !ok || isNullGraphQLValue(rawPR) {
		return reviewThreadConnection{}, fmt.Errorf("review thread response missing PR alias %s.pr0", repoAlias)
	}
	var rawPage struct {
		ReviewThreads json.RawMessage `json:"reviewThreads"`
	}
	if err := json.Unmarshal(rawPR, &rawPage); err != nil {
		return reviewThreadConnection{}, fmt.Errorf("decode review thread PR alias %s.pr0: %w", repoAlias, err)
	}
	if isNullGraphQLValue(rawPage.ReviewThreads) {
		return reviewThreadConnection{}, fmt.Errorf(
			"review thread response missing connection %s.pr0.reviewThreads",
			repoAlias,
		)
	}
	var page reviewThreadConnection
	if err := json.Unmarshal(rawPage.ReviewThreads, &page); err != nil {
		return reviewThreadConnection{}, fmt.Errorf("decode review thread connection %s.pr0: %w", repoAlias, err)
	}
	return page, nil
}

func advanceReviewThreadContinuation(
	continuation reviewThreadContinuation,
	page reviewThreadConnection,
) (reviewThreadContinuation, bool, error) {
	if continuation.RemainingPages <= 0 {
		return continuation, false, reviewThreadPageLimitError(continuation.Ref)
	}
	continuation.RemainingPages--
	for _, thread := range page.Nodes {
		if !thread.IsResolved {
			continuation.Status.UnresolvedReviewThreads++
		}
	}
	if !page.PageInfo.HasNextPage {
		return continuation, false, nil
	}
	if continuation.RemainingPages == 0 {
		return continuation, false, reviewThreadPageLimitError(continuation.Ref)
	}
	nextCursor := page.PageInfo.EndCursor
	if nextCursor == "" {
		return continuation, false, fmt.Errorf(
			"review thread pagination for %s/%s#%d returned an empty cursor",
			continuation.Ref.Owner, continuation.Ref.Repo, continuation.Ref.Number,
		)
	}
	if _, seen := continuation.SeenCursors[nextCursor]; seen {
		return continuation, false, fmt.Errorf(
			"review thread pagination for %s/%s#%d repeated cursor %q",
			continuation.Ref.Owner, continuation.Ref.Repo, continuation.Ref.Number, nextCursor,
		)
	}
	continuation.SeenCursors[nextCursor] = struct{}{}
	continuation.Cursor = nextCursor
	return continuation, true, nil
}

func reviewThreadPageLimitError(ref graphQLPRRef) error {
	return fmt.Errorf(
		"review thread pagination for %s/%s#%d exceeded its page limit",
		ref.Owner, ref.Repo, ref.Number,
	)
}

func isNullGraphQLValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

// decodeBatchedPRChunk maps the aliased response back to the input refs and
// fills in the result map. Refs whose alias is missing or null are skipped
// (e.g. PR was deleted upstream); the next poller tick will retry.
func decodeBatchedPRChunk(
	refs []graphQLPRRef,
	data map[string]json.RawMessage,
	result map[string]*PRStatus,
) ([]reviewThreadContinuation, error) {
	continuations := make([]reviewThreadContinuation, 0)
	for repoIdx, group := range groupGraphQLPRRefs(refs) {
		raw, ok := data[fmt.Sprintf("repo%d", repoIdx)]
		if !ok || len(raw) == 0 {
			continue
		}
		repoBlock := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &repoBlock); err != nil {
			return nil, fmt.Errorf("decode repo block: %w", err)
		}
		for prIdx, ref := range group.Refs {
			alias := fmt.Sprintf("%s%d", graphQLPRBatchAlias, prIdx)
			rawPR, ok := repoBlock[alias]
			if !ok || len(rawPR) == 0 || string(rawPR) == "null" {
				continue
			}
			var raw batchedPRResult
			if err := json.Unmarshal(rawPR, &raw); err != nil {
				return nil, fmt.Errorf("decode pr alias %s: %w", alias, err)
			}
			status := convertBatchedPRResult(&raw, ref.Owner, ref.Repo, ref.Number)
			result[prStatusCacheKey(ref.Owner, ref.Repo, ref.Number)] = status
			if raw.ReviewThreads.PageInfo.HasNextPage {
				continuation, err := newReviewThreadContinuation(
					ref, raw.ReviewThreads, status,
				)
				if err != nil {
					return nil, err
				}
				continuations = append(continuations, continuation)
			}
		}
	}
	return continuations, nil
}

// branchBatchResult is the outcome of one batched branch lookup.
//
// Statuses holds the branches that resolved to exactly one OPEN PR, keyed by
// graphqlBranchKey. ResolvedEmpty holds the branch keys whose repository alias
// resolved and reported ZERO open PRs — a definitive "no PR on this branch"
// that callers must NOT re-ask one watch at a time. A branch in neither set is
// unknown: its alias did not resolve, or it carried several open PRs and
// selectBatchedBranchPRNode deliberately refused to guess.
//
// Both maps are read-only at the call sites (they are shared through the
// service singleflight).
type branchBatchResult struct {
	Statuses      map[string]*PRStatus
	ResolvedEmpty map[string]struct{}
}

// runBatchedBranchQuery executes the branch-lookup query in chunks and maps
// each branch name to its unambiguous OPEN PR (if any). Result keys are
// "owner/repo/branch" so callers can index by their input refs.
func runBatchedBranchQuery(
	ctx context.Context, exec GraphQLExecutor, refs []graphQLBranchRef,
) (branchBatchResult, error) {
	if exec == nil || len(refs) == 0 {
		return branchBatchResult{}, nil
	}
	out := branchBatchResult{
		Statuses:      make(map[string]*PRStatus, len(refs)),
		ResolvedEmpty: make(map[string]struct{}, len(refs)),
	}
	// Same accumulation pattern as runBatchedPRQuery — see that function
	// for the rationale (one dead-repo chunk must not drop later chunks).
	var allMissing []repoRef
	var allResidual []graphQLError
	for _, chunk := range chunkedRefs(refs) {
		query, vars := buildBatchedBranchQuery(chunk)
		var resp struct {
			Data   map[string]json.RawMessage `json:"data"`
			Errors []graphQLError             `json:"errors"`
		}
		if err := exec.ExecuteGraphQL(ctx, query, vars, &resp); err != nil {
			return branchBatchResult{}, err
		}
		missing, residual := classifyBatchedErrors(resp.Errors, aliasMapForBranchRefs(chunk))
		if err := decodeBatchedBranchChunk(chunk, resp.Data, &out); err != nil {
			return branchBatchResult{}, err
		}
		allMissing = append(allMissing, missing...)
		allResidual = append(allResidual, residual...)
	}
	statuses, err := finishBatchedQuery(ctx, exec, out.Statuses, allMissing, allResidual, nil)
	if statuses == nil {
		// finishBatchedQuery dropped the partial decode (residual errors, or a
		// failed review-thread continuation). The negatives decoded alongside
		// it are no longer trustworthy as a whole-response answer, so surface
		// nothing rather than let a caller skip a fallback on their strength.
		return branchBatchResult{}, err
	}
	out.Statuses = statuses
	return out, err
}

func decodeBatchedBranchChunk(
	refs []graphQLBranchRef,
	data map[string]json.RawMessage,
	out *branchBatchResult,
) error {
	for i, ref := range refs {
		alias := fmt.Sprintf("b%d", i)
		raw, ok := data[alias]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var inner struct {
			PullRequests struct {
				Nodes []batchedBranchPRNode `json:"nodes"`
			} `json:"pullRequests"`
		}
		if err := json.Unmarshal(raw, &inner); err != nil {
			return fmt.Errorf("decode branch alias %s: %w", alias, err)
		}
		node, ok := selectBatchedBranchPRNode(inner.PullRequests.Nodes)
		if !ok {
			// Zero nodes is a definitive answer: the repository resolved and
			// has no open PR on this branch. Two or more is ambiguous and
			// stays unknown so the caller keeps its per-watch fallback.
			if len(inner.PullRequests.Nodes) == 0 {
				out.ResolvedEmpty[graphqlBranchKey(ref.Owner, ref.Repo, ref.Branch)] = struct{}{}
			}
			continue
		}
		status := convertBatchedPRResult(&node.batchedPRResult, ref.Owner, ref.Repo, node.Number)
		out.Statuses[graphqlBranchKey(ref.Owner, ref.Repo, ref.Branch)] = status
	}
	return nil
}

func selectBatchedBranchPRNode(nodes []batchedBranchPRNode) (*batchedBranchPRNode, bool) {
	if len(nodes) != 1 {
		return nil, false
	}
	return &nodes[0], true
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
