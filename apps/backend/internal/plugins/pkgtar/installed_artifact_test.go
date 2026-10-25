package pkgtar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxRuntimeInstallationCapturesPackageDigest(t *testing.T) {
	installed, err := Install(bytes.NewReader(writeValidPackage(t, "1.0.0")), t.TempDir())
	require.NoError(t, err)
	manifest, err := os.ReadFile(filepath.Join(installed.InstallPath, checksumsFileName))
	require.NoError(t, err)
	digest := sha256.Sum256(manifest)
	require.Equal(t, hex.EncodeToString(digest[:]), installed.PackageDigest)
}

func TestSandboxRuntimeReadsPinnedInstalledBytes(t *testing.T) {
	installed, err := Install(bytes.NewReader(writeValidPackage(t, "1.0.0")), t.TempDir())
	require.NoError(t, err)
	name := "server/plugin-" + hostPlatformKey
	expected := buildValidFiles("1.0.0")[name]
	digest := sha256.Sum256(expected)
	spec := InstalledFileSpec{Name: name, PackageDigest: installed.PackageDigest, Digest: hex.EncodeToString(digest[:]), MaxBytes: 1024}
	got, err := ReadInstalledFile(t.Context(), installed.InstallPath, spec)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestSandboxRuntimeRefusesChangedOrForeignFiles(t *testing.T) {
	for _, kind := range []string{"missing package digest", "wrong package digest", "wrong file digest", "changed manifest", "changed binary", "unlisted", "traversal", "symlink", "directory", "oversized", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			installed, err := Install(bytes.NewReader(writeValidPackage(t, "1.0.0")), t.TempDir())
			require.NoError(t, err)
			name := "server/plugin-" + hostPlatformKey
			expected := buildValidFiles("1.0.0")[name]
			digest := sha256.Sum256(expected)
			spec := InstalledFileSpec{Name: name, PackageDigest: installed.PackageDigest, Digest: hex.EncodeToString(digest[:]), MaxBytes: 1024}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch kind {
			case "missing package digest":
				spec.PackageDigest = ""
			case "wrong package digest":
				spec.PackageDigest = strings.Repeat("a", 64)
			case "wrong file digest":
				spec.Digest = strings.Repeat("a", 64)
			case "changed manifest":
				require.NoError(t, os.WriteFile(filepath.Join(installed.InstallPath, checksumsFileName), []byte("changed"), 0600))
			case "changed binary":
				require.NoError(t, os.WriteFile(filepath.Join(installed.InstallPath, name), []byte("changed"), 0600))
			case "unlisted":
				spec.Name = "extra"
				require.NoError(t, os.WriteFile(filepath.Join(installed.InstallPath, "extra"), expected, 0600))
			case "traversal":
				spec.Name = "../foreign"
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				require.NoError(t, os.WriteFile(outside, expected, 0600))
				require.NoError(t, os.Remove(filepath.Join(installed.InstallPath, name)))
				require.NoError(t, os.Symlink(outside, filepath.Join(installed.InstallPath, name)))
			case "directory":
				require.NoError(t, os.Remove(filepath.Join(installed.InstallPath, name)))
				require.NoError(t, os.Mkdir(filepath.Join(installed.InstallPath, name), 0700))
			case "oversized":
				spec.MaxBytes = 1
			case "cancelled":
				cancel()
			}
			got, err := ReadInstalledFile(ctx, installed.InstallPath, spec)
			require.Error(t, err)
			require.Empty(t, got)
		})
	}
}
