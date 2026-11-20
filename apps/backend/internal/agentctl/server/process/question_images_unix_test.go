//go:build !windows

package process

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestQuestionImageFIFOAndSourceReplacement(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	outside := t.TempDir()
	mgr := NewManager(&config.InstanceConfig{WorkDir: root, WorkspaceSourceRoots: []string{source}}, logger.Default())
	require.NoError(t, syscall.Mkfifo(filepath.Join(root, "pipe.png"), 0600))
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := mgr.ReadQuestionImage(ctx, "pipe.png", "")
	require.Error(t, err)
	require.NoError(t, os.Symlink(source, filepath.Join(root, "attached")))
	moved := source + "-moved"
	require.NoError(t, os.Rename(source, moved))
	t.Cleanup(func() { _ = os.RemoveAll(moved) })
	require.NoError(t, os.Symlink(outside, source))
	content, err := mgr.ReadQuestionImage(ctx, "secret.png", "attached")
	require.Error(t, err)
	require.Nil(t, content)
}
