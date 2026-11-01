package launchwrapper

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// trustBoundaryForTest makes root the trusted ancestor boundary, so the
// operating system's temporary directory chain is not judged. Only tests can
// call it; production always walks to the filesystem root.
func trustBoundaryForTest(t *testing.T, root string) {
	t.Helper()
	previous := nodeTrustBoundary
	nodeTrustBoundary = func() string { return root }
	t.Cleanup(func() { nodeTrustBoundary = previous })
}

type fakeDirInfo struct {
	mode os.FileMode
	uid  uint32
}

func (f fakeDirInfo) Name() string       { return "dir" }
func (f fakeDirInfo) Size() int64        { return 0 }
func (f fakeDirInfo) Mode() os.FileMode  { return f.mode }
func (f fakeDirInfo) ModTime() time.Time { return time.Time{} }
func (f fakeDirInfo) IsDir() bool        { return true }
func (f fakeDirInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

// @covers AC-AGENTS-NODEWRAP-001.3
func TestAncestorCheckHasNoStickyException(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX ownership model")
	}
	owner := os.Geteuid()
	for name, info := range map[string]fakeDirInfo{
		"root sticky world-writable":  {mode: os.ModeDir | os.ModeSticky | 0o777, uid: 0},
		"root sticky group-writable":  {mode: os.ModeDir | os.ModeSticky | 0o775, uid: 0},
		"owner sticky world-writable": {mode: os.ModeDir | os.ModeSticky | 0o777, uid: uint32(owner)},
		"root world-writable":         {mode: os.ModeDir | 0o777, uid: 0},
	} {
		require.ErrorIs(t, checkOwnedNotWritable(info, owner, true), ErrNodeFile, name)
	}
	for name, info := range map[string]fakeDirInfo{
		"root 0755":   {mode: os.ModeDir | 0o755, uid: 0},
		"owner 0700":  {mode: os.ModeDir | 0o700, uid: uint32(owner)},
		"root sticky": {mode: os.ModeDir | os.ModeSticky | 0o755, uid: 0},
	} {
		require.NoError(t, checkOwnedNotWritable(info, owner, true), name)
	}
}

// @covers AC-AGENTS-NODEWRAP-001.3
func TestWritableAndStickyAncestorsAreRefusedUnderDefaultBoundary(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX ownership model")
	}
	require.Empty(t, nodeTrustBoundary(), "production default trusts nothing above the filesystem root")
	for name, mode := range map[string]os.FileMode{
		"world writable": 0o777,
		"sticky":         0o777 | os.ModeSticky,
		"group writable": 0o775,
	} {
		t.Run(name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			bad := filepath.Join(root, "bad")
			require.NoError(t, os.Mkdir(bad, 0o755))
			wrapper := writeExecutable(t, bad, "wrap", "#!/bin/sh\n")
			path := writeNodeFile(t, bad, 0o600, wrapper)
			require.NoError(t, os.Chmod(bad, mode))

			_, err = LoadNodeFile(path, -1)
			require.ErrorIs(t, err, ErrNodeFile, "default boundary")

			trustBoundaryForTest(t, root)
			_, err = LoadNodeFile(path, -1)
			require.ErrorIs(t, err, ErrNodeFile, "the offending ancestor is judged even when the temp root is trusted")

			require.NoError(t, os.Chmod(bad, 0o755))
			_, err = LoadNodeFile(path, -1)
			require.NoError(t, err, "same tree is accepted once the ancestor is not writable")
		})
	}
}

// @covers AC-AGENTS-NODEWRAP-001.3
func TestBoundaryOutsideThePathIsIgnored(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX ownership model")
	}
	dir := secureDir(t)
	wrapper := writeExecutable(t, dir, "wrap", "#!/bin/sh\n")
	path := writeNodeFile(t, dir, 0o600, wrapper)
	trustBoundaryForTest(t, filepath.Join(t.TempDir(), "elsewhere"))
	_, err := LoadNodeFile(path, -1)
	if err == nil {
		t.Skip("whole temp chain happens to be private on this host")
	}
	require.ErrorIs(t, err, ErrNodeFile)
}
