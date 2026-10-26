package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func reserveFixture(t *testing.T) (string, []byte, string) {
	t.Helper()
	content := []byte("runtime")
	return t.TempDir(), content, runtimeDigest(content)
}

func TestReserveRuntimeRootNamesTempBeforeMaterialize(t *testing.T) {
	base, content, digest := reserveFixture(t)
	reserved, err := ReserveRuntimeRoot(base)
	require.NoError(t, err)
	root := reserved.Root()
	require.Equal(t, base, filepath.Dir(root))
	require.True(t, strings.HasPrefix(filepath.Base(root), "attempt-"))
	require.Equal(t, filepath.Join(root, "tmp"), reserved.TempDir())
	info, err := os.Stat(root)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	files, err := reserved.Materialize(t.Context(), content, digest, nil, nil)
	require.NoError(t, err)
	require.Equal(t, root, files.Root)
	require.Equal(t, reserved.TempDir(), files.Directories.Temp)
	require.NoError(t, reserved.Close(), "close after materialize leaves the attempt files to RuntimeFiles")
	_, err = os.Stat(root)
	require.NoError(t, err)
	require.NoError(t, files.Close())
	_, err = os.Stat(root)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestReservedRootCloseRemovesUncommittedRoot(t *testing.T) {
	base, _, _ := reserveFixture(t)
	reserved, err := ReserveRuntimeRoot(base)
	require.NoError(t, err)
	require.NoError(t, reserved.Close())
	require.NoError(t, reserved.Close(), "close is idempotent")
	entries, err := os.ReadDir(base)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestReservedRootFailedMaterializeKeepsOwnershipUntilClose(t *testing.T) {
	base, content, _ := reserveFixture(t)
	reserved, err := ReserveRuntimeRoot(base)
	require.NoError(t, err)
	_, err = reserved.Materialize(t.Context(), content, strings.Repeat("0", 64), nil, nil)
	require.ErrorIs(t, err, ErrInvalidPolicy)
	_, statErr := os.Stat(reserved.Root())
	require.NoError(t, statErr, "the reservation still owns the root after a refused materialize")
	require.NoError(t, reserved.Close())
	_, statErr = os.Stat(reserved.Root())
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestReservedRootsAreNeverReused(t *testing.T) {
	base, _, _ := reserveFixture(t)
	first, err := ReserveRuntimeRoot(base)
	require.NoError(t, err)
	second, err := ReserveRuntimeRoot(base)
	require.NoError(t, err)
	require.NotEqual(t, first.Root(), second.Root())
	require.NoError(t, first.Close())
	require.NoError(t, second.Close())
}

func TestReserveRuntimeRootRejectsUnsafeBase(t *testing.T) {
	_, err := ReserveRuntimeRoot("relative")
	require.ErrorIs(t, err, ErrInvalidPolicy)
}
