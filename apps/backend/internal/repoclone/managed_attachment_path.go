package repoclone

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
)

// InspectManagedAttachmentPath admits only exact paths derived from the registered
// provider identity. Missing managed paths may be cloned; occupied or foreign
// paths are never repaired by deleting, redirecting, or rewriting their origin.
func (c *Cloner) InspectManagedAttachmentPath(ctx context.Context, repo *models.Repository, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if repo == nil || repo.SourceType == "local" || (repo.SourceType != "provider" && repo.LocalPath != "") {
		return false, ErrRepositoryOwnershipMismatch
	}
	destination, err := c.managedAttachmentDestination(repo)
	if err != nil {
		return false, err
	}
	if path == "" {
		path = destination
	}
	if err := c.validateAttachmentCandidate(repo, path, destination); err != nil {
		return false, err
	}
	if err := c.inspectAttachmentAncestors(path); err != nil {
		return false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if !sameAttachmentPath(path, destination) {
			return false, fmt.Errorf("missing legacy managed source: create a separate provider registration with POST /api/v1/workspaces/%s/repositories and select its returned repository ID; preserve the old registration", repo.WorkspaceID)
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, ErrRepositoryOwnershipMismatch
	}
	if err := checkAttachmentGitDirectory(path); err != nil {
		return false, err
	}
	return true, c.verifyAttachmentCheckout(ctx, repo, path)
}

func (c *Cloner) validateAttachmentCandidate(repo *models.Repository, path, destination string) error {
	if !filepath.IsAbs(path) {
		return ErrRepositoryOwnershipMismatch
	}
	if !sameAttachmentPath(path, destination) && !c.isLegacyAttachmentPath(repo, path) {
		return ErrRepositoryOwnershipMismatch
	}
	return nil
}
func sameAttachmentPath(left, right string) bool {
	rel, err := filepath.Rel(filepath.Clean(left), filepath.Clean(right))
	return err == nil && rel == "."
}

func (c *Cloner) isLegacyAttachmentPath(repo *models.Repository, path string) bool {
	if repo.ProviderScope != "" {
		return false
	}
	_, providerPath, legacy, _, ok, err := c.ManagedCloneRelocationPaths(repo)
	return err == nil && ok && (sameAttachmentDerivedPath(path, providerPath) || sameAttachmentDerivedPath(path, legacy))
}

func sameAttachmentDerivedPath(saved, derived string) bool {
	absolute, err := filepath.Abs(derived)
	return err == nil && sameAttachmentPath(saved, absolute)
}

func (c *Cloner) inspectAttachmentAncestors(path string) error {
	base, err := c.ExpandedBasePath()
	if err != nil {
		return err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return err
	}
	return checkAttachmentPathAncestors(path, base)
}

func checkAttachmentPathAncestors(path, base string) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return ErrRepositoryOwnershipMismatch
		}
		if sameAttachmentPath(current, base) {
			return nil
		}
		if filepath.Dir(current) == current {
			return ErrRepositoryOwnershipMismatch
		}
	}
}

func checkAttachmentGitDirectory(path string) error {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil {
		return fmt.Errorf("inspect managed Git directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrRepositoryOwnershipMismatch
	}
	// A managed clone owns its Git common directory. A commondir file redirects
	// repository operations even when .git itself is an ordinary directory.
	if _, err := os.Lstat(filepath.Join(gitDir, "commondir")); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return ErrRepositoryOwnershipMismatch
	}
	return checkAttachmentGitEntries(gitDir)
}

func (c *Cloner) verifyAttachmentCheckout(ctx context.Context, repo *models.Repository, path string) error {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	wantPaths := []string{physical, filepath.Join(physical, ".git"), filepath.Join(physical, ".git")}
	for i, option := range []string{"--show-toplevel", "--absolute-git-dir", "--git-common-dir"} {
		got, err := attachmentGitPath(ctx, path, option)
		if err != nil {
			return err
		}
		if !sameAttachmentPath(got, wantPaths[i]) {
			return ErrRepositoryOwnershipMismatch
		}
	}
	want := strings.TrimSpace(repo.RemoteURL)
	if want == "" {
		want, err = c.BuildCloneURLWithHost(ctx, repo.Provider, repo.ProviderHost, repo.ProviderOwner, repo.ProviderName)
		if err != nil {
			return err
		}
	}
	got, err := c.originURLLocked(ctx, path)
	if err != nil {
		return err
	}
	gotIdentity, wantIdentity := attachmentOriginIdentity(got), attachmentOriginIdentity(want)
	if strings.EqualFold(repo.Provider, "github") {
		gotIdentity, wantIdentity = strings.ToLower(gotIdentity), strings.ToLower(wantIdentity)
	}
	if gotIdentity != wantIdentity {
		return ErrManagedCloneOriginMismatch
	}
	return nil
}

func attachmentOriginIdentity(origin string) string {
	if https := CanonicalHTTPSCloneURL(origin); https != "" {
		origin = https
	}
	return strings.TrimSuffix(strings.TrimSuffix(origin, "/"), ".git")
}

func checkAttachmentGitEntries(gitDir string) error {
	// Git writes through nested ref, log, worktree and object namespaces. Checking
	// only their roots would admit a descendant symlink to unrelated storage.
	// WalkDir inspects links themselves and never follows them.
	if err := filepath.WalkDir(gitDir, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrRepositoryOwnershipMismatch
		}
		return nil
	}); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(gitDir, "objects", "info", "alternates")); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return ErrRepositoryOwnershipMismatch
	}
	return nil
}

func (c *Cloner) managedAttachmentDestination(repo *models.Repository) (string, error) {
	path, err := c.WorkspaceProviderRepositoryPath(repo.WorkspaceID, repo.Provider, repo.ProviderHost, repo.ProviderScope, repo.ProviderRepoID, repo.ProviderOwner, repo.ProviderName)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func attachmentGitPath(ctx context.Context, path, option string) (string, error) {
	cmd := subproc.NewGitCommand(ctx, "-C", path, "rev-parse", option)
	cleanup, err := configureGitCommand(cmd, nil)
	if err != nil {
		return "", err
	}
	output, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd)
	cleanup()
	if err != nil {
		return "", formatGitOriginError(path, output, err)
	}
	// Each query emits one path followed by Git's record newline.
	gitPath := strings.TrimSuffix(string(output), "\n")
	if option == "--git-common-dir" {
		// Relative common directories are rooted at the inspected checkout.
		if !filepath.IsAbs(gitPath) {
			gitPath = filepath.Join(path, gitPath)
		}
		return filepath.EvalSymlinks(gitPath)
	}
	return gitPath, nil
}
