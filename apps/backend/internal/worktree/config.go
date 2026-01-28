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

	// BasePath is the base directory for worktree storage.
	// Supports ~ expansion for home directory.
	// Default: ~/.kandev/worktrees
	BasePath string `mapstructure:"base_path"`

	// BranchPrefix is the prefix used for worktree branch names.
	// Default: feature/
	BranchPrefix string `mapstructure:"branch_prefix"`
}

// DefaultBranchPrefix is used when no repository-specific prefix is provided.
const DefaultBranchPrefix = "feature/"

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	if c.BranchPrefix == "" {
		c.BranchPrefix = DefaultBranchPrefix
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

// NormalizeBranchPrefix trims and falls back to the default prefix.
func NormalizeBranchPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return DefaultBranchPrefix
	}
	return trimmed
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
