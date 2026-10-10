//go:build unix

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const uploadPermissionUmaskEnv = "KANDEV_TEST_UPLOAD_PERMISSION_UMASK"

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.8
func TestWriteFileStreamUploadUmaskCompatibility(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mask := range []string{"0022", "0077"} {
		t.Run(mask, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestWriteFileStreamUploadUmaskHelper$", "-test.timeout=30s")
			cmd.WaitDelay = time.Second
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if key != uploadPermissionUmaskEnv && key != kandevTestFixtureEnv && key != contributionHistoryGitShimModeEnv {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, uploadPermissionUmaskEnv+"="+mask)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("owned umask child %s failed after wait: %v\n%s", mask, err, output)
			}
		})
	}
}

func TestWriteFileStreamUploadUmaskHelper(t *testing.T) {
	var mask int
	switch os.Getenv(uploadPermissionUmaskEnv) {
	case "0022":
		mask = 0o022
	case "0077":
		mask = 0o077
	default:
		t.Skip("requires an explicitly marked isolated umask child")
	}
	previous := syscall.Umask(mask)
	defer syscall.Umask(previous)
	defaultMode := os.FileMode(0o644 &^ mask)
	for _, tc := range []struct {
		name       string
		resolution UploadResolution
	}{
		{name: "nested/new.dat", resolution: UploadResolutionNone},
		{name: "missing-replace.dat", resolution: UploadResolutionReplace},
		{name: "missing-keep-both.dat", resolution: UploadResolutionKeepBoth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			written, size, err := wt.WriteFileStream(tc.name, tc.resolution, strings.NewReader("new bytes"))
			assertUploadPermissionResult(t, written, size, err, tc.name, "new bytes")
			assertUploadPermissionFile(t, filepath.Join(dir, filepath.FromSlash(written)), "new bytes", defaultMode)
			if tc.name == "nested/new.dat" {
				assertUploadPermissionMode(t, filepath.Join(dir, "nested"), os.FileMode(0o755&^mask))
			}
			assertUploadPermissionNoTemps(t, dir)
		})
	}
	for _, mode := range []os.FileMode{0o600, 0o755} {
		t.Run(mode.String(), func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			path := filepath.Join(dir, "original.dat")
			seedUploadPermissionFile(t, path, "original bytes", mode)
			written, size, err := wt.WriteFileStream("original.dat", UploadResolutionKeepBoth, strings.NewReader("separate bytes"))
			assertUploadPermissionResult(t, written, size, err, "original-1.dat", "separate bytes")
			assertUploadPermissionFile(t, path, "original bytes", mode)
			assertUploadPermissionFile(t, filepath.Join(dir, written), "separate bytes", defaultMode)
			written, size, err = wt.WriteFileStream("original.dat", UploadResolutionReplace, strings.NewReader("replacement bytes"))
			assertUploadPermissionResult(t, written, size, err, "original.dat", "replacement bytes")
			assertUploadPermissionFile(t, path, "replacement bytes", mode)
			assertUploadPermissionNoTemps(t, dir)
		})
	}
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.7
func TestWriteFileStreamReplacementOmitsSpecialFlags(t *testing.T) {
	for _, flag := range []os.FileMode{os.ModeSetuid, os.ModeSetgid, os.ModeSticky} {
		t.Run(flag.String(), func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			path := filepath.Join(dir, "ordinary.dat")
			seedUploadPermissionFile(t, path, "old", 0o600)
			if err := os.Chmod(path, 0o600|flag); err != nil {
				t.Skipf("native special flag setup unavailable: %v", err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&flag == 0 {
				t.Skip("filesystem does not retain the requested special flag")
			}
			written, size, err := wt.WriteFileStream("ordinary.dat", UploadResolutionReplace, strings.NewReader("new"))
			assertUploadPermissionResult(t, written, size, err, "ordinary.dat", "new")
			assertUploadPermissionFile(t, path, "new", 0o600)
			info, err = os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
				t.Fatalf("replacement acquired special flags: %v", info.Mode())
			}
		})
	}
}
