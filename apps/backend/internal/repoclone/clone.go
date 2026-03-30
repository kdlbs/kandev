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

// ghCredentialHelper is the git credential helper command that delegates to gh CLI.
const ghCredentialHelper = "!gh auth git-credential"

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

// NewCloner creates a new Cloner with the given config, git protocol, and data directory.
// If cfg.BasePath is empty, it defaults to dataDir+"/repos".
func NewCloner(cfg Config, protocol string, dataDir string, log *logger.Logger) *Cloner {
	if cfg.BasePath == "" && dataDir != "" {
		cfg.BasePath = filepath.Join(dataDir, "repos")
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

// BuildCloneURL constructs a protocol-aware clone URL for the given provider/owner/name.
// This ensures the clone URL matches the user's configured git protocol (SSH vs HTTPS).
func (c *Cloner) BuildCloneURL(provider, owner, name string) (string, error) {
	return CloneURL(provider, owner, name, c.protocol)
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

	return targetPath, c.clone(ctx, cloneURL, targetPath, owner, name)
}

// gitCmd creates a git command with non-interactive environment settings.
// When the configured protocol is HTTPS, it adds gh CLI as the credential
// helper so that git can authenticate using the user's gh auth token.
func (c *Cloner) gitCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	env := os.Environ()
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if c.protocol == ProtocolHTTPS {
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=credential.helper",
			"GIT_CONFIG_VALUE_0="+ghCredentialHelper,
		)
	}
	cmd.Env = env
	return cmd
}

func (c *Cloner) fetch(ctx context.Context, repoPath string) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd := c.gitCmd(ctx, "-C", repoPath, "fetch", "--all", "--prune", "--force")
	if out, err := cmd.CombinedOutput(); err != nil {
		c.logger.Warn("git fetch failed (non-fatal)",
			zap.String("path", repoPath),
			zap.String("output", string(out)),
			zap.Error(err))
	}
}

func (c *Cloner) clone(ctx context.Context, cloneURL, targetPath, owner, name string) error {
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	c.logger.Info("cloning repository",
		zap.String("url", cloneURL),
		zap.String("target", targetPath))

	// Try gh repo clone first — handles auth for both SSH and HTTPS.
	if owner != "" && name != "" {
		if err := c.ghClone(ctx, owner, name, targetPath); err == nil {
			return nil
		}
		// Clean up any partial clone so the fallback can retry into a fresh path.
		if rmErr := os.RemoveAll(targetPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("cleanup failed gh clone target: %w", rmErr)
		}
	}

	// Fallback: git clone with credential helper.
	cmd := c.gitCmd(ctx, "clone", cloneURL, targetPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}
	return nil
}

// ghClone attempts to clone using gh repo clone, which handles authentication
// automatically via the user's gh CLI session.
func (c *Cloner) ghClone(ctx context.Context, owner, name, targetPath string) error {
	nwo := owner + "/" + name
	cmd := exec.CommandContext(ctx, "gh", "repo", "clone", nwo, targetPath)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.logger.Debug("gh repo clone failed, falling back to git clone",
			zap.String("repo", nwo),
			zap.String("output", string(out)),
			zap.Error(err))
		return fmt.Errorf("gh repo clone: %w", err)
	}
	return nil
}
