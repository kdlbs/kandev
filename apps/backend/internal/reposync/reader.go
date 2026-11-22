package reposync

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

// GitHubClientProvider exposes workspace-routed GitHub repository reads.
type GitHubClientProvider interface {
	BindWorkspaceGitHubHost(context.Context, string, string) (context.Context, error)
	CommitWorkspaceGitHubHost(context.Context, string, func(context.Context) error) error
	ListRepoDirectoryForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]github.RepoContentEntry, error)
	GetRepoFileContentForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]byte, error)
}

// GitLabClientProvider exposes workspace-routed GitLab repository reads.
type GitLabClientProvider interface {
	ListRepoTreeForWorkspace(
		ctx context.Context, workspaceID, projectPath, path, ref string,
	) ([]gitlab.RepoTreeEntry, error)
	GetRepoFileContentForWorkspace(
		ctx context.Context, workspaceID, projectPath, path, ref string,
	) ([]byte, error)
}

// Compile-time checks that both integrations' services satisfy the
// interfaces, so drift breaks the build rather than DI wiring.
var (
	_ GitHubClientProvider = (*github.Service)(nil)
	_ GitLabClientProvider = (*gitlab.Service)(nil)
)

// Source identifies one directory read through a workspace's connection.
type Source struct {
	WorkspaceID string
	Provider    string
	RepoOwner   string
	RepoName    string
	ProjectPath string
	Branch      string
	Path        string
}

// Entry is a provider-neutral directory listing entry.
type Entry struct {
	Name   string
	Path   string
	IsFile bool
	IsDir  bool
}

// FileGetter fetches one file's content for an already-fixed source.
type FileGetter func(ctx context.Context, path string) ([]byte, error)

// Reader dispatches repository reads to the configured provider. Either
// client may be nil; reads for that provider then fail with a not-configured
// error instead of failing construction.
type Reader struct {
	GitHub GitHubClientProvider
	GitLab GitLabClientProvider
}

// List lists the immediate entries of src.Path in src's repository and ref.
func (r Reader) List(ctx context.Context, src Source) ([]Entry, error) {
	if src.Provider == ProviderGitLab {
		return r.listGitLab(ctx, src)
	}
	return r.listGitHub(ctx, src)
}

func (r Reader) listGitHub(ctx context.Context, src Source) ([]Entry, error) {
	if r.GitHub == nil {
		return nil, ErrGitHubClientNotConfigured
	}
	raw, err := r.GitHub.ListRepoDirectoryForWorkspace(
		ctx, src.WorkspaceID, src.RepoOwner, src.RepoName, src.Path, src.Branch,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s/%s@%s:%s: %w", src.RepoOwner, src.RepoName, src.Branch, src.Path, err)
	}
	entries := make([]Entry, len(raw))
	for i, e := range raw {
		entries[i] = Entry{Name: e.Name, Path: e.Path, IsFile: e.Type == "file", IsDir: e.Type == "dir"}
	}
	return entries, nil
}

func (r Reader) listGitLab(ctx context.Context, src Source) ([]Entry, error) {
	if r.GitLab == nil {
		return nil, ErrGitLabClientNotConfigured
	}
	raw, err := r.GitLab.ListRepoTreeForWorkspace(ctx, src.WorkspaceID, src.ProjectPath, src.Path, src.Branch)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s@%s:%s: %w", src.ProjectPath, src.Branch, src.Path, err)
	}
	entries := make([]Entry, len(raw))
	for i, e := range raw {
		entries[i] = Entry{
			Name: e.Name, Path: e.Path,
			IsFile: e.Type == gitlab.TreeEntryTypeBlob, IsDir: e.Type == gitlab.TreeEntryTypeTree,
		}
	}
	return entries, nil
}

// IsFileTooLarge reports whether a file read failed only because the provider
// refused to return a file of that size.
func IsFileTooLarge(err error) bool {
	return errors.Is(err, github.ErrRepoFileTooLarge) || errors.Is(err, gitlab.ErrRepoFileTooLarge)
}

// Getter returns a FileGetter bound to src's provider, workspace, and ref.
func (r Reader) Getter(src Source) (FileGetter, error) {
	if src.Provider == ProviderGitLab {
		if r.GitLab == nil {
			return nil, ErrGitLabClientNotConfigured
		}
		return func(ctx context.Context, path string) ([]byte, error) {
			return r.GitLab.GetRepoFileContentForWorkspace(ctx, src.WorkspaceID, src.ProjectPath, path, src.Branch)
		}, nil
	}
	if r.GitHub == nil {
		return nil, ErrGitHubClientNotConfigured
	}
	return func(ctx context.Context, path string) ([]byte, error) {
		return r.GitHub.GetRepoFileContentForWorkspace(
			ctx, src.WorkspaceID, src.RepoOwner, src.RepoName, path, src.Branch,
		)
	}, nil
}
