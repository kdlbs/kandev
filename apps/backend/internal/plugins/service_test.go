package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

// fakeRuntime is a controllable PluginRuntime for tests: Start/Stop just
// flip an in-memory "running" flag (recording every call) instead of
// spawning a real subprocess, so Service's install/activate/status-machine
// logic can be tested without internal/plugins/runtime's real go-plugin
// machinery.
type fakeRuntime struct {
	mu sync.Mutex

	running       map[string]bool
	startErr      map[string]error
	restartCounts map[string]int

	startCalls   []string
	stopCalls    []string
	lastStartCtx context.Context

	// blockStarted/blockProceed, when set via blockNextStart, make the very
	// next Start call signal blockStarted and then wait on blockProceed
	// before continuing — used by concurrency tests to pause a caller
	// mid-Start and prove a competing caller is blocked out.
	blockStarted chan struct{}
	blockProceed chan struct{}
}

type fakeUserStateCleanup struct {
	deleteErr error
	delete    func(context.Context, string) error
	calls     int
}

func (f *fakeUserStateCleanup) DeleteAllForPlugin(ctx context.Context, pluginID string) error {
	f.calls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.delete(ctx, pluginID)
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{
		running:       map[string]bool{},
		startErr:      map[string]error{},
		restartCounts: map[string]int{},
	}
}

// blockNextStart arms a one-shot block on the next Start call: Start closes
// the returned started channel once it is entered, then waits until the
// returned release func is called before returning.
func (r *fakeRuntime) blockNextStart() (started <-chan struct{}, release func()) {
	s := make(chan struct{})
	p := make(chan struct{})
	r.mu.Lock()
	r.blockStarted = s
	r.blockProceed = p
	r.mu.Unlock()
	return s, func() { close(p) }
}

func (r *fakeRuntime) Start(ctx context.Context, rec *store.Record, hostFactory func(string) pluginsdk.Host) error {
	r.mu.Lock()
	r.startCalls = append(r.startCalls, rec.ID)
	r.lastStartCtx = ctx
	started, proceed := r.blockStarted, r.blockProceed
	r.blockStarted, r.blockProceed = nil, nil
	r.mu.Unlock()

	if started != nil {
		close(started)
		<-proceed
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.startErr[rec.ID]; err != nil {
		return err
	}
	_ = hostFactory(rec.ID) // exercise the factory, mirroring the real manager
	r.running[rec.ID] = true
	return nil
}

func (r *fakeRuntime) Stop(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopCalls = append(r.stopCalls, id)
	r.running[id] = false
}

func (r *fakeRuntime) Get(id string) (*pluginsdk.RemotePlugin, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running[id] {
		return nil, false
	}
	return nil, true
}

func (r *fakeRuntime) Ping(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running[id] {
		return fmt.Errorf("not running")
	}
	return nil
}

func (r *fakeRuntime) Running(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[id]
}

func (r *fakeRuntime) RestartCount(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.restartCounts[id]
}

func (r *fakeRuntime) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range r.running {
		r.running[id] = false
	}
}

func (r *fakeRuntime) setStartErr(id string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startErr[id] = err
}

func (r *fakeRuntime) clearStartErr(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.startErr, id)
}

func (r *fakeRuntime) startCallCount(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, got := range r.startCalls {
		if got == id {
			n++
		}
	}
	return n
}

func (r *fakeRuntime) startCtx() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastStartCtx
}

func (r *fakeRuntime) stopped(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.stopCalls {
		if got == id {
			return true
		}
	}
	return false
}

// fakeSecretRevealer is a minimal in-memory SecretVault for tests.
type fakeSecretRevealer struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeSecretRevealer() *fakeSecretRevealer {
	return &fakeSecretRevealer{values: map[string]string{}}
}

func (v *fakeSecretRevealer) set(ref, value string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.values[ref] = value
}

func (v *fakeSecretRevealer) Reveal(_ context.Context, ref string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	value, ok := v.values[ref]
	if !ok {
		return "", fmt.Errorf("%w: %s", secrets.ErrNotFound, ref)
	}
	return value, nil
}

func (v *fakeSecretRevealer) Set(_ context.Context, id, _, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.values[id] = value
	return nil
}

func (v *fakeSecretRevealer) Delete(_ context.Context, id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.values[id]; !ok {
		return fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	delete(v.values, id)
	return nil
}

func (v *fakeSecretRevealer) ListIDs(_ context.Context) ([]string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	ids := make([]string, 0, len(v.values))
	for id := range v.values {
		ids = append(ids, id)
	}
	return ids, nil
}

func (v *fakeSecretRevealer) get(id string) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	value, ok := v.values[id]
	return value, ok
}

// testPackage builds a valid, runtime-managed plugin tar.gz for the CURRENT
// host platform (so pkgtar.Install's platform check passes), with a
// capabilities.events subscription and (optionally) a UI bundle.
func testPackage(t *testing.T, id, version string, withUIBundle bool) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: %s
display_name: Test Plugin
capabilities:
  events: ["task.*"]
  state: true
  secrets: true
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, version, platformKey)
	if withUIBundle {
		manifestYAML += "ui:\n  bundle: \"/ui/bundle.js\"\n  styles: [\"/ui/style.css\"]\n"
	}

	var buf bytes.Buffer
	files := map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}
	if withUIBundle {
		files["ui/bundle.js"] = []byte("export default {};")
		files["ui/style.css"] = []byte("body{}")
	}
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

// newTestService wires a Service against a real FSStore rooted at a temp
// plugins dir, a fresh Registry, and a fakeRuntime — mirroring what Provide
// does, minus the real runtime.Manager.
func newTestService(t *testing.T) (*Service, *store.FSStore, *fakeRuntime) {
	t.Helper()
	svc, _, fsStore, rt := newTestServiceWithDir(t)
	return svc, fsStore, rt
}

func newTestServiceWithDir(t *testing.T) (*Service, string, *store.FSStore, *fakeRuntime) {
	t.Helper()
	dir := t.TempDir()
	fsStore := store.NewFSStore(dir)
	reg := NewRegistry()
	svc := NewService(fsStore, reg, nil, testLogger(t))
	svc.SetPluginsDir(dir)
	rt := newFakeRuntime()
	svc.SetRuntime(rt)
	return svc, dir, fsStore, rt
}

func installTestPlugin(t *testing.T, svc *Service, id string) *store.Record {
	t.Helper()
	rec, err := svc.Install(context.Background(), testPackage(t, id, "1.0.0", false))
	if err != nil {
		t.Fatalf("Install(%q): %v", id, err)
	}
	return rec
}

func TestServiceListReturnsInstalledPlugins(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	installTestPlugin(t, svc, "kandev-plugin-jira")

	list := svc.List()
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}
}

func TestServiceGetMissingReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.Get("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get() error = %v, want store.ErrNotFound", err)
	}
}

func TestServiceUpdateConfigPersists(t *testing.T) {
	svc, fsStore, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	if err := svc.UpdateConfig(context.Background(), "kandev-plugin-slack", map[string]any{"default_channel": "#dev"}); err != nil {
		t.Fatalf("UpdateConfig() unexpected error: %v", err)
	}

	cfg, err := fsStore.GetConfig("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("GetConfig() unexpected error: %v", err)
	}
	if cfg["default_channel"] != "#dev" {
		t.Fatalf("GetConfig() default_channel = %v, want %q", cfg["default_channel"], "#dev")
	}
}

func TestServiceUpdateConfigMissingReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	err := svc.UpdateConfig(context.Background(), "missing", map[string]any{"a": "b"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("UpdateConfig() error = %v, want store.ErrNotFound", err)
	}
}

func TestServiceUninstallStopsRuntimeRemovesPackageAndRecord(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	rec := installTestPlugin(t, svc, "kandev-plugin-slack")
	installDir := filepath.Dir(rec.InstallPath) // .../plugins/kandev-plugin-slack

	if err := svc.Uninstall(context.Background(), "kandev-plugin-slack"); err != nil {
		t.Fatalf("Uninstall() unexpected error: %v", err)
	}

	if !rt.stopped("kandev-plugin-slack") {
		t.Fatal("Uninstall() did not stop the runtime process")
	}
	if _, err := svc.Get("kandev-plugin-slack"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get() after Uninstall() error = %v, want store.ErrNotFound", err)
	}
	if _, err := fsStore.Get("kandev-plugin-slack"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("store.Get() after Uninstall() error = %v, want store.ErrNotFound", err)
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("expected the extracted package dir to be removed, stat err = %v", err)
	}
}

// TestServiceUninstallDeletesPluginState pins the fix for the plugin_state
// leak on Uninstall: removing a plugin previously deleted its file tree and
// registry record but never its plugin_state rows, so a reinstalled (or
// id-reused) plugin silently inherited stale state from a previous install.
func TestServiceUninstallDeletesPluginState(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.SetState(newTestStateStore(t))
	installTestPlugin(t, svc, "kandev-plugin-slack")

	ctx := context.Background()
	if err := svc.StateStore().Set(ctx, "kandev-plugin-slack", "instance", "", "install_id", json.RawMessage(`"abc"`)); err != nil {
		t.Fatalf("seed plugin_state: %v", err)
	}

	if err := svc.Uninstall(context.Background(), "kandev-plugin-slack"); err != nil {
		t.Fatalf("Uninstall() unexpected error: %v", err)
	}

	entries, err := svc.StateStore().List(ctx, "kandev-plugin-slack", "instance", "")
	if err != nil {
		t.Fatalf("List() after Uninstall(): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("plugin_state entries after Uninstall() = %d, want 0 (state should be deleted)", len(entries))
	}
}

// TestServiceUninstallDeletesPluginUserStateForEveryUser pins AC20: the
// per-user counterpart of TestServiceUninstallDeletesPluginState — uninstall
// must purge plugin_user_state rows for every user who wrote one, not just
// whichever user happened to trigger the uninstall.
func TestServiceUninstallDeletesPluginUserStateForEveryUser(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.SetUserState(newTestUserStateStore(t))
	installTestPlugin(t, svc, "kandev-plugin-notes")

	ctx := context.Background()
	if _, err := svc.UserState().Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_1", "note", json.RawMessage(`"a"`), nil); err != nil {
		t.Fatalf("seed user_1: %v", err)
	}
	if _, err := svc.UserState().Set(ctx, "kandev-plugin-notes", "user_2", "task", "task_1", "note", json.RawMessage(`"b"`), nil); err != nil {
		t.Fatalf("seed user_2: %v", err)
	}

	if err := svc.Uninstall(context.Background(), "kandev-plugin-notes"); err != nil {
		t.Fatalf("Uninstall() unexpected error: %v", err)
	}

	for _, userID := range []string{"user_1", "user_2"} {
		entries, err := svc.UserState().List(ctx, "kandev-plugin-notes", userID, "task", "task_1")
		if err != nil {
			t.Fatalf("List(%s) after Uninstall(): %v", userID, err)
		}
		if len(entries) != 0 {
			t.Fatalf("plugin_user_state entries for %s after Uninstall() = %d, want 0", userID, len(entries))
		}
	}
}

func TestServiceUninstallFailsClosedWhenUserStateCleanupFails(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	svc.SetUserState(newTestUserStateStore(t))
	rec := installTestPlugin(t, svc, "kandev-plugin-notes")
	ctx := context.Background()
	if _, err := svc.UserState().Set(ctx, rec.ID, "user_1", "task", "task_1", "note", json.RawMessage(`"a"`), nil); err != nil {
		t.Fatalf("seed user state: %v", err)
	}

	cleanupErr := errors.New("user state database unavailable")
	cleanup := &fakeUserStateCleanup{
		deleteErr: cleanupErr,
		delete:    svc.UserState().DeleteAllForPlugin,
	}
	svc.setUserStateCleanupStore(cleanup)
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	err := svc.Uninstall(ctx, rec.ID)
	if err == nil || !strings.Contains(err.Error(), cleanupErr.Error()) {
		t.Fatalf("Uninstall() error = %v, want user-state cleanup failure", err)
	}
	if cleanup.calls != 1 {
		t.Fatalf("DeleteAllForPlugin calls after failed uninstall = %d, want 1", cleanup.calls)
	}
	if !rt.stopped(rec.ID) {
		t.Fatal("Uninstall() did not stop the runtime before cleanup failure")
	}
	if _, err := svc.Get(rec.ID); err != nil {
		t.Fatalf("Get() after failed uninstall: %v, want installed record", err)
	}
	if _, err := fsStore.Get(rec.ID); err != nil {
		t.Fatalf("store.Get() after failed uninstall: %v, want installed record", err)
	}
	if _, err := os.Stat(rec.InstallPath); err != nil {
		t.Fatalf("installed package after failed uninstall: %v", err)
	}
	if deliverer.refreshCount != 1 {
		t.Fatalf("deliverer refreshes after failed uninstall = %d, want stopped-state reconciliation only", deliverer.refreshCount)
	}

	cleanup.deleteErr = nil
	if err := svc.Uninstall(ctx, rec.ID); err != nil {
		t.Fatalf("retry Uninstall() error: %v", err)
	}
	if cleanup.calls != 2 {
		t.Fatalf("DeleteAllForPlugin calls after retry = %d, want 2", cleanup.calls)
	}
	if _, err := svc.Get(rec.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get() after successful retry = %v, want store.ErrNotFound", err)
	}
	entries, err := svc.UserState().List(ctx, rec.ID, "user_1", "task", "task_1")
	if err != nil {
		t.Fatalf("List() after successful retry: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("user state rows after successful retry = %d, want 0", len(entries))
	}
}

func TestServiceUninstallMissingReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	if err := svc.Uninstall(context.Background(), "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Uninstall() error = %v, want store.ErrNotFound", err)
	}
}

func TestServiceSetStatusInvalidTransitionRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack") // already active

	// active -> registered is not a legal single-hop edge.
	err := svc.SetStatus("kandev-plugin-slack", StatusRegistered)
	var invalidErr *ErrInvalidTransition
	if !errors.As(err, &invalidErr) {
		t.Fatalf("SetStatus() error = %v, want *ErrInvalidTransition", err)
	}
}

func TestServiceSetStatusIntoUninstalledRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	err := svc.SetStatus("kandev-plugin-slack", StatusUninstalled)
	var invalidErr *ErrInvalidTransition
	if !errors.As(err, &invalidErr) {
		t.Fatalf("SetStatus() error = %v, want *ErrInvalidTransition (use Uninstall instead)", err)
	}
}

func TestServiceEnableIsIdempotentWhenAlreadyActive(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	if err := svc.Enable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Enable() on an already-active plugin: %v", err)
	}
	if got := rt.startCallCount("kandev-plugin-slack"); got != 1 {
		t.Fatalf("runtime Start called %d times, want 1 (Enable on an active plugin must be a no-op)", got)
	}
}

func TestServiceDisableFromActiveStopsRuntime(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable() unexpected error: %v", err)
	}

	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusDisabled {
		t.Fatalf("Get() Status = %q, want %q", got.Status, StatusDisabled)
	}
	if !rt.stopped("kandev-plugin-slack") {
		t.Fatal("Disable() did not stop the runtime process")
	}
}

func TestServiceDisableIsIdempotentWhenAlreadyDisabled(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("first Disable(): %v", err)
	}

	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("second Disable() expected no error (idempotent), got %v", err)
	}
}

func TestServiceDisabledCanReEnableAndRespawns(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}

	if err := svc.Enable("kandev-plugin-slack"); err != nil {
		t.Fatalf("re-Enable() unexpected error: %v", err)
	}
	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusActive {
		t.Fatalf("Get() Status = %q, want %q", got.Status, StatusActive)
	}
	if want := 2; rt.startCallCount("kandev-plugin-slack") != want {
		t.Fatalf("runtime Start called %d times, want %d (install + re-enable)", rt.startCallCount("kandev-plugin-slack"), want)
	}
}

func TestServiceEnableFailurePersistsDiagnosticAndSuccessfulRetryClearsIt(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}

	rt.setStartErr("kandev-plugin-slack", errors.New("handshake failed: binary exited"))
	if err := svc.Enable("kandev-plugin-slack"); err == nil {
		t.Fatal("Enable() unexpectedly succeeded with a configured start failure")
	}

	failed, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get() after failed Enable(): %v", err)
	}
	if failed.Status != StatusError {
		t.Fatalf("Status after failed Enable() = %q, want %q", failed.Status, StatusError)
	}
	if failed.LastError == "" || !strings.Contains(failed.LastError, "handshake failed") {
		t.Fatalf("LastError = %q, want the start failure", failed.LastError)
	}
	if failed.LastErrorAt == nil {
		t.Fatal("LastErrorAt is nil after failed Enable()")
	}
	onDisk, err := fsStore.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("store.Get() after failed Enable(): %v", err)
	}
	if onDisk.LastError != failed.LastError || onDisk.LastErrorAt == nil {
		t.Fatalf("persisted diagnostic = (%q, %v), want (%q, non-nil)", onDisk.LastError, onDisk.LastErrorAt, failed.LastError)
	}

	rt.clearStartErr("kandev-plugin-slack")
	if err := svc.Enable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Enable() retry unexpected error: %v", err)
	}
	recovered, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get() after successful retry: %v", err)
	}
	if recovered.Status != StatusActive {
		t.Fatalf("Status after successful retry = %q, want %q", recovered.Status, StatusActive)
	}
	if recovered.LastError != "" || recovered.LastErrorAt != nil {
		t.Fatalf("diagnostic after successful retry = (%q, %v), want empty/nil", recovered.LastError, recovered.LastErrorAt)
	}
}

func TestServiceFailedRetryReplacesDiagnostic(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}

	rt.setStartErr("kandev-plugin-slack", errors.New("first handshake failure"))
	if err := svc.Enable("kandev-plugin-slack"); err == nil {
		t.Fatal("first Enable() unexpectedly succeeded")
	}
	first, _ := svc.Get("kandev-plugin-slack")

	rt.setStartErr("kandev-plugin-slack", errors.New("second executable failure"))
	if err := svc.Enable("kandev-plugin-slack"); err == nil {
		t.Fatal("second Enable() unexpectedly succeeded")
	}
	second, _ := svc.Get("kandev-plugin-slack")
	if second.Status != StatusError {
		t.Fatalf("Status after failed retry = %q, want %q", second.Status, StatusError)
	}
	if second.LastError == first.LastError || !strings.Contains(second.LastError, "second executable failure") {
		t.Fatalf("LastError after failed retry = %q, want replacement diagnostic", second.LastError)
	}
}

// TestServiceEnable_ConcurrentCallsForSameID_OnlyOneActivationNoError proves
// Enable is guarded by a per-plugin lock: two near-simultaneous Enable calls
// for the same disabled plugin id must not both pass the StatusActive
// idempotency check and race into activate. Without the lock, the second
// call's SetStatus(Active) races the first, and the loser observes
// *ErrInvalidTransition ("active -> active") even though the plugin is
// correctly active by the time both calls return.
func TestServiceEnable_ConcurrentCallsForSameID_OnlyOneActivationNoError(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}

	started, release := rt.blockNextStart()

	firstErr := make(chan error, 1)
	go func() {
		firstErr <- svc.Enable("kandev-plugin-slack")
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first Enable() never reached the blocked runtime Start call")
	}

	secondErr := make(chan error, 1)
	go func() {
		secondErr <- svc.Enable("kandev-plugin-slack")
	}()

	// The second Enable() must still be blocked behind the per-plugin lock:
	// give it a bounded window to (incorrectly) complete, and fail if it does.
	select {
	case err := <-secondErr:
		t.Fatalf("second Enable() completed (err=%v) while the first was still starting — Enable is not per-plugin lock-guarded", err)
	case <-time.After(200 * time.Millisecond):
	}

	release()

	select {
	case err := <-firstErr:
		if err != nil {
			t.Fatalf("first Enable() unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first Enable() never returned after release")
	}
	select {
	case err := <-secondErr:
		if err != nil {
			t.Fatalf("second Enable() unexpected error (must be a no-op observing the now-active status): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second Enable() never returned after the first released the lock")
	}

	// Install's initial activate() already called Start once; this Enable
	// (after the earlier Disable) must add exactly one more — never two —
	// proving the second concurrent Enable() did not also spawn.
	if got, want := rt.startCallCount("kandev-plugin-slack"), 2; got != want {
		t.Fatalf("runtime Start called %d times for kandev-plugin-slack, want exactly %d (no double activation)", got, want)
	}
	rec, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if rec.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", rec.Status, StatusActive)
	}
}

// TestServiceActivatePassesABoundedStartContext pins the fix for bounded
// startup: activate previously passed context.Background() straight through
// to runtime.Start, so a hung plugin binary could block Enable/Install
// indefinitely (up to go-plugin's own default). activate must instead pass a
// context with a deadline, so a caller can observe/rely on the bound even if
// a future runtime.Manager becomes context-aware for the handshake itself.
func TestServiceActivatePassesABoundedStartContext(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	ctx := rt.startCtx()
	if ctx == nil {
		t.Fatal("runtime.Start() was never called")
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("activate() passed a context with no deadline to runtime.Start(), want a bounded context.WithTimeout")
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

// fakeDeliverer records Refresh/Flush calls so tests can assert the
// Service -> Deliverer attach-point contract without depending on the real
// delivery package.
type fakeDeliverer struct {
	refreshCount int
	flushedIDs   []string
}

func (f *fakeDeliverer) Refresh()              { f.refreshCount++ }
func (f *fakeDeliverer) Flush(pluginID string) { f.flushedIDs = append(f.flushedIDs, pluginID) }

func TestServiceInstallNotifiesDelivererRefresh(t *testing.T) {
	svc, _, _ := newTestService(t)
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	installTestPlugin(t, svc, "kandev-plugin-slack")

	if deliverer.refreshCount != 1 {
		t.Fatalf("Refresh() call count = %d, want 1", deliverer.refreshCount)
	}
}

func TestServiceUninstallNotifiesDelivererRefresh(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	if err := svc.Uninstall(context.Background(), "kandev-plugin-slack"); err != nil {
		t.Fatalf("Uninstall(): %v", err)
	}

	if deliverer.refreshCount != 1 {
		t.Fatalf("Refresh() call count after Uninstall() = %d, want 1", deliverer.refreshCount)
	}
}

func TestServiceWithoutDelivererDoesNotPanic(t *testing.T) {
	svc, _, _ := newTestService(t)
	// No SetDeliverer call — Install/SetStatus/Uninstall must tolerate a
	// nil deliverer (delivery not wired yet, e.g. in unit tests or before
	// backendapp attaches it).
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}
	if err := svc.Uninstall(context.Background(), "kandev-plugin-slack"); err != nil {
		t.Fatalf("Uninstall(): %v", err)
	}
}

func TestServiceDelivererAccessorReturnsAttached(t *testing.T) {
	svc, _, _ := newTestService(t)
	if svc.Deliverer() != nil {
		t.Fatalf("Deliverer() = %v, want nil before SetDeliverer", svc.Deliverer())
	}
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)
	if svc.Deliverer() != Deliverer(deliverer) {
		t.Fatalf("Deliverer() did not return the attached deliverer")
	}
}

func TestServiceRegistryAccessorReturnsSameRegistry(t *testing.T) {
	dir := t.TempDir()
	fsStore := store.NewFSStore(dir)
	reg := NewRegistry()
	svc := NewService(fsStore, reg, nil, testLogger(t))

	if svc.Registry() != reg {
		t.Fatalf("Registry() did not return the injected registry instance")
	}
}

func TestServiceRevealSecretWithoutVaultReturnsError(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.RevealSecret(context.Background(), "some-ref"); err == nil {
		t.Fatal("RevealSecret() expected error when no vault configured, got nil")
	}
}

func TestServiceRevealSecretResolvesThroughVault(t *testing.T) {
	svc, _, _ := newTestService(t)
	vault := newFakeSecretRevealer()
	vault.set("my-secret-ref", "s3cr3t")
	svc.SetSecrets(vault)

	got, err := svc.RevealSecret(context.Background(), "my-secret-ref")
	if err != nil {
		t.Fatalf("RevealSecret() unexpected error: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("RevealSecret() = %q, want %q", got, "s3cr3t")
	}
}

func TestServiceActiveUIPluginsFiltersByStatusAndBundle(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.Install(context.Background(), testPackage(t, "kandev-plugin-with-ui", "1.0.0", true)); err != nil {
		t.Fatalf("Install(with bundle): %v", err)
	}

	// Active but no bundle declared — must be excluded.
	installTestPlugin(t, svc, "kandev-plugin-no-ui")

	active := svc.ActiveUIPlugins()
	if len(active) != 1 {
		t.Fatalf("ActiveUIPlugins() len = %d, want 1: %+v", len(active), active)
	}
	if active[0].ID != "kandev-plugin-with-ui" {
		t.Fatalf("ActiveUIPlugins()[0].ID = %q, want %q", active[0].ID, "kandev-plugin-with-ui")
	}
}

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
