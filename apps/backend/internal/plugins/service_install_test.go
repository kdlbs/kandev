package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/store"
)

func TestServiceInstallActivatesOnSuccessfulSpawn(t *testing.T) {
	svc, _, rt := newTestService(t)

	rec := installTestPlugin(t, svc, "kandev-plugin-slack")
	if rec.Status != StatusActive {
		t.Fatalf("Install() Status = %q, want %q", rec.Status, StatusActive)
	}
	if !rt.Running("kandev-plugin-slack") {
		t.Fatal("Install() did not spawn the plugin via the runtime manager")
	}

	got, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.InstallPath == "" {
		t.Fatal("Get().InstallPath is empty after Install()")
	}
}

func TestServiceInstallDuplicateVersionReturnsErrVersionExists(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	_, err := svc.Install(context.Background(), testPackage(t, "kandev-plugin-slack", "1.0.0", false))
	if !errors.Is(err, pkgtar.ErrVersionExists) {
		t.Fatalf("Install() duplicate error = %v, want pkgtar.ErrVersionExists", err)
	}
}

func TestServiceInstallSpawnFailureLeavesRecordInErrorStatus(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	rt.setStartErr("kandev-plugin-slack", errors.New("spawn failed"))

	rec, err := svc.Install(context.Background(), testPackage(t, "kandev-plugin-slack", "1.0.0", false))
	if err == nil {
		t.Fatal("Install() expected an error when the spawn fails")
	}
	if rec == nil {
		t.Fatal("Install() expected a non-nil record even when the spawn fails (package was extracted)")
	}
	if rec.Status != StatusError {
		t.Fatalf("Install() Status = %q, want %q", rec.Status, StatusError)
	}

	onDisk, getErr := fsStore.Get("kandev-plugin-slack")
	if getErr != nil {
		t.Fatalf("store.Get(): %v", getErr)
	}
	if onDisk.Status != StatusError {
		t.Fatalf("store.Get().Status = %q, want %q", onDisk.Status, StatusError)
	}
}

// TestServiceInstallOverActivePluginRestartsWithNewVersion pins the fix for
// installing a new version over an already-active plugin: activate's
// "already running" short-circuit previously skipped spawning entirely, so
// the live subprocess kept running the OLD version's binary even though the
// record/install_path now pointed at the new one. Install must stop the old
// process and start the new install_path so the running process matches the
// installed version.
func TestServiceInstallOverActivePluginRestartsWithNewVersion(t *testing.T) {
	svc, _, rt := newTestService(t)
	rec1 := installTestPlugin(t, svc, "kandev-plugin-slack") // v1.0.0, active + running
	if !rt.Running("kandev-plugin-slack") {
		t.Fatal("sanity check: plugin not running after first install")
	}

	rec2, err := svc.Install(context.Background(), testPackage(t, "kandev-plugin-slack", "1.1.0", false))
	if err != nil {
		t.Fatalf("second Install() unexpected error: %v", err)
	}
	if rec2.Version != "1.1.0" {
		t.Fatalf("rec2.Version = %q, want %q", rec2.Version, "1.1.0")
	}
	if rec2.InstallPath == rec1.InstallPath {
		t.Fatal("rec2.InstallPath == rec1.InstallPath, want the new version's own install dir")
	}
	if !rt.stopped("kandev-plugin-slack") {
		t.Fatal("Install() over an active plugin did not stop the old runtime process")
	}
	if got := rt.startCallCount("kandev-plugin-slack"); got != 2 {
		t.Fatalf("runtime Start called %d times, want 2 (initial install + restart on reinstall)", got)
	}
	if !rt.Running("kandev-plugin-slack") {
		t.Fatal("plugin not running after reinstalling over an already-active version")
	}
	if rec2.Status != StatusActive {
		t.Fatalf("rec2.Status = %q, want %q", rec2.Status, StatusActive)
	}
}

// failingSaveStore wraps a real store.Store, letting a test arm exactly one
// Save call to fail with a simulated error — used to exercise Install's
// cleanup-on-persist-failure path without a real disk-full/permission
// simulation.
type failingSaveStore struct {
	store.Store
	mu       sync.Mutex
	failNext bool
}

func (s *failingSaveStore) Save(rec *store.Record) error {
	s.mu.Lock()
	fail := s.failNext
	s.failNext = false
	s.mu.Unlock()
	if fail {
		return errors.New("simulated save failure")
	}
	return s.Store.Save(rec)
}

func (s *failingSaveStore) armFailNextSave() {
	s.mu.Lock()
	s.failNext = true
	s.mu.Unlock()
}

// TestServiceInstallUpgradeSaveFailurePreservesOldVersionAndData pins the
// fix for a failed upgrade destroying every installed version plus the
// plugin's writable data directory: cleanup on a store.Save failure must
// remove only the freshly extracted version directory, and must restart the
// previous version's process (already stopped to make way for the new
// spawn) rather than leaving the plugin down.
func TestServiceInstallUpgradeSaveFailurePreservesOldVersionAndData(t *testing.T) {
	dir := t.TempDir()
	fsStore := store.NewFSStore(dir)
	failing := &failingSaveStore{Store: fsStore}
	reg := NewRegistry()
	svc := NewService(failing, reg, nil, testLogger(t))
	svc.SetPluginsDir(dir)
	rt := newFakeRuntime()
	svc.SetRuntime(rt)

	rec1 := installTestPlugin(t, svc, "kandev-plugin-slack") // v1.0.0, active + running
	if !rt.Running("kandev-plugin-slack") {
		t.Fatal("sanity check: plugin not running after first install")
	}

	// Simulate the old version's writable data directory holding user data.
	dataDir := filepath.Join(dir, "kandev-plugin-slack", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(dataDir): %v", err)
	}
	marker := filepath.Join(dataDir, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker): %v", err)
	}

	failing.armFailNextSave()
	_, err := svc.Install(context.Background(), testPackage(t, "kandev-plugin-slack", "1.1.0", false))
	if err == nil {
		t.Fatal("Install() expected an error from the simulated Save failure")
	}

	if _, statErr := os.Stat(rec1.InstallPath); statErr != nil {
		t.Fatalf("old version dir %q was removed by cleanup: %v", rec1.InstallPath, statErr)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("plugin data dir marker was removed by cleanup: %v", statErr)
	}
	newVersionDir := filepath.Join(dir, "kandev-plugin-slack", "1.1.0")
	if _, statErr := os.Stat(newVersionDir); !os.IsNotExist(statErr) {
		t.Fatalf("new version dir %q still exists after the Save failure: stat err = %v", newVersionDir, statErr)
	}
	if !rt.Running("kandev-plugin-slack") {
		t.Fatal("the previous version's process was not restarted after the failed upgrade")
	}

	onDisk, getErr := fsStore.Get("kandev-plugin-slack")
	if getErr != nil {
		t.Fatalf("store.Get(): %v", getErr)
	}
	if onDisk.Version != "1.0.0" {
		t.Fatalf("store record Version = %q, want %q (the old version's record must survive unchanged)", onDisk.Version, "1.0.0")
	}
}

// minKandevVersionPackage builds a valid, runtime-managed plugin tar.gz
// declaring min_kandev_version, for TestServiceInstall_MinKandevVersion*.
func minKandevVersionPackage(t *testing.T, id, version, minKandevVersion string) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: %s
display_name: Test Plugin
min_kandev_version: %q
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, version, minKandevVersion, platformKey)

	var buf bytes.Buffer
	files := map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

// TestServiceInstall_RejectsPackageNewerThanRunningKandevVersion pins
// enforcement of manifest.min_kandev_version: once a running kandev version
// is wired via SetKandevVersion, a package that requires a newer kandev
// must be rejected (and its extracted package directory cleaned up) rather
// than installed and left to fail confusingly at spawn time.
func TestServiceInstall_RejectsPackageNewerThanRunningKandevVersion(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	svc.SetKandevVersion("1.0.0")

	_, err := svc.Install(context.Background(), minKandevVersionPackage(t, "kandev-plugin-slack", "1.0.0", "2.0.0"))
	if err == nil {
		t.Fatal("Install() expected an error for a package requiring a newer kandev version")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "kandev-plugin-slack", "1.0.0")); !os.IsNotExist(statErr) {
		t.Fatalf("Install() left the extracted package on disk after rejecting it: stat err = %v", statErr)
	}
	if _, getErr := svc.Get("kandev-plugin-slack"); getErr == nil {
		t.Fatal("Install() registered a record for a package it rejected on min_kandev_version")
	}
}

// TestServiceInstall_AllowsPackageAtOrBelowRunningKandevVersion pins the
// non-blocking side of the same check.
func TestServiceInstall_AllowsPackageAtOrBelowRunningKandevVersion(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.SetKandevVersion("2.0.0")

	rec, err := svc.Install(context.Background(), minKandevVersionPackage(t, "kandev-plugin-slack", "1.0.0", "1.5.0"))
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}
	if rec.Status != StatusActive {
		t.Fatalf("Install() Status = %q, want %q", rec.Status, StatusActive)
	}
}

// TestServiceInstall_NoEnforcementWithoutKandevVersionWired pins the
// no-op default: a Service that never called SetKandevVersion (e.g. every
// other test in this file, and any caller that hasn't wired it yet) must
// keep installing packages that declare min_kandev_version exactly as
// before.
func TestServiceInstall_NoEnforcementWithoutKandevVersionWired(t *testing.T) {
	svc, _, _ := newTestService(t)

	rec, err := svc.Install(context.Background(), minKandevVersionPackage(t, "kandev-plugin-slack", "1.0.0", "999.0.0"))
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}
	if rec.Status != StatusActive {
		t.Fatalf("Install() Status = %q, want %q", rec.Status, StatusActive)
	}
}

func TestValidateInstallURL_AcceptsHTTPAndHTTPS(t *testing.T) {
	for _, raw := range []string{
		"https://example.com/plugin.tar.gz",
		"http://example.com/plugin.tar.gz",
	} {
		if err := validateInstallURL(raw); err != nil {
			t.Fatalf("validateInstallURL(%q) unexpected error: %v", raw, err)
		}
	}
}

func TestValidateInstallURL_RejectsNonHTTPScheme(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://example.com/plugin",
		"ftp://example.com/plugin.tar.gz",
	} {
		if err := validateInstallURL(raw); err == nil {
			t.Fatalf("validateInstallURL(%q) expected error, got nil", raw)
		}
	}
}

func TestValidateInstallURL_RejectsEmptyHost(t *testing.T) {
	if err := validateInstallURL("https:///plugin.tar.gz"); err == nil {
		t.Fatal("validateInstallURL() expected error for empty host, got nil")
	}
}

func TestValidateInstallURL_RejectsMalformedURL(t *testing.T) {
	if err := validateInstallURL("://not-a-url"); err == nil {
		t.Fatal("validateInstallURL() expected error for malformed URL, got nil")
	}
}

func TestServiceInstallFromURL_RejectsNonHTTPSchemeBeforeAnyRequest(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.InstallFromURL(context.Background(), "file:///etc/passwd")
	if err == nil {
		t.Fatal("InstallFromURL() expected error for file:// scheme, got nil")
	}
}
