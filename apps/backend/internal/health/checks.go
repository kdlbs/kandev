package health

import "context"

// GitHubStatusProvider abstracts the GitHub service status check.
type GitHubStatusProvider interface {
	IsAuthenticated() bool
	AuthMethod() string
}

// GitHubChecker checks GitHub CLI/auth availability.
type GitHubChecker struct {
	provider GitHubStatusProvider
}

// NewGitHubChecker creates a checker for GitHub integration status.
// The provider may be nil if the GitHub service was not initialized.
func NewGitHubChecker(provider GitHubStatusProvider) *GitHubChecker {
	return &GitHubChecker{provider: provider}
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
	return nil
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

func (c *AgentChecker) Check(ctx context.Context) []Issue {
	if c.provider == nil {
		return nil
	}
	available, err := c.provider.HasAvailableAgents(ctx)
	if err != nil {
		return nil
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
