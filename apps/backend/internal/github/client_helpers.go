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
	hasIssues := hasFailingChecks(checks) || hasChangesRequested(reviews)
	return &PRFeedback{
		PR:        pr,
		Reviews:   reviews,
		Comments:  comments,
		Checks:    checks,
		HasIssues: hasIssues,
	}, nil
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

// mergeChecks deduplicates check runs and commit statuses by normalized name/url.
// If both sources refer to the same item, check-run data wins.
func mergeChecks(checkRuns, statusChecks []CheckRun) []CheckRun {
	merged := make([]CheckRun, 0, len(checkRuns)+len(statusChecks))
	byKey := make(map[string]int)

	for _, check := range statusChecks {
		key := checkMergeKey(check)
		byKey[key] = len(merged)
		merged = append(merged, check)
	}
	for _, check := range checkRuns {
		key := checkMergeKey(check)
		if idx, ok := byKey[key]; ok {
			merged[idx] = check
			continue
		}
		byKey[key] = len(merged)
		merged = append(merged, check)
	}
	return merged
}

func checkMergeKey(check CheckRun) string {
	name := strings.ToLower(strings.TrimSpace(check.Name))
	url := strings.ToLower(strings.TrimSpace(check.HTMLURL))
	return name + "|" + url
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
