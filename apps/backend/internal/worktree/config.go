package worktree

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Config holds configuration for the worktree manager.
type Config struct {
	// Enabled controls whether worktree mode is active.
	Enabled bool `mapstructure:"enabled"`

	// TasksBasePath is the base directory for per-task worktree storage.
	// Each task gets a subdirectory containing one repo worktree (future: multiple).
	// Supports ~ expansion for home directory.
	// Default: ~/.kandev/tasks
	TasksBasePath string `mapstructure:"tasks_base_path"`

	// BranchPrefix is the prefix used for worktree branch names.
	// Default: feature/
	BranchPrefix string `mapstructure:"branch_prefix"`

	// FetchTimeoutSeconds is the timeout for pre-worktree git fetch.
	// If <= 0, manager default is used.
	FetchTimeoutSeconds int `mapstructure:"fetch_timeout_seconds"`

	// PullTimeoutSeconds is the timeout for pre-worktree git pull.
	// If <= 0, manager default is used.
	PullTimeoutSeconds int `mapstructure:"pull_timeout_seconds"`
}

// DefaultBranchPrefix is used when no repository-specific prefix is provided.
const DefaultBranchPrefix = "feature/"

// DefaultBranchNameTemplate is the branch template used for new repositories.
const DefaultBranchNameTemplate = "feature/{title}-{suffix}"

// BranchNameTemplateInput contains the values available to branch name templates.
// Users write literal prefixes directly in Template; {prefix} is not supported.
type BranchNameTemplateInput struct {
	Template string
	TaskID   string
	Title    string
	Ticket   string
	Suffix   string
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	if c.BranchPrefix == "" {
		c.BranchPrefix = DefaultBranchPrefix
	}
	return nil
}

// SetTasksBasePathFallback sets the TasksBasePath from the data directory if not already configured.
func (c *Config) SetTasksBasePathFallback(dataDir string) {
	if c.TasksBasePath == "" && dataDir != "" {
		c.TasksBasePath = filepath.Join(dataDir, "tasks")
	}
}

// ExpandedTasksBasePath returns the tasks base path with ~ expanded to the user's home directory.
func (c *Config) ExpandedTasksBasePath() (string, error) {
	path := c.TasksBasePath
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return path, nil
}

// TaskWorktreePath returns the full path for a worktree inside a task directory.
//
// Layout:
//   - branchSlug == "" → {tasksBase}/{taskDirName}/{repoName}         (legacy single-branch path)
//   - branchSlug != "" → {tasksBase}/{taskDirName}/{repoName}-{slug}  (sibling at task root)
//
// Additional branches sit as siblings of the primary worktree under the task
// root instead of nesting inside it. Nesting (e.g. `<task>/<repo>/<slug>/`)
// places the second worktree INSIDE the first worktree's working tree, which
// pollutes the primary's git scope (file scans, status, etc.) and surfaces a
// surprise subdirectory to the agent working in the primary.
func (c *Config) TaskWorktreePath(taskDirName, repoName, branchSlug string) (string, error) {
	basePath, err := c.ExpandedTasksBasePath()
	if err != nil {
		return "", err
	}
	if branchSlug == "" {
		return filepath.Join(basePath, taskDirName, repoName), nil
	}
	return filepath.Join(basePath, taskDirName, repoName+"-"+branchSlug), nil
}

// BranchName returns the branch name for a given task ID and suffix.
// Format: {prefix}{taskID}-{suffix} e.g. feature/abc123-xyz
func (c *Config) BranchName(taskID, suffix string) string {
	return c.BranchPrefix + taskID + "-" + suffix
}

// SemanticBranchName returns a branch name using a semantic name derived from task title.
// Format: {prefix}{semanticName}-{suffix} e.g. feature/fix-login-bug-abc
func (c *Config) SemanticBranchName(semanticName, suffix string) string {
	return c.BranchPrefix + semanticName + "-" + suffix
}

// isASCIIAlphaNum reports whether r is an ASCII letter or digit. Restricting
// to ASCII (rather than the broader unicode.IsLetter/IsDigit) keeps branch
// and worktree-directory names usable across tools and filesystems — Unicode
// letters such as CJK ideographs, Cyrillic, or Arabic characters are valid
// git refs but break many downstream consumers (see issue #1081).
func isASCIIAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// SanitizeForBranch converts a task title into a valid git branch name component.
// It:
// - Converts to lowercase
// - Replaces spaces and non-ASCII-alphanumeric characters with hyphens
// - Removes consecutive hyphens
// - Truncates to maxLen characters
// - Removes leading/trailing hyphens
func SanitizeForBranch(title string, maxLen int) string {
	if title == "" {
		return ""
	}

	// Convert to lowercase
	result := strings.ToLower(title)

	// Replace any character that's not ASCII-alphanumeric with a hyphen.
	// Git branch names allow a broader character set, but we restrict to ASCII
	// alphanumerics + hyphens for cleaner, portable names.
	var sb strings.Builder
	for _, r := range result {
		if isASCIIAlphaNum(r) {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	result = sb.String()

	// Remove consecutive hyphens
	result = repoDirHyphenRun.ReplaceAllString(result, "-")

	// Remove leading and trailing hyphens
	result = strings.Trim(result, "-")

	// Truncate to maxLen
	if len(result) > maxLen {
		result = result[:maxLen]
		// Remove trailing hyphen after truncation
		result = strings.TrimRight(result, "-")
	}

	return result
}

// NormalizeBranchPrefix trims and falls back to the default prefix.
func NormalizeBranchPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return DefaultBranchPrefix
	}
	return trimmed
}

// NormalizeBranchNameTemplate trims and falls back to the default template.
func NormalizeBranchNameTemplate(template string) string {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return DefaultBranchNameTemplate
	}
	return trimmed
}

// ValidateBranchNameTemplate ensures a branch template can render to a safe git branch.
func ValidateBranchNameTemplate(template string) error {
	_, err := RenderTaskBranchName(BranchNameTemplateInput{
		Template: template,
		TaskID:   "task-123",
		Title:    "Example task",
		Ticket:   "TICKET-123",
		Suffix:   "abc",
	})
	return err
}

// RenderTaskBranchName applies a repository branch-name template and validates
// that the rendered value is safe to pass to git as a local branch name.
func RenderTaskBranchName(input BranchNameTemplateInput) (string, error) {
	template := NormalizeBranchNameTemplate(input.Template)

	title := SanitizeForBranch(input.Title, 20)
	if title == "" {
		title = SanitizeForBranch(input.TaskID, 20)
	}
	if title == "" {
		title = "task"
	}

	titleFull := SanitizeForBranch(input.Title, 80)
	if titleFull == "" {
		titleFull = SanitizeForBranch(input.TaskID, 80)
	}
	if titleFull == "" {
		titleFull = "task"
	}

	taskID := SanitizeForBranch(input.TaskID, 80)
	ticket := SanitizeForBranch(input.Ticket, 80)
	suffix := SanitizeForBranch(input.Suffix, 16)

	replacements := map[string]string{
		"{title}":      title,
		"{title_full}": titleFull,
		"{task_id}":    taskID,
		"{ticket}":     ticket,
		"{issue_key}":  ticket,
		"{suffix}":     suffix,
	}
	rendered := template
	for placeholder, value := range replacements {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}
	rendered = strings.TrimSpace(rendered)
	if !isSafeRenderedBranchName(rendered) {
		return "", fmt.Errorf("invalid branch name")
	}
	return rendered, nil
}

// TaskBranchNameWithSuffix builds the per-task branch name from a caller-supplied suffix.
// The caller is responsible for generating the suffix (e.g. SmallSuffix(3)) so that
// naming is deterministic when the suffix is derived externally.
func TaskBranchNameWithSuffix(taskTitle, taskID, prefix, suffix string) string {
	normalizedPrefix := branchPrefixWithSeparator(prefix)
	branch, err := RenderTaskBranchName(BranchNameTemplateInput{
		Template: normalizedPrefix + "{title}-{suffix}",
		TaskID:   taskID,
		Title:    taskTitle,
		Suffix:   suffix,
	})
	if err == nil {
		return branch
	}
	return normalizedPrefix + "task-" + suffix
}

func branchPrefixWithSeparator(prefix string) string {
	normalizedPrefix := NormalizeBranchPrefix(prefix)
	if !strings.HasSuffix(normalizedPrefix, "/") && !strings.HasSuffix(normalizedPrefix, "-") {
		normalizedPrefix += "-"
	}
	return normalizedPrefix
}

// ValidateBranchPrefix ensures a prefix contains only safe branch characters.
func ValidateBranchPrefix(prefix string) error {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return nil
	}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid branch prefix")
	}
	if strings.Contains(trimmed, "..") || strings.Contains(trimmed, "@{") {
		return fmt.Errorf("invalid branch prefix")
	}
	return nil
}

var renderedBranchNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

func isSafeRenderedBranchName(branch string) bool {
	if branch == "" || len(branch) > 255 {
		return false
	}
	if strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
		return false
	}
	if strings.Contains(branch, "//") || strings.HasSuffix(branch, "/") {
		return false
	}
	for _, component := range strings.Split(branch, "/") {
		if component == "" ||
			strings.HasPrefix(component, ".") ||
			strings.HasSuffix(component, ".") ||
			strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	return renderedBranchNameRegex.MatchString(branch)
}

// TicketForBranchName resolves the stable external ticket value used by branch templates.
func TicketForBranchName(identifier string, metadata map[string]any) string {
	if trimmed := strings.TrimSpace(identifier); trimmed != "" {
		return trimmed
	}
	if value := metadataString(metadata, "jira_issue_key"); value != "" {
		return value
	}
	if value := metadataString(metadata, "linear_issue_identifier"); value != "" {
		return value
	}
	if repo := metadataString(metadata, "issue_repo"); repo != "" {
		if number := metadataNumberString(metadata, "issue_number"); number != "" {
			return SanitizeForBranch(repo+"-"+number, 80)
		}
	}
	if repo := metadataString(metadata, "pr_repo"); repo != "" {
		if number := metadataNumberString(metadata, "pr_number"); number != "" {
			return SanitizeForBranch(repo+"-"+number, 80)
		}
	}
	return ""
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func metadataNumberString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	switch value := metadata[key].(type) {
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatInt(int64(value), 10)
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

const branchSuffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// SmallSuffix returns a random suffix capped at 3 characters.
func SmallSuffix(maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if maxLen > 3 {
		maxLen = 3
	}
	buf := make([]byte, maxLen)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("x", maxLen)
	}
	for i := range buf {
		buf[i] = branchSuffixAlphabet[int(buf[i])%len(branchSuffixAlphabet)]
	}
	return string(buf)
}

// SanitizeRepoDirName converts a repository display name into a single,
// filesystem-safe path segment. Path separators and other unsafe characters
// are replaced with hyphens, runs of hyphens are collapsed, and surrounding
// hyphens/dots are trimmed. Returns an empty string when the input has no
// usable characters.
//
// This guards against names like "owner/repo" producing nested subdirectories
// when used as the worktree directory under a multi-repo task root — the
// extra path level breaks sibling-repo detection in agentctl.
//
// Limitation: distinct names that differ only in unsafe characters (e.g.
// "acme/widget-config" and "acme-widget-config") collapse to the same
// segment. Two such repos in one task would collide on `git worktree add`.
// Acceptable in practice — provider repos (the common case) are uniquely
// identified by owner/name and won't collide with each other.
func SanitizeRepoDirName(name string) string {
	if name == "" {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(name))
	for _, r := range name {
		switch {
		case isASCIIAlphaNum(r):
			sb.WriteRune(r)
		case r == '_', r == '.', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	result := repoDirHyphenRun.ReplaceAllString(sb.String(), "-")
	return strings.Trim(result, "-.")
}

var repoDirHyphenRun = regexp.MustCompile(`-+`)

// SanitizeBranchSlug converts a git branch name into a filesystem-safe single
// path segment for use under the per-repo worktree directory. Forward slashes
// (common in branch names like "feature/foo") collapse to hyphens so the slug
// never introduces extra path levels. Returns the empty string when the input
// has no usable characters — callers MUST treat that as "no nesting" rather
// than as an empty path segment.
//
// Determinism: same branch name always produces the same slug. Two branches
// that differ only in unsafe characters (e.g. "feat/a" and "feat-a") collapse
// to the same slug and would collide on disk; the service layer should reject
// such duplicates before reaching the worktree manager.
func SanitizeBranchSlug(branch string) string {
	if branch == "" {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(branch))
	for _, r := range branch {
		switch {
		case isASCIIAlphaNum(r):
			sb.WriteRune(r)
		case r == '_', r == '.', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	result := repoDirHyphenRun.ReplaceAllString(sb.String(), "-")
	return strings.Trim(result, "-.")
}

// SemanticWorktreeName generates a semantic worktree directory name from a task title.
// Format: {sanitizedTitle}_{suffix} e.g. fix-login-bug_ab12cd34
// The title is truncated to 20 characters before adding the suffix.
func SemanticWorktreeName(taskTitle, suffix string) string {
	semanticName := SanitizeForBranch(taskTitle, 20)
	if semanticName == "" {
		// Fallback to just suffix if title is empty or all special chars
		return suffix
	}
	return semanticName + "_" + suffix
}
