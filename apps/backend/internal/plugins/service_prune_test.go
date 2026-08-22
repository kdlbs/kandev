package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"testing"

	"github.com/kandev/kandev/internal/plugins/store"
)

const prunePluginID = "kandev-plugin-slack"

// seedVersionDir writes a version directory shaped exactly like one
// pkgtar.Install extracts (a manifest.yaml whose id/version match the
// directory name, plus a payload file), so the prune scan sees a real
// superseded install rather than an empty directory.
func seedVersionDir(t *testing.T, pluginsDir, id, version string) string {
	t.Helper()
	dir := filepath.Join(pluginsDir, id, version)
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: %s
display_name: Test Plugin
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, version, platformKey)
	if err := os.WriteFile(filepath.Join(dir, manifestFileName), []byte(manifestYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.yaml): %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server", "plugin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(server/plugin): %v", err)
	}
	return dir
}

// versionDirsOnDisk lists every directory entry under <pluginsDir>/<id>/,
// sorted, so a test can assert the exact surviving set (including entries a
// prune must never touch, like "data").
func versionDirsOnDisk(t *testing.T, pluginsDir, id string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(pluginsDir, id))
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", filepath.Join(pluginsDir, id), err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func assertVersionDirs(t *testing.T, pluginsDir, id string, want ...string) {
	t.Helper()
	got := versionDirsOnDisk(t, pluginsDir, id)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("version dirs on disk = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("version dirs on disk = %v, want %v", got, want)
		}
	}
}

// TestServiceInstallPrunesSupersededVersions pins the retention rule: after a
// successful install whose new version started cleanly, only the newly active
// version and the immediately-previous one (the rollback target) survive.
// Every install used to leave its predecessor behind forever — 46 copies of a
// single plugin, 3.74GB of superseded versions on a real machine.
func TestServiceInstallPrunesSupersededVersions(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)

	installTestPlugin(t, svc, prunePluginID) // 1.0.0
	if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, "1.1.0", false)); err != nil {
		t.Fatalf("Install(1.1.0): %v", err)
	}
	if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, "1.2.0", false)); err != nil {
		t.Fatalf("Install(1.2.0): %v", err)
	}

	assertVersionDirs(t, dir, prunePluginID, "1.1.0", "1.2.0")
}

// TestServiceInstallPrunesAccumulatedVersions covers the already-broken
// instance: many superseded versions predate the fix, and one successful
// install must collapse them to the retained pair.
func TestServiceInstallPrunesAccumulatedVersions(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0, active
	for _, v := range []string{"0.1.0", "0.2.0", "0.9.9"} {
		seedVersionDir(t, dir, prunePluginID, v)
	}

	if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, "1.1.0", false)); err != nil {
		t.Fatalf("Install(1.1.0): %v", err)
	}

	assertVersionDirs(t, dir, prunePluginID, "1.0.0", "1.1.0")
}

// TestServiceInstallPruneKeepsRollbackTarget pins that the first upgrade
// prunes nothing: the previous version is the rollback target a failed
// restart falls back to.
func TestServiceInstallPruneKeepsRollbackTarget(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0

	if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, "1.1.0", false)); err != nil {
		t.Fatalf("Install(1.1.0): %v", err)
	}

	assertVersionDirs(t, dir, prunePluginID, "1.0.0", "1.1.0")
}

// TestServiceInstallPruneKeepsDataDirectory pins the hard constraint: the
// plugin's writable data directory is durable state that survives every
// upgrade and is removed only on uninstall.
func TestServiceInstallPruneKeepsDataDirectory(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0

	marker := filepath.Join(dir, prunePluginID, "data", "marker.txt")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatalf("MkdirAll(data): %v", err)
	}
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker): %v", err)
	}

	for _, v := range []string{"1.1.0", "1.2.0"} {
		if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, v, false)); err != nil {
			t.Fatalf("Install(%s): %v", v, err)
		}
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("plugin data dir marker was removed by the prune: %v", err)
	}
	assertVersionDirs(t, dir, prunePluginID, "1.1.0", "1.2.0", "data")
}

// TestServiceInstallPruneNeverRemovesDataDirWithManifest pins the data
// directory's exclusion by name, independently of the manifest check that
// also happens to cover it. The directory is plugin-writable, so its contents
// are not something the host gets to reason about: a plugin that writes its
// own manifest.yaml there must not be able to talk the prune into deleting
// its durable state.
func TestServiceInstallPruneNeverRemovesDataDirWithManifest(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0
	seedVersionDir(t, dir, prunePluginID, pluginDataDirName)
	marker := filepath.Join(dir, prunePluginID, pluginDataDirName, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker): %v", err)
	}

	for _, v := range []string{"1.1.0", "1.2.0"} {
		if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, v, false)); err != nil {
			t.Fatalf("Install(%s): %v", v, err)
		}
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("plugin data dir was removed by the prune: %v", err)
	}
}

// TestServiceInstallPruneSkipsNonVersionDirectories pins that the prune only
// removes directories that actually parse as an installed version: an
// operator's stray directory, a directory whose manifest names a different
// version, and pkgtar's in-flight ".tmp-*" staging directory are all left
// alone.
func TestServiceInstallPruneSkipsNonVersionDirectories(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0
	seedVersionDir(t, dir, prunePluginID, "0.9.0")

	pluginDir := filepath.Join(dir, prunePluginID)
	if err := os.MkdirAll(filepath.Join(pluginDir, "notes"), 0o755); err != nil {
		t.Fatalf("MkdirAll(notes): %v", err)
	}
	// pkgtar extracts into a ".tmp-*" dir under the plugin directory before
	// renaming it into place, so a concurrent install has a fully populated
	// staging dir here that the prune must not delete out from under it.
	staging := seedVersionDir(t, dir, prunePluginID, "1.3.0")
	if err := os.Rename(staging, filepath.Join(pluginDir, ".tmp-inflight")); err != nil {
		t.Fatalf("Rename(.tmp-inflight): %v", err)
	}
	// A directory whose manifest declares a different version than its own
	// name is not something the installer produced; leave it to the operator.
	mismatched := seedVersionDir(t, dir, prunePluginID, "0.8.0")
	if err := os.Rename(mismatched, filepath.Join(pluginDir, "misnamed")); err != nil {
		t.Fatalf("Rename(misnamed): %v", err)
	}

	for _, v := range []string{"1.1.0", "1.2.0"} {
		if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, v, false)); err != nil {
			t.Fatalf("Install(%s): %v", v, err)
		}
	}

	assertVersionDirs(t, dir, prunePluginID, "1.1.0", "1.2.0", "notes", ".tmp-inflight", "misnamed")
}

// TestServiceInstallSpawnFailurePrunesNothing pins that pruning happens only
// after the new version is confirmed started. A failed spawn must leave every
// other version on disk, because the operator's way out is the previous
// version.
func TestServiceInstallSpawnFailurePrunesNothing(t *testing.T) {
	svc, dir, _, rt := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0
	seedVersionDir(t, dir, prunePluginID, "0.9.0")

	rt.setStartErr(prunePluginID, errors.New("spawn failed"))
	if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, "1.1.0", false)); err == nil {
		t.Fatal("Install() expected an error when the new version fails to spawn")
	}

	assertVersionDirs(t, dir, prunePluginID, "0.9.0", "1.0.0", "1.1.0")
}

// TestServiceInstallSaveFailurePrunesNothingAndRestartsPrevious pins the
// other failure mode: rollbackFailedInstall removes only the just-extracted
// version and restarts the previous one, and the prune never runs.
func TestServiceInstallSaveFailurePrunesNothingAndRestartsPrevious(t *testing.T) {
	dir := t.TempDir()
	fsStore := store.NewFSStore(dir)
	failing := &failingSaveStore{Store: fsStore}
	svc := NewService(failing, NewRegistry(), nil, testLogger(t))
	svc.SetPluginsDir(dir)
	rt := newFakeRuntime()
	svc.SetRuntime(rt)

	installTestPlugin(t, svc, prunePluginID) // 1.0.0, active + running
	seedVersionDir(t, dir, prunePluginID, "0.9.0")

	failing.armFailNextSave()
	if _, err := svc.Install(context.Background(), testPackage(t, prunePluginID, "1.1.0", false)); err == nil {
		t.Fatal("Install() expected an error from the simulated Save failure")
	}

	assertVersionDirs(t, dir, prunePluginID, "0.9.0", "1.0.0")
	if !rt.Running(prunePluginID) {
		t.Fatal("the previous version's process was not restarted after the failed upgrade")
	}
}

// TestServiceInstallPruneFailureDoesNotFailInstall pins that cleanup is
// best-effort: a plugin that installed and started correctly is never
// reported as failed because a superseded directory could not be removed.
func TestServiceInstallPruneFailureDoesNotFailInstall(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0
	seedVersionDir(t, dir, prunePluginID, "0.9.0")

	pluginDir := filepath.Join(dir, prunePluginID)
	requireUnwritableDir(t, pluginDir)

	rec, err := svc.Install(context.Background(), testPackage(t, prunePluginID, "1.1.0", false))
	if err != nil {
		t.Fatalf("Install() failed because the prune could not delete a directory: %v", err)
	}
	if rec.Status != StatusActive {
		t.Fatalf("Install() Status = %q, want %q", rec.Status, StatusActive)
	}
}

// requireUnwritableDir makes dir reject child removal, skipping the test when
// the current executor does not enforce the permission bit (root, or a
// filesystem/OS that ignores it). New files must be creatable first, since
// pkgtar extracts into this same directory.
func requireUnwritableDir(t *testing.T, dir string) {
	t.Helper()
	probeParent := filepath.Join(t.TempDir(), "probe")
	if err := os.MkdirAll(filepath.Join(probeParent, "child"), 0o755); err != nil {
		t.Fatalf("MkdirAll(probe): %v", err)
	}
	if err := os.Chmod(probeParent, 0o555); err != nil {
		t.Skipf("chmod unsupported on this platform: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(probeParent, 0o755) })
	if err := os.RemoveAll(filepath.Join(probeParent, "child")); err == nil {
		t.Skip("this executor ignores the directory write bit (running as root?)")
	}

	// The install itself must still be able to create the new version
	// directory, so make only the superseded version's own contents
	// undeletable rather than the plugin directory.
	stale := filepath.Join(dir, "0.9.0")
	if err := os.Chmod(stale, 0o555); err != nil {
		t.Skipf("chmod unsupported on this platform: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(stale, 0o755) })
}

// TestStartActivePluginsPrunesAfterHealthyStart pins boot-time recovery: an
// instance that already accumulated superseded versions collapses them on the
// next restart, without waiting for an install that may never come.
func TestStartActivePluginsPrunesAfterHealthyStart(t *testing.T) {
	svc, dir, _, rt := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0, active
	for _, v := range []string{"0.7.0", "0.8.0", "0.9.0"} {
		seedVersionDir(t, dir, prunePluginID, v)
	}
	rt.Stop(prunePluginID) // simulate a fresh process: registered active, not running

	svc.StartActivePlugins(context.Background())

	if !rt.Running(prunePluginID) {
		t.Fatal("StartActivePlugins() did not start the active plugin")
	}
	assertVersionDirs(t, dir, prunePluginID, "0.9.0", "1.0.0")
}

// TestStartActivePluginsDoesNotPruneWhenStartFails pins the same
// confirmed-start precondition at boot.
func TestStartActivePluginsDoesNotPruneWhenStartFails(t *testing.T) {
	svc, dir, _, rt := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0, active
	seedVersionDir(t, dir, prunePluginID, "0.8.0")
	seedVersionDir(t, dir, prunePluginID, "0.9.0")
	rt.Stop(prunePluginID)
	rt.setStartErr(prunePluginID, errors.New("spawn failed"))

	svc.StartActivePlugins(context.Background())

	assertVersionDirs(t, dir, prunePluginID, "0.8.0", "0.9.0", "1.0.0")
}

// TestStartActivePluginsDoesNotPruneDisabledPlugin pins that a plugin nobody
// started keeps every version an operator staged for it.
func TestStartActivePluginsDoesNotPruneDisabledPlugin(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, prunePluginID) // 1.0.0, active
	seedVersionDir(t, dir, prunePluginID, "0.8.0")
	seedVersionDir(t, dir, prunePluginID, "0.9.0")
	if err := svc.Disable(prunePluginID); err != nil {
		t.Fatalf("Disable(): %v", err)
	}

	svc.StartActivePlugins(context.Background())

	assertVersionDirs(t, dir, prunePluginID, "0.8.0", "0.9.0", "1.0.0")
}
