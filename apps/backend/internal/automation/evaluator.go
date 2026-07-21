package automation

import (
	"context"
	"encoding/json"
	"fmt"
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
	svc      *Service
	ghClient github.Client
	logger   *logger.Logger

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewGitHubEvaluator creates a new GitHub trigger evaluator.
func NewGitHubEvaluator(svc *Service, ghClient github.Client, log *logger.Logger) *GitHubEvaluator {
	return &GitHubEvaluator{
		svc:      svc,
		ghClient: ghClient,
		logger:   log,
	}
}

// Start begins the polling loop.
func (e *GitHubEvaluator) Start(ctx context.Context) {
	if e.started || e.ghClient == nil {
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
	e.evaluatePushTriggers(ctx)
	e.evaluateCITriggers(ctx)
}

func (e *GitHubEvaluator) evaluatePRTriggers(ctx context.Context) {
	triggers, err := e.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeGitHubPR)
	if err != nil {
		e.logger.Error("failed to list github_pr triggers", zap.Error(err))
		return
	}
	for i := range triggers {
		t := &triggers[i]
		e.checkPRTrigger(ctx, t)
	}
}

func (e *GitHubEvaluator) checkPRTrigger(ctx context.Context, t *AutomationTrigger) {
	var cfg GitHubPRTriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		e.logger.Debug("invalid github_pr trigger config",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}

	repos := cfg.Repos
	if len(repos) == 0 {
		return // Cannot poll without specific repos
	}

	for _, repo := range repos {
		// Use ListReviewRequestedPRs with a custom query scoped to this repo.
		query := fmt.Sprintf("is:pr is:open repo:%s/%s", repo.Owner, repo.Name)
		prs, err := e.ghClient.ListReviewRequestedPRs(ctx, "", "", query)
		if err != nil {
			e.logger.Debug("failed to list PRs for automation trigger",
				zap.String("repo", repo.Owner+"/"+repo.Name),
				zap.Error(err))
			continue
		}
		for _, pr := range prs {
			if pr == nil {
				continue
			}
			if cfg.ExcludeDraft && pr.Draft {
				continue
			}
			if !matchesBranches(pr.BaseBranch, cfg.Branches) {
				continue
			}
			if !matchesAuthors(pr.AuthorLogin, cfg.Authors) {
				continue
			}

			dedupKey := fmt.Sprintf("pr:%s/%s#%d", repo.Owner, repo.Name, pr.Number)
			e.firePRTrigger(ctx, t, pr, dedupKey)
		}
	}
}

func (e *GitHubEvaluator) firePRTrigger(ctx context.Context, t *AutomationTrigger, pr *github.PR, dedupKey string) {
	data, _ := json.Marshal(map[string]interface{}{
		"number":       pr.Number,
		"title":        pr.Title,
		"html_url":     pr.HTMLURL,
		"author_login": pr.AuthorLogin,
		"repo":         fmt.Sprintf("%s/%s", pr.RepoOwner, pr.RepoName),
		"head_branch":  pr.HeadBranch,
		"base_branch":  pr.BaseBranch,
		"body":         pr.Body,
		"draft":        pr.Draft,
		"state":        pr.State,
	})

	if err := e.svc.FireTrigger(ctx, t.AutomationID, t.ID, TriggerTypeGitHubPR, data, dedupKey); err != nil {
		e.logger.Error("failed to fire PR trigger",
			zap.String("trigger_id", t.ID), zap.Error(err))
	}
}

func (e *GitHubEvaluator) evaluatePushTriggers(ctx context.Context) {
	triggers, err := e.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeGitHubPush)
	if err != nil {
		e.logger.Error("failed to list github_push triggers", zap.Error(err))
		return
	}
	for i := range triggers {
		t := &triggers[i]
		e.checkPushTrigger(ctx, t)
	}
}

func (e *GitHubEvaluator) checkPushTrigger(ctx context.Context, t *AutomationTrigger) {
	var cfg GitHubPushTriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		return
	}
	// Push trigger evaluation requires comparing commit SHAs.
	// For now, mark as evaluated; full implementation needs commit tracking.
	now := time.Now().UTC()
	_ = e.svc.Store().UpdateTriggerEvaluatedAt(ctx, t.ID, now)
}

func (e *GitHubEvaluator) evaluateCITriggers(ctx context.Context) {
	triggers, err := e.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeGitHubCI)
	if err != nil {
		e.logger.Error("failed to list github_ci triggers", zap.Error(err))
		return
	}
	for i := range triggers {
		t := &triggers[i]
		e.checkCITrigger(ctx, t)
	}
}

func (e *GitHubEvaluator) checkCITrigger(ctx context.Context, t *AutomationTrigger) {
	var cfg GitHubCITriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		return
	}
	// CI trigger evaluation requires tracking check run completion.
	// For now, mark as evaluated; full implementation needs check run tracking.
	now := time.Now().UTC()
	_ = e.svc.Store().UpdateTriggerEvaluatedAt(ctx, t.ID, now)
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

// matchesAuthors checks if an author matches the filter list.
// Empty filter means match all.
func matchesAuthors(author string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f == author {
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
