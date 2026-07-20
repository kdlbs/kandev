//go:build linux || darwin

package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenGitExcludeNoFollowRejectsSymlink(t *testing.T) {
	gitDir := t.TempDir()
	infoDir := filepath.Join(gitDir, "info")
	require.NoError(t, os.Mkdir(infoDir, 0755))
	externalPath := filepath.Join(t.TempDir(), "external")
	require.NoError(t, os.WriteFile(externalPath, []byte("unchanged"), 0644))
	require.NoError(t, os.Symlink(externalPath, filepath.Join(infoDir, "exclude")))

	file, err := openGitExcludeNoFollow(gitDir)
	if file != nil {
		require.NoError(t, file.Close())
	}
	require.Error(t, err)
	external, readErr := os.ReadFile(externalPath)
	require.NoError(t, readErr)
	require.Equal(t, "unchanged", string(external))
}
