// Integration test proving Pack() accepts the real
// cmd/plugin-fixture/fixture-package directory (the same source
// `make e2e-plugin-package` packs) and produces an archive pkgtar.Install
// accepts — the exact contract the e2e suite depends on.
package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/stretchr/testify/require"
)

// stageFixturePackage copies cmd/plugin-fixture/fixture-package's
// manifest.yaml and ui/ into a fresh temp dir, plus a dummy (non-executed)
// binary at the manifest's declared path for the test host's GOOS/GOARCH —
// pkgtar.Install only requires the file to exist, not to actually run.
func stageFixturePackage(t *testing.T) string {
	t.Helper()
	srcDir := filepath.Join("..", "plugin-fixture", "fixture-package")

	manifestData, err := os.ReadFile(filepath.Join(srcDir, "manifest.yaml"))
	require.NoError(t, err)
	m, err := manifest.Parse(manifestData)
	require.NoError(t, err)
	execPath, ok := m.ExecutableFor(runtime.GOOS, runtime.GOARCH)
	require.True(t, ok, "fixture-package manifest.yaml has no runtime.executables entry for the test host %s-%s", runtime.GOOS, runtime.GOARCH)

	stage := t.TempDir()
	require.NoError(t, copyFile(filepath.Join(srcDir, "manifest.yaml"), filepath.Join(stage, "manifest.yaml")))
	require.NoError(t, copyFile(filepath.Join(srcDir, "ui", "bundle.js"), filepath.Join(stage, "ui", "bundle.js")))

	dummyBinPath := filepath.Join(stage, filepath.FromSlash(execPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(dummyBinPath), 0o755))
	require.NoError(t, os.WriteFile(dummyBinPath, []byte("#!/bin/sh\necho fixture\n"), 0o755))

	return stage
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func TestPack_FixturePackageDir_ProducesInstallablePackage(t *testing.T) {
	stage := stageFixturePackage(t)

	var buf bytes.Buffer
	require.NoError(t, Pack(stage, &buf, PackOptions{PlatformOnly: true}))

	destRoot := t.TempDir()
	result, err := pkgtar.Install(&buf, destRoot)
	require.NoError(t, err)
	require.Equal(t, "kandev-plugin-e2e", result.Manifest.ID)
	require.Equal(t, "1.0.0", result.Version)
}
