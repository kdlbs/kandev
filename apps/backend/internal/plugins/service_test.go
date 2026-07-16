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

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/store"
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

	startCalls []string
	stopCalls  []string

	// blockStarted/blockProceed, when set via blockNextStart, make the very
	// next Start call signal blockStarted and then wait on blockProceed
	// before continuing — used by concurrency tests to pause a caller
	// mid-Start and prove a competing caller is blocked out.
	blockStarted chan struct{}
	blockProceed chan struct{}
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

func (r *fakeRuntime) Start(_ context.Context, rec *store.Record, hostFactory func(string) pluginsdk.Host) error {
	r.mu.Lock()
	r.startCalls = append(r.startCalls, rec.ID)
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

// fakeSecretRevealer is a minimal in-memory SecretRevealer for tests.
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
		return "", store.ErrNotFound
	}
	return value, nil
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

	if err := svc.UpdateConfig("kandev-plugin-slack", map[string]any{"default_channel": "#dev"}); err != nil {
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
	err := svc.UpdateConfig("missing", map[string]any{"a": "b"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("UpdateConfig() error = %v, want store.ErrNotFound", err)
	}
}

func TestServiceUninstallStopsRuntimeRemovesPackageAndRecord(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	rec := installTestPlugin(t, svc, "kandev-plugin-slack")
	installDir := filepath.Dir(rec.InstallPath) // .../plugins/kandev-plugin-slack

	if err := svc.Uninstall("kandev-plugin-slack"); err != nil {
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

func TestServiceUninstallMissingReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	if err := svc.Uninstall("missing"); !errors.Is(err, store.ErrNotFound) {
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

func TestServiceHandleStatusChangeUnhealthyTransitionsToErrorAndRefreshesDeliverer(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	svc.handleStatusChange("kandev-plugin-slack", false)

	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusError {
		t.Fatalf("Status after unhealthy transition = %q, want %q", got.Status, StatusError)
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
	svc.handleStatusChange("kandev-plugin-slack", false) // degrade first
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	svc.handleStatusChange("kandev-plugin-slack", true)

	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusActive {
		t.Fatalf("Status after recovery = %q, want %q", got.Status, StatusActive)
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

	svc.handleStatusChange("kandev-plugin-slack", false)

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

	if err := svc.Uninstall("kandev-plugin-slack"); err != nil {
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
	if err := svc.Uninstall("kandev-plugin-slack"); err != nil {
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
