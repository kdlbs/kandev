package automation

import (
	"context"
	"fmt"
	"slices"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
)

// evaluatePRRepo lists one repository's open PRs once and evaluates every
// selected event against them, maintaining stored observations.
func (e *GitHubEvaluator) evaluatePRRepo(
	ctx context.Context, t *AutomationTrigger, automation *Automation,
	cfg *GitHubPRTriggerConfig, sel prEventSelection, repo github.RepoFilter, cycle prRepoCycle,
) {
	prs, complete, err := e.githubService.ListAutomationOpenPRs(ctx, automation.WorkspaceID, repo.Owner, repo.Name)
	if err != nil {
		e.logger.Debug("failed to list PRs for automation trigger",
			zap.String(automationRepoKey, repo.Owner+"/"+repo.Name), zap.Error(err))
		return
	}
	// Every selection reads observations: opened eligibility skips a PR this
	// trigger already observed, and an opened-only trigger still records which
	// follow-up events were removed.
	observations, err := e.svc.Store().ListPRObservations(ctx, t.ID, repo.Owner, repo.Name)
	if err != nil {
		e.logger.Warn("failed to load PR observations",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}
	seen := make(map[int]bool, len(prs))
	for _, pr := range prs {
		if pr == nil {
			continue
		}
		seen[pr.Number] = true
		e.evaluatePR(ctx, t, cfg, sel, repo, pr, observations[pr.Number])
	}
	e.logger.Debug("evaluated github_pr trigger repository",
		zap.String("trigger_id", t.ID), zap.String(automationRepoKey, repo.Owner+"/"+repo.Name),
		zap.Int("open_prs", len(prs)), zap.Bool("complete", complete))
	if !complete {
		return
	}
	if len(observations) > 0 {
		e.pruneObservations(ctx, t.ID, repo, observations, seen)
	}
	if cycle.claim(automation.ID, repo) {
		e.runEndOfPRActions(ctx, automation, repo, seen)
	}
}

// evaluatePR fires at most one github_pr firing for a PR and stores its
// observation according to the outcome rule.
func (e *GitHubEvaluator) evaluatePR(
	ctx context.Context, t *AutomationTrigger, cfg *GitHubPRTriggerConfig,
	sel prEventSelection, repo github.RepoFilter, pr *github.PR, obs *PRObservation,
) {
	matched := prMatchesFilters(pr, cfg)
	if sel.opened && matched && openedEligible(t, cfg, pr, obs) && e.handleOpened(ctx, t, sel, repo, pr, obs) {
		return
	}
	if !sel.observes() {
		e.recordTrackedEvents(ctx, sel, obs)
		return
	}
	if obs == nil {
		e.storeObservation(ctx, baselineObservation(t.ID, repo.Owner, repo.Name, pr, sel))
		return
	}
	firing, next := choosePRFollowUp(pr, cfg, sel, obs, matched, e.now())
	if firing != nil && firing.event == PREventReviewRequested && !e.reserveReviewGeneration(ctx, t, repo, pr, next, firing) {
		firing = nil
	}
	if firing == nil {
		if !observationsEqual(obs, next) {
			e.storeObservation(ctx, next)
		}
		return
	}
	result, err := e.firePRTrigger(ctx, t, pr, repo.Owner, repo.Name, firing)
	if firingAdvancesObservation(result, err) {
		next = processedObservation(obs, pr, firing, sel)
	}
	if !observationsEqual(obs, next) {
		e.storeObservation(ctx, next)
	}
}

// recordTrackedEvents stores which events an existing observation still
// tracks when no follow-up event is selected, so a follow-up event selected
// again later starts from the pull request's state at that time.
func (e *GitHubEvaluator) recordTrackedEvents(ctx context.Context, sel prEventSelection, obs *PRObservation) {
	if obs == nil {
		return
	}
	next := cloneObservation(obs)
	next.TrackedEvents = sel.trackedFor(obs)
	if !observationsEqual(obs, next) {
		e.storeObservation(ctx, next)
	}
}

// handleOpened fires opened for a matching PR whose key is not held yet. It
// reports true when this poll's handling of the PR is complete: the firing was
// not a duplicate, or it failed for a PR that already has an observation.
func (e *GitHubEvaluator) handleOpened(
	ctx context.Context, t *AutomationTrigger, sel prEventSelection, repo github.RepoFilter, pr *github.PR, obs *PRObservation,
) bool {
	firing := &prFiring{event: PREventOpened, dedupKey: fmt.Sprintf("pr:%s/%s#%d", repo.Owner, repo.Name, pr.Number)}
	held, err := e.svc.Store().HasRunWithDedupKey(ctx, t.AutomationID, firing.dedupKey)
	if err != nil {
		e.logger.Warn("failed to check opened dedup key; retrying next poll",
			zap.String("trigger_id", t.ID), zap.Int("pr_number", pr.Number), zap.Error(err))
		return obs != nil
	}
	if held {
		return false
	}
	result, err := e.firePRTrigger(ctx, t, pr, repo.Owner, repo.Name, firing)
	if err == nil && !result.Duplicate {
		e.recordOpenedOutcome(ctx, t, sel, repo, pr, obs, firing, result)
		return true
	}
	return err != nil && obs != nil
}

// recordOpenedOutcome stores the observation after a non-duplicate opened
// firing. A first sighting always records the baseline; an existing
// observation advances only when the outcome rule says it was processed.
func (e *GitHubEvaluator) recordOpenedOutcome(
	ctx context.Context, t *AutomationTrigger, sel prEventSelection, repo github.RepoFilter,
	pr *github.PR, obs *PRObservation, firing *prFiring, result FireResult,
) {
	if !sel.observes() {
		return
	}
	if obs == nil {
		e.storeObservation(ctx, baselineObservation(t.ID, repo.Owner, repo.Name, pr, sel))
		return
	}
	if result.Deferred {
		return
	}
	next := processedObservation(obs, pr, firing, sel)
	if !observationsEqual(obs, next) {
		e.storeObservation(ctx, next)
	}
}

// reserveReviewGeneration gives a review_requested firing its dedup key and
// stores it, with the reviewers it covers, on the observation before
// dispatch. A retry for those reviewers reuses the reserved generation, so a run committed before its observation
// advanced deduplicates instead of dispatching again. A new request takes the
// next sequence after every key this automation already used for the PR, so a
// reopened PR, a recreated trigger, or a second trigger never reuses a held
// key. It reports false when the run history cannot be read or the
// reservation cannot be stored; the poll retries.
func (e *GitHubEvaluator) reserveReviewGeneration(
	ctx context.Context, t *AutomationTrigger, repo github.RepoFilter, pr *github.PR, next *PRObservation, firing *prFiring,
) bool {
	prefix := reviewRequestDedupPrefix(repo.Owner, repo.Name, pr.Number)
	if next.PendingReviewGeneration > 0 && reservationCovers(next.PendingReviewReviewers, firing.reviewers) {
		firing.generation = next.PendingReviewGeneration
		firing.dedupKey = fmt.Sprintf("%s%d", prefix, firing.generation)
		return true
	}
	used, err := e.svc.Store().MaxReviewRequestGeneration(ctx, t.AutomationID, prefix)
	if err != nil {
		e.logger.Warn("failed to read review-request generation; retrying next poll",
			zap.String("trigger_id", t.ID), zap.Int("pr_number", pr.Number), zap.Error(err))
		return false
	}
	firing.generation = max(next.ReviewGeneration, used) + 1
	firing.dedupKey = fmt.Sprintf("%s%d", prefix, firing.generation)
	next.PendingReviewGeneration = firing.generation
	next.PendingReviewReviewers = append([]string(nil), firing.reviewers...)
	if err := e.svc.Store().UpsertPRObservation(ctx, next); err != nil {
		e.logger.Warn("failed to reserve review-request generation; retrying next poll",
			zap.String("trigger_id", t.ID), zap.Int("pr_number", pr.Number), zap.Error(err))
		next.clearReviewReservation()
		return false
	}
	return true
}

// reservationCovers reports whether every reviewer of a firing was part of
// the reservation, so reusing its key cannot swallow a later reviewer.
func reservationCovers(reserved, reviewers []string) bool {
	for _, reviewer := range reviewers {
		if !slices.Contains(reserved, reviewer) {
			return false
		}
	}
	return true
}

func (e *GitHubEvaluator) storeObservation(ctx context.Context, obs *PRObservation) {
	if err := e.svc.Store().UpsertPRObservation(ctx, obs); err != nil {
		e.logger.Warn("failed to store PR observation",
			zap.String("trigger_id", obs.TriggerID), zap.Int("pr_number", obs.PRNumber), zap.Error(err))
	}
}

// pruneObservations deletes observations for PRs missing from a complete
// listing, so a reopened PR starts from a new baseline.
func (e *GitHubEvaluator) pruneObservations(
	ctx context.Context, triggerID string, repo github.RepoFilter,
	observations map[int]*PRObservation, seen map[int]bool,
) {
	for number := range observations {
		if seen[number] {
			continue
		}
		if err := e.svc.Store().DeletePRObservation(ctx, triggerID, repo.Owner, repo.Name, number); err != nil {
			e.logger.Warn("failed to prune PR observation",
				zap.String("trigger_id", triggerID), zap.Int("pr_number", number), zap.Error(err))
		}
	}
}
