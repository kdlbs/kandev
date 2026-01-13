package worktree

import (
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

	// BasePath is the base directory for worktree storage.
	// Supports ~ expansion for home directory.
	// Default: ~/.kandev/worktrees
	BasePath string `mapstructure:"base_path"`

	// MaxPerRepo is the maximum number of concurrent worktrees per repository.
	// Default: 10
	MaxPerRepo int `mapstructure:"max_per_repo"`

	// BranchPrefix is the prefix used for worktree branch names.
	// Default: kandev/
	BranchPrefix string `mapstructure:"branch_prefix"`
}

// DefaultConfig returns the default worktree configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:      true,
		BasePath:     "~/.kandev/worktrees",
		MaxPerRepo:   10,
		BranchPrefix: "kandev/",
	}
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	if c.MaxPerRepo <= 0 {
		c.MaxPerRepo = 10
	}
	if c.BranchPrefix == "" {
		c.BranchPrefix = "kandev/"
	}
	if c.BasePath == "" {
		c.BasePath = "~/.kandev/worktrees"
	}
	return nil
}

// ExpandedBasePath returns the base path with ~ expanded to the user's home directory.
func (c *Config) ExpandedBasePath() (string, error) {
	path := c.BasePath
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return path, nil
}

// WorktreePath returns the full path for a worktree given a unique worktree ID.
func (c *Config) WorktreePath(worktreeID string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(basePath, worktreeID), nil
}

// BranchName returns the branch name for a given task ID and suffix.
// Format: {prefix}{taskID}_{suffix} e.g. kandev/abc123_xy7z
func (c *Config) BranchName(taskID, suffix string) string {
	return c.BranchPrefix + taskID + "_" + suffix
}

// SemanticBranchName returns a branch name using a semantic name derived from task title.
// Format: {prefix}{semanticName}_{suffix} e.g. kandev/fix-login-bug_ab12cd34
func (c *Config) SemanticBranchName(semanticName, suffix string) string {
	return c.BranchPrefix + semanticName + "_" + suffix
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
	re := regexp.MustCompile(`-+`)
	result = re.ReplaceAllString(result, "-")

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

