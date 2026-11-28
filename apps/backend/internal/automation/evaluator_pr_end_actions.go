package automation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
)

// GitHub PR states reported by a single-PR read.
const (
	githubPRStateOpen   = "open"
	githubPRStateMerged = "merged"
	githubPRStateClosed = "closed"
)

// prRepoCycle records which automation and repository pairs already ran
// their end-of-PR pass in one evaluation cycle, so several github_pr triggers
// on the same repository act once.
type prRepoCycle map[string]bool

func (c prRepoCycle) claim(automationID string, repo github.RepoFilter) bool {
	key := automationID + "|" + repo.Owner + "/" + repo.Name
	if c[key] {
		return false
	}
	c[key] = true
	return true
}

// runEndOfPRActions applies the automation's end-of-PR actions to mapped PRs
// that are missing from a complete open listing, then removes their mappings.
// A PR that cannot be read, is still open, or whose task is busy is retried
// on a later poll.
func (e *GitHubEvaluator) runEndOfPRActions(
	ctx context.Context, a *Automation, repo github.RepoFilter, open map[int]bool,
) {
	if a.ContinuationPolicy != ContinuationPolicyReusePerPR {
		return
	}
	mappings, err := e.svc.Store().ListPRContinuationsForRepo(ctx, a.ID, repo.Owner, repo.Name)
	if err != nil {
		e.logger.Warn("failed to list PR continuations", zap.String("automation_id", a.ID), zap.Error(err))
		return
	}
	for i := range mappings {
		if open[mappings[i].PRNumber] {
			continue
		}
		e.endPRContinuation(ctx, a, &mappings[i])
	}
}

func (e *GitHubEvaluator) endPRContinuation(ctx context.Context, a *Automation, m *PRContinuation) {
	pr, err := e.githubService.GetAutomationPR(ctx, a.WorkspaceID, m.RepoOwner, m.RepoName, m.PRNumber)
	if err != nil || pr == nil || pr.State == "" || pr.State == githubPRStateOpen {
		return
	}
	// The PR read can be slow; act on the automation's current settings so a
	// disabled automation or a changed action is never applied from a stale
	// snapshot.
	a, err = e.svc.GetAutomation(ctx, a.ID)
	if err != nil || a == nil || !a.Enabled || a.ContinuationPolicy != ContinuationPolicyReusePerPR {
		return
	}
	action, outcome := normalizePREndAction(a.PRClosedAction), githubPRStateClosed
	if pr.State == githubPRStateMerged || pr.MergedAt != nil {
		action, outcome = normalizePREndAction(a.PRMergedAction), githubPRStateMerged
	}
	if !e.applyPREndAction(ctx, a, m, action, outcome) {
		return
	}
	if err := e.svc.Store().DeletePRContinuation(ctx, a.ID, m.PRSubject, m.TaskID); err != nil {
		e.logger.Warn("failed to remove PR continuation", zap.String("automation_id", a.ID),
			zap.String("pr_subject", m.PRSubject), zap.Error(err))
		return
	}
	e.logger.Info("applied end-of-PR action",
		zap.String("automation_id", a.ID), zap.String("pr_subject", m.PRSubject),
		zap.String("task_id", m.TaskID), zap.String("outcome", outcome), zap.String("action", string(action)))
}

// applyPREndAction applies the end-of-PR action once the PR task is idle. It
// reports false when the action must be retried later. Every action, keep
// included, waits for an idle task: a run still working on the PR keeps its
// mapping, so a reopen during that run continues the same task.
func (e *GitHubEvaluator) applyPREndAction(
	ctx context.Context, a *Automation, m *PRContinuation, action PREndAction, outcome string,
) bool {
	busy, err := e.svc.prTaskBusy(ctx, a.ID, m.TaskID)
	if err != nil || busy {
		return false
	}
	if action == PREndActionKeep {
		return true
	}
	if e.svc.prTasks == nil {
		return false
	}
	switch action {
	case PREndActionArchive:
		err = e.svc.prTasks.ArchiveAutomationPRTask(ctx, m.TaskID)
	case PREndActionDelete:
		err = e.svc.prTasks.DeleteAutomationPRTask(ctx, m.TaskID, fmt.Sprintf("automation: PR %s", outcome))
	}
	if err != nil {
		e.logger.Warn("failed to apply end-of-PR action", zap.String("automation_id", a.ID),
			zap.String("task_id", m.TaskID), zap.String("action", string(action)), zap.Error(err))
		return false
	}
	return true
}
