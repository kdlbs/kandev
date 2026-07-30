package plugins

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceStartActivePluginsSpawnsOnlyActiveManagedNotAlreadyRunning(t *testing.T) {
	svc, dir, fsStore, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	// Simulate a fresh boot: the registry is reloaded from disk (status
	// "active" persisted from the previous run), but the runtime manager
	// has no live process yet.
	reg2 := NewRegistry()
	if err := reg2.Load(fsStore); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	svc2 := NewService(fsStore, reg2, nil, testLogger(t))
	svc2.SetPluginsDir(dir)
	rt2 := newFakeRuntime()
	svc2.SetRuntime(rt2)

	svc2.StartActivePlugins(context.Background())

	if !rt2.Running("kandev-plugin-slack") {
		t.Fatal("StartActivePlugins() did not spawn the active plugin")
	}
}

func TestServiceStartActivePluginsFailurePersistsDiagnosticAndRefreshesDeliverer(t *testing.T) {
	svc, dir, fsStore, _ := newTestServiceWithDir(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	reg2 := NewRegistry()
	if err := reg2.Load(fsStore); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	svc2 := NewService(fsStore, reg2, nil, testLogger(t))
	svc2.SetPluginsDir(dir)
	rt2 := newFakeRuntime()
	rt2.setStartErr("kandev-plugin-slack", errors.New("boot handshake failed"))
	svc2.SetRuntime(rt2)
	deliverer := &fakeDeliverer{}
	svc2.SetDeliverer(deliverer)

	svc2.StartActivePlugins(context.Background())

	got, err := svc2.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get() after boot failure: %v", err)
	}
	if got.Status != StatusError {
		t.Fatalf("Status after boot failure = %q, want %q", got.Status, StatusError)
	}
	if !strings.Contains(got.LastError, "boot handshake failed") || got.LastErrorAt == nil {
		t.Fatalf("diagnostic after boot failure = (%q, %v), want boot failure and timestamp", got.LastError, got.LastErrorAt)
	}
	// bootScan refreshes once at the end of its normal reconciliation, then
	// the failed active spawn refreshes again so delivery sees StatusError.
	if deliverer.refreshCount != 2 {
		t.Fatalf("Refresh() calls after boot failure = %d, want 2", deliverer.refreshCount)
	}
}

// TestServiceStartActivePluginsRecoversManagedErrorOnHealthySpawn pins boot
// recovery for sticky error plugins: a managed plugin left in StatusError
// with no live process must get one spawn attempt. Success → active and
// last_error cleared (the provider-usage sticky-error gap).
func TestServiceStartActivePluginsRecoversManagedErrorOnHealthySpawn(t *testing.T) {
	svc, dir, fsStore, _ := newTestServiceWithDir(t)
	rec := installTestPlugin(t, svc, "kandev-provider-usage")
	// Simulate a prior failed run persisted to disk.
	at := time.Now().UTC().Truncate(time.Second)
	rec.Status = StatusError
	rec.LastError = "spawn failed: previous boot"
	rec.LastErrorAt = &at
	if err := fsStore.Save(rec); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	reg2 := NewRegistry()
	if err := reg2.Load(fsStore); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	svc2 := NewService(fsStore, reg2, nil, testLogger(t))
	svc2.SetPluginsDir(dir)
	rt2 := newFakeRuntime()
	svc2.SetRuntime(rt2)

	svc2.StartActivePlugins(context.Background())

	if !rt2.Running("kandev-provider-usage") {
		t.Fatal("StartActivePlugins() did not spawn the error plugin")
	}
	got, err := svc2.Get("kandev-provider-usage")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Status != StatusActive {
		t.Fatalf("Status after boot recovery = %q, want %q", got.Status, StatusActive)
	}
	if got.LastError != "" {
		t.Fatalf("LastError after recovery = %q, want empty", got.LastError)
	}
	if got.LastErrorAt != nil {
		t.Fatalf("LastErrorAt after recovery = %v, want nil", got.LastErrorAt)
	}
}

// TestServiceStartActivePluginsErrorSpawnFailureWritesLastError pins that a
// boot recovery spawn failure keeps status=error and refreshes last_error
// (including when the plugin was already error — same-status must not noop
// the failure reason).
func TestServiceStartActivePluginsErrorSpawnFailureWritesLastError(t *testing.T) {
	svc, dir, fsStore, _ := newTestServiceWithDir(t)
	rec := installTestPlugin(t, svc, "kandev-provider-usage")
	oldAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rec.Status = StatusError
	rec.LastError = "old reason"
	rec.LastErrorAt = &oldAt
	if err := fsStore.Save(rec); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	reg2 := NewRegistry()
	if err := reg2.Load(fsStore); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	svc2 := NewService(fsStore, reg2, nil, testLogger(t))
	svc2.SetPluginsDir(dir)
	rt2 := newFakeRuntime()
	rt2.setStartErr("kandev-provider-usage", errors.New("handshake timeout"))
	svc2.SetRuntime(rt2)

	svc2.StartActivePlugins(context.Background())

	if rt2.Running("kandev-provider-usage") {
		t.Fatal("StartActivePlugins() should not mark plugin running on spawn failure")
	}
	got, err := svc2.Get("kandev-provider-usage")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Status != StatusError {
		t.Fatalf("Status after failed boot spawn = %q, want %q", got.Status, StatusError)
	}
	if got.LastError == "" || !strings.Contains(got.LastError, "handshake timeout") {
		t.Fatalf("LastError = %q, want it to contain handshake timeout", got.LastError)
	}
	if got.LastErrorAt == nil {
		t.Fatal("LastErrorAt = nil after spawn failure, want set")
	}
	if got.LastErrorAt.Before(oldAt) {
		t.Fatalf("LastErrorAt = %v, want >= old stamp %v", got.LastErrorAt, oldAt)
	}
	if got.LastError == "old reason" {
		t.Fatal("LastError was not refreshed on boot spawn failure")
	}
}

// TestServiceStartActivePluginsSkipsDisabled pins that StatusDisabled
// (including sideloads) never auto-spawns at boot.
func TestServiceStartActivePluginsSkipsDisabled(t *testing.T) {
	svc, dir, fsStore, _ := newTestServiceWithDir(t)
	rec := installTestPlugin(t, svc, "kandev-plugin-sideload")
	if err := svc.Disable("kandev-plugin-sideload"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}
	// Stop any process from install so Running is false on the fresh runtime.
	_ = rec

	reg2 := NewRegistry()
	if err := reg2.Load(fsStore); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	svc2 := NewService(fsStore, reg2, nil, testLogger(t))
	svc2.SetPluginsDir(dir)
	rt2 := newFakeRuntime()
	svc2.SetRuntime(rt2)

	svc2.StartActivePlugins(context.Background())

	if rt2.Running("kandev-plugin-sideload") {
		t.Fatal("StartActivePlugins() spawned a disabled plugin")
	}
	if rt2.startCallCount("kandev-plugin-sideload") != 0 {
		t.Fatalf("Start() call count for disabled = %d, want 0", rt2.startCallCount("kandev-plugin-sideload"))
	}
}

// TestServiceActivateFailureWritesLastError pins Enable/activate spawn
// failure recording last_error on the record.
func TestServiceActivateFailureWritesLastError(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}
	rt.setStartErr("kandev-plugin-slack", errors.New("binary not executable"))

	err := svc.Enable("kandev-plugin-slack")
	if err == nil {
		t.Fatal("Enable() expected error, got nil")
	}

	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusError {
		t.Fatalf("Status = %q, want %q", got.Status, StatusError)
	}
	if got.LastError == "" || !strings.Contains(got.LastError, "binary not executable") {
		t.Fatalf("LastError = %q, want it to contain binary not executable", got.LastError)
	}
	if got.LastErrorAt == nil {
		t.Fatal("LastErrorAt = nil, want set")
	}
}

// TestServiceActivateSuccessClearsLastError pins that a successful Enable
// from error clears last_error fields.
func TestServiceActivateSuccessClearsLastError(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	rt.Stop("kandev-plugin-slack")
	at := time.Now().UTC()
	// Force error state with a stale reason (as after a prior failure).
	if err := svc.SetStatus("kandev-plugin-slack", StatusError); err != nil {
		// active → error is allowed
		t.Fatalf("SetStatus(error): %v", err)
	}
	// Set last_error via store after status (until service writers land).
	rec, _ := fsStore.Get("kandev-plugin-slack")
	rec.LastError = "stale failure"
	rec.LastErrorAt = &at
	if err := fsStore.Save(rec); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	// Reload registry so in-memory matches disk.
	svc.Registry().Add(rec)

	if err := svc.Enable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Enable(): %v", err)
	}

	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", got.Status, StatusActive)
	}
	if got.LastError != "" || got.LastErrorAt != nil {
		t.Fatalf("last_error fields not cleared: LastError=%q LastErrorAt=%v", got.LastError, got.LastErrorAt)
	}
}

func TestServiceHandleStatusChangeUnhealthyTransitionsToErrorAndRefreshesDeliverer(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	svc.handleStatusChange("kandev-plugin-slack", false, errors.New("ping timeout"))

	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusError {
		t.Fatalf("Status after unhealthy transition = %q, want %q", got.Status, StatusError)
	}
	if got.LastError == "" || !strings.Contains(got.LastError, "ping timeout") || got.LastErrorAt == nil {
		t.Fatalf("diagnostic after unhealthy transition = (%q, %v), want ping timeout/non-nil", got.LastError, got.LastErrorAt)
	}
	if deliverer.refreshCount != 1 {
		t.Fatalf("Refresh() call count = %d, want 1", deliverer.refreshCount)
	}
	if len(deliverer.flushedIDs) != 0 {
		t.Fatalf("Flush() should not be called on a degrade transition, got %v", deliverer.flushedIDs)
	}
}

func TestServiceHandleStatusChangeHealthyRecoversAndFlushesDeliverer(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	svc.handleStatusChange("kandev-plugin-slack", false, errors.New("ping timeout")) // degrade first
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	svc.handleStatusChange("kandev-plugin-slack", true, nil)

	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusActive {
		t.Fatalf("Status after recovery = %q, want %q", got.Status, StatusActive)
	}
	if got.LastError != "" || got.LastErrorAt != nil {
		t.Fatalf("diagnostic after recovery = (%q, %v), want empty/nil", got.LastError, got.LastErrorAt)
	}
	if len(deliverer.flushedIDs) != 1 || deliverer.flushedIDs[0] != "kandev-plugin-slack" {
		t.Fatalf("Flush() calls = %v, want [kandev-plugin-slack]", deliverer.flushedIDs)
	}
}

func TestServiceHandleStatusChangePersistsRestartCountBestEffort(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	rt.mu.Lock()
	rt.restartCounts["kandev-plugin-slack"] = 3
	rt.mu.Unlock()

	svc.handleStatusChange("kandev-plugin-slack", false, errors.New("ping timeout"))

	got, _ := svc.Get("kandev-plugin-slack")
	if got.RestartCount != 3 {
		t.Fatalf("Get().RestartCount = %d, want 3", got.RestartCount)
	}
	onDisk, err := fsStore.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("store.Get(): %v", err)
	}
	if onDisk.RestartCount != 3 {
		t.Fatalf("store.Get().RestartCount = %d, want 3", onDisk.RestartCount)
	}
}
