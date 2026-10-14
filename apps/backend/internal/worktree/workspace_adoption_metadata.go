package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
)

const maximumAdoptionMetadataEntries = 64 * 1024

// Adoption requires self-contained reference/object storage, beyond ordinary
// checkout health. Do not widen the healthy-checkout admission classifier.
func validateAdoptionMetadata(ctx context.Context, h storageworkspaces.DirectoryHandle, path string) error {
	if validateMainGitMetadataLayout(h) != "" {
		return models.ErrWorkspaceAdoptionStale
	}
	entries := 0
	roots := []string{"refs", "info", "objects"}
	for _, name := range roots {
		mode, err := h.LstatEntry(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !mode.IsDir() || mode&os.ModeSymlink != 0 {
			return models.ErrWorkspaceAdoptionStale
		}
		child, err := h.OpenSubdirectory(name)
		if err != nil {
			return err
		}
		err = inspectAdoptionMetadataTree(ctx, child, filepath.Join(path, name), name, &entries, 0)
		_ = child.Close()
		if err != nil {
			return err
		}
	}
	return h.VerifyPath(path)
}

func inspectAdoptionMetadataTree(ctx context.Context, h storageworkspaces.DirectoryHandle, path, relative string, count *int, depth int) error {
	if depth > 32 {
		return models.ErrWorkspaceAdoptionStale
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := h.ReadDir()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		*count++
		if *count > maximumAdoptionMetadataEntries {
			return models.ErrWorkspaceAdoptionStale
		}
		if err := inspectAdoptionMetadataEntry(ctx, h, path, relative, entry.Name(), count, depth); err != nil {
			return err
		}
	}
	return h.VerifyPath(path)
}

func inspectAdoptionMetadataEntry(ctx context.Context, h storageworkspaces.DirectoryHandle, path, relative, name string, count *int, depth int) error {
	rel := filepath.ToSlash(filepath.Join(relative, name))
	switch rel {
	case "info/grafts", "objects/info/alternates", "objects/info/http-alternates":
		return models.ErrWorkspaceAdoptionStale
	}
	mode, err := h.LstatEntry(name)
	if err != nil {
		return err
	}
	if mode&os.ModeSymlink != 0 {
		return models.ErrWorkspaceAdoptionStale
	}
	if mode.IsDir() {
		child, err := h.OpenSubdirectory(name)
		if err != nil {
			return err
		}
		defer func() { _ = child.Close() }()
		return inspectAdoptionMetadataTree(ctx, child, filepath.Join(path, name), rel, count, depth+1)
	}
	if !mode.IsRegular() {
		return models.ErrWorkspaceAdoptionStale
	}
	// Promisor packs may fetch/write objects during nominally read-only traversal.
	if strings.HasPrefix(rel, "objects/pack/") && strings.HasSuffix(name, ".promisor") {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}

func (i *WorkspaceAdoptionInspection) pinSourceMetadata(ctx context.Context) error {
	if i.handles[i.source] == nil {
		source, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(i.source), i.source)
		if err != nil {
			return err
		}
		i.handles[i.source] = source
	}
	path := filepath.Join(i.source, ".git")
	if i.handles[path] == nil {
		metadata, err := i.handles[i.source].OpenSubdirectory(".git")
		if err != nil {
			return err
		}
		i.handles[path] = metadata
	}
	return validateAdoptionMetadata(ctx, i.handles[path], path)
}

func (i *WorkspaceAdoptionInspection) candidateMetadata() (storageworkspaces.DirectoryHandle, error) {
	path := i.Proof.CandidatePath
	mode, err := i.handles[path].LstatEntry(".git")
	if err != nil {
		return nil, err
	}
	if !mode.IsDir() || mode&os.ModeSymlink != 0 {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	gitPath := filepath.Join(path, ".git")
	if h := i.handles[gitPath]; h != nil {
		return h, nil
	}
	metadata, err := i.handles[path].OpenSubdirectory(".git")
	if err != nil {
		return nil, err
	}
	i.handles[gitPath] = metadata
	return metadata, nil
}
