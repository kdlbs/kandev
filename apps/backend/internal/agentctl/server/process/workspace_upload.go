package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// UploadResolution says what to do when an upload's destination already exists.
// The zero value refuses to touch an existing file, so a caller that forgets to
// resolve a conflict cannot silently overwrite one.
type UploadResolution string

const (
	// UploadResolutionNone writes only when the destination is free.
	UploadResolutionNone UploadResolution = ""
	// UploadResolutionReplace overwrites an existing destination.
	UploadResolutionReplace UploadResolution = "replace"
	// UploadResolutionKeepBoth writes beside an existing destination under the
	// next free "name-<n>.ext" variant. The hyphen form is deliberate over the
	// Windows "name (n)" convention: these land in a code workspace, where a
	// space and parentheses need quoting in every shell command and show up in
	// git paths. It also matches the attachment installer.
	UploadResolutionKeepBoth UploadResolution = "keep_both"
)

// ErrUploadConflict reports that the destination exists and the caller supplied
// no resolution. It is deliberately distinct from a validation error so the HTTP
// layer can map it to 409 rather than 400.
var ErrUploadConflict = errors.New("upload destination already exists")

// maxKeepBothAttempts bounds the "name-<n>.ext" search. It matches the bound the
// attachment installer already uses.
const maxKeepBothAttempts = 10000

// uploadTempPrefix marks in-flight upload temporaries so a crash leaves an
// identifiable artifact rather than something that looks like workspace content.
const uploadTempPrefix = ".kandev-upload-"

// UploadConflict is one destination path that already exists.
type UploadConflict struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// ParseUploadResolution converts the wire value into an UploadResolution,
// rejecting anything unrecognized rather than falling back to a default.
func ParseUploadResolution(raw string) (UploadResolution, error) {
	switch UploadResolution(strings.TrimSpace(raw)) {
	case UploadResolutionNone:
		return UploadResolutionNone, nil
	case UploadResolutionReplace:
		return UploadResolutionReplace, nil
	case UploadResolutionKeepBoth:
		return UploadResolutionKeepBoth, nil
	default:
		return UploadResolutionNone, fmt.Errorf("unknown upload resolution: %q", raw)
	}
}

// CheckUploadConflicts reports which of the candidate destinations already
// exist. Every candidate is resolved through the same containment rules as the
// write, so a path that could never be written is an error here rather than a
// conflict the user would be asked to resolve.
func (wt *WorkspaceTracker) CheckUploadConflicts(reqPaths []string) ([]UploadConflict, error) {
	conflicts := make([]UploadConflict, 0, len(reqPaths))
	for _, reqPath := range reqPaths {
		path, err := wt.resolveMutationPath(reqPath)
		if err != nil {
			return nil, err
		}
		info, statErr := path.root.Stat(path.rel)
		_ = path.root.Close()
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, fmt.Errorf("failed to stat %s: %w", reqPath, statErr)
		}
		conflicts = append(conflicts, UploadConflict{Path: reqPath, IsDir: info.IsDir()})
	}
	return conflicts, nil
}

// WriteFileStream streams src into the workspace at reqPath and returns the
// path actually written, which differs from reqPath when the caller asked to
// keep both copies.
//
// The write is staged through a temporary file in the destination directory and
// renamed into place, so an agent reading the workspace never observes a
// partially written file, and a failed upload leaves the destination untouched.
func (wt *WorkspaceTracker) WriteFileStream(
	reqPath string,
	resolution UploadResolution,
	src io.Reader,
) (string, int64, error) {
	path, err := wt.resolveMutationPath(reqPath)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = path.root.Close() }()
	runWorkspaceMutationBarrier()

	dir := filepath.Dir(path.rel)
	if err := path.root.MkdirAll(dir, 0o755); err != nil {
		return "", 0, fmt.Errorf("failed to create directories: %w", err)
	}

	tmpRel, tmp, err := createUploadTemp(path.root, dir)
	if err != nil {
		return "", 0, err
	}
	published := false
	defer func() {
		_ = tmp.Close()
		if !published {
			_ = path.root.Remove(tmpRel)
		}
	}()

	written, err := io.Copy(tmp, src)
	if err != nil {
		return "", 0, fmt.Errorf("failed to write upload: %w", err)
	}
	targetRel, targetReq, err := wt.publishUpload(path.root, tmp, tmpRel, path.rel, reqPath, resolution)
	if err != nil {
		return "", 0, err
	}

	published = true
	notifyRel := wt.mutationNotificationPath(filepath.Join(filepath.Dir(path.safe), filepath.Base(targetRel)))
	wt.notifyFileChange(notifyRel, types.FileOpCreate)

	return targetReq, written, nil
}

// publishUpload preserves the final destination's ordinary permissions before
// publishing complete bytes. Source reads happen outside the mutation lock.
func (wt *WorkspaceTracker) publishUpload(
	root *os.Root,
	tmp *os.File,
	tmpRel, rel, reqPath string,
	resolution UploadResolution,
) (string, string, error) {
	wt.mutationMu.Lock()
	defer wt.mutationMu.Unlock()

	targetRel, targetReq, info, err := resolveUploadTarget(root, rel, reqPath, resolution)
	if err != nil {
		return "", "", err
	}
	if info != nil && info.Mode().IsRegular() {
		if err := tmp.Chmod(info.Mode().Perm()); err != nil {
			return "", "", fmt.Errorf("failed to preserve upload permissions: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		return "", "", fmt.Errorf("failed to flush upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close upload: %w", err)
	}
	if err := root.Rename(tmpRel, targetRel); err != nil {
		return "", "", fmt.Errorf("failed to place upload: %w", err)
	}
	return targetRel, targetReq, nil
}

// resolveUploadTarget applies the caller's resolution and returns the rooted
// relative path, request-shaped path and existing regular Replace metadata.
func resolveUploadTarget(
	root *os.Root,
	rel string,
	reqPath string,
	resolution UploadResolution,
) (string, string, os.FileInfo, error) {
	info, err := root.Stat(rel)
	switch {
	case os.IsNotExist(err):
		// Destination is free; the resolution is irrelevant.
		return rel, reqPath, nil, nil
	case err != nil:
		return "", "", nil, fmt.Errorf("failed to stat target: %w", err)
	}

	switch resolution {
	case UploadResolutionReplace:
		if info.IsDir() {
			return "", "", nil, fmt.Errorf("cannot replace destination: %s is a directory", reqPath)
		}
		return rel, reqPath, info, nil
	case UploadResolutionKeepBoth:
		freeRel, err := nextFreeUploadName(root, rel)
		if err != nil {
			return "", "", nil, err
		}
		return freeRel, replaceLastSegment(reqPath, filepath.Base(freeRel)), nil, nil
	default:
		return "", "", nil, fmt.Errorf("%w: %s", ErrUploadConflict, reqPath)
	}
}

// nextFreeUploadName finds the first unused "name-<n>.ext" beside rel.
func nextFreeUploadName(root *os.Root, rel string) (string, error) {
	dir := filepath.Dir(rel)
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	for i := 1; i <= maxKeepBothAttempts; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext))
		if _, err := root.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("failed to stat candidate name: %w", err)
		}
	}
	return "", fmt.Errorf("no available name for %s", rel)
}

// replaceLastSegment swaps the final segment of a slash-separated request path.
func replaceLastSegment(reqPath, name string) string {
	idx := strings.LastIndex(reqPath, "/")
	if idx < 0 {
		return name
	}
	return reqPath[:idx+1] + name
}

// createUploadTemp opens a uniquely named temporary file inside dir through the
// rooted handle, so the staging file is subject to the same containment as the
// destination.
func createUploadTemp(root *os.Root, dir string) (string, *os.File, error) {
	for attempt := 0; attempt < 1000; attempt++ {
		name := fmt.Sprintf("%s%d-%d", uploadTempPrefix, os.Getpid(), attempt)
		rel := filepath.Join(dir, name)
		f, err := root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			return rel, f, nil
		}
		if !os.IsExist(err) {
			return "", nil, fmt.Errorf("failed to stage upload: %w", err)
		}
	}
	return "", nil, errors.New("failed to stage upload: no free temporary name")
}
