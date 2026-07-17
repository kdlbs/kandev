// Command plugin-pack tests. Exercises Pack() (the packaging core) against
// real temp-dir package layouts, verifying both the raw tar contents (for
// -platform-only filtering) and that pkgtar.Install accepts the produced
// archive end to end.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/stretchr/testify/require"
)

// writeTestPackageDir builds a plugin package source directory (not yet
// packed) under t.TempDir(), with a manifest.yaml declaring runtime
// executables for the current host platform plus two extra platforms, a
// dummy binary for each, and a ui/bundle.js. Returns the dir.
func writeTestPackageDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	hostKey := runtime.GOOS + "-" + runtime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: "kandev-plugin-pack-test"
api_version: 1
version: "1.0.0"
display_name: "Pack Test Plugin"
description: "fixture for plugin-pack tests"
author: "kandev"

runtime:
  type: binary
  executables:
    %s: "server/plugin-%s"
    linux-arm64: "server/plugin-linux-arm64"
    windows-amd64: "server/plugin-windows-amd64.exe"

capabilities:
  state: true
`, hostKey, hostKey)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifestYAML), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "server"), 0o755))
	for _, name := range []string{
		"plugin-" + hostKey,
		"plugin-linux-arm64",
		"plugin-windows-amd64.exe",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "server", name), []byte("#!/bin/sh\necho fake-binary\n"), 0o755))
	}

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ui"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ui", "bundle.js"), []byte("console.log('hi');"), 0o644))

	return dir
}

// tarEntryNames decompresses and lists every regular-file entry name in a
// tar.gz archive.
func tarEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if hdr.Typeflag == tar.TypeReg {
			names = append(names, hdr.Name)
		}
	}
	return names
}

func TestPack_ProducesInstallableTarball(t *testing.T) {
	dir := writeTestPackageDir(t)

	var buf bytes.Buffer
	require.NoError(t, Pack(dir, &buf, PackOptions{}))

	names := tarEntryNames(t, buf.Bytes())
	require.Contains(t, names, "manifest.yaml")
	require.Contains(t, names, "server/plugin-"+runtime.GOOS+"-"+runtime.GOARCH)
	require.Contains(t, names, "server/plugin-linux-arm64")
	require.Contains(t, names, "server/plugin-windows-amd64.exe")
	require.Contains(t, names, "ui/bundle.js")
	require.Contains(t, names, "checksums.txt")

	destRoot := t.TempDir()
	result, err := pkgtar.Install(&buf, destRoot)
	require.NoError(t, err)
	require.Equal(t, "kandev-plugin-pack-test", result.Manifest.ID)
	require.Equal(t, "1.0.0", result.Version)
}

func TestPack_PlatformOnly_IncludesOnlyHostBinary(t *testing.T) {
	dir := writeTestPackageDir(t)

	var buf bytes.Buffer
	require.NoError(t, Pack(dir, &buf, PackOptions{PlatformOnly: true}))

	names := tarEntryNames(t, buf.Bytes())
	require.Contains(t, names, "server/plugin-"+runtime.GOOS+"-"+runtime.GOARCH)
	require.NotContains(t, names, "server/plugin-linux-arm64")
	require.NotContains(t, names, "server/plugin-windows-amd64.exe")
	require.Contains(t, names, "manifest.yaml")
	require.Contains(t, names, "ui/bundle.js")

	// The filtered package must still install cleanly on the host platform.
	destRoot := t.TempDir()
	result, err := pkgtar.Install(&buf, destRoot)
	require.NoError(t, err)
	require.Equal(t, "kandev-plugin-pack-test", result.Manifest.ID)
}

func TestPack_MissingManifest_Errors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ui.js"), []byte("x"), 0o644))

	var buf bytes.Buffer
	err := Pack(dir, &buf, PackOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "manifest.yaml")
}

func TestPackToFile_WritesTarballToDisk(t *testing.T) {
	dir := writeTestPackageDir(t)
	out := filepath.Join(t.TempDir(), "nested", "out.tar.gz")

	require.NoError(t, packToFile(dir, out, false))

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	names := tarEntryNames(t, data)
	require.Contains(t, names, "manifest.yaml")
}
