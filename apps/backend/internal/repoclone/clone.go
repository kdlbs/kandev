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

const (
	gitNoTags               = "--no-tags"
	githubProvider          = "github"
	gitlabProvider          = "gitlab"
	protocolHTTP            = "http"
	gitHubCredentialEnv     = "KANDEV_REPOCLONE_GITHUB_TOKEN"
	gitHubCredentialUserEnv = "KANDEV_REPOCLONE_GITHUB_USERNAME"
	gitCredentialHelper     = `!f() { if [ "$1" = get ]; then printf '%s\n' "username=$KANDEV_REPOCLONE_GITHUB_USERNAME" "password=$KANDEV_REPOCLONE_GITHUB_TOKEN"; fi; }; f`
	managedWorkspacesDir    = "workspaces"
	providerCloneDir        = "_providers"
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
	repoMus     sync.Map
}

// GitCredentialProvider resolves the workspace automation identity selected
// for Git transport. Implementations must never return a personal credential.
type GitCredentialProvider interface {
	ResolveGitCredential(
		ctx context.Context,
		workspaceID, provider, owner, name string,
	) (username, password string, err error)
}

// NewCloner creates a new Cloner with the given configuration.
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

// BuildCloneURL constructs a protocol-aware clone URL for a provider repository.
func (c *Cloner) BuildCloneURL(provider, owner, name string) (string, error) {
	return CloneURL(provider, owner, name, c.protocol)
}

// BuildCloneURLWithHost constructs a clone URL using a persisted provider origin.
func (c *Cloner) BuildCloneURLWithHost(provider, host, owner, name string) (string, error) {
	return CloneURLWithHost(provider, host, owner, name, c.protocol)
}

// RepoPath returns the legacy owner/name clone path.
func (c *Cloner) RepoPath(owner, name string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	if err := validateOwnerPath(owner); err != nil {
		return "", err
	}
	if err := validatePathSegment(name); err != nil {
		return "", err
	}
	return filepath.Join(basePath, filepath.FromSlash(owner), name), nil
}

// ProviderRepoPath returns the legacy provider/origin clone path.
func (c *Cloner) ProviderRepoPath(provider, providerHost, owner, name string) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	if err := validateOwnerPath(owner); err != nil {
		return "", err
	}
	if err := validatePathSegment(name); err != nil {
		return "", err
	}
	return filepath.Join(
		basePath, providerCloneDir, clonePathSegment(provider),
		providerHostPathSegment(providerHost), filepath.FromSlash(owner), name,
	), nil
}

// WorkspaceRepoPath returns the isolated path for a default-origin provider repository.
func (c *Cloner) WorkspaceRepoPath(workspaceID, provider, owner, name string) (string, error) {
	return c.WorkspaceProviderRepoPath(workspaceID, provider, "", owner, name)
}

// WorkspaceProviderRepoPath isolates managed clones by workspace, provider,
// and non-default provider origin.
func (c *Cloner) WorkspaceProviderRepoPath(
	workspaceID, provider, providerHost, owner, name string,
) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = githubProvider
	}
	for _, segment := range []string{workspaceID, provider, name} {
		if err := validatePathSegment(segment); err != nil {
			return "", err
		}
	}
	if err := validateOwnerPath(owner); err != nil {
		return "", err
	}
	parts := []string{basePath, managedWorkspacesDir, workspaceID, provider}
	if host := nonDefaultProviderHostSegment(provider, providerHost); host != "" {
		parts = append(parts, host)
	}
	parts = append(parts, filepath.FromSlash(owner), name)
	return filepath.Join(parts...), nil
}

func nonDefaultProviderHostSegment(provider, providerHost string) string {
	parsedHost := strings.TrimSpace(providerHost)
	if parsed, err := url.Parse(parsedHost); err == nil && parsed.Host != "" {
		parsedHost = parsed.Host
	}
	parsedHost = strings.ToLower(strings.TrimSpace(parsedHost))
	if parsedHost == "" || provider == githubProvider && parsedHost == "github.com" ||
		provider == gitlabProvider && parsedHost == "gitlab.com" {
		return ""
	}
	return clonePathSegment(parsedHost)
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

func validatePathSegment(segment string) error {
	if segment == "" || segment == "." || segment == ".." || filepath.Clean(segment) != segment ||
		strings.ContainsAny(segment, `/\`) {
		return fmt.Errorf("invalid managed clone path segment %q", segment)
	}
	return nil
}

func validateOwnerPath(owner string) error {
	if strings.Contains(owner, `\`) {
		return fmt.Errorf("invalid managed clone owner path %q", owner)
	}
	for _, segment := range strings.Split(owner, "/") {
		if err := validatePathSegment(segment); err != nil {
			return fmt.Errorf("invalid managed clone owner path %q: %w", owner, err)
		}
	}
	return nil
}

// ShouldRecloneForWorkspace reports whether a managed path is not isolated to the workspace.
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

// EnsureCloned retains the legacy public-clone behavior for non-workspace callers.
func (c *Cloner) EnsureCloned(ctx context.Context, cloneURL, owner, name string) (string, error) {
	return c.EnsureClonedWithAuth(ctx, cloneURL, owner, name, "", "")
}

// EnsureClonedWithAuth scopes an HTTPS credential to one exact provider origin.
func (c *Cloner) EnsureClonedWithAuth(
	ctx context.Context, cloneURL, owner, name, credentialOrigin, token string,
) (string, error) {
	targetPath, err := c.RepoPath(owner, name)
	if err != nil {
		return "", err
	}
	auth, err := credentialAuth(cloneURL, credentialOrigin, token)
	if err != nil {
		return "", err
	}
	return c.ensureClonedAtPath(ctx, cloneURL, targetPath, auth)
}

// EnsureClonedForProvider retains the legacy provider/origin clone layout.
func (c *Cloner) EnsureClonedForProvider(
	ctx context.Context, cloneURL, provider, providerHost, owner, name, credentialOrigin, token string,
) (string, error) {
	targetPath, err := c.ProviderRepoPath(provider, providerHost, owner, name)
	if err != nil {
		return "", err
	}
	auth, err := credentialAuth(cloneURL, credentialOrigin, token)
	if err != nil {
		return "", err
	}
	return c.ensureClonedAtPath(ctx, cloneURL, targetPath, auth)
}

// EnsureWorkspaceCloned clones a repository into its workspace-isolated path.
func (c *Cloner) EnsureWorkspaceCloned(
	ctx context.Context, workspaceID, provider, cloneURL, owner, name string,
) (string, error) {
	return c.EnsureWorkspaceClonedForProvider(
		ctx, workspaceID, cloneURL, provider, "", owner, name, "", "",
	)
}

// EnsureWorkspaceClonedForProvider combines workspace isolation with exact
// provider-origin credential scoping.
func (c *Cloner) EnsureWorkspaceClonedForProvider(
	ctx context.Context, workspaceID, cloneURL, provider, providerHost,
	owner, name, credentialOrigin, token string,
) (string, error) {
	targetPath, err := c.WorkspaceProviderRepoPath(workspaceID, provider, providerHost, owner, name)
	if err != nil {
		return "", err
	}
	cloneURL, auth, err := c.workspaceCloneAuth(
		ctx, workspaceID, provider, cloneURL, owner, name, credentialOrigin, token,
	)
	if err != nil {
		return "", err
	}
	return c.ensureClonedAtPath(ctx, cloneURL, targetPath, auth)
}

type cloneAuth struct {
	origin   string
	username string
	password string
}

func (c *Cloner) workspaceCloneAuth(
	ctx context.Context, workspaceID, provider, cloneURL, owner, name, credentialOrigin, token string,
) (string, *cloneAuth, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = githubProvider
	}
	if provider != githubProvider {
		auth, err := credentialAuth(cloneURL, credentialOrigin, token)
		return cloneURL, auth, err
	}
	httpsURL, err := CloneURL(provider, owner, name, ProtocolHTTPS)
	if err != nil {
		return "", nil, err
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
	if err != nil || parsed.Host == "" {
		return "", nil, fmt.Errorf("parse managed clone URL %q: host is required", cloneURL)
	}
	if username == "" {
		username = "x-access-token"
	}
	return httpsURL, &cloneAuth{
		origin: parsed.Scheme + "://" + parsed.Host, username: username, password: password,
	}, nil
}

func credentialAuth(cloneURL, credentialOrigin, token string) (*cloneAuth, error) {
	if strings.TrimSpace(token) == "" {
		return nil, nil
	}
	if !isHTTPCloneURL(cloneURL) {
		return nil, validateSSHCredentialHost(cloneURL, credentialOrigin)
	}
	origin, err := matchingCredentialOrigin(cloneURL, credentialOrigin)
	if err != nil {
		return nil, err
	}
	return &cloneAuth{origin: origin, username: "oauth2", password: token}, nil
}

func (c *Cloner) ensureClonedAtPath(
	ctx context.Context, cloneURL, targetPath string, auth *cloneAuth,
) (string, error) {
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

// EnsureClonedWithBasicAuth retains the legacy basic-auth clone path.
func (c *Cloner) EnsureClonedWithBasicAuth(
	ctx context.Context, cloneURL, owner, name, username, password string,
) (string, error) {
	targetPath, err := c.RepoPath(owner, name)
	if err != nil {
		return "", err
	}
	return c.ensureClonedWithBasicAuth(ctx, targetPath, cloneURL, username, password)
}

// EnsureWorkspaceClonedWithBasicAuth uses an ephemeral Authorization header
// and stores the clone under the workspace/provider/origin path.
func (c *Cloner) EnsureWorkspaceClonedWithBasicAuth(
	ctx context.Context, workspaceID, provider, providerHost,
	cloneURL, owner, name, username, password string,
) (string, error) {
	targetPath, err := c.WorkspaceProviderRepoPath(workspaceID, provider, providerHost, owner, name)
	if err != nil {
		return "", err
	}
	return c.ensureClonedWithBasicAuth(ctx, targetPath, cloneURL, username, password)
}

func (c *Cloner) ensureClonedWithBasicAuth(
	ctx context.Context, targetPath, cloneURL, username, password string,
) (string, error) {
	mu := c.repoMu(targetPath)
	mu.Lock()
	defer mu.Unlock()
	header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	gitDir := filepath.Join(targetPath, ".git")
	if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
		c.fetchWithHTTPHeader(ctx, targetPath, cloneURL, header)
		return targetPath, nil
	}
	return targetPath, c.cloneWithHTTPHeader(ctx, cloneURL, targetPath, header)
}

func (c *Cloner) fetch(ctx context.Context, repoPath string, auth *cloneAuth) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd := exec.CommandContext(
		ctx, "git", "-C", repoPath, "fetch", "--all", "--prune", "--force", gitNoTags,
	)
	configureGitCommand(cmd, auth)
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		c.logger.Warn("git fetch failed (non-fatal)",
			zap.String("path", repoPath), zap.String("output", redactCloneOutput(string(out), authToken(auth))), zap.Error(err))
	}
}

func (c *Cloner) clone(ctx context.Context, cloneURL, targetPath string, auth *cloneAuth) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	c.logger.Info("cloning repository", zap.String("url", redactCloneURL(cloneURL)), zap.String("target", targetPath))
	cmd := exec.CommandContext(
		ctx, "git", "clone", "--filter=blob:none", gitNoTags, "--", cloneURL, targetPath,
	)
	configureGitCommand(cmd, auth)
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", redactCloneOutput(string(out), authToken(auth)), err)
	}
	return nil
}

func (c *Cloner) fetchWithHTTPHeader(ctx context.Context, repoPath, authURL, header string) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd := exec.CommandContext(
		ctx, "git", "-C", repoPath, "fetch", "--all", "--prune", "--force", gitNoTags,
	)
	configureHTTPHeaderCommand(cmd, authURL, header)
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		c.logger.Warn("authenticated git fetch failed (non-fatal)",
			zap.String("path", repoPath), zap.String("output", string(out)), zap.Error(err))
	}
}

func (c *Cloner) cloneWithHTTPHeader(ctx context.Context, cloneURL, targetPath, header string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	c.logger.Info("cloning authenticated repository", zap.String("url", redactCloneURL(cloneURL)), zap.String("target", targetPath))
	cmd := exec.CommandContext(
		ctx, "git", "clone", "--filter=blob:none", gitNoTags, "--", cloneURL, targetPath,
	)
	configureHTTPHeaderCommand(cmd, cloneURL, header)
	if out, err := subproc.RunGitCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}
	return nil
}

func configureGitCommand(cmd *exec.Cmd, auth *cloneAuth) {
	env := cleanGitEnvironment()
	if auth == nil {
		env = append(env,
			"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=",
		)
		cmd.Env = env
		return
	}
	env = append(env,
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=credential."+auth.origin+".helper", "GIT_CONFIG_VALUE_1="+gitCredentialHelper,
		"GIT_CONFIG_KEY_2=credential.useHttpPath", "GIT_CONFIG_VALUE_2=true",
		gitHubCredentialEnv+"="+auth.password,
		gitHubCredentialUserEnv+"="+auth.username,
	)
	cmd.Env = env
}

func configureHTTPHeaderCommand(cmd *exec.Cmd, authURL, header string) {
	cmd.Env = append(cleanGitEnvironment(),
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=http."+authURL+".extraHeader", "GIT_CONFIG_VALUE_1="+header,
	)
}

func cleanGitEnvironment() []string {
	env := withoutEnv(os.Environ(),
		"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", gitHubCredentialEnv, gitHubCredentialUserEnv,
		"GIT_ASKPASS", "SSH_ASKPASS", "GIT_SSH", "GIT_SSH_COMMAND",
		"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
	)
	return append(env,
		"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
	)
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

func authToken(auth *cloneAuth) string {
	if auth == nil {
		return ""
	}
	return auth.password
}
