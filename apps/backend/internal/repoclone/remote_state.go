package repoclone

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/subproc"
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
	cmd := subproc.NewGitCommand(ctx, "-C", repositoryPath, "ls-remote", "--refs", "origin")
	cleanup, err := configureGitCommand(cmd, nil)
	if err != nil {
		return RemoteRefStateUnknown, err
	}
	defer cleanup()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := subproc.RunGitOutputClass(ctx, subproc.GitLifecycle, cmd)
	if err != nil {
		diagnostic := redactCloneOutput(stderr.String(), "")
		return RemoteRefStateUnknown, fmt.Errorf("inspect local repository refs: %s: %w",
			strings.TrimSpace(diagnostic), err)
	}
	return parseRemoteRefState(string(output))
}

func (c *Cloner) remoteRefState(ctx context.Context, cloneURL string, auth *cloneAuth) (RemoteRefState, error) {
	cmd := subproc.NewGitCommand(ctx, "ls-remote", "--refs", "--", cloneURL)
	cleanup, err := configureGitCommand(cmd, auth)
	if err != nil {
		return RemoteRefStateUnknown, err
	}
	defer cleanup()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := subproc.RunGitOutputClass(ctx, subproc.GitLifecycle, cmd)
	if err != nil {
		diagnostic := redactCloneOutput(stderr.String(), authToken(auth))
		return RemoteRefStateUnknown, fmt.Errorf("inspect remote refs: %s: %w",
			strings.TrimSpace(diagnostic), err)
	}
	return parseRemoteRefState(string(output))
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
