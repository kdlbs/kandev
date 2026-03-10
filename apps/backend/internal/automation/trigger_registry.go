package automation

import "encoding/json"

// PlaceholderInfo describes a single {{key}} placeholder available for a trigger type.
type PlaceholderInfo struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

// TriggerTypeInfo describes a trigger type and its associated metadata.
// Served to clients so they can build UIs dynamically.
type TriggerTypeInfo struct {
	Type             TriggerType       `json:"type"`
	Label            string            `json:"label"`
	Description      string            `json:"description"`
	Category         string            `json:"category"` // "schedule", "github", "webhook"
	Enabled          bool              `json:"enabled"`
	Placeholders     []PlaceholderInfo `json:"placeholders"`
	DefaultPrompt    string            `json:"default_prompt"`
	DefaultTaskTitle string            `json:"default_task_title"`
	DefaultConfig    json.RawMessage   `json:"default_config"`
}

// Common placeholders available for every trigger type.
var commonPlaceholders = []PlaceholderInfo{
	{Key: "trigger.type", Description: "Trigger type (scheduled, github_pr, webhook, etc.)", Example: "scheduled"},
	{Key: "trigger.timestamp", Description: "When the trigger fired", Example: "2026-03-08T12:00:00Z"},
	{Key: "data.*", Description: "Access any field from trigger data", Example: "data.action"},
}

var triggerTypeRegistry = []TriggerTypeInfo{
	{
		Type:             TriggerTypeScheduled,
		Label:            "Scheduled",
		Description:      "Runs on a cron schedule",
		Category:         "schedule",
		Enabled:          true,
		Placeholders:     commonPlaceholders,
		DefaultPrompt:    "Run scheduled automation.\n\nTrigger: {{trigger.type}}",
		DefaultTaskTitle: "",
		DefaultConfig:    json.RawMessage(`{"cron_expression":"0 */6 * * *","timezone":"UTC"}`),
	},
	{
		Type:        TriggerTypeGitHubPR,
		Label:       "New pull requests",
		Description: "Polls GitHub for PRs matching your filters (repo, branch, author, labels)",
		Category:    "github",
		Enabled:     true,
		Placeholders: append([]PlaceholderInfo{
			{Key: "pr.number", Description: "Pull request number", Example: "42"},
			{Key: "pr.title", Description: "Pull request title", Example: "Fix the bug"},
			{Key: "pr.url", Description: "Pull request URL", Example: "https://github.com/org/repo/pull/42"},
			{Key: "pr.author", Description: "PR author login", Example: "alice"},
			{Key: "pr.repo", Description: "Repository (owner/name)", Example: "org/repo"},
			{Key: "pr.branch", Description: "Head branch name", Example: "fix-bug"},
			{Key: "pr.base_branch", Description: "Base branch name", Example: "main"},
			{Key: "pr.body", Description: "Pull request body text", Example: "Fixes #123"},
		}, commonPlaceholders...),
		DefaultPrompt:    "Review PR #{{pr.number}} in {{pr.repo}}: {{pr.title}}\n\n{{pr.body}}\n\nBranch: {{pr.branch}} → {{pr.base_branch}}",
		DefaultTaskTitle: "[Auto] {{pr.repo}}#{{pr.number}} — {{pr.title}}",
		DefaultConfig:    json.RawMessage(`{"events":["opened"],"repos":[],"exclude_draft":false}`),
	},
	{
		Type:        TriggerTypeGitHubPush,
		Label:       "Push to branch",
		Description: "Triggers when commits are pushed to matching branches",
		Category:    "github",
		Enabled:     false,
		Placeholders: append([]PlaceholderInfo{
			{Key: "push.branch", Description: "Branch that was pushed to", Example: "main"},
			{Key: "push.repo", Description: "Repository (owner/name)", Example: "org/repo"},
			{Key: "push.sha", Description: "Commit SHA", Example: "abc1234"},
			{Key: "push.message", Description: "Commit message", Example: "feat: add feature"},
		}, commonPlaceholders...),
		DefaultPrompt:    "Review push to {{push.branch}} in {{push.repo}}\n\nCommit: {{push.sha}}\n{{push.message}}",
		DefaultTaskTitle: "[Auto] Push to {{push.branch}} — {{push.repo}}",
		DefaultConfig:    json.RawMessage(`{"repos":[],"branches":["main"]}`),
	},
	{
		Type:        TriggerTypeGitHubCI,
		Label:       "CI check result",
		Description: "Triggers when a CI check completes with matching conclusion",
		Category:    "github",
		Enabled:     false,
		Placeholders: append([]PlaceholderInfo{
			{Key: "ci.check_name", Description: "CI check/workflow name", Example: "build"},
			{Key: "ci.conclusion", Description: "CI conclusion", Example: "failure"},
			{Key: "ci.repo", Description: "Repository (owner/name)", Example: "org/repo"},
			{Key: "ci.url", Description: "CI run URL", Example: "https://github.com/..."},
		}, commonPlaceholders...),
		DefaultPrompt:    "CI check '{{ci.check_name}}' {{ci.conclusion}} in {{ci.repo}}\n\n{{ci.url}}",
		DefaultTaskTitle: "[Auto] CI {{ci.conclusion}} — {{ci.check_name}}",
		DefaultConfig:    json.RawMessage(`{"repos":[],"conclusions":["failure"]}`),
	},
	{
		Type:        TriggerTypeWebhook,
		Label:       "Webhook",
		Description: "Triggers when an HTTP POST is received at the automation's webhook URL",
		Category:    "webhook",
		Enabled:     true,
		Placeholders: append([]PlaceholderInfo{
			{Key: "webhook.body", Description: "Full webhook request body (JSON)", Example: `{"event":"deploy"}`},
		}, commonPlaceholders...),
		DefaultPrompt:    "Process webhook event.\n\n{{webhook.body}}",
		DefaultTaskTitle: "",
		DefaultConfig:    json.RawMessage(`{}`),
	},
}

// GetTriggerTypes returns metadata for all known trigger types.
func GetTriggerTypes() []TriggerTypeInfo {
	return triggerTypeRegistry
}

// GetTriggerTypeInfo returns metadata for a specific trigger type, or nil if not found.
func GetTriggerTypeInfo(t TriggerType) *TriggerTypeInfo {
	for i := range triggerTypeRegistry {
		if triggerTypeRegistry[i].Type == t {
			return &triggerTypeRegistry[i]
		}
	}
	return nil
}
