package automation

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/github"
)

// GitHub PR trigger event values persisted in GitHubPRTriggerConfig.Events.
const (
	PREventOpened          = "opened"
	PREventNewCommits      = "new_commits"
	PREventReviewRequested = "review_requested"

	maxQuietPeriodMinutes = 1440
	githubReviewerTeam    = "team"
)

// Trigger data keys added by the PR follow-up events.
const (
	automationHeadSHAKey            = "head_sha"
	automationPreviousHeadSHAKey    = "previous_head_sha"
	automationPREventKey            = "event"
	automationRequestedReviewersKey = "requested_reviewers"
	automationPRSubjectKey          = "pr_subject"
)

// prEventSelection is the decoded, exact-match view of a trigger's events.
type prEventSelection struct {
	opened, newCommits, reviewRequested bool
}

func selectPREvents(events []string) prEventSelection {
	return prEventSelection{
		opened:          slices.Contains(events, PREventOpened),
		newCommits:      slices.Contains(events, PREventNewCommits),
		reviewRequested: slices.Contains(events, PREventReviewRequested),
	}
}

func (s prEventSelection) any() bool { return s.opened || s.newCommits || s.reviewRequested }

// observes reports whether the trigger needs stored PR observations.
func (s prEventSelection) observes() bool { return s.newCommits || s.reviewRequested }

// validateGitHubPRConfig checks only the fields this feature owns, so a legacy
// config with an unexpected shape elsewhere keeps saving as before.
func validateGitHubPRConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var cfg struct {
		QuietPeriodMinutes int      `json:"quiet_period_minutes"`
		Reviewers          []string `json:"reviewers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("invalid github_pr trigger config: %w", err)
	}
	if cfg.QuietPeriodMinutes < 0 || cfg.QuietPeriodMinutes > maxQuietPeriodMinutes {
		return fmt.Errorf("quiet_period_minutes must be between 0 and %d", maxQuietPeriodMinutes)
	}
	return nil
}

// PRSubject identifies the pull request a github_pr firing is about.
func PRSubject(owner, repo string, number int) string {
	return fmt.Sprintf("%s/%s#%d", owner, repo, number)
}

func prMatchesFilters(pr *github.PR, cfg *GitHubPRTriggerConfig) bool {
	if cfg.ExcludeDraft && pr.Draft {
		return false
	}
	return matchesBranches(pr.BaseBranch, cfg.Branches) &&
		matchesAuthors(pr.AuthorLogin, cfg.Authors) &&
		matchesLabels(pr.Labels, cfg.Labels)
}

// matchesLabels returns true for an empty filter, or when the PR carries at
// least one of the filter labels.
func matchesLabels(labels, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if slices.Contains(labels, f) {
			return true
		}
	}
	return false
}

// requestedReviewerKeys normalizes a PR's requested reviewers to lowercased
// user logins and `<owner>/<team-slug>` team keys, sorted.
func requestedReviewerKeys(pr *github.PR) []string {
	keys := make([]string, 0, len(pr.RequestedReviewers))
	for _, reviewer := range pr.RequestedReviewers {
		if reviewer.Login == "" {
			continue
		}
		key := strings.ToLower(reviewer.Login)
		if reviewer.Type == githubReviewerTeam {
			key = strings.ToLower(pr.RepoOwner) + "/" + key
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return slices.Compact(keys)
}

// newlyRequestedReviewers returns current reviewers absent from previous that
// match the reviewer filter (empty filter matches anyone).
func newlyRequestedReviewers(current, previous, filters []string) []string {
	var result []string
	for _, key := range current {
		if slices.Contains(previous, key) || !matchesReviewer(key, filters) {
			continue
		}
		result = append(result, key)
	}
	return result
}

func matchesReviewer(key string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if strings.EqualFold(strings.TrimSpace(f), key) {
			return true
		}
	}
	return false
}

// prFiring is the single github_pr firing chosen for one PR in one poll.
type prFiring struct {
	event        string
	previousHead string
	reviewers    []string
	dedupKey     string
}

func baselineObservation(triggerID, owner, repo string, pr *github.PR) *PRObservation {
	return &PRObservation{
		TriggerID: triggerID, RepoOwner: owner, RepoName: repo, PRNumber: pr.Number,
		HeadSHA: pr.HeadSHA, Requested: requestedReviewerKeys(pr),
	}
}

func cloneObservation(obs *PRObservation) *PRObservation {
	next := *obs
	next.Requested = append([]string(nil), obs.Requested...)
	if obs.PendingSince != nil {
		since := *obs.PendingSince
		next.PendingSince = &since
	}
	return &next
}

func observationsEqual(a, b *PRObservation) bool {
	if a.HeadSHA != b.HeadSHA || a.PendingHeadSHA != b.PendingHeadSHA ||
		a.ReviewGeneration != b.ReviewGeneration || !slices.Equal(a.Requested, b.Requested) {
		return false
	}
	if (a.PendingSince == nil) != (b.PendingSince == nil) {
		return false
	}
	return a.PendingSince == nil || a.PendingSince.Equal(*b.PendingSince)
}

// choosePRFollowUp picks at most one ready new_commits or review_requested
// firing for a PR whose observation exists. next is the observation to store
// when no firing advances it: it tracks quiet-period state, filter misses, and
// reviewer removals, but never marks a ready change as processed.
func choosePRFollowUp(
	pr *github.PR, cfg *GitHubPRTriggerConfig, sel prEventSelection,
	obs *PRObservation, matched bool, now time.Time,
) (*prFiring, *PRObservation) {
	next := cloneObservation(obs)
	current := requestedReviewerKeys(pr)
	if !matched {
		next.HeadSHA, next.Requested = pr.HeadSHA, current
		next.PendingHeadSHA, next.PendingSince = "", nil
		return nil, next
	}
	if firing := chooseNewCommits(pr, cfg, sel, next, now); firing != nil {
		return firing, next
	}
	if sel.reviewRequested {
		if newly := newlyRequestedReviewers(current, obs.Requested, cfg.Reviewers); len(newly) > 0 {
			return &prFiring{
				event: PREventReviewRequested, previousHead: obs.HeadSHA, reviewers: newly,
				dedupKey: fmt.Sprintf("pr:%s/%s#%d/review-requested/%d",
					obs.RepoOwner, obs.RepoName, pr.Number, obs.ReviewGeneration+1),
			}, next
		}
	}
	next.Requested = current
	return nil, next
}

// chooseNewCommits returns a ready new_commits firing, updating next's
// quiet-period state when the new head has not settled yet.
func chooseNewCommits(
	pr *github.PR, cfg *GitHubPRTriggerConfig, sel prEventSelection, next *PRObservation, now time.Time,
) *prFiring {
	if !sel.newCommits || pr.HeadSHA == "" || pr.HeadSHA == next.HeadSHA {
		next.PendingHeadSHA, next.PendingSince = "", nil
		return nil
	}
	firing := &prFiring{
		event: PREventNewCommits, previousHead: next.HeadSHA,
		dedupKey: fmt.Sprintf("pr:%s/%s#%d@%s", next.RepoOwner, next.RepoName, pr.Number, pr.HeadSHA),
	}
	quiet := time.Duration(cfg.QuietPeriodMinutes) * time.Minute
	if quiet <= 0 {
		return firing
	}
	if next.PendingHeadSHA != pr.HeadSHA || next.PendingSince == nil {
		since := now.UTC()
		next.PendingHeadSHA, next.PendingSince = pr.HeadSHA, &since
		return nil
	}
	if now.Sub(*next.PendingSince) >= quiet {
		return firing
	}
	return nil
}

// processedObservation is the observation after a firing was admitted,
// deduplicated, or skipped by the concurrency cap.
func processedObservation(obs *PRObservation, pr *github.PR, firing *prFiring) *PRObservation {
	next := cloneObservation(obs)
	next.HeadSHA, next.Requested = pr.HeadSHA, requestedReviewerKeys(pr)
	next.PendingHeadSHA, next.PendingSince = "", nil
	if firing.event == PREventReviewRequested {
		next.ReviewGeneration++
	}
	return next
}

// firingAdvancesObservation applies the outcome rule: admitted, duplicate, and
// capacity-skipped firings are processed; deferrals and errors are retried.
func firingAdvancesObservation(result FireResult, err error) bool {
	return err == nil && !result.Deferred
}
