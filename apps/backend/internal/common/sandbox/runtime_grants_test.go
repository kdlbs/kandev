package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyWorkDirectoryReference(t *testing.T) {
	intent := Grants{Filesystem: []FilesystemGrant{{Reference: ReferenceWorkDir, Writable: true}}}
	canonical, err := Canonicalize(intent, nil)
	require.NoError(t, err)
	require.Equal(t, intent.Filesystem, canonical.Filesystem)
	first, second := t.TempDir(), t.TempDir()
	for _, workDir := range []string{first, second} {
		resolved, err := ResolveWorkDirectory(canonical, workDir, nil)
		require.NoError(t, err)
		require.Equal(t, []FilesystemGrant{{Path: workDir, Writable: true}}, resolved.Filesystem)
	}
	require.Equal(t, ReferenceWorkDir, canonical.Filesystem[0].Reference)
	require.NoError(t, WithinCeiling(Grants{Filesystem: []FilesystemGrant{{Reference: ReferenceWorkDir}}}, intent))
	require.ErrorIs(t, WithinCeiling(intent, Grants{Filesystem: []FilesystemGrant{{Reference: ReferenceWorkDir}}}), ErrOutsideCeiling)
	require.ErrorIs(t, WithinCeiling(intent, Grants{Filesystem: []FilesystemGrant{{Path: first, Writable: true}}}), ErrOutsideCeiling)
	for _, grant := range []FilesystemGrant{{Reference: "unknown"}, {Reference: ReferenceWorkDir, Path: first}} {
		_, err = Canonicalize(Grants{Filesystem: []FilesystemGrant{grant}}, nil)
		require.ErrorIs(t, err, ErrInvalidPolicy)
	}
}

func TestSandboxPolicyWorkDirectoryRejectsUnsafeAllocation(t *testing.T) {
	intent := Grants{Filesystem: []FilesystemGrant{{Reference: ReferenceWorkDir, Writable: true}}}
	control := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(control, link))
	for _, dir := range []string{"", control, link, filepath.Join(control, "missing")} {
		_, err := ResolveWorkDirectory(intent, dir, []string{control})
		require.ErrorIs(t, err, ErrInvalidPolicy)
	}
}
