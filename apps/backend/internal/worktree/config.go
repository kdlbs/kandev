package worktree

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
// Format: {tasksBase}/{taskDirName}/{repoName}
func (c *Config) TaskWorktreePath(taskDirName, repoName string) (string, error) {
	basePath, err := c.ExpandedTasksBasePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(basePath, taskDirName, repoName), nil
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

// SanitizeForBranch converts a task title into a valid git branch name component.
// It:
// - Converts to lowercase
// - Replaces spaces and special characters with hyphens
// - Removes consecutive hyphens
// - Truncates to maxLen characters
// - Removes leading/trailing hyphens
func SanitizeForBranch(title string, maxLen int) string {
	if title == "" {
		return ""
	}

	// Convert to lowercase
	result := strings.ToLower(title)

	// Replace any character that's not alphanumeric with a hyphen
	// Git branch names allow: a-z, A-Z, 0-9, /, ., _, -
	// We'll use only alphanumeric and hyphens for cleaner names
	var sb strings.Builder
	for _, r := range result {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
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

// TaskBranchName returns the standard per-task branch name used by isolated
// executors. The random suffix keeps similarly titled tasks from colliding.
func TaskBranchName(taskTitle, taskID, prefix string) string {
	return TaskBranchNameWithSuffix(taskTitle, taskID, prefix, SmallSuffix(3))
}

// TaskBranchNameWithSuffix is the deterministic form of TaskBranchName used by
// worktree naming, where the caller already generated the suffix.
func TaskBranchNameWithSuffix(taskTitle, taskID, prefix, suffix string) string {
	sanitized := SanitizeForBranch(taskTitle, 20)
	if sanitized == "" {
		sanitized = SanitizeForBranch(taskID, 20)
	}
	if sanitized == "" {
		sanitized = "task"
	}
	return NormalizeBranchPrefix(prefix) + sanitized + "-" + suffix
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
		case unicode.IsLetter(r), unicode.IsDigit(r):
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
