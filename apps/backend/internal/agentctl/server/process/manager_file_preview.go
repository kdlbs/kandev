package process

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func (m *Manager) filePreviewStore() *filePreviewPool {
	m.filePreviewOnce.Do(func() {
		m.filePreviews = newFilePreviewPool()
		if err := m.RegisterInstanceCleanupHook(func() error { m.filePreviews.close(); return nil }); err != nil {
			m.filePreviews.close()
		}
	})
	return m.filePreviews
}

// PrepareFilePreview selects metadata without reading file content. The backend
// must obtain user permission before redeeming an external selection token.
func (m *Manager) PrepareFilePreview(ctx context.Context, path, repo string) (*types.FilePreviewMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release, err := m.admitStart()
	if err != nil {
		return nil, err
	}
	defer release()
	tracker, err := m.GetWorkspaceTrackerFor(repo)
	if err != nil || tracker == nil {
		return nil, errFilePreviewUnavailable
	}
	reserved, err := m.filePreviewStore().reserve()
	if err != nil {
		return nil, err
	}
	defer reserved()
	preview, err := prepareFilePreview(tracker.workDir, path)
	if err != nil {
		return nil, err
	}
	_, revision := tracker.recoveryExclusionSnapshot()
	preview.check = func() error {
		current, err := m.GetWorkspaceTrackerFor(repo)
		if err != nil || current != tracker {
			return errFilePreviewChanged
		}
		exclusions, now := tracker.recoveryExclusionSnapshot()
		if now != revision {
			return ErrWorkspaceExclusionsChanged
		}
		return checkPreviewExclusions(tracker.workDir, preview.Path, exclusions)
	}
	if err := preview.validate(); err != nil {
		_ = preview.Close()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = preview.Close()
		return nil, err
	}
	token, err := m.filePreviewStore().add(preview)
	if err != nil {
		_ = preview.Close()
		return nil, err
	}
	return &types.FilePreviewMetadata{Token: token, Path: preview.Path, External: preview.External, SizeBytes: preview.Size}, nil
}

func checkPreviewExclusions(root, path string, exclusions []string) error {
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errFilePreviewUnavailable
	}
	rel, err := filepath.Rel(base, path)
	if err == nil && isRootOwnershipMarkerPath(rel) {
		return errFilePreviewUnavailable
	}
	for _, excluded := range exclusions {
		excludedPath := filepath.Join(base, excluded)
		if relative, err := filepath.Rel(excludedPath, path); err == nil && !pathEscapesRoot(relative) {
			return errFilePreviewUnavailable
		}
		target, err := filepath.EvalSymlinks(excludedPath)
		// A deleted recovery artifact cannot expose bytes. Keep checking the
		// other exclusions, including existing symlink targets.
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return errFilePreviewUnavailable
		}
		relative, err := filepath.Rel(target, path)
		if err == nil && !pathEscapesRoot(relative) {
			return errFilePreviewUnavailable
		}
	}
	return nil
}

// CopyFilePreview redeems one instance-owned selection after host authorization.
func (m *Manager) CopyFilePreview(ctx context.Context, token string, w io.Writer) error {
	return m.filePreviewStore().copyTo(ctx, token, w)
}

// CancelFilePreview releases an unused selection or cancels its active transfer.
func (m *Manager) CancelFilePreview(token string) { m.filePreviewStore().remove(token) }
