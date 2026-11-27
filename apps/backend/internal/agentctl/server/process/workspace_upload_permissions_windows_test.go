package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.7
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.9
func TestWriteFileStreamReplacementWindowsPermissions(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "writable", mode: 0o600},
		{name: "read only", mode: 0o400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			path := filepath.Join(dir, "attribute.txt")
			seedUploadPermissionFile(t, path, "original Windows content", tc.mode)
			subscriber := watchUploadPermissionChanges(t, wt)
			incoming := "complete Windows replacement"
			written, size, err := wt.WriteFileStream("attribute.txt", UploadResolutionReplace, strings.NewReader(incoming))
			if err != nil {
				if tc.mode&0o200 != 0 {
					t.Fatalf("writable destination replacement failed: %v", err)
				}
				if written != "" || size != 0 {
					t.Fatalf("failed native replacement reported success: path=%q size=%d", written, size)
				}
				assertUploadPermissionFile(t, path, "original Windows content", tc.mode)
				assertUploadPermissionNoChange(t, subscriber)
			} else {
				assertUploadPermissionResult(t, written, size, err, "attribute.txt", incoming)
				assertUploadPermissionFile(t, path, incoming, tc.mode)
				assertUploadPermissionChange(t, subscriber, "attribute.txt")
			}
			assertUploadPermissionNoTemps(t, dir)
		})
	}
}
