package process

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/imagepreview"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestQuestionImageBoundsAndRoots(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(&config.InstanceConfig{WorkDir: root}, logger.Default())
	var small bytes.Buffer
	require.NoError(t, png.Encode(&small, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	exact := make([]byte, imagepreview.MaxBytes)
	copy(exact, small.Bytes())
	require.NoError(t, os.WriteFile(filepath.Join(root, "exact.png"), exact, 0600))
	result, err := mgr.ReadQuestionImage(t.Context(), "exact.png", "")
	require.NoError(t, err)
	require.Len(t, result.Bytes, imagepreview.MaxBytes)
	require.NoError(t, os.WriteFile(filepath.Join(root, "big.png"), append(exact, 0), 0600))
	result, err = mgr.ReadQuestionImage(t.Context(), "big.png", "")
	require.ErrorIs(t, err, imagepreview.TooLarge)
	require.Nil(t, result)
	var large bytes.Buffer
	require.NoError(t, png.Encode(&large, image.NewGray(image.Rect(0, 0, 4001, 4000))))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pixels.png"), large.Bytes(), 0600))
	_, err = mgr.ReadQuestionImage(t.Context(), "pixels.png", "")
	require.ErrorIs(t, err, imagepreview.TooLarge)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = mgr.ReadQuestionImage(cancelled, "exact.png", "")
	require.ErrorIs(t, err, imagepreview.Unavailable)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.png"), small.Bytes(), 0600))
	moved := root + "-moved"
	require.NoError(t, os.Rename(root, moved))
	t.Cleanup(func() { _ = os.RemoveAll(moved) })
	require.NoError(t, os.Symlink(outside, root))
	result, err = mgr.ReadQuestionImage(t.Context(), "secret.png", "")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestQuestionImageMissingWorkspaceDoesNotUseParent(t *testing.T) {
	parent := t.TempDir()
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "secret.png"), data.Bytes(), 0600))
	mgr := NewManager(&config.InstanceConfig{WorkDir: filepath.Join(parent, "missing")}, logger.Default())
	content, err := mgr.ReadQuestionImage(t.Context(), "secret.png", "")
	require.Error(t, err)
	require.Nil(t, content)
}
