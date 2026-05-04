package health

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// GitHubStatusProvider abstracts the GitHub service status check.
type GitHubStatusProvider interface {
	IsAuthenticated() bool
	AuthMethod() string
}

// GitHubRateLimitProvider exposes per-resource exhaustion state so the health
// dialog can surface "GitHub API rate limit exhausted" alongside other setup
// issues. Implemented by *github.Service.
type GitHubRateLimitProvider interface {
	ExhaustedRateLimits() []GitHubRateLimitStatus
}

// GitHubRateLimitStatus is a minimal DTO so the health package does not have
// to import the github package (which would create a cycle).
type GitHubRateLimitStatus struct {
	Resource string
	ResetAt  time.Time
}

// GitHubChecker checks GitHub CLI/auth availability and rate-limit health.
type GitHubChecker struct {
	provider     GitHubStatusProvider
	rateProvider GitHubRateLimitProvider
}

// NewGitHubChecker creates a checker for GitHub integration status.
// The provider may be nil if the GitHub service was not initialized.
func NewGitHubChecker(provider GitHubStatusProvider) *GitHubChecker {
	return &GitHubChecker{provider: provider}
}

// WithRateLimitProvider attaches a provider so the checker also surfaces
// rate-limit exhaustion. Optional — when nil the rate-limit issue is omitted.
func (c *GitHubChecker) WithRateLimitProvider(p GitHubRateLimitProvider) *GitHubChecker {
	c.rateProvider = p
	return c
}

func (c *GitHubChecker) Check(_ context.Context) []Issue {
	if c.provider == nil {
		return []Issue{{
			ID:       "github_unavailable",
			Category: "github",
			Title:    "GitHub integration unavailable",
			Message:  "Install the gh CLI and run 'gh auth login', or add a GITHUB_TOKEN secret.",
			Severity: SeverityWarning,
			FixURL:   "/settings/workspace/{workspaceId}/github",
			FixLabel: "Configure GitHub",
		}}
	}
	if !c.provider.IsAuthenticated() {
		return []Issue{{
			ID:       "github_not_authenticated",
			Category: "github",
			Title:    "GitHub not authenticated",
			Message:  "Run 'gh auth login' or add a GITHUB_TOKEN secret.",
			Severity: SeverityWarning,
			FixURL:   "/settings/workspace/{workspaceId}/github",
			FixLabel: "Configure GitHub",
		}}
	}
	return c.rateLimitIssues()
}

// rateLimitIssues materializes one Issue per exhausted resource bucket.
// Returns nil when the rate-limit provider is missing or every bucket has
// quota — the issue auto-clears as soon as that happens.
func (c *GitHubChecker) rateLimitIssues() []Issue {
	if c.rateProvider == nil {
		return nil
	}
	exhausted := c.rateProvider.ExhaustedRateLimits()
	if len(exhausted) == 0 {
		return nil
	}
	issues := make([]Issue, 0, len(exhausted))
	for _, status := range exhausted {
		issues = append(issues, Issue{
			ID:       fmt.Sprintf("github_rate_limit_%s", status.Resource),
			Category: "github",
			Title:    fmt.Sprintf("GitHub API rate limit exhausted (%s)", status.Resource),
			Message:  rateLimitMessage(status),
			Severity: SeverityWarning,
			FixURL:   "/settings/workspace/{workspaceId}/github",
			FixLabel: "View status",
		})
	}
	return issues
}

func rateLimitMessage(status GitHubRateLimitStatus) string {
	if status.ResetAt.IsZero() {
		return "PR/issue checks are paused until the limit resets."
	}
	wait := time.Until(status.ResetAt).Round(time.Minute)
	if wait <= 0 {
		return "PR/issue checks are paused until the limit resets."
	}
	return fmt.Sprintf("PR/issue checks are paused; resets in %s.", wait)
}

// AgentDiscoveryProvider abstracts the agent discovery check.
type AgentDiscoveryProvider interface {
	HasAvailableAgents(ctx context.Context) (bool, error)
}

// AgentChecker checks whether any AI agents are detected.
type AgentChecker struct {
	provider AgentDiscoveryProvider
}

// NewAgentChecker creates a checker for agent availability.
func NewAgentChecker(provider AgentDiscoveryProvider) *AgentChecker {
	return &AgentChecker{provider: provider}
}

const agentCheckTimeout = 10 * time.Second

func (c *AgentChecker) Check(ctx context.Context) []Issue {
	if c.provider == nil {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, agentCheckTimeout)
	defer cancel()
	available, err := c.provider.HasAvailableAgents(checkCtx)
	if err != nil {
		title := "Agent detection failed"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			title = "Agent detection timed out"
		}
		return []Issue{{
			ID:       "agent_detection_failed",
			Category: "agents",
			Title:    title,
			Message:  "Could not verify agent installations. Check Settings > Agents for details.",
			Severity: SeverityWarning,
			FixURL:   "/settings/agents",
			FixLabel: "Check Agents",
		}}
	}
	if !available {
		return []Issue{{
			ID:       "no_agents",
			Category: "agents",
			Title:    "No AI agents detected",
			Message:  "Install an AI coding agent (e.g. Claude Code, Codex) to start using KanDev.",
			Severity: SeverityWarning,
			FixURL:   "/settings/agents",
			FixLabel: "Configure Agents",
		}}
	}
	return nil
}
