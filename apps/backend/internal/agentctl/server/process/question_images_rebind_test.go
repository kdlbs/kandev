package process

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func writeQuestionImageFixture(t *testing.T, root string, width int) []byte {
	t.Helper()
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, width, 2))))
	require.NoError(t, os.WriteFile(filepath.Join(root, "screen.png"), data.Bytes(), 0600))
	return data.Bytes()
}

func TestQuestionImageWorkspaceRebindAndRollback(t *testing.T) {
	original, replacement := t.TempDir(), t.TempDir()
	before := writeQuestionImageFixture(t, original, 2)
	after := writeQuestionImageFixture(t, replacement, 3)
	mgr := NewManager(&config.InstanceConfig{WorkDir: original}, logger.Default())
	t.Cleanup(mgr.stopWorkspaceTrackers)
	require.NoError(t, mgr.RebindWorkspace(t.Context(), replacement))
	result, err := mgr.ReadQuestionImage(t.Context(), "screen.png", "")
	require.NoError(t, err)
	require.Equal(t, after, result.Bytes)
	require.Error(t, mgr.RebindWorkspace(t.Context(), filepath.Join(replacement, "missing")))
	result, err = mgr.ReadQuestionImage(t.Context(), "screen.png", "")
	require.NoError(t, err)
	require.Equal(t, after, result.Bytes)
	require.NoError(t, mgr.RebindWorkspace(t.Context(), original))
	result, err = mgr.ReadQuestionImage(t.Context(), "screen.png", "")
	require.NoError(t, err)
	require.Equal(t, before, result.Bytes)
}

func TestQuestionImageWorkspaceRescan(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "primary")
	initGitRepoAt(t, primary)
	writeQuestionImageFixture(t, primary, 2)
	expected := writeQuestionImageFixture(t, taskRoot, 3)
	mgr := NewManager(&config.InstanceConfig{WorkDir: primary}, logger.Default())
	t.Cleanup(mgr.stopWorkspaceTrackers)
	require.NoError(t, mgr.RescanRepositories(t.Context(), taskRoot))
	result, err := mgr.ReadQuestionImage(t.Context(), "screen.png", "")
	require.NoError(t, err)
	require.Equal(t, expected, result.Bytes)
}
