// Package repoclone handles automatic cloning and fetching of git repositories.
package repoclone

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
)

const (
	gitNoTags               = "--no-tags"
	githubProvider          = "github"
	gitHubCredentialEnv     = "KANDEV_REPOCLONE_GITHUB_TOKEN"
	gitHubCredentialUserEnv = "KANDEV_REPOCLONE_GITHUB_USERNAME"
	gitCredentialHelper     = `!f() { if [ "$1" = get ]; then printf '%s\n' "username=$KANDEV_REPOCLONE_GITHUB_USERNAME" "password=$KANDEV_REPOCLONE_GITHUB_TOKEN"; fi; }; f`
	managedWorkspacesDir    = "workspaces"
)

var ErrWorkspaceCredentialUnavailable = errors.New("workspace Git credential is unavailable")

// Config holds configuration for the repository cloner.
type Config struct {
	// BasePath is the base directory for cloned repos.
	// Supports ~ expansion for home directory.
	// Default: ~/.kandev/repos
	BasePath string `mapstructure:"basePath"`
}

// Cloner handles git clone and fetch operations.
type Cloner struct {
	config      Config
	protocol    string
	logger      *logger.Logger
	credentials GitCredentialProvider
	// repoMus is a map of per-repo path → *sync.Mutex to prevent concurrent
	// clone or fetch operations on the same repository directory.
	repoMus sync.Map
}

// GitCredentialProvider resolves the workspace automation identity selected
// for Git transport. Implementations must never return a personal credential.
type GitCredentialProvider interface {
	ResolveGitCredential(
		ctx context.Context,
		workspaceID, provider, owner, name string,
	) (username, password string, err error)
}

// NewCloner creates a new Cloner with the given config, git protocol, and data directory.
// If cfg.BasePath is empty, it defaults to dataDir+"/repos".
func NewCloner(cfg Config, protocol string, dataDir string, log *logger.Logger) *Cloner {
	if cfg.BasePath == "" && dataDir != "" {
		cfg.BasePath = filepath.Join(dataDir, "repos")
	}
	return &Cloner{config: cfg, protocol: protocol, logger: log}
}

// SetGitCredentialProvider configures workspace-scoped Git transport auth.
func (c *Cloner) SetGitCredentialProvider(provider GitCredentialProvider) {
	c.credentials = provider
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

// WorkspaceRepoPath returns the isolated local path for a managed repository.
func (c *Cloner) WorkspaceRepoPath(workspaceID, provider, owner, name string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = githubProvider
	}
	segments := []string{workspaceID, provider, owner, name}
	for _, segment := range segments {
		if err := validatePathSegment(segment); err != nil {
			return "", err
		}
	}
	return filepath.Join(basePath, managedWorkspacesDir, workspaceID, provider, owner, name), nil
}

func validatePathSegment(segment string) error {
	if segment == "" || segment == "." || segment == ".." || filepath.Clean(segment) != segment ||
		strings.ContainsAny(segment, `/\\`) {
		return fmt.Errorf("invalid managed clone path segment %q", segment)
	}
	return nil
}

// ShouldRecloneForWorkspace reports whether path is managed by this cloner but
// belongs to the legacy shared layout or another workspace.
func (c *Cloner) ShouldRecloneForWorkspace(workspaceID, path string) bool {
	basePath, err := c.ExpandedBasePath()
	if err != nil || path == "" {
		return false
	}
	managed, err := pathWithin(basePath, path)
	if err != nil || !managed {
		return false
	}
	if err := validatePathSegment(workspaceID); err != nil {
		return true
	}
	workspaceRoot := filepath.Join(basePath, managedWorkspacesDir, workspaceID)
	isolated, isolatedErr := pathWithin(workspaceRoot, path)
	return isolatedErr != nil || !isolated
}

func pathWithin(root, path string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

// EnsureWorkspaceCloned clones or fetches a repository using only the
// workspace's configured automation identity. Managed GitHub clones never use
// the host's gh session, global git credential helpers, or SSH agent.
func (c *Cloner) EnsureWorkspaceCloned(
	ctx context.Context,
	workspaceID, provider, cloneURL, owner, name string,
) (string, error) {
	targetPath, err := c.WorkspaceRepoPath(workspaceID, provider, owner, name)
	if err != nil {
		return "", err
	}
	cloneURL, auth, err := c.workspaceCloneAuth(ctx, workspaceID, provider, cloneURL, owner, name)
	if err != nil {
		return "", err
	}

	mu := c.repoMu(targetPath)
	mu.Lock()
	defer mu.Unlock()

	gitDir := filepath.Join(targetPath, ".git")
	if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
		c.fetch(ctx, targetPath, auth)
		return targetPath, nil
	}

	return targetPath, c.clone(ctx, cloneURL, targetPath, auth)
}

type cloneAuth struct {
	host     string
	username string
	password string
}

func (c *Cloner) workspaceCloneAuth(
	ctx context.Context,
	workspaceID, provider, cloneURL, owner, name string,
) (string, *cloneAuth, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = githubProvider
	}
	httpsURL, err := CloneURL(provider, owner, name, ProtocolHTTPS)
	if err != nil {
		return "", nil, err
	}
	if provider != githubProvider {
		return httpsURL, nil, nil
	}
	if c.credentials == nil {
		return "", nil, ErrWorkspaceCredentialUnavailable
	}
	username, password, err := c.credentials.ResolveGitCredential(ctx, workspaceID, provider, owner, name)
	if err != nil {
		return "", nil, fmt.Errorf("resolve workspace Git credential: %w", err)
	}
	if strings.TrimSpace(password) == "" {
		return "", nil, ErrWorkspaceCredentialUnavailable
	}
	parsed, err := url.Parse(httpsURL)
	if err != nil {
		return "", nil, fmt.Errorf("parse managed clone URL %q: %w", cloneURL, err)
	}
	if parsed.Host == "" {
		return "", nil, fmt.Errorf("parse managed clone URL %q: host is required", cloneURL)
	}
	return httpsURL, &cloneAuth{host: parsed.Host, username: username, password: password}, nil
}

// gitCmd creates a git command that ignores all ambient credential sources.
func (c *Cloner) gitCmd(ctx context.Context, auth *cloneAuth, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	env := withoutEnv(os.Environ(),
		"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", gitHubCredentialEnv, gitHubCredentialUserEnv,
		"GIT_ASKPASS", "SSH_ASKPASS", "GIT_SSH", "GIT_SSH_COMMAND",
		"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
	)
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
	)
	if auth != nil {
		username := auth.username
		if username == "" {
			username = "x-access-token"
		}
		env = append(env,
			"GIT_CONFIG_COUNT=3",
			"GIT_CONFIG_KEY_0=credential.helper",
			"GIT_CONFIG_VALUE_0=",
			"GIT_CONFIG_KEY_1=credential.https://"+auth.host+".helper",
			"GIT_CONFIG_VALUE_1="+gitCredentialHelper,
			"GIT_CONFIG_KEY_2=credential.useHttpPath",
			"GIT_CONFIG_VALUE_2=true",
			gitHubCredentialEnv+"="+auth.password,
			gitHubCredentialUserEnv+"="+username,
		)
	} else {
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=credential.helper",
			"GIT_CONFIG_VALUE_0=",
		)
	}
	cmd.Env = env
	return cmd
}

func withoutEnv(env []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		_, remove := blocked[key]
		if remove || strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func (c *Cloner) fetch(ctx context.Context, repoPath string, auth *cloneAuth) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd := c.gitCmd(ctx, auth, "-C", repoPath, "fetch", "--all", "--prune", "--force", gitNoTags)
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		c.logger.Warn("git fetch failed (non-fatal)",
			zap.String("path", repoPath),
			zap.String("output", string(out)),
			zap.Error(err))
	}
}

func (c *Cloner) clone(ctx context.Context, cloneURL, targetPath string, auth *cloneAuth) error {
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	c.logger.Info("cloning repository",
		zap.String("url", cloneURL),
		zap.String("target", targetPath))

	cmd := c.gitCmd(ctx, auth, "clone", "--filter=blob:none", gitNoTags, cloneURL, targetPath)
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}
	return nil
}
