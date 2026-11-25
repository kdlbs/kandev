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

// noTrackedEvents marks an observation that tracks no event, as distinct from
// a legacy row whose empty list tracks every event.
const noTrackedEvents = "none"

// tracked lists the selected events in a stable order.
func (s prEventSelection) tracked() string {
	var events []string
	if s.opened {
		events = append(events, PREventOpened)
	}
	if s.newCommits {
		events = append(events, PREventNewCommits)
	}
	if s.reviewRequested {
		events = append(events, PREventReviewRequested)
	}
	return strings.Join(events, ",")
}

// trackedFor is tracked for an existing observation. A pull request whose
// observation lacks opened was open before opened was selected, so opened
// stays untracked for it until the observation is pruned.
func (s prEventSelection) trackedFor(obs *PRObservation) string {
	if s.opened && !obs.tracks(PREventOpened) {
		s.opened = false
	}
	if tracked := s.tracked(); tracked != "" {
		return tracked
	}
	// An empty list means a row written before events were tracked, which
	// tracks every event; an observation that tracks nothing says so.
	return noTrackedEvents
}

// tracks reports whether an observation was recorded for event. Rows written
// before events were tracked count as tracking every event.
func (o *PRObservation) tracks(event string) bool {
	return o.TrackedEvents == "" || slices.Contains(strings.Split(o.TrackedEvents, ","), event)
}

// openedEligible reports whether opened may fire for pr. By default only a
// pull request created at or after the trigger qualifies, and one observed
// before opened was selected does not; an unknown creation time counts as new.
func openedEligible(t *AutomationTrigger, cfg *GitHubPRTriggerConfig, pr *github.PR, obs *PRObservation) bool {
	if cfg.IncludeExistingPRs {
		return true
	}
	if obs != nil && !obs.tracks(PREventOpened) {
		return false
	}
	return pr.CreatedAt.IsZero() || !pr.CreatedAt.Before(t.CreatedAt)
}

// validateGitHubPRConfig checks only the fields this feature owns, so a legacy
// config with an unexpected shape elsewhere keeps saving as before.
func validateGitHubPRConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var cfg struct {
		QuietPeriodMinutes int      `json:"quiet_period_minutes"`
		Reviewers          []string `json:"reviewers"`
		IncludeExistingPRs bool     `json:"include_existing_prs"`
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
	// generation is the review-request sequence a review_requested firing uses.
	generation int64
}

func reviewRequestDedupPrefix(owner, repo string, number int) string {
	return fmt.Sprintf("pr:%s/%s#%d/review-requested/", owner, repo, number)
}

func baselineObservation(triggerID, owner, repo string, pr *github.PR, sel prEventSelection) *PRObservation {
	return &PRObservation{
		TriggerID: triggerID, RepoOwner: owner, RepoName: repo, PRNumber: pr.Number,
		HeadSHA: pr.HeadSHA, Requested: requestedReviewerKeys(pr), TrackedEvents: sel.tracked(),
	}
}

func cloneObservation(obs *PRObservation) *PRObservation {
	next := *obs
	next.Requested = append([]string(nil), obs.Requested...)
	next.PendingReviewReviewers = append([]string(nil), obs.PendingReviewReviewers...)
	if obs.PendingSince != nil {
		since := *obs.PendingSince
		next.PendingSince = &since
	}
	return &next
}

// clearReviewReservation releases a reserved review-request generation.
func (o *PRObservation) clearReviewReservation() {
	o.PendingReviewGeneration, o.PendingReviewReviewers = 0, nil
}

func observationsEqual(a, b *PRObservation) bool {
	if a.HeadSHA != b.HeadSHA || a.PendingHeadSHA != b.PendingHeadSHA || a.TrackedEvents != b.TrackedEvents ||
		a.ReviewGeneration != b.ReviewGeneration || a.PendingReviewGeneration != b.PendingReviewGeneration ||
		!slices.Equal(a.Requested, b.Requested) || !slices.Equal(a.PendingReviewReviewers, b.PendingReviewReviewers) {
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
	// An event added to the trigger after this PR was first seen starts from
	// the PR's current state rather than firing for older changes.
	if sel.newCommits && !obs.tracks(PREventNewCommits) {
		next.HeadSHA, next.PendingHeadSHA, next.PendingSince = pr.HeadSHA, "", nil
	}
	if sel.reviewRequested && !obs.tracks(PREventReviewRequested) {
		next.Requested = current
	}
	next.TrackedEvents = sel.trackedFor(obs)
	if !matched {
		next.HeadSHA, next.Requested = pr.HeadSHA, current
		next.PendingHeadSHA, next.PendingSince = "", nil
		next.clearReviewReservation()
		return nil, next
	}
	if firing := chooseNewCommits(pr, cfg, sel, next, now); firing != nil {
		return firing, next
	}
	if sel.reviewRequested {
		if newly := newlyRequestedReviewers(current, next.Requested, cfg.Reviewers); len(newly) > 0 {
			return &prFiring{
				event: PREventReviewRequested, previousHead: next.HeadSHA, reviewers: newly,
			}, next
		}
	}
	next.Requested = current
	next.clearReviewReservation()
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
func processedObservation(obs *PRObservation, pr *github.PR, firing *prFiring, sel prEventSelection) *PRObservation {
	next := cloneObservation(obs)
	next.TrackedEvents = sel.trackedFor(obs)
	next.HeadSHA, next.Requested = pr.HeadSHA, requestedReviewerKeys(pr)
	next.PendingHeadSHA, next.PendingSince = "", nil
	next.clearReviewReservation()
	if firing.event == PREventReviewRequested {
		next.ReviewGeneration = firing.generation
	}
	return next
}

// firingAdvancesObservation applies the outcome rule: admitted, duplicate, and
// capacity-skipped firings are processed; deferrals and errors are retried.
func firingAdvancesObservation(result FireResult, err error) bool {
	return err == nil && !result.Deferred
}
