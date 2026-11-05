package sandbox

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxApprovalFilesystemOperationStaysWithinCeiling(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	control := filepath.Join(root, "control")
	require.NoError(t, os.Mkdir(work, 0700))
	require.NoError(t, os.Mkdir(control, 0700))
	existing := filepath.Join(work, "file")
	require.NoError(t, os.WriteFile(existing, []byte("fixture"), 0600))
	ceiling := Grants{Filesystem: []FilesystemGrant{{Path: work, Writable: true}}}
	require.NoError(t, ValidateFilesystemOperation(existing, false, ceiling, []string{control}))
	require.NoError(t, ValidateFilesystemOperation(filepath.Join(work, "new-file"), true, ceiling, []string{control}))
	require.Error(t, ValidateFilesystemOperation(filepath.Join(work, "missing"), false, ceiling, []string{control}))
	require.Error(t, ValidateFilesystemOperation(control, true, ceiling, []string{control}))
	require.Error(t, ValidateFilesystemOperation(root, false, Grants{Filesystem: []FilesystemGrant{{Path: root}}}, []string{control}))
	require.NoError(t, os.Symlink(control, filepath.Join(work, "alias")))
	require.Error(t, ValidateFilesystemOperation(filepath.Join(work, "alias", "new-file"), true, ceiling, []string{control}))
	ceiling.Filesystem[0].Writable = false
	require.Error(t, ValidateFilesystemOperation(existing, true, ceiling, nil))
}
