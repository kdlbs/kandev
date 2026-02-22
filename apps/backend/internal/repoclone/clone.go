// Package repoclone handles automatic cloning and fetching of git repositories.
package repoclone

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Config holds configuration for the repository cloner.
type Config struct {
	// BasePath is the base directory for cloned repos.
	// Supports ~ expansion for home directory.
	// Default: ~/.kandev/repos
	BasePath string `mapstructure:"basePath"`
}

// Cloner handles git clone and fetch operations.
type Cloner struct {
	config   Config
	protocol string
	logger   *logger.Logger
	// repoMus is a map of per-repo path → *sync.Mutex to prevent concurrent
	// clone or fetch operations on the same repository directory.
	repoMus sync.Map
}

// NewCloner creates a new Cloner with the given config and git protocol.
func NewCloner(cfg Config, protocol string, log *logger.Logger) *Cloner {
	if cfg.BasePath == "" {
		cfg.BasePath = "~/.kandev/repos"
	}
	return &Cloner{config: cfg, protocol: protocol, logger: log}
}

// repoMu returns (or lazily creates) the mutex for a repository path.
func (c *Cloner) repoMu(path string) *sync.Mutex {
	mu, _ := c.repoMus.LoadOrStore(path, &sync.Mutex{})
	return mu.(*sync.Mutex) //nolint:forcetypeassert // LoadOrStore always stores *sync.Mutex
}

// ExpandedBasePath returns the base path with ~ expanded to the user's home directory.
func (c *Cloner) ExpandedBasePath() (string, error) {
	path := c.config.BasePath
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	return path, nil
}

// RepoPath returns the full local path for a repository.
func (c *Cloner) RepoPath(owner, name string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(basePath, owner, name), nil
}

// EnsureCloned clones the repository if it doesn't exist locally, or fetches if it does.
// The cloneURL is the full git URL (HTTPS or SSH) to clone from.
// Returns the local filesystem path to the repository.
// Concurrent calls for the same repository are serialised to prevent double-clone races.
func (c *Cloner) EnsureCloned(ctx context.Context, cloneURL, owner, name string) (string, error) {
	targetPath, err := c.RepoPath(owner, name)
	if err != nil {
		return "", err
	}

	mu := c.repoMu(targetPath)
	mu.Lock()
	defer mu.Unlock()

	gitDir := filepath.Join(targetPath, ".git")
	if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
		c.fetch(ctx, targetPath)
		return targetPath, nil
	}

	return targetPath, c.clone(ctx, cloneURL, targetPath)
}

func (c *Cloner) fetch(ctx context.Context, repoPath string) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "--all", "--prune")
	if out, err := cmd.CombinedOutput(); err != nil {
		c.logger.Warn("git fetch failed (non-fatal)",
			zap.String("path", repoPath),
			zap.String("output", string(out)),
			zap.Error(err))
	}
}

func (c *Cloner) clone(ctx context.Context, cloneURL, targetPath string) error {
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	c.logger.Info("cloning repository",
		zap.String("url", cloneURL),
		zap.String("target", targetPath))

	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, targetPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}
	return nil
}
