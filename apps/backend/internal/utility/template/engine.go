package template

import (
	"regexp"
	"strings"
)

// Context holds the data available for template resolution.
type Context struct {
	// Git-related
	GitDiff      string // Output of git diff --staged or git diff
	CommitLog    string // Output of git log for the branch
	ChangedFiles string // List of changed files
	DiffSummary  string // Summary of changes (insertions, deletions)
	BranchName   string // Current branch name
	BaseBranch   string // Base branch (main/master)

	// Task-related
	TaskTitle       string // Title of the current task
	TaskDescription string // Description of the current task

	// Session-related
	SessionID     string // Current session ID
	WorkspacePath string // Path to the workspace/worktree

	// User input
	UserPrompt string // User's original prompt text (for enhance-prompt)

	// Conversation
	ConversationHistory string // Formatted conversation transcript (for summarize-session)

	// Custom key-value pairs for extensibility
	Custom map[string]string
}

// Engine resolves template variables in prompts.
type Engine struct{}

// NewEngine creates a new template engine.
func NewEngine() *Engine {
	return &Engine{}
}

// templatePattern matches {{Key}} placeholders.
var templatePattern = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)

// Resolve resolves template variables in the given prompt using the provided context.
// Uses {{Key}} format (no dot) for consistency with scriptengine.
// Unknown placeholders are left unchanged.
func (e *Engine) Resolve(promptTemplate string, ctx *Context) (string, error) {
	return e.ResolveWithOptions(promptTemplate, ctx, ResolveOptions{})
}

// ResolveOptions configures template resolution.
type ResolveOptions struct {
	// MissingAsEmpty substitutes unknown placeholders with an empty string
	// instead of leaving them as {{Var}}. Used for sessionless utility execution
	// where some context variables (e.g. GitDiff, TaskTitle) are meaningless.
	MissingAsEmpty bool
}

// ResolveWithOptions resolves template variables with the given options.
func (e *Engine) ResolveWithOptions(promptTemplate string, ctx *Context, opts ResolveOptions) (string, error) {
	if ctx == nil {
		ctx = &Context{}
	}
	data := e.buildTemplateData(ctx)

	result := templatePattern.ReplaceAllStringFunc(promptTemplate, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		if val, ok := data[key]; ok {
			return val
		}
		if opts.MissingAsEmpty {
			return ""
		}
		return match
	})

	return result, nil
}

// buildTemplateData creates the data map for template execution.
func (e *Engine) buildTemplateData(ctx *Context) map[string]string {
	data := map[string]string{
		"GitDiff":             ctx.GitDiff,
		"CommitLog":           ctx.CommitLog,
		"ChangedFiles":        ctx.ChangedFiles,
		"DiffSummary":         ctx.DiffSummary,
		"BranchName":          ctx.BranchName,
		"BaseBranch":          ctx.BaseBranch,
		"TaskTitle":           ctx.TaskTitle,
		"TaskDescription":     ctx.TaskDescription,
		"SessionID":           ctx.SessionID,
		"WorkspacePath":       ctx.WorkspacePath,
		"UserPrompt":          ctx.UserPrompt,
		"ConversationHistory": ctx.ConversationHistory,
	}

	// Add custom key-value pairs
	for k, v := range ctx.Custom {
		data[k] = v
	}

	return data
}

// AvailableVariables returns the list of available template variables.
// Integration-specific variables (Slack, future webhooks, etc.) are listed
// in their own category so the UI can group them and users can tell which
// ones make sense in a given utility-agent context.
func (e *Engine) AvailableVariables() []VariableInfo {
	const categorySlack = "slack"
	return []VariableInfo{
		{Name: "GitDiff", Description: "Output of git diff --staged or git diff", Example: "diff --git a/main.go...", Category: "git"},
		{Name: "CommitLog", Description: "Git log for the current branch", Example: "abc123 Fix bug in handler", Category: "git"},
		{Name: "ChangedFiles", Description: "List of changed files", Example: "src/main.go\nsrc/utils.go", Category: "git"},
		{Name: "DiffSummary", Description: "Summary of changes (files, insertions, deletions)", Example: "3 files, +50 -10", Category: "git"},
		{Name: "BranchName", Description: "Current git branch name", Example: "feature/add-login", Category: "git"},
		{Name: "BaseBranch", Description: "Base branch (main/master/develop)", Example: "main", Category: "git"},
		{Name: "TaskTitle", Description: "Title of the current task", Example: "Add user authentication", Category: "task"},
		{Name: "TaskDescription", Description: "Description of the current task", Example: "Implement OAuth2 login flow", Category: "task"},
		{Name: "SessionID", Description: "Current session ID", Example: "sess_abc123", Category: "session"},
		{Name: "WorkspacePath", Description: "Path to the workspace/worktree", Example: "/home/user/project", Category: "session"},
		{Name: "UserPrompt", Description: "User's original prompt text", Example: "Fix the login bug", Category: "input"},
		{Name: "ConversationHistory", Description: "Formatted conversation transcript", Example: "User: Fix the bug\nAgent: I'll look into...", Category: "session"},
		{Name: "SlackInstruction", Description: "User's instruction (the !kandev message minus the prefix)", Example: "investigate the safari login bug", Category: categorySlack},
		{Name: "SlackThread", Description: "Surrounding Slack thread, one line per message", Example: "alice (1714658000.0): bug repros every Monday\nbob (1714659000.0): seeing it on safari only", Category: categorySlack},
		{Name: "SlackPermalink", Description: "Canonical URL of the triggering Slack message", Example: "https://acme.slack.com/archives/C1/p1714659000000100", Category: categorySlack},
		{Name: "SlackUser", Description: "Sender of the !kandev message (@name or user id)", Example: "@alice", Category: categorySlack},
		{Name: "SlackChannelID", Description: "Slack channel id of the trigger message", Example: "C0123ABCD", Category: categorySlack},
		{Name: "SlackTS", Description: "Slack timestamp of the trigger message", Example: "1714659000.000100", Category: categorySlack},
	}
}

// VariableInfo describes a template variable.
type VariableInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Example     string `json:"example"`
	Category    string `json:"category"` // "git", "task", "session", "input", "slack"
}
