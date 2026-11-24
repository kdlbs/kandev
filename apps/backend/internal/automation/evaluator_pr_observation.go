package automation

import (
	"context"
	"fmt"

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
	var observations map[int]*PRObservation
	if sel.observes() {
		observations, err = e.svc.Store().ListPRObservations(ctx, t.ID, repo.Owner, repo.Name)
		if err != nil {
			e.logger.Warn("failed to load PR observations",
				zap.String("trigger_id", t.ID), zap.Error(err))
			return
		}
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
	if sel.observes() {
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
	if sel.opened && matched {
		firing := &prFiring{event: PREventOpened, dedupKey: fmt.Sprintf("pr:%s/%s#%d", repo.Owner, repo.Name, pr.Number)}
		result, err := e.firePRTrigger(ctx, t, pr, repo.Owner, repo.Name, firing)
		if err == nil && !result.Duplicate {
			e.recordOpenedOutcome(ctx, t, sel, repo, pr, obs, firing, result)
			return
		}
		if err != nil && obs != nil {
			return
		}
	}
	if !sel.observes() {
		return
	}
	if obs == nil {
		e.storeObservation(ctx, baselineObservation(t.ID, repo.Owner, repo.Name, pr))
		return
	}
	firing, next := choosePRFollowUp(pr, cfg, sel, obs, matched, e.now())
	if firing == nil {
		if !observationsEqual(obs, next) {
			e.storeObservation(ctx, next)
		}
		return
	}
	result, err := e.firePRTrigger(ctx, t, pr, repo.Owner, repo.Name, firing)
	if firingAdvancesObservation(result, err) {
		next = processedObservation(obs, pr, firing)
	}
	if !observationsEqual(obs, next) {
		e.storeObservation(ctx, next)
	}
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
		e.storeObservation(ctx, baselineObservation(t.ID, repo.Owner, repo.Name, pr))
		return
	}
	if result.Deferred {
		return
	}
	next := processedObservation(obs, pr, firing)
	if !observationsEqual(obs, next) {
		e.storeObservation(ctx, next)
	}
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
