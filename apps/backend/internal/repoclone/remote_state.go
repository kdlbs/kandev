package repoclone

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
)

// RemoteRefState describes what an authenticated remote advertised after a
// clone or strict refresh.
type RemoteRefState string

const (
	RemoteRefStateUnknown RemoteRefState = "unknown"
	RemoteRefStateHasRefs RemoteRefState = "has_refs"
	RemoteRefStateEmpty   RemoteRefState = "empty"
)

// InspectRemoteRefState probes a remote with the supplied credential scope.
// An empty successful advertisement is distinct from an unavailable or
// malformed advertisement, which remains unknown and fails closed upstream.
func (c *Cloner) InspectRemoteRefState(
	ctx context.Context, cloneURL, credentialOrigin, token string,
) (RemoteRefState, error) {
	auth, err := credentialAuth(cloneURL, credentialOrigin, token)
	if err != nil {
		return RemoteRefStateUnknown, err
	}
	return c.remoteRefState(ctx, cloneURL, auth)
}

// InspectLocalRepositoryRemoteRefState probes the origin configured in a
// user-owned checkout without resolving or exposing the configured URL. This
// keeps local repositories on the same typed state path as managed clones
// while preserving local Git credential-helper behavior.
func (c *Cloner) InspectLocalRepositoryRemoteRefState(
	ctx context.Context, repositoryPath string,
) (RemoteRefState, error) {
	if strings.TrimSpace(repositoryPath) == "" {
		return RemoteRefStateUnknown, fmt.Errorf("repository path is required")
	}
	output, stderr, err := runConfiguredGitOutput(ctx, gitFetchTimeout,
		[]string{"-C", repositoryPath, "ls-remote", "--refs", "origin"},
		func(cmd *exec.Cmd) (func(), error) { return configureGitCommand(cmd, nil) },
	)
	if err != nil {
		diagnostic := redactCloneOutput(stderr, "")
		return RemoteRefStateUnknown, fmt.Errorf("inspect local repository refs: %s: %w",
			strings.TrimSpace(diagnostic), err)
	}
	return parseRemoteRefState(string(output))
}

// ListLocalOriginBranches reads the origin configured in a host checkout and
// lists its heads through the same workspace credential boundary used by
// managed clones. HTTPS origins use the provider credential broker; SSH
// origins retain Git's supported SSH transport and host agent behavior.
func (c *Cloner) ListLocalOriginBranches(
	ctx context.Context, repositoryPath string, request GitCredentialRequest,
) ([]string, error) {
	if strings.TrimSpace(repositoryPath) == "" {
		return nil, fmt.Errorf("repository path is required")
	}
	origin, err := c.originURLLocked(ctx, repositoryPath)
	if err != nil {
		return nil, err
	}
	request.CloneURL = origin
	var auth *cloneAuth
	if isHTTPCloneURL(origin) {
		_, auth, err = c.workspaceCloneAuthRequest(ctx, request, "", "")
		if err != nil {
			return nil, err
		}
	}
	output, stderr, err := runConfiguredGitOutput(ctx, gitFetchTimeout,
		[]string{"-C", repositoryPath, "ls-remote", "--heads", "origin"},
		func(cmd *exec.Cmd) (func(), error) { return configureGitCommand(cmd, auth) },
	)
	if err != nil {
		diagnostic := redactCloneOutput(stderr, authToken(auth))
		return nil, fmt.Errorf("inspect local repository branches: %s: %w",
			strings.TrimSpace(diagnostic), err)
	}
	return parseRemoteBranchNames(string(output))
}

func parseRemoteBranchNames(output string) ([]string, error) {
	branches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "refs/heads/") {
			continue
		}
		name := strings.TrimPrefix(fields[1], "refs/heads/")
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		branches = append(branches, name)
	}
	return branches, nil
}

func (c *Cloner) remoteRefState(ctx context.Context, cloneURL string, auth *cloneAuth) (RemoteRefState, error) {
	output, stderr, err := runConfiguredGitOutput(ctx, gitFetchTimeout,
		[]string{"ls-remote", "--refs", "--", cloneURL},
		func(cmd *exec.Cmd) (func(), error) { return configureGitCommand(cmd, auth) },
	)
	if err != nil {
		diagnostic := redactCloneOutput(stderr, authToken(auth))
		return RemoteRefStateUnknown, fmt.Errorf("inspect remote refs: %s: %w",
			strings.TrimSpace(diagnostic), err)
	}
	return parseRemoteRefState(string(output))
}

func runConfiguredGitOutput(
	ctx context.Context,
	execTimeout time.Duration,
	args []string,
	configure func(*exec.Cmd) (func(), error),
) ([]byte, string, error) {
	template := subproc.NewGitCommand(ctx, args...)
	cleanup, err := configure(template)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()

	var stderr bytes.Buffer
	output, runErr, execCtxErr := subproc.RunGitOutputAfterAcquire(
		ctx,
		subproc.GitLifecycle,
		execTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, args...)
			cmd.Dir = template.Dir
			cmd.Env = append([]string(nil), template.Env...)
			cmd.ExtraFiles = append([]*os.File(nil), template.ExtraFiles...)
			cmd.Stderr = &stderr
			return cmd
		},
	)
	if runErr == nil {
		runErr = execCtxErr
	}
	return output, stderr.String(), runErr
}

func parseRemoteRefState(output string) (RemoteRefState, error) {
	hasRef := false
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.HasPrefix(fields[1], "refs/") {
			return RemoteRefStateUnknown, fmt.Errorf("malformed remote ref advertisement")
		}
		hasRef = true
	}
	if !hasRef {
		return RemoteRefStateEmpty, nil
	}
	return RemoteRefStateHasRefs, nil
}

// EnsureWorkspaceClonedWithCredentialRequestAndState performs the existing
// workspace clone flow and returns the authenticated remote-ref state.
func (c *Cloner) EnsureWorkspaceClonedWithCredentialRequestAndState(
	ctx context.Context, request GitCredentialRequest, credentialOrigin, token string,
) (string, RemoteRefState, error) {
	if hasCheckoutOptions(request) {
		return c.ensureWorkspaceCheckoutCache(ctx, request, credentialOrigin, token)
	}
	targetPath, err := c.WorkspaceProviderRepositoryPath(
		request.WorkspaceID, request.Provider, request.ProviderHost, request.ProviderScope,
		request.ProviderRepositoryID, request.Owner, request.Name,
	)
	if err != nil {
		return "", RemoteRefStateUnknown, err
	}
	cloneURL, auth, err := c.workspaceCloneAuthRequest(ctx, request, credentialOrigin, token)
	if err != nil {
		return "", RemoteRefStateUnknown, err
	}
	if _, err := c.ensureClonedAtPathWithOriginVerification(
		ctx, cloneURL, targetPath, auth, request.ProviderScope != "" && request.ProviderRepositoryID != "",
	); err != nil {
		return targetPath, RemoteRefStateUnknown, err
	}
	state, err := c.remoteRefState(ctx, cloneURL, auth)
	if err != nil {
		return targetPath, RemoteRefStateUnknown, err
	}
	return targetPath, state, nil
}

// EnsureWorkspaceClonedWithBasicAuthAndState is the typed-state variant of
// EnsureWorkspaceClonedWithBasicAuth for providers using PAT/basic auth.
func (c *Cloner) EnsureWorkspaceClonedWithBasicAuthAndState(
	ctx context.Context, workspaceID, provider, providerHost,
	cloneURL, owner, name, username, password string,
) (string, RemoteRefState, error) {
	targetPath, err := c.WorkspaceProviderRepoPath(workspaceID, provider, providerHost, owner, name)
	if err != nil {
		return "", RemoteRefStateUnknown, err
	}
	if _, err := c.ensureClonedWithBasicAuth(ctx, targetPath, cloneURL, username, password); err != nil {
		return targetPath, RemoteRefStateUnknown, err
	}
	origin, err := gitCredentialOrigin(cloneURL)
	if err != nil {
		return targetPath, RemoteRefStateUnknown, err
	}
	state, err := c.remoteRefState(ctx, cloneURL, &cloneAuth{
		origin: origin, username: username, password: password,
	})
	if err != nil {
		return targetPath, RemoteRefStateUnknown, err
	}
	return targetPath, state, nil
}

// RefreshWorkspaceRepositoryWithCredentialRequestAndState strictly refreshes
// a workspace checkout and returns the authenticated remote-ref state.
func (c *Cloner) RefreshWorkspaceRepositoryWithCredentialRequestAndState(
	ctx context.Context, request GitCredentialRequest, repositoryPath, credentialOrigin, token string,
) (RemoteRefState, error) {
	targetPath, err := c.WorkspaceProviderRepositoryPath(
		request.WorkspaceID, request.Provider, request.ProviderHost, request.ProviderScope,
		request.ProviderRepositoryID, request.Owner, request.Name,
	)
	if err == nil && hasCheckoutOptions(request) {
		options, validationErr := models.NormalizeRepositoryCheckoutOptions(request.CheckoutOptions)
		if validationErr != nil {
			return RemoteRefStateUnknown, validationErr
		}
		targetPath, err = c.checkoutCachePath(request, options)
	}
	if err != nil {
		return RemoteRefStateUnknown, err
	}
	if !sameFilesystemPath(targetPath, repositoryPath) {
		return RemoteRefStateUnknown, fmt.Errorf("repository path does not match the scoped workspace checkout")
	}
	cloneURL, auth, err := c.workspaceCloneAuthRequest(ctx, request, credentialOrigin, token)
	if err != nil {
		return RemoteRefStateUnknown, err
	}
	if err := c.refreshWorkspaceRepository(ctx, targetPath, cloneURL, auth, request.PRNumber); err != nil {
		return RemoteRefStateUnknown, err
	}
	return c.remoteRefState(ctx, cloneURL, auth)
}

// RefreshWorkspaceRepositoryWithBasicAuthAndState is the typed-state variant
// of RefreshWorkspaceRepositoryWithBasicAuth for providers using PAT/basic
// authentication.
func (c *Cloner) RefreshWorkspaceRepositoryWithBasicAuthAndState(
	ctx context.Context, workspaceID, provider, providerHost,
	cloneURL, owner, name, repositoryPath, username, password string,
) (RemoteRefState, error) {
	targetPath, err := c.WorkspaceProviderRepoPath(workspaceID, provider, providerHost, owner, name)
	if err != nil {
		return RemoteRefStateUnknown, err
	}
	if !sameFilesystemPath(targetPath, repositoryPath) {
		return RemoteRefStateUnknown, fmt.Errorf("repository path does not match the workspace checkout")
	}
	origin, err := gitCredentialOrigin(cloneURL)
	if err != nil {
		return RemoteRefStateUnknown, err
	}
	auth := &cloneAuth{origin: origin, username: username, password: password}
	if err := c.refreshWorkspaceRepository(ctx, targetPath, cloneURL, auth, 0); err != nil {
		return RemoteRefStateUnknown, err
	}
	return c.remoteRefState(ctx, cloneURL, auth)
}
