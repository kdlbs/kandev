package worktree

import (
	"os"
	"path/filepath"
	"strings"
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

// WorktreePath returns the full path for a worktree given a task ID.
func (c *Config) WorktreePath(taskID string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(basePath, taskID), nil
}

// BranchName returns the branch name for a given task ID.
func (c *Config) BranchName(taskID string) string {
	return c.BranchPrefix + taskID
}

