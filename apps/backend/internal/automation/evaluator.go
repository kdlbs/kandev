package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
)

const defaultGitHubPollInterval = 60 * time.Second

// GitHubEvaluator polls GitHub for events matching automation triggers.
// It handles github_pr, github_push, and github_ci trigger types.
type GitHubEvaluator struct {
	svc           *Service
	githubService *github.Service
	logger        *logger.Logger
	now           func() time.Time

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewGitHubEvaluator creates a new GitHub trigger evaluator.
func NewGitHubEvaluator(svc *Service, githubService *github.Service, log *logger.Logger) *GitHubEvaluator {
	return &GitHubEvaluator{
		svc:           svc,
		githubService: githubService,
		logger:        log,
		now:           time.Now,
	}
}

// Start begins the polling loop.
func (e *GitHubEvaluator) Start(ctx context.Context) {
	if e.started || e.githubService == nil {
		return
	}
	e.started = true
	ctx, e.cancel = context.WithCancel(ctx)

	e.wg.Add(1)
	go e.loop(ctx)

	e.logger.Info("automation GitHub evaluator started")
}

// Stop cancels the polling loop and waits for it to finish.
func (e *GitHubEvaluator) Stop() {
	if !e.started {
		return
	}
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	e.started = false
	e.logger.Info("automation GitHub evaluator stopped")
}

func (e *GitHubEvaluator) loop(ctx context.Context) {
	defer e.wg.Done()

	e.evaluate(ctx)

	ticker := time.NewTicker(defaultGitHubPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluate(ctx)
		}
	}
}

func (e *GitHubEvaluator) evaluate(ctx context.Context) {
	e.evaluatePRTriggers(ctx)
}

func (e *GitHubEvaluator) evaluatePRTriggers(ctx context.Context) {
	triggers, err := e.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeGitHubPR)
	if err != nil {
		e.logger.Error("failed to list github_pr triggers", zap.Error(err))
		return
	}
	cycle := prRepoCycle{}
	for i := range triggers {
		t := &triggers[i]
		e.checkPRTrigger(ctx, t, cycle)
	}
}

func (e *GitHubEvaluator) checkPRTrigger(ctx context.Context, t *AutomationTrigger, cycle prRepoCycle) {
	var cfg GitHubPRTriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		e.logger.Debug("invalid github_pr trigger config",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}
	sel := selectPREvents(cfg.Events)
	if !sel.any() {
		return
	}
	if len(cfg.Repos) == 0 {
		return // Cannot poll without specific repos
	}
	automation, err := e.svc.GetAutomation(ctx, t.AutomationID)
	if err != nil || automation == nil || automation.WorkspaceID == "" {
		e.logger.Debug("automation trigger is missing workspace ownership",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}
	for _, repo := range cfg.Repos {
		e.evaluatePRRepo(ctx, t, automation, &cfg, sel, repo, cycle)
	}
}

func (e *GitHubEvaluator) firePRTrigger(
	ctx context.Context, t *AutomationTrigger, pr *github.PR, owner, repo string, firing *prFiring,
) (FireResult, error) {
	data, _ := json.Marshal(map[string]interface{}{
		automationPRNumberKey:           pr.Number,
		automationTitleKey:              pr.Title,
		automationHTMLURLKey:            pr.HTMLURL,
		automationAuthorLoginKey:        pr.AuthorLogin,
		automationRepoKey:               fmt.Sprintf("%s/%s", owner, repo),
		automationHeadBranchKey:         pr.HeadBranch,
		automationBaseBranchKey:         pr.BaseBranch,
		automationBodyKey:               pr.Body,
		automationHeadSHAKey:            pr.HeadSHA,
		automationPreviousHeadSHAKey:    firing.previousHead,
		automationPREventKey:            firing.event,
		automationRequestedReviewersKey: strings.Join(firing.reviewers, ", "),
		automationPRSubjectKey:          PRSubject(owner, repo, pr.Number),
		"draft":                         pr.Draft,
		"state":                         pr.State,
	})

	result, err := e.svc.FireTrigger(ctx, t.AutomationID, t.ID, TriggerTypeGitHubPR, data, DedupKey(firing.dedupKey))
	if err != nil {
		e.logger.Error("failed to fire PR trigger",
			zap.String("trigger_id", t.ID), zap.String("event", firing.event), zap.Error(err))
	}
	return result, err
}

// matchesBranches checks if a branch matches any of the filter patterns.
// Empty filter means match all.
func matchesBranches(branch string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if matchGlob(f, branch) {
			return true
		}
	}
	return false
}

// matchesRepo checks if owner/name matches any entry in the repo filter list.
// An empty list never matches — webhook-driven push/CI triggers cannot act
// without knowing which repo(s) to scope to.
func matchesRepo(owner, name string, repos []github.RepoFilter) bool {
	for _, r := range repos {
		if r.Owner == owner && r.Name == name {
			return true
		}
	}
	return false
}

// matchesAuthors checks if an author matches the filter list.
// Empty filter means match all.
func matchesAuthors(author string, filters []string) bool {
	return matchesFilterValue(author, filters)
}

// matchesFilterValue reports whether value equals one of the filters, treating
// an empty filter list as "match all". Used for exact-string filters such as
// PR authors, CI conclusions, and CI check names.
func matchesFilterValue(value string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f == value {
			return true
		}
	}
	return false
}

// matchGlob performs simple glob matching (supports * wildcard).
func matchGlob(pattern, s string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == s {
		return true
	}
	// Handle trailing wildcard: "release/*" matches "release/v1"
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(s) >= len(prefix) && s[:len(prefix)] == prefix
	}
	return false
}
