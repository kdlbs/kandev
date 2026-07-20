// Package repoclone handles automatic cloning and fetching of git repositories.
package repoclone

import (
	"context"
	"encoding/base64"
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

// ghCredentialHelper is the git credential helper command that delegates to gh CLI.
const (
	ghCredentialHelper = "!gh auth git-credential"
	gitNoTags          = "--no-tags"
	protocolHTTP       = "http"
	providerCloneDir   = "_providers"
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

// BuildCloneURLWithHost constructs a clone URL using a persisted provider
// origin. This is required for self-managed GitLab repositories.
func (c *Cloner) BuildCloneURLWithHost(provider, host, owner, name string) (string, error) {
	return CloneURLWithHost(provider, host, owner, name, c.protocol)
}

// RepoPath returns the full local path for a repository.
func (c *Cloner) RepoPath(owner, name string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(basePath, owner, name)
	relativePath, err := filepath.Rel(basePath, targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve repository path: %w", err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("repository path %q escapes clone base", targetPath)
	}
	return targetPath, nil
}

// ProviderRepoPath returns a clone path isolated by provider and provider host.
// Repositories can share owner/name across GitHub, GitLab.com, and self-managed
// GitLab instances, so owner/name alone is not a stable clone identity.
func (c *Cloner) ProviderRepoPath(provider, providerHost, owner, name string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(
		basePath,
		providerCloneDir,
		clonePathSegment(provider),
		providerHostPathSegment(providerHost),
		owner,
		name,
	), nil
}

func providerHostPathSegment(providerHost string) string {
	host := strings.TrimSpace(providerHost)
	if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	return clonePathSegment(host)
}

func clonePathSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	result.Grow(len(value))
	lastWasSeparator := false
	for _, char := range value {
		allowed := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '.' || char == '_'
		if allowed {
			result.WriteRune(char)
			lastWasSeparator = false
		} else if !lastWasSeparator {
			result.WriteByte('-')
			lastWasSeparator = true
		}
	}
	segment := strings.Trim(result.String(), "-.")
	if segment == "" {
		return "unknown"
	}
	return segment
}

// EnsureCloned clones the repository if it doesn't exist locally, or fetches if it does.
// The cloneURL is the full git URL (HTTPS or SSH) to clone from.
// Returns the local filesystem path to the repository.
// Concurrent calls for the same repository are serialised to prevent double-clone races.
func (c *Cloner) EnsureCloned(ctx context.Context, cloneURL, owner, name string) (string, error) {
	return c.EnsureClonedWithAuth(ctx, cloneURL, owner, name, "", "")
}

// EnsureClonedWithAuth scopes an HTTPS credential to one exact provider
// origin. The token is passed only through the child process environment and
// is never embedded in the clone URL or persisted in .git/config.
func (c *Cloner) EnsureClonedWithAuth(
	ctx context.Context,
	cloneURL, owner, name, credentialOrigin, token string,
) (string, error) {
	targetPath, err := c.RepoPath(owner, name)
	if err != nil {
		return "", err
	}
	return c.ensureClonedAtPath(ctx, cloneURL, targetPath, owner, name, credentialOrigin, token)
}

// EnsureClonedForProvider clones or fetches a provider-backed repository using
// a path keyed by both provider and host.
func (c *Cloner) EnsureClonedForProvider(
	ctx context.Context,
	cloneURL, provider, providerHost, owner, name, credentialOrigin, token string,
) (string, error) {
	targetPath, err := c.ProviderRepoPath(provider, providerHost, owner, name)
	if err != nil {
		return "", err
	}
	return c.ensureClonedAtPath(ctx, cloneURL, targetPath, owner, name, credentialOrigin, token)
}

func (c *Cloner) ensureClonedAtPath(
	ctx context.Context,
	cloneURL, targetPath, owner, name, credentialOrigin, token string,
) (string, error) {

	mu := c.repoMu(targetPath)
	mu.Lock()
	defer mu.Unlock()

	gitDir := filepath.Join(targetPath, ".git")
	if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
		c.fetch(ctx, targetPath, cloneURL, credentialOrigin, token)
		return targetPath, nil
	}

	return targetPath, c.clone(ctx, cloneURL, targetPath, owner, name, credentialOrigin, token)
}

// EnsureClonedWithBasicAuth clones or fetches using an ephemeral HTTP
// Authorization header. The credential is carried only in the Git child
// process environment, never in the URL, command arguments, or logs.
func (c *Cloner) EnsureClonedWithBasicAuth(
	ctx context.Context, cloneURL, owner, name, username, password string,
) (string, error) {
	targetPath, err := c.RepoPath(owner, name)
	if err != nil {
		return "", err
	}
	mu := c.repoMu(targetPath)
	mu.Lock()
	defer mu.Unlock()
	header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	gitDir := filepath.Join(targetPath, ".git")
	if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
		cmd := c.gitCmdWithHTTPHeader(ctx, header, "-C", targetPath, "fetch", "--all", "--prune", "--force", gitNoTags)
		if out, fetchErr := subproc.RunGitCombinedOutput(ctx, cmd); fetchErr != nil {
			c.logger.Warn("authenticated git fetch failed", zap.String("path", targetPath), zap.String("output", string(out)), zap.Error(fetchErr))
		}
		return targetPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("create parent directory: %w", err)
	}
	c.logger.Info("cloning authenticated repository", zap.String("url", cloneURL), zap.String("target", targetPath))
	cmd := c.gitCmdWithHTTPHeader(ctx, header, "clone", "--filter=blob:none", gitNoTags, cloneURL, targetPath)
	if out, cloneErr := subproc.RunGitCombinedOutput(ctx, cmd); cloneErr != nil {
		return "", fmt.Errorf("git clone failed: %s: %w", string(out), cloneErr)
	}
	return targetPath, nil
}

func (c *Cloner) gitCmdWithHTTPHeader(ctx context.Context, header string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0="+header,
	)
	return cmd
}

// gitCmd creates a git command with non-interactive environment settings.
// When the configured protocol is HTTPS, it adds gh CLI as the credential
// helper so that git can authenticate using the user's gh auth token.
func (c *Cloner) gitCmd(
	ctx context.Context,
	cloneURL, credentialOrigin, token string,
	args ...string,
) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	env := os.Environ()
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	credentialEnv, err := gitLabCredentialEnv(cloneURL, credentialOrigin, token)
	if err != nil {
		return nil, err
	}
	env = append(env, credentialEnv...)
	if token == "" && c.protocol == ProtocolHTTPS {
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=credential.helper",
			"GIT_CONFIG_VALUE_0="+ghCredentialHelper,
		)
	}
	cmd.Env = env
	return cmd, nil
}

func gitLabCredentialEnv(cloneURL, credentialOrigin, token string) ([]string, error) {
	if token == "" {
		return nil, nil
	}
	if !isHTTPCloneURL(cloneURL) {
		return nil, validateSSHCredentialHost(cloneURL, credentialOrigin)
	}
	origin, err := matchingCredentialOrigin(cloneURL, credentialOrigin)
	if err != nil {
		return nil, err
	}
	return []string{
		"KANDEV_GIT_CREDENTIAL_TOKEN=" + token,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential." + origin + ".helper",
		`GIT_CONFIG_VALUE_0=!f() { echo "username=oauth2"; echo "password=$KANDEV_GIT_CREDENTIAL_TOKEN"; }; f`,
	}, nil
}

func isHTTPCloneURL(cloneURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(cloneURL))
	return err == nil && (parsed.Scheme == protocolHTTP || parsed.Scheme == ProtocolHTTPS)
}

func validateSSHCredentialHost(cloneURL, credentialOrigin string) error {
	credential, err := url.Parse(strings.TrimRight(strings.TrimSpace(credentialOrigin), "/"))
	if err != nil || (credential.Scheme != protocolHTTP && credential.Scheme != ProtocolHTTPS) ||
		credential.Hostname() == "" || credential.User != nil ||
		(credential.Path != "" && credential.Path != "/") || credential.RawQuery != "" || credential.Fragment != "" {
		return errors.New("configured credential host is invalid")
	}
	remoteHost := sshCloneHostname(cloneURL)
	if remoteHost == "" || !strings.EqualFold(remoteHost, credential.Hostname()) {
		return errors.New("clone URL does not match the configured credential host")
	}
	return nil
}

func sshCloneHostname(cloneURL string) string {
	trimmed := strings.TrimSpace(cloneURL)
	if strings.HasPrefix(trimmed, "ssh://") {
		parsed, err := url.Parse(trimmed)
		if err == nil && parsed.Hostname() != "" && parsed.Path != "" {
			return parsed.Hostname()
		}
		return ""
	}
	if _, after, ok := strings.Cut(trimmed, "@"); ok {
		trimmed = after
	}
	host, path, ok := strings.Cut(trimmed, ":")
	if !ok || host == "" || path == "" {
		return ""
	}
	return host
}

func matchingCredentialOrigin(cloneURL, credentialOrigin string) (string, error) {
	clone, cloneErr := url.Parse(strings.TrimSpace(cloneURL))
	credential, credentialErr := url.Parse(strings.TrimRight(strings.TrimSpace(credentialOrigin), "/"))
	if cloneErr != nil || credentialErr != nil || clone.User != nil || credential.User != nil ||
		(clone.Scheme != protocolHTTP && clone.Scheme != ProtocolHTTPS) ||
		clone.Scheme != credential.Scheme || !strings.EqualFold(clone.Host, credential.Host) ||
		(credential.Path != "" && credential.Path != "/") {
		return "", errors.New("clone URL does not match the configured credential host")
	}
	return credential.Scheme + "://" + credential.Host, nil
}

func (c *Cloner) fetch(ctx context.Context, repoPath, cloneURL, credentialOrigin, token string) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd, err := c.gitCmd(ctx, cloneURL, credentialOrigin, token, "-C", repoPath, "fetch", "--all", "--prune", "--force", gitNoTags)
	if err != nil {
		c.logger.Warn("git fetch credential host mismatch", zap.String("path", repoPath))
		return
	}
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		c.logger.Warn("git fetch failed (non-fatal)",
			zap.String("path", repoPath),
			zap.String("output", string(out)),
			zap.Error(err))
	}
}

func (c *Cloner) clone(ctx context.Context, cloneURL, targetPath, owner, name, credentialOrigin, token string) error {
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	c.logger.Info("cloning repository",
		zap.String("url", redactCloneURL(cloneURL)),
		zap.String("target", targetPath))

	// Try gh repo clone first — handles auth for both SSH and HTTPS.
	if owner != "" && name != "" && strings.Contains(cloneURL, "github.com") {
		if err := c.ghClone(ctx, owner, name, targetPath); err == nil {
			return nil
		}
		// Clean up any partial clone so the fallback can retry into a fresh path.
		if rmErr := os.RemoveAll(targetPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("cleanup failed gh clone target: %w", rmErr)
		}
	}

	// Fallback: git clone with credential helper.
	cmd, err := c.gitCmd(ctx, cloneURL, credentialOrigin, token, "clone", "--filter=blob:none", gitNoTags, cloneURL, targetPath)
	if err != nil {
		return err
	}
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", redactCloneOutput(string(out), token), err)
	}
	return nil
}

func redactCloneURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

func redactCloneOutput(output, token string) string {
	if token == "" {
		return output
	}
	return strings.ReplaceAll(output, token, "[REDACTED]")
}

// ghClone attempts to clone using gh repo clone, which handles authentication
// automatically via the user's gh CLI session.
func (c *Cloner) ghClone(ctx context.Context, owner, name, targetPath string) error {
	nwo := owner + "/" + name
	cmd := exec.CommandContext(ctx, "gh", "repo", "clone", nwo, targetPath, "--", "--filter=blob:none", gitNoTags)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := subproc.RunGHCombinedOutput(ctx, cmd)
	if err != nil {
		c.logger.Debug("gh repo clone failed, falling back to git clone",
			zap.String("repo", nwo),
			zap.String("output", string(out)),
			zap.Error(err))
		return fmt.Errorf("gh repo clone: %w", err)
	}
	return nil
}
