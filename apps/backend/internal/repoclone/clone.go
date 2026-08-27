// Package repoclone handles automatic cloning and fetching of git repositories.
package repoclone

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
)

const (
	gitNoTags                    = "--no-tags"
	githubProvider               = "github"
	githubHTTPSOrigin            = "https://github.com"
	gitlabProvider               = "gitlab"
	protocolHTTP                 = "http"
	gitHubCredentialEnv          = "KANDEV_REPOCLONE_GITHUB_TOKEN"
	gitHubCredentialUserEnv      = "KANDEV_REPOCLONE_GITHUB_USERNAME"
	gitCredentialUsernameFileEnv = "KANDEV_GIT_USERNAME_FILE"
	gitCredentialPasswordFileEnv = "KANDEV_GIT_PASSWORD_FILE"
	gitCredentialMaxBytes        = 16 * 1024
	managedWorkspacesDir         = "workspaces"
	providerCloneDir             = "_providers"
	maxGitDiagnosticBytes        = 4096
)

var (
	ErrWorkspaceCredentialUnavailable = errors.New("workspace Git credential is unavailable")
	ErrRepositoryOwnershipMismatch    = errors.New("managed repository ownership mismatch")
	ErrManagedCloneOriginMismatch     = errors.New("managed repository origin does not match requested clone URL")
	gitURLUserInfoPattern             = regexp.MustCompile(`(?i)(https?://)[^/\s@]+@`)
	gitCredentialPattern              = regexp.MustCompile(`(?i)\b(password|token|secret|authorization)(\s*[:=]\s*)[^\r\n]+`)
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
	config      Config
	protocol    string
	logger      *logger.Logger
	credentials GitCredentialProvider
	repoMus     sync.Map
}

// GitCredentialProvider resolves the workspace automation identity selected
// for Git transport. Implementations must never return a personal credential.
type GitCredentialProvider interface {
	ResolveGitCredential(context.Context, GitCredentialRequest) (username, password string, err error)
}

// GitCredentialRequest is the exact credential-free repository identity a
// host materialization is about to clone. ProviderHost and CloneURL remain
// separate so resolvers can enforce same-origin routing before returning a
// transient secret.
type GitCredentialRequest struct {
	WorkspaceID          string
	TaskID               string
	SessionID            string
	RepositoryID         string
	Provider             string
	ProviderHost         string
	ProviderScope        string
	ProviderRepositoryID string
	CloneURL             string
	Owner                string
	Name                 string
	CheckoutBranch       string
	PRNumber             int
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
	return c.WorkspaceProviderRepositoryPath(workspaceID, provider, providerHost, "", "", owner, name)
}

// WorkspaceProviderRepositoryPath isolates managed clones using the provider's
// opaque connection scope and immutable repository ID. Legacy callers that do
// not yet carry both fields retain the origin/owner/name layout.
func (c *Cloner) WorkspaceProviderRepositoryPath(
	workspaceID, provider, providerHost, providerScope, providerRepositoryID, owner, name string,
) (string, error) {
	basePath, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = githubProvider
	}
	for _, segment := range []string{workspaceID, provider} {
		if err := validatePathSegment(segment); err != nil {
			return "", err
		}
	}
	if providerScope != "" || providerRepositoryID != "" {
		if strings.TrimSpace(providerScope) == "" || strings.TrimSpace(providerRepositoryID) == "" {
			return "", errors.New("provider scope and repository ID must be supplied together")
		}
		return filepath.Join(
			basePath, managedWorkspacesDir, workspaceID, provider, "_scopes",
			stableIdentitySegment(providerScope), stableIdentitySegment(providerRepositoryID),
		), nil
	}
	if err := validatePathSegment(name); err != nil {
		return "", err
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

func stableIdentitySegment(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
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
	return c.EnsureWorkspaceClonedWithCredentialRequest(ctx, GitCredentialRequest{
		WorkspaceID: workspaceID, Provider: provider, ProviderHost: providerHost,
		CloneURL: cloneURL, Owner: owner, Name: name,
	}, credentialOrigin, token)
}

// EnsureWorkspaceClonedWithCredentialRequest preserves the exact host-derived
// task/session/repository scope while resolving a plugin provider credential.
func (c *Cloner) EnsureWorkspaceClonedWithCredentialRequest(
	ctx context.Context, request GitCredentialRequest, credentialOrigin, token string,
) (string, error) {
	targetPath, err := c.WorkspaceProviderRepositoryPath(
		request.WorkspaceID, request.Provider, request.ProviderHost, request.ProviderScope,
		request.ProviderRepositoryID, request.Owner, request.Name,
	)
	if err != nil {
		return "", err
	}
	cloneURL, auth, err := c.workspaceCloneAuthRequest(ctx, request, credentialOrigin, token)
	if err != nil {
		return "", err
	}
	return c.ensureClonedAtPathWithOriginVerification(
		ctx, cloneURL, targetPath, auth, request.ProviderScope != "" && request.ProviderRepositoryID != "",
	)
}

// RefreshWorkspaceRepositoryWithCredentialRequest strictly refreshes one
// workspace-managed checkout using the same exact task/session/repository scope
// as its initial clone. Unlike the best-effort fetch used by Ensure*, this
// operation returns failures so callers can avoid falling through to an
// unauthenticated worktree fetch.
func (c *Cloner) RefreshWorkspaceRepositoryWithCredentialRequest(
	ctx context.Context, request GitCredentialRequest, repositoryPath, credentialOrigin, token string,
) error {
	targetPath, err := c.WorkspaceProviderRepositoryPath(
		request.WorkspaceID, request.Provider, request.ProviderHost, request.ProviderScope,
		request.ProviderRepositoryID, request.Owner, request.Name,
	)
	if err != nil {
		return err
	}
	if !sameFilesystemPath(targetPath, repositoryPath) {
		return errors.New("repository path does not match the scoped workspace checkout")
	}
	cloneURL, auth, err := c.workspaceCloneAuthRequest(ctx, request, credentialOrigin, token)
	if err != nil {
		return err
	}
	return c.refreshWorkspaceRepository(ctx, targetPath, cloneURL, auth, request.PRNumber)
}

// RefreshWorkspaceRepositoryWithBasicAuth strictly refreshes one existing
// workspace-managed checkout with the same basic-auth contract as cloning.
func (c *Cloner) RefreshWorkspaceRepositoryWithBasicAuth(
	ctx context.Context, workspaceID, provider, providerHost,
	cloneURL, owner, name, repositoryPath, username, password string,
) error {
	targetPath, err := c.WorkspaceProviderRepoPath(workspaceID, provider, providerHost, owner, name)
	if err != nil {
		return err
	}
	if !sameFilesystemPath(targetPath, repositoryPath) {
		return errors.New("repository path does not match the workspace checkout")
	}
	origin, err := gitCredentialOrigin(cloneURL)
	if err != nil {
		return err
	}
	return c.refreshWorkspaceRepository(ctx, targetPath, cloneURL, &cloneAuth{
		origin: origin, username: username, password: password,
	}, 0)
}

func (c *Cloner) refreshWorkspaceRepository(
	ctx context.Context, targetPath, cloneURL string, auth *cloneAuth, prNumber int,
) error {
	mu := c.repoMu(targetPath)
	mu.Lock()
	defer mu.Unlock()
	if !gitCheckoutExists(targetPath) {
		return errors.New("scoped workspace checkout is not a Git repository")
	}
	if err := c.setOriginURLLocked(ctx, targetPath, cloneURL); err != nil {
		return err
	}
	runFetch := func(refspec string) error {
		args := []string{"-C", targetPath, "fetch", "--prune", "--force", gitNoTags, "origin"}
		if refspec != "" {
			args = append(args, refspec)
		}
		cmd := subproc.NewGitCommand(ctx, args...)
		cleanup, err := configureGitCommand(cmd, auth)
		if err != nil {
			return err
		}
		defer cleanup()
		if out, runErr := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); runErr != nil {
			return fmt.Errorf("refresh scoped workspace repository: %s: %w",
				redactCloneOutput(string(out), authToken(auth)), runErr)
		}
		return nil
	}
	if err := runFetch(""); err != nil {
		return err
	}
	if prNumber > 0 {
		prRefspec := fmt.Sprintf("pull/%d/head:refs/remotes/origin/pr/%d", prNumber, prNumber)
		if err := runFetch(prRefspec); err != nil {
			return fmt.Errorf("refresh scoped workspace pull request: %w", err)
		}
	}
	return nil
}

func sameFilesystemPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && leftAbs == rightAbs
}

func gitCheckoutExists(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

// SetOriginURL updates a managed checkout's origin without exposing credentials.
func (c *Cloner) SetOriginURL(ctx context.Context, repositoryPath, originURL string) error {
	if strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(originURL) == "" {
		return errors.New("repository path and origin URL are required")
	}
	mu := c.repoMu(repositoryPath)
	mu.Lock()
	defer mu.Unlock()
	return c.setOriginURLLocked(ctx, repositoryPath, originURL)
}

func (c *Cloner) setOriginURLLocked(ctx context.Context, repositoryPath, originURL string) error {
	current, err := c.originURLLocked(ctx, repositoryPath)
	if err != nil {
		return err
	}
	if current == strings.TrimSpace(originURL) {
		return nil
	}

	cmd := subproc.NewGitCommand(ctx, "-C", repositoryPath, "remote", "set-url", "origin", "--", originURL)
	cleanup, err := configureGitCommand(cmd, nil)
	if err != nil {
		return err
	}
	output, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd)
	cleanup()
	if err != nil {
		return fmt.Errorf("set repository origin: %w", formatGitOriginError(repositoryPath, output, err))
	}
	return nil
}

func (c *Cloner) verifyOriginURLLocked(ctx context.Context, repositoryPath, originURL string) error {
	current, err := c.originURLLocked(ctx, repositoryPath)
	if err != nil {
		return err
	}
	if current != strings.TrimSpace(originURL) {
		return fmt.Errorf("%w: checkout %q", ErrManagedCloneOriginMismatch, repositoryPath)
	}
	return nil
}

func (c *Cloner) originURLLocked(ctx context.Context, repositoryPath string) (string, error) {
	// `git remote get-url` expands url.*.insteadOf rules before returning the
	// value. Read the local config so canonical origins do not get rewritten on
	// every launch or resume.
	cmd := subproc.NewGitCommand(ctx, "-C", repositoryPath, "config", "--local", "--get", "remote.origin.url")
	cleanup, err := configureGitCommand(cmd, nil)
	if err != nil {
		return "", err
	}
	currentOutput, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd)
	cleanup()
	if err != nil {
		return "", fmt.Errorf("inspect repository origin: %w", formatGitOriginError(repositoryPath, currentOutput, err))
	}
	return strings.TrimSpace(string(currentOutput)), nil
}

func formatGitOriginError(repositoryPath string, output []byte, err error) error {
	diagnostic := redactCloneOutput(string(output), "")
	if strings.Contains(strings.ToLower(diagnostic), "detected dubious ownership") {
		return fmt.Errorf(
			"%w: Git rejected managed checkout %q because its filesystem owner differs from the Kandev service account; ensure that account owns the checkout or reinstall Kandev with the intended account (git: %s)",
			ErrRepositoryOwnershipMismatch, repositoryPath, diagnostic,
		)
	}
	if diagnostic == "" {
		return err
	}
	return fmt.Errorf("git reported %s: %w", diagnostic, err)
}

type cloneAuth struct {
	origin   string
	username string
	password string
}

func (c *Cloner) workspaceCloneAuth(
	ctx context.Context, workspaceID, provider, providerHost, cloneURL, owner, name, credentialOrigin, token string,
) (string, *cloneAuth, error) {
	return c.workspaceCloneAuthRequest(ctx, GitCredentialRequest{
		WorkspaceID: workspaceID, Provider: provider, ProviderHost: providerHost,
		CloneURL: cloneURL, Owner: owner, Name: name,
	}, credentialOrigin, token)
}

func (c *Cloner) workspaceCloneAuthRequest(
	ctx context.Context, request GitCredentialRequest, credentialOrigin, token string,
) (string, *cloneAuth, error) {
	provider := strings.ToLower(strings.TrimSpace(request.Provider))
	if provider == "" {
		provider = githubProvider
	}
	if provider != githubProvider && (provider == gitlabProvider || provider == "azure_devops" || strings.TrimSpace(token) != "") {
		auth, err := credentialAuth(request.CloneURL, credentialOrigin, token)
		return request.CloneURL, auth, err
	}
	httpsURL := request.CloneURL
	if provider == githubProvider {
		var err error
		httpsURL, err = githubHTTPSCloneURL(
			request.CloneURL, request.ProviderHost, request.Owner, request.Name,
		)
		if err != nil {
			return "", nil, err
		}
	} else if err := ValidateHTTPSCloneOrigin(request.CloneURL, request.ProviderHost); err != nil {
		return "", nil, fmt.Errorf("validate provider clone origin: %w", err)
	}
	if c.credentials == nil {
		return "", nil, ErrWorkspaceCredentialUnavailable
	}
	request.Provider = provider
	request.CloneURL = httpsURL
	username, password, err := c.credentials.ResolveGitCredential(ctx, request)
	if err != nil {
		return "", nil, fmt.Errorf("resolve workspace Git credential: %w", err)
	}
	if strings.TrimSpace(password) == "" {
		return "", nil, ErrWorkspaceCredentialUnavailable
	}
	parsed, err := url.Parse(httpsURL)
	if err != nil || parsed.Host == "" {
		return "", nil, fmt.Errorf("parse managed clone URL %q: host is required", request.CloneURL)
	}
	if username == "" {
		username = "x-access-token"
	}
	return httpsURL, &cloneAuth{
		origin: parsed.Scheme + "://" + parsed.Host, username: username, password: password,
	}, nil
}

func githubHTTPSCloneURL(cloneURL, providerHost, owner, name string) (string, error) {
	expectedHost := strings.TrimSpace(providerHost)
	if expectedHost == "" {
		expectedHost = githubHTTPSOrigin
	}
	if ValidateHTTPSCloneOrigin(cloneURL, expectedHost) == nil {
		return strings.TrimSpace(cloneURL), nil
	}
	return CloneURLWithHost(githubProvider, expectedHost, owner, name, ProtocolHTTPS)
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
	return c.ensureClonedAtPathWithOriginVerification(ctx, cloneURL, targetPath, auth, false)
}

func (c *Cloner) ensureClonedAtPathWithOriginVerification(
	ctx context.Context, cloneURL, targetPath string, auth *cloneAuth, verifyExistingOrigin bool,
) (string, error) {
	mu := c.repoMu(targetPath)
	mu.Lock()
	defer mu.Unlock()

	gitDir := filepath.Join(targetPath, ".git")
	if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
		if verifyExistingOrigin {
			if err := c.verifyOriginURLLocked(ctx, targetPath, cloneURL); err != nil {
				return targetPath, err
			}
		}
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

// EnsureWorkspaceClonedWithBasicAuth keeps basic authentication in the
// short-lived Git helper file descriptors and stores the clone under the
// workspace/provider/origin path.
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
	origin, err := gitCredentialOrigin(cloneURL)
	if err != nil {
		return targetPath, err
	}
	auth := &cloneAuth{origin: origin, username: username, password: password}
	gitDir := filepath.Join(targetPath, ".git")
	if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
		if err := c.verifyOriginURLLocked(ctx, targetPath, cloneURL); err != nil {
			return targetPath, err
		}
		c.fetch(ctx, targetPath, auth)
		return targetPath, nil
	}
	return targetPath, c.clone(ctx, cloneURL, targetPath, auth)
}

func gitCredentialOrigin(cloneURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(cloneURL))
	if err != nil || parsed.Scheme != ProtocolHTTPS || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("parse managed clone URL %q: HTTPS host is required", cloneURL)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func (c *Cloner) fetch(ctx context.Context, repoPath string, auth *cloneAuth) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd := subproc.NewGitCommand(
		ctx, "-C", repoPath, "fetch", "--all", "--prune", "--force", gitNoTags,
	)
	cleanup, err := configureGitCommand(cmd, auth)
	if err != nil {
		c.logger.Warn("configure Git fetch credentials failed", zap.String("path", repoPath), zap.Error(err))
		return
	}
	defer cleanup()
	if out, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
		c.logger.Warn("git fetch failed (non-fatal)",
			zap.String("path", repoPath), zap.String("output", redactCloneOutput(string(out), authToken(auth))), zap.Error(err))
	}
}

func (c *Cloner) clone(ctx context.Context, cloneURL, targetPath string, auth *cloneAuth) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	c.logger.Info("cloning repository", zap.String("url", redactCloneURL(cloneURL)), zap.String("target", targetPath))
	args := []string{"clone"}
	if auth == nil {
		args = append(args, "--filter=blob:none")
	}
	args = append(args, gitNoTags, "--", cloneURL, targetPath)
	cmd := subproc.NewGitCommand(ctx, args...)
	cleanup, err := configureGitCommand(cmd, auth)
	if err != nil {
		return err
	}
	defer cleanup()
	if out, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", redactCloneOutput(string(out), authToken(auth)), err)
	}
	return nil
}

func (c *Cloner) fetchWithHTTPHeader(ctx context.Context, repoPath, authURL, header string) {
	c.logger.Debug("repository already cloned, fetching", zap.String("path", repoPath))
	cmd := subproc.NewGitCommand(
		ctx, "-C", repoPath, "fetch", "--all", "--prune", "--force", gitNoTags,
	)
	configureHTTPHeaderCommand(cmd, authURL, header)
	if out, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
		c.logger.Warn("authenticated git fetch failed (non-fatal)",
			zap.String("path", repoPath), zap.String("output", string(out)), zap.Error(err))
	}
}

func (c *Cloner) cloneWithHTTPHeader(ctx context.Context, cloneURL, targetPath, header string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	c.logger.Info("cloning authenticated repository", zap.String("url", redactCloneURL(cloneURL)), zap.String("target", targetPath))
	cmd := subproc.NewGitCommand(
		ctx, "clone", "--filter=blob:none", gitNoTags, "--", cloneURL, targetPath,
	)
	configureHTTPHeaderCommand(cmd, cloneURL, header)
	if out, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}
	return nil
}

func configureGitCommand(cmd *exec.Cmd, auth *cloneAuth) (func(), error) {
	env := cleanGitEnvironment()
	if auth == nil {
		env = append(env,
			"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=",
		)
		cmd.Env = env
		return func() {}, nil
	}
	if err := validateCloneAuth(auth); err != nil {
		return nil, err
	}
	helperPath, files, helperEnv, cleanup, err := gitCredentialHelperCommand(auth)
	if err != nil {
		return nil, err
	}
	env = append(env,
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=credential."+auth.origin+".helper", "GIT_CONFIG_VALUE_1=!"+helperPath,
		"GIT_CONFIG_KEY_2=credential.useHttpPath", "GIT_CONFIG_VALUE_2=true",
	)
	env = append(env, helperEnv...)
	cmd.Env = env
	cmd.ExtraFiles = files
	return cleanup, nil
}

func configureHTTPHeaderCommand(cmd *exec.Cmd, authURL, header string) {
	cmd.Env = append(cleanGitEnvironment(),
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=http."+authURL+".extraHeader", "GIT_CONFIG_VALUE_1="+header,
	)
}

func validateCloneAuth(auth *cloneAuth) error {
	if auth == nil || strings.TrimSpace(auth.password) == "" || len(auth.password) > gitCredentialMaxBytes ||
		strings.ContainsAny(auth.username, "\r\n") || strings.ContainsAny(auth.password, "\r\n") {
		return ErrWorkspaceCredentialUnavailable
	}
	return nil
}

func cleanGitEnvironment() []string {
	env := withoutEnv(os.Environ(),
		"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", gitHubCredentialEnv, gitHubCredentialUserEnv,
		gitCredentialUsernameFileEnv, gitCredentialPasswordFileEnv,
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
	redacted := output
	if token != "" {
		redacted = strings.ReplaceAll(redacted, token, "[REDACTED]")
	}
	redacted = gitURLUserInfoPattern.ReplaceAllString(redacted, `${1}[REDACTED]@`)
	redacted = gitCredentialPattern.ReplaceAllString(redacted, `${1}${2}[REDACTED]`)
	if len(redacted) > maxGitDiagnosticBytes {
		return redacted[:maxGitDiagnosticBytes]
	}
	return redacted
}

func authToken(auth *cloneAuth) string {
	if auth == nil {
		return ""
	}
	return auth.password
}
