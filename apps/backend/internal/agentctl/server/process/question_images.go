package process

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/common/imagepreview"
)

// ReadQuestionImage reads only a raster file inside an authorized workspace root.
func (m *Manager) ReadQuestionImage(ctx context.Context, path, repo string) (*imagepreview.Content, error) {
	ref := protocol.QuestionImage{ID: "preview", Path: path, Repo: repo, Alt: "Preview"}
	if err := ref.Validate(); err != nil {
		return nil, imagepreview.Unsupported
	}
	m.questionImageOnce.Do(func() { m.questionImageSlots = make(chan struct{}, 2) })
	select {
	case m.questionImageSlots <- struct{}{}:
		defer func() { <-m.questionImageSlots }()
	case <-ctx.Done():
		return nil, imagepreview.Unavailable
	}
	m.repoTrackersMu.RLock()
	root, err := m.openQuestionImageRoot(repo)
	m.repoTrackersMu.RUnlock()
	if err != nil {
		return nil, questionImageFileError(err)
	}
	defer func() { _ = root.Close() }()
	if err = ctx.Err(); err != nil {
		return nil, imagepreview.Unavailable
	}
	info, err := root.Stat(filepath.FromSlash(path))
	if err != nil {
		return nil, questionImageFileError(err)
	}
	if !info.Mode().IsRegular() {
		return nil, imagepreview.Unsupported
	}
	file, err := openQuestionImageFile(root, filepath.FromSlash(path))
	if err != nil {
		return nil, questionImageFileError(err)
	}
	defer func() { _ = file.Close() }()
	info, err = file.Stat()
	if err != nil {
		return nil, imagepreview.Unavailable
	}
	if !info.Mode().IsRegular() {
		return nil, imagepreview.Unsupported
	}
	if info.Size() > imagepreview.MaxBytes {
		return nil, imagepreview.TooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, imagepreview.MaxBytes+1))
	if err != nil || ctx.Err() != nil {
		return nil, imagepreview.Unavailable
	}
	if len(data) > imagepreview.MaxBytes {
		return nil, imagepreview.TooLarge
	}
	return decodeQuestionImage(ctx, data, path)
}

func decodeQuestionImage(ctx context.Context, data []byte, path string) (*imagepreview.Content, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, imagepreview.InvalidImage
	}
	if format != "png" && format != "jpeg" {
		return nil, imagepreview.Unsupported
	}
	suffix := strings.ToLower(filepath.Ext(path))
	if (format == "png" && suffix != ".png") || (format == "jpeg" && suffix != ".jpg" && suffix != ".jpeg") {
		return nil, imagepreview.InvalidImage
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, imagepreview.InvalidImage
	}
	if config.Width > imagepreview.MaxPixels/config.Height {
		return nil, imagepreview.TooLarge
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return nil, imagepreview.InvalidImage
	}
	if ctx.Err() != nil {
		return nil, imagepreview.Unavailable
	}
	return &imagepreview.Content{Bytes: data, MIME: "image/" + format}, nil
}

func questionImageFileError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return imagepreview.NotFound
	}
	return imagepreview.Unavailable
}

// setWorkspaceWorkDirLocked keeps image reads and trackers in the same workspace.
// The caller holds repoTrackersMu. An unchanged root retains its original scope.
func (m *Manager) setWorkspaceWorkDirLocked(workDir string) {
	if workDir != m.cfg.WorkDir {
		m.questionImageWorkDir, _ = filepath.EvalSymlinks(workDir)
	}
	m.cfg.WorkDir = workDir
}

// openQuestionImageRoot selects a manifest-authorized root, then opens through
// directory handles. A replacement symlink can never redirect the root open.
// The caller holds repoTrackersMu while selecting and opening the handle.
func (m *Manager) openQuestionImageRoot(repo string) (*os.Root, error) {
	if m.questionImageWorkDir == "" {
		return nil, imagepreview.Unavailable
	}
	selected := filepath.Join(m.questionImageWorkDir, filepath.FromSlash(repo))
	resolved, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return nil, err
	}
	for _, base := range append([]string{m.questionImageWorkDir}, m.workspaceSourceRoots...) {
		relative, relErr := filepath.Rel(base, resolved)
		if relErr != nil || pathEscapesRoot(relative) {
			continue
		}
		root, openErr := openQuestionImageBase(base)
		if openErr != nil {
			return nil, openErr
		}
		if relative == "." {
			return root, nil
		}
		child, childErr := root.OpenRoot(relative)
		_ = root.Close()
		return child, childErr
	}
	return nil, imagepreview.Unavailable
}

func openQuestionImageBase(path string) (*os.Root, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(absolute) + string(os.PathSeparator)
	root, err := os.OpenRoot(volume)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(absolute, volume), string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		before, statErr := root.Lstat(part)
		if statErr != nil {
			_ = root.Close()
			return nil, statErr
		}
		if !before.IsDir() {
			_ = root.Close()
			return nil, imagepreview.Unavailable
		}
		next, openErr := root.OpenRoot(part)
		_ = root.Close()
		if openErr != nil {
			return nil, openErr
		}
		after, statErr := next.Stat(".")
		if statErr != nil || !os.SameFile(before, after) {
			_ = next.Close()
			return nil, imagepreview.Unavailable
		}
		root = next
	}
	return root, nil
}
