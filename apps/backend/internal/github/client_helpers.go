package github

import (
	"context"
	"time"
)

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
	checks, err := c.ListCheckRuns(ctx, owner, repo, pr.HeadBranch)
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

// convertRawComments converts raw ghComment structs into the domain PRComment type.
func convertRawComments(raw []ghComment) []PRComment {
	comments := make([]PRComment, len(raw))
	for i, c := range raw {
		comments[i] = PRComment{
			ID:           c.ID,
			Author:       c.User.Login,
			AuthorAvatar: c.User.AvatarURL,
			Body:         c.Body,
			Path:         c.Path,
			Line:         c.Line,
			Side:         c.Side,
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
			InReplyTo:    c.InReplyTo,
		}
	}
	return comments
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
