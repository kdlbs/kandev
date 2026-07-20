package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"
)

// writeManifestOnly writes just a manifest.yaml (a "dir sideload": a
// version directory dropped straight onto disk, with no {id}.yml record and
// none of the other package files pkgtar.Install would extract).
func writeManifestOnly(t *testing.T, pluginsDir, id, version string) string {
	t.Helper()
	versionDir := filepath.Join(pluginsDir, id, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifestYAML := "id: " + id + "\napi_version: 1\nversion: \"" + version + "\"\ndisplay_name: Sideloaded\n" +
		"runtime:\n  type: binary\n  executables:\n    " + goruntime.GOOS + "-" + goruntime.GOARCH + ": server/plugin\n"
	if err := os.WriteFile(filepath.Join(versionDir, "manifest.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatalf("WriteFile manifest.yaml: %v", err)
	}
	return versionDir
}

// dropTarball writes a valid plugin tar.gz package directly under
// pluginsDir (simulating an operator copying a package file onto disk
// instead of using the install UI/API).
func dropTarball(t *testing.T, pluginsDir, id, filename string) string {
	t.Helper()
	dest := filepath.Join(pluginsDir, filename)
	data := testPackage(t, id, "1.0.0", false).Bytes()
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", filename, err)
	}
	return dest
}

func TestServiceSync_RegistersDirSideloadAsDisabled(t *testing.T) {
	svc, dir, _, rt := newTestServiceWithDir(t)
	versionDir := writeManifestOnly(t, dir, "kandev-plugin-side", "1.0.0")

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "kandev-plugin-side" {
		t.Fatalf("Sync().Added = %v, want [kandev-plugin-side]", result.Added)
	}

	rec, err := svc.Get("kandev-plugin-side")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if rec.Status != StatusDisabled {
		t.Fatalf("sideloaded record Status = %q, want %q", rec.Status, StatusDisabled)
	}
	if rec.InstallPath != versionDir {
		t.Fatalf("sideloaded record InstallPath = %q, want %q", rec.InstallPath, versionDir)
	}
	if rec.Signed {
		t.Fatal("sideloaded record Signed = true, want false (unverified sideload)")
	}
	if rt.Running("kandev-plugin-side") {
		t.Fatal("Sync() must never spawn a disabled sideload")
	}
}

func TestServiceSync_ManifestIDMismatchIsAnError(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	writeManifestOnly(t, dir, "wrong-id-inside", "1.0.0")
	// Rename the directory so the on-disk id ("dir-id") does not match the
	// manifest's own id ("wrong-id-inside").
	if err := os.Rename(filepath.Join(dir, "wrong-id-inside"), filepath.Join(dir, "dir-id")); err != nil {
		t.Fatalf("os.Rename: %v", err)
	}

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if len(result.Added) != 0 {
		t.Fatalf("Sync().Added = %v, want none (mismatched manifest id)", result.Added)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Sync().Errors = %v, want exactly one entry", result.Errors)
	}
	if _, err := svc.Get("dir-id"); err == nil {
		t.Fatal("Get(\"dir-id\") succeeded, want not found (mismatched manifest must not be registered)")
	}
}

func TestServiceSync_MultipleVersionDirsPicksGreatestAndSkipsOthers(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	writeManifestOnly(t, dir, "kandev-plugin-multi", "1.0.0")
	writeManifestOnly(t, dir, "kandev-plugin-multi", "2.0.0")

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "kandev-plugin-multi" {
		t.Fatalf("Sync().Added = %v, want [kandev-plugin-multi]", result.Added)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Sync().Errors = %v, want exactly one skip entry for the 1.0.0 dir", result.Errors)
	}

	rec, err := svc.Get("kandev-plugin-multi")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if rec.Version != "2.0.0" {
		t.Fatalf("registered Version = %q, want %q (semver-greatest)", rec.Version, "2.0.0")
	}
}

// TestServiceSync_MultipleVersionDirsPicksSemverGreatestNotLexical pins the
// fix for a plain lexical sort picking the wrong "latest" version: "9.0.0"
// sorts after "10.0.0" as a string, but 10.0.0 is the actually-newer
// semver.
func TestServiceSync_MultipleVersionDirsPicksSemverGreatestNotLexical(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	writeManifestOnly(t, dir, "kandev-plugin-semver", "9.0.0")
	writeManifestOnly(t, dir, "kandev-plugin-semver", "10.0.0")

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "kandev-plugin-semver" {
		t.Fatalf("Sync().Added = %v, want [kandev-plugin-semver]", result.Added)
	}

	rec, err := svc.Get("kandev-plugin-semver")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if rec.Version != "10.0.0" {
		t.Fatalf("registered Version = %q, want %q (semver-greatest, not lexically greatest)", rec.Version, "10.0.0")
	}
}

func TestServiceSync_TarballInstallSucceedsAndDeletesFile(t *testing.T) {
	svc, dir, _, rt := newTestServiceWithDir(t)
	tarPath := dropTarball(t, dir, "kandev-plugin-drop", "kandev-plugin-drop-1.0.0.tar.gz")

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if len(result.Installed) != 1 || result.Installed[0] != "kandev-plugin-drop" {
		t.Fatalf("Sync().Installed = %v, want [kandev-plugin-drop]", result.Installed)
	}
	if !rt.Running("kandev-plugin-drop") {
		t.Fatal("Sync() did not spawn the successfully installed tarball plugin")
	}
	rec, err := svc.Get("kandev-plugin-drop")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if rec.Status != StatusActive {
		t.Fatalf("installed-from-tarball record Status = %q, want %q", rec.Status, StatusActive)
	}
	if _, statErr := os.Stat(tarPath); !os.IsNotExist(statErr) {
		t.Fatalf("dropped tarball still exists after successful install, stat err = %v", statErr)
	}
}

func TestServiceSync_TarballValidationFailureLeavesFileAndAddsError(t *testing.T) {
	svc, dir, _, _ := newTestServiceWithDir(t)
	junkPath := filepath.Join(dir, "junk.tar.gz")
	if err := os.WriteFile(junkPath, []byte("not a real gzip archive"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if len(result.Installed) != 0 {
		t.Fatalf("Sync().Installed = %v, want none", result.Installed)
	}
	if len(result.Errors) != 1 || result.Errors[0].Path != junkPath {
		t.Fatalf("Sync().Errors = %v, want one entry for %q", result.Errors, junkPath)
	}
	if _, statErr := os.Stat(junkPath); statErr != nil {
		t.Fatalf("corrupt tarball was removed, want it left in place: %v", statErr)
	}
}

func TestServiceSync_MissingInstallPathSetsErrorAndStopsRuntime(t *testing.T) {
	svc, _, rt := newTestService(t)
	rec := installTestPlugin(t, svc, "kandev-plugin-vanished")
	if err := os.RemoveAll(rec.InstallPath); err != nil {
		t.Fatalf("RemoveAll(InstallPath): %v", err)
	}

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if len(result.Missing) != 1 || result.Missing[0] != "kandev-plugin-vanished" {
		t.Fatalf("Sync().Missing = %v, want [kandev-plugin-vanished]", result.Missing)
	}
	if !rt.stopped("kandev-plugin-vanished") {
		t.Fatal("Sync() did not stop the runtime process for a plugin with a missing install path")
	}
	got, err := svc.Get("kandev-plugin-vanished")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Status != StatusError {
		t.Fatalf("Status after missing-install detection = %q, want %q", got.Status, StatusError)
	}
}

// TestServiceSync_MissingInstallAlreadyErrorIsIdempotent proves the "direct
// status write if transition invalid" fallback: once a plugin is already
// StatusError, canTransition(error, error) is false (SetStatus rejects
// same-status "transitions"), so Sync must not itself fail or panic —
// running it again still reports the plugin as missing.
func TestServiceSync_MissingInstallAlreadyErrorIsIdempotent(t *testing.T) {
	svc, _, _ := newTestService(t)
	rec := installTestPlugin(t, svc, "kandev-plugin-vanished")
	if err := os.RemoveAll(rec.InstallPath); err != nil {
		t.Fatalf("RemoveAll(InstallPath): %v", err)
	}

	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync() unexpected error: %v", err)
	}

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("second Sync() unexpected error: %v", err)
	}
	if len(result.Missing) != 1 || result.Missing[0] != "kandev-plugin-vanished" {
		t.Fatalf("second Sync().Missing = %v, want [kandev-plugin-vanished]", result.Missing)
	}
	got, err := svc.Get("kandev-plugin-vanished")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Status != StatusError {
		t.Fatalf("Status after second Sync() = %q, want %q", got.Status, StatusError)
	}
}

// TestServiceSync_EmptyResultSerializesEmptyArraysNotNull proves that a
// no-op Sync (nothing added/installed/missing/errored) round-trips through
// JSON as `[]`, not `null`. The frontend's SyncResult type
// (apps/web/lib/types/plugins.ts) declares these fields as non-nullable
// arrays and reads result.added.length unconditionally
// (apps/web/lib/plugins/sync-summary.ts) — a `null` there throws
// "can't access property 'length', e.added is null".
func TestServiceSync_EmptyResultSerializesEmptyArraysNotNull(t *testing.T) {
	svc, _, _ := newTestService(t)

	result, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() unexpected error: %v", err)
	}
	if result.Added == nil || result.Installed == nil || result.Missing == nil || result.Errors == nil {
		t.Fatalf("Sync() left a nil slice field: %+v", result)
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{"added", "installed", "missing", "errors"} {
		if raw, ok := fields[key]; !ok || string(raw) == "null" {
			t.Fatalf("Sync() JSON field %q = %s, want an empty array: %s", key, raw, data)
		}
	}
}

// TestServiceSync_ConcurrentCallsAreSerialized proves Sync is guarded by a
// mutex: a second concurrent Sync call must not proceed until the first one
// has fully returned. Pauses the first call mid-Install (inside the fake
// runtime's Start), starts the competing call, proves it is blocked out,
// then releases the first call and confirms the second only completes
// afterward.
func TestServiceSync_ConcurrentCallsAreSerialized(t *testing.T) {
	svc, dir, _, rt := newTestServiceWithDir(t)
	dropTarball(t, dir, "kandev-plugin-first", "kandev-plugin-first-1.0.0.tar.gz")

	started, release := rt.blockNextStart()

	firstDone := make(chan *SyncResult, 1)
	go func() {
		result, err := svc.Sync(context.Background())
		if err != nil {
			t.Errorf("first Sync() unexpected error: %v", err)
		}
		firstDone <- result
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first Sync() never reached the blocked runtime Start call")
	}

	secondDone := make(chan *SyncResult, 1)
	go func() {
		result, err := svc.Sync(context.Background())
		if err != nil {
			t.Errorf("second Sync() unexpected error: %v", err)
		}
		secondDone <- result
	}()

	// The second call must still be blocked on the sync mutex: give it a
	// bounded window to (incorrectly) complete, and fail if it does.
	select {
	case <-secondDone:
		t.Fatal("second Sync() completed while the first was still in flight — Sync is not mutex-guarded")
	case <-time.After(200 * time.Millisecond):
	}

	release()

	var firstResult, secondResult *SyncResult
	select {
	case firstResult = <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("first Sync() never returned after release")
	}
	select {
	case secondResult = <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("second Sync() never returned after the first released the mutex")
	}

	if len(firstResult.Installed) != 1 || firstResult.Installed[0] != "kandev-plugin-first" {
		t.Fatalf("first Sync().Installed = %v, want [kandev-plugin-first]", firstResult.Installed)
	}
	// The tarball is already gone by the time the second call scans, so it
	// must not be double-installed.
	if len(secondResult.Installed) != 0 {
		t.Fatalf("second Sync().Installed = %v, want none (tarball already consumed)", secondResult.Installed)
	}
}
