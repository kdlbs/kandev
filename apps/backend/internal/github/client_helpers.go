package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// buildReviewSearchQuery assembles the full GitHub search query.
// When customQuery is non-empty, it is used verbatim as the entire query.
// Otherwise a query is built from scope + optional filter qualifier.
func buildReviewSearchQuery(scope, filter, customQuery string) string {
	if customQuery != "" {
		return customQuery
	}
	var base string
	if scope == ReviewScopeUser {
		base = "type:pr state:open user-review-requested:@me -is:draft"
	} else {
		base = "type:pr state:open review-requested:@me -is:draft"
	}
	if filter != "" {
		base += " " + filter
	}
	return base
}

// getPRFeedback fetches aggregated feedback for a PR using any Client implementation.
// This shared function eliminates duplication between GHClient and PATClient.
func getPRFeedback(ctx context.Context, c Client, owner, repo string, number int) (*PRFeedback, error) {
	pr, err := c.GetPR(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	reviews, err := c.ListPRReviews(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	comments, err := c.ListPRComments(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, err
	}
	checks, err := c.ListCheckRuns(ctx, owner, repo, pr.HeadSHA)
	if err != nil {
		return nil, err
	}
	// Ensure non-nil slices so JSON serialization produces [] instead of null.
	if reviews == nil {
		reviews = []PRReview{}
	}
	if comments == nil {
		comments = []PRComment{}
	}
	if checks == nil {
		checks = []CheckRun{}
	}
	hasIssues := hasFailingChecks(checks) || hasChangesRequested(reviews)
	return &PRFeedback{
		PR:        pr,
		Reviews:   reviews,
		Comments:  comments,
		Checks:    checks,
		HasIssues: hasIssues,
	}, nil
}

// getPRStatus fetches lightweight PR status using any Client implementation.
// Unlike getPRFeedback, it skips comments to reduce API calls.
func getPRStatus(ctx context.Context, c Client, owner, repo string, number int) (*PRStatus, error) {
	pr, err := c.GetPR(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	reviews, err := c.ListPRReviews(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	checks, err := c.ListCheckRuns(ctx, owner, repo, pr.HeadSHA)
	if err != nil {
		return nil, err
	}
	if reviews == nil {
		reviews = []PRReview{}
	}
	if checks == nil {
		checks = []CheckRun{}
	}
	reviewState, pendingReviewCount := deriveReviewSyncState(pr, reviews)
	total, passing := countCheckResults(checks)
	return &PRStatus{
		PR:             pr,
		ReviewState:    reviewState,
		ChecksState:    computeOverallCheckStatus(checks),
		MergeableState: pr.MergeableState,
		// ReviewCount is the number of distinct reviewers whose latest review
		// state is APPROVED — it's the value the popover renders as
		// "Approved (N)" / "Approved N / M required". Counting raw review
		// entries (`len(reviews)`) double-counts authors who left multiple
		// reviews and also includes COMMENTED / CHANGES_REQUESTED rows.
		ReviewCount:        countApprovedReviewers(reviews),
		PendingReviewCount: pendingReviewCount,
		ChecksTotal:        total,
		ChecksPassing:      passing,
		// REST path actually counted check runs + reviews — distinguish
		// from the batched-GraphQL path that only carries rollup state.
		ChecksPopulated:       true,
		ReviewCountsPopulated: true,
	}, nil
}

// reviewSample is a normalized review row consumed by latestReviewStateByAuthor.
// Adapter functions below convert PRReview (REST) and reviewNode (GraphQL)
// to this shape so the per-reviewer dedup rule is only spelled out once.
type reviewSample struct {
	author string
	state  string
	at     time.Time
}

// latestReviewStateByAuthor reduces a list of reviews to "one entry per
// reviewer, latest binding state wins". COMMENTED and PENDING never
// override a prior APPROVED / CHANGES_REQUESTED for the same author so a
// reviewer who approved-then-commented still counts as approved.
// Anonymous reviewers (deleted users) get a unique synthetic key so each
// review still counts independently.
func latestReviewStateByAuthor(samples []reviewSample) map[string]reviewSample {
	byAuthor := make(map[string]reviewSample, len(samples))
	for _, s := range samples {
		key := s.author
		if key == "" {
			key = "<anon>:" + s.at.UTC().Format(time.RFC3339Nano)
		}
		state := strings.ToUpper(s.state)
		if state != reviewStateApproved && state != reviewStateChangesRequested {
			if _, ok := byAuthor[key]; ok {
				continue
			}
		}
		prev, ok := byAuthor[key]
		if !ok || !s.at.Before(prev.at) {
			byAuthor[key] = reviewSample{author: s.author, state: state, at: s.at}
		}
	}
	return byAuthor
}

// countApprovedAuthors counts distinct reviewers whose latest binding state is
// APPROVED. Shared between the REST and GraphQL count helpers.
func countApprovedAuthors(samples []reviewSample) int {
	count := 0
	for _, s := range latestReviewStateByAuthor(samples) {
		if s.state == reviewStateApproved {
			count++
		}
	}
	return count
}

// reduceReviewSummary collapses the per-author map into a single overall
// review state. CHANGES_REQUESTED wins across authors; APPROVED otherwise;
// "" when nothing is binding. Shared between REST and GraphQL paths.
func reduceReviewSummary(samples []reviewSample) string {
	hasApproval := false
	for _, l := range latestReviewStateByAuthor(samples) {
		switch l.state {
		case reviewStateChangesRequested:
			return computedReviewStateChangesRequested
		case reviewStateApproved:
			hasApproval = true
		}
	}
	if hasApproval {
		return computedReviewStateApproved
	}
	return ""
}

// countApprovedReviewers returns the number of distinct authors whose latest
// review state is APPROVED. REST-path adapter over the shared helper.
func countApprovedReviewers(reviews []PRReview) int {
	samples := make([]reviewSample, len(reviews))
	for i, r := range reviews {
		samples[i] = reviewSample{author: r.Author, state: r.State, at: r.CreatedAt}
	}
	return countApprovedAuthors(samples)
}

// countCheckResults returns (total, passing) counts using the same logic as
// computeOverallCheckStatus: skipped/neutral and still-running checks are
// excluded from the total so the X/Y badge communicates pass rate rather
// than conflating in-flight checks with failures. The pending state is
// carried separately via ChecksState.
func countCheckResults(checks []CheckRun) (int, int) {
	total, passing := 0, 0
	for _, c := range checks {
		if c.Status != checkStatusCompleted {
			continue
		}
		switch c.Conclusion {
		case checkConclusionSkipped, checkConclusionNeutral:
			continue
		case checkConclusionFail, checkConclusionTimedOut,
			checkConclusionCancelled, checkConclusionActionRequired:
			total++
		default:
			total++
			passing++
		}
	}
	return total, passing
}

// convertRawCheckRuns converts raw ghCheckRun structs into the domain CheckRun type.
func convertRawCheckRuns(raw []ghCheckRun) []CheckRun {
	checks := make([]CheckRun, len(raw))
	for i, cr := range raw {
		conclusion := ""
		if cr.Conclusion != nil {
			conclusion = *cr.Conclusion
		}
		output := ""
		if cr.Output.Summary != nil {
			output = *cr.Output.Summary
		}
		checks[i] = CheckRun{
			Name:        cr.Name,
			Source:      checkSourceCheckRun,
			Status:      cr.Status,
			Conclusion:  conclusion,
			HTMLURL:     cr.HTMLURL,
			Output:      output,
			StartedAt:   parseTimePtr(cr.StartedAt),
			CompletedAt: parseTimePtr(cr.CompletedAt),
		}
	}
	return checks
}

// convertRawComments converts raw review comments into the domain PRComment type.
func convertRawComments(raw []ghComment) []PRComment {
	comments := make([]PRComment, len(raw))
	for i, c := range raw {
		comments[i] = PRComment{
			ID:           c.ID,
			Author:       c.User.Login,
			AuthorAvatar: c.User.AvatarURL,
			AuthorIsBot:  isGitHubBot(c.User.Type),
			Body:         c.Body,
			Path:         c.Path,
			Line:         c.Line,
			Side:         c.Side,
			CommentType:  commentTypeReview,
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
			InReplyTo:    c.InReplyTo,
		}
	}
	return comments
}

// ghIssueComment is the JSON shape for issue comments from the GitHub API.
type ghIssueComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	User      struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		Type      string `json:"type"`
	} `json:"user"`
}

// convertRawIssueComments converts PR conversation comments into PRComment rows.
func convertRawIssueComments(raw []ghIssueComment) []PRComment {
	comments := make([]PRComment, len(raw))
	for i, c := range raw {
		comments[i] = PRComment{
			ID:           c.ID,
			Author:       c.User.Login,
			AuthorAvatar: c.User.AvatarURL,
			AuthorIsBot:  isGitHubBot(c.User.Type),
			Body:         c.Body,
			CommentType:  commentTypeIssue,
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
		}
	}
	return comments
}

// mergeAndSortPRComments combines review and issue comments and sorts by creation time.
func mergeAndSortPRComments(reviewComments, issueComments []PRComment) []PRComment {
	comments := make([]PRComment, 0, len(reviewComments)+len(issueComments))
	comments = append(comments, reviewComments...)
	comments = append(comments, issueComments...)
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].CreatedAt.Equal(comments[j].CreatedAt) {
			return comments[i].ID < comments[j].ID
		}
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})
	return comments
}

func isGitHubBot(userType string) bool {
	return strings.EqualFold(userType, githubUserTypeBot)
}

// ghStatusContext is the JSON shape from /commits/:ref/status.
type ghStatusContext struct {
	Context     string `json:"context"`
	State       string `json:"state"` // error, failure, pending, success
	TargetURL   string `json:"target_url"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// convertRawStatusContexts converts commit status contexts into CheckRun rows.
func convertRawStatusContexts(raw []ghStatusContext) []CheckRun {
	checks := make([]CheckRun, len(raw))
	for i, st := range raw {
		status := checkStatusCompleted
		conclusion := st.State
		switch st.State {
		case commitStatusSuccess:
			conclusion = checkStatusSuccess
		case commitStatusFailure, commitStatusError:
			conclusion = checkConclusionFail
		case commitStatusPending:
			status = "in_progress"
			conclusion = ""
		}
		checks[i] = CheckRun{
			Name:       st.Context,
			Source:     checkSourceStatusContext,
			Status:     status,
			Conclusion: conclusion,
			HTMLURL:    st.TargetURL,
			Output:     st.Description,
			StartedAt:  parseTimePtr(st.CreatedAt),
		}
		if status == checkStatusCompleted {
			checks[i].CompletedAt = parseTimePtr(st.UpdatedAt)
		}
	}
	return checks
}

// mergeChecks deduplicates check runs and commit statuses by normalized name.
// Check-run source wins over status-context; among same-source duplicates the
// most recent entry (by StartedAt) is kept.
func mergeChecks(checkRuns, statusChecks []CheckRun) []CheckRun {
	merged := make([]CheckRun, 0, len(checkRuns)+len(statusChecks))
	byKey := make(map[string]int)

	for _, check := range statusChecks {
		key := checkMergeKey(check)
		if idx, ok := byKey[key]; ok {
			if isNewerCheck(check, merged[idx]) {
				merged[idx] = check
			}
			continue
		}
		byKey[key] = len(merged)
		merged = append(merged, check)
	}
	for _, check := range checkRuns {
		key := checkMergeKey(check)
		if idx, ok := byKey[key]; ok {
			if merged[idx].Source == checkSourceStatusContext || isNewerCheck(check, merged[idx]) {
				merged[idx] = check
			}
			continue
		}
		byKey[key] = len(merged)
		merged = append(merged, check)
	}
	return merged
}

func checkMergeKey(check CheckRun) string {
	return strings.ToLower(strings.TrimSpace(check.Name))
}

// isNewerCheck reports whether a started more recently than b.
func isNewerCheck(a, b CheckRun) bool {
	if a.StartedAt == nil {
		return false
	}
	if b.StartedAt == nil {
		return true
	}
	return a.StartedAt.After(*b.StartedAt)
}

// convertSearchItemToPR converts common search result fields into a PR struct.
func convertSearchItemToPR(
	number int, title, htmlURL, state, authorLogin, repositoryURL string,
	draft bool, createdAt, updatedAt time.Time,
) *PR {
	owner, repo := parseRepoURL(repositoryURL)
	return &PR{
		Number:      number,
		Title:       title,
		HTMLURL:     htmlURL,
		State:       state,
		AuthorLogin: authorLogin,
		Draft:       draft,
		RepoOwner:   owner,
		RepoName:    repo,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

// clampSearchPage normalizes page and perPage to GitHub's Search API limits:
// page >= 1, 1 <= perPage <= 100. Defaults: page=1, perPage=50.
func clampSearchPage(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 50
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

// hasTypeQualifier reports whether the query already pins the result type via
// `type:pr`, `type:issue`, `is:pr`, or `is:issue` (case-insensitive).
func hasTypeQualifier(query string) bool {
	lower := strings.ToLower(query)
	for _, tok := range strings.Fields(lower) {
		switch tok {
		case "type:pr", "type:issue", "is:pr", "is:issue":
			return true
		}
	}
	return false
}

// buildIssueSearchQuery assembles a GitHub search query for issues.
// The `type:issue` qualifier is always injected (unless the caller already
// pinned it) because `/search/issues` returns both PRs and issues otherwise.
func buildIssueSearchQuery(filter, customQuery string) string {
	return buildSearchQuery("type:issue", filter, customQuery)
}

// buildPRSearchQuery assembles a GitHub search query for pull requests.
// The `type:pr` qualifier is always injected (unless the caller already pinned
// it) because `/search/issues` returns both PRs and issues otherwise.
func buildPRSearchQuery(filter, customQuery string) string {
	return buildSearchQuery("type:pr", filter, customQuery)
}

func buildSearchQuery(typeQualifier, filter, customQuery string) string {
	if customQuery != "" {
		if hasTypeQualifier(customQuery) {
			return customQuery
		}
		return typeQualifier + " " + customQuery
	}
	if filter != "" {
		return typeQualifier + " " + filter
	}
	return typeQualifier
}

// issueSearchItem is an issue item from the GitHub search API.
type issueSearchItem struct {
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	Body          string    `json:"body"`
	HTMLURL       string    `json:"html_url"`
	State         string    `json:"state"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	ClosedAt      *string   `json:"closed_at"`
	RepositoryURL string    `json:"repository_url"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
}

// convertSearchItemToIssue converts a search API item into an Issue struct.
func convertSearchItemToIssue(item issueSearchItem) *Issue {
	owner, repo := parseRepoURL(item.RepositoryURL)
	labels := make([]string, len(item.Labels))
	for i, l := range item.Labels {
		labels[i] = l.Name
	}
	assignees := make([]string, len(item.Assignees))
	for i, a := range item.Assignees {
		assignees[i] = a.Login
	}
	issue := &Issue{
		Number:      item.Number,
		Title:       item.Title,
		Body:        item.Body,
		HTMLURL:     item.HTMLURL,
		State:       item.State,
		AuthorLogin: item.User.Login,
		RepoOwner:   owner,
		RepoName:    repo,
		Labels:      labels,
		Assignees:   assignees,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
	if item.ClosedAt != nil {
		issue.ClosedAt = parseTimePtr(*item.ClosedAt)
	}
	return issue
}

// parseIssueSearchResults parses GitHub search API response items into Issue structs,
// filtering out any pull requests (which also appear in search/issues results).
func parseIssueSearchResults(items []issueSearchItem) []*Issue {
	var issues []*Issue
	for _, item := range items {
		// GitHub search/issues returns both issues and PRs; skip PRs
		if item.PullRequest != nil {
			continue
		}
		issues = append(issues, convertSearchItemToIssue(item))
	}
	return issues
}

// latestReviewByAuthor returns a map of the most recent review per author.
func latestReviewByAuthor(reviews []PRReview) map[string]PRReview {
	latest := make(map[string]PRReview)
	for _, r := range reviews {
		existing, ok := latest[r.Author]
		if !ok || r.CreatedAt.After(existing.CreatedAt) {
			latest[r.Author] = r
		}
	}
	return latest
}

// hasFailingChecks returns true if any completed check run has failed.
func hasFailingChecks(checks []CheckRun) bool {
	for _, c := range checks {
		if c.Status == checkStatusCompleted && c.Conclusion == checkConclusionFail {
			return true
		}
	}
	return false
}

// hasChangesRequested returns true if any review has requested changes.
func hasChangesRequested(reviews []PRReview) bool {
	for _, r := range reviews {
		if r.State == reviewStateChangesRequested {
			return true
		}
	}
	return false
}

// --- PR files and commits parsing ---

// ghPRFile is the JSON shape from the GitHub REST API for PR files.
type ghPRFile struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"` // added, removed, modified, renamed, copied, changed, unchanged
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Patch            string `json:"patch"`
	PreviousFilename string `json:"previous_filename"`
}

// ghPRCommit is the JSON shape from the GitHub REST API for PR commits.
type ghPRCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}

// parsePRFilesJSON parses the JSON response from the PR files API.
func parsePRFilesJSON(data string) ([]PRFile, error) {
	var raw []ghPRFile
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, fmt.Errorf("parse PR files: %w", err)
	}
	return convertRawPRFiles(raw), nil
}

// convertRawPRFiles converts raw GitHub API file structs to domain PRFile.
func convertRawPRFiles(raw []ghPRFile) []PRFile {
	files := make([]PRFile, len(raw))
	for i, f := range raw {
		files[i] = PRFile{
			Filename:  f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
			Patch:     buildFullDiff(f.Filename, f.Status, f.Patch, f.PreviousFilename),
			OldPath:   f.PreviousFilename,
		}
	}
	return files
}

// buildFullDiff wraps a GitHub patch fragment into a complete unified diff.
func buildFullDiff(filename, status, patch, previousFilename string) string {
	if patch == "" {
		return ""
	}
	var b strings.Builder
	switch status {
	case "added":
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", filename, filename)
		b.WriteString("new file mode 100644\n")
		fmt.Fprintf(&b, "--- /dev/null\n")
		fmt.Fprintf(&b, "+++ b/%s\n", filename)
	case "removed":
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", filename, filename)
		b.WriteString("deleted file mode 100644\n")
		fmt.Fprintf(&b, "--- a/%s\n", filename)
		fmt.Fprintf(&b, "+++ /dev/null\n")
	case "renamed":
		oldName := previousFilename
		if oldName == "" {
			oldName = filename
		}
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", oldName, filename)
		fmt.Fprintf(&b, "rename from %s\n", oldName)
		fmt.Fprintf(&b, "rename to %s\n", filename)
		fmt.Fprintf(&b, "--- a/%s\n", oldName)
		fmt.Fprintf(&b, "+++ b/%s\n", filename)
	default:
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", filename, filename)
		fmt.Fprintf(&b, "--- a/%s\n", filename)
		fmt.Fprintf(&b, "+++ b/%s\n", filename)
	}
	b.WriteString(patch)
	if !strings.HasSuffix(patch, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

// parsePRCommitsJSON parses the JSON response from the PR commits API.
func parsePRCommitsJSON(data string) ([]PRCommitInfo, error) {
	var raw []ghPRCommit
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, fmt.Errorf("parse PR commits: %w", err)
	}
	return convertRawPRCommits(raw), nil
}

// convertRawPRCommits converts raw GitHub API commit structs to domain PRCommitInfo.
func convertRawPRCommits(raw []ghPRCommit) []PRCommitInfo {
	commits := make([]PRCommitInfo, len(raw))
	for i, c := range raw {
		author := ""
		if c.Author != nil {
			author = c.Author.Login
		}
		// First line of commit message
		msg := c.Commit.Message
		if idx := strings.Index(msg, "\n"); idx >= 0 {
			msg = msg[:idx]
		}
		commits[i] = PRCommitInfo{
			SHA:         c.SHA,
			Message:     msg,
			AuthorLogin: author,
			AuthorDate:  c.Commit.Author.Date,
		}
	}
	return commits
}
