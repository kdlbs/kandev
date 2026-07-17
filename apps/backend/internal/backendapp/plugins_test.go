package backendapp

import (
	"bytes"
	"context"
	"fmt"
	goruntime "runtime"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/delivery"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func testPluginsLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

// alwaysUpRuntime is a minimal plugins.PluginRuntime fake that always
// reports a successful spawn without touching a real subprocess, so
// Service.Install/Enable can reach StatusActive in tests that only exercise
// boot-payload/registry/status behavior.
type alwaysUpRuntime struct{}

func (alwaysUpRuntime) Start(context.Context, *store.Record, func(string) pluginsdk.Host) error {
	return nil
}
func (alwaysUpRuntime) Stop(string)                                {}
func (alwaysUpRuntime) Get(string) (*pluginsdk.RemotePlugin, bool) { return nil, true }
func (alwaysUpRuntime) Ping(string) error                          { return nil }
func (alwaysUpRuntime) Running(string) bool                        { return true }
func (alwaysUpRuntime) RestartCount(string) int                    { return 0 }
func (alwaysUpRuntime) StopAll()                                   {}

// testPluginPackage builds a valid, runtime-managed plugin tar.gz for the
// current host platform, with a capabilities.events subscription and
// (optionally) a UI bundle — mirroring internal/plugins/service_test.go's
// testPackage (kept separate here since that helper is unexported to that
// package).
func testPluginPackage(t *testing.T, id string, withUIBundle bool) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: "1.0.0"
display_name: Test Plugin %s
capabilities:
  events: ["task.created"]
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, id, platformKey)
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

// newTestPluginsService returns a plugins.Service backed by a temp-dir
// FSStore and an always-succeeding fake runtime, mirroring
// internal/plugins' own test helpers (kept separate here since those are
// unexported to that package). Install/Enable reach StatusActive without
// spawning a real subprocess.
func newTestPluginsService(t *testing.T) *plugins.Service {
	t.Helper()
	dir := t.TempDir()
	fsStore := store.NewFSStore(dir)
	registry := plugins.NewRegistry()
	if err := registry.Load(fsStore); err != nil {
		t.Fatalf("load registry: %v", err)
	}
	svc := plugins.NewService(fsStore, registry, nil, testPluginsLogger(t))
	svc.SetPluginsDir(dir)
	svc.SetRuntime(alwaysUpRuntime{})
	return svc
}

func installTestPluginForBoot(t *testing.T, svc *plugins.Service, id string, withUIBundle bool) *store.Record {
	t.Helper()
	rec, err := svc.Install(context.Background(), testPluginPackage(t, id, withUIBundle))
	if err != nil {
		t.Fatalf("Install(%q): %v", id, err)
	}
	return rec
}

func TestBootActivePluginsGatedOnFeatureFlag(t *testing.T) {
	svc := newTestPluginsService(t)
	installTestPluginForBoot(t, svc, "kandev-plugin-hello", true)

	got := bootActivePlugins(routeParams{
		features: config.FeaturesConfig{Plugins: false},
		services: &Services{Plugins: svc},
	})
	if got != nil {
		t.Fatalf("bootActivePlugins() with features.Plugins=false = %v, want nil", got)
	}
}

func TestBootActivePluginsPopulatesFromActiveUIPlugins(t *testing.T) {
	svc := newTestPluginsService(t)

	installTestPluginForBoot(t, svc, "kandev-plugin-hello", true)

	// Disabled — must be excluded even though it declares a bundle.
	installTestPluginForBoot(t, svc, "kandev-plugin-inactive", true)
	if err := svc.Disable("kandev-plugin-inactive"); err != nil {
		t.Fatalf("Disable(inactive): %v", err)
	}

	got := bootActivePlugins(routeParams{
		features: config.FeaturesConfig{Plugins: true},
		services: &Services{Plugins: svc},
	})
	if len(got) != 1 {
		t.Fatalf("bootActivePlugins() len = %d, want 1: %+v", len(got), got)
	}
	entry := got[0]
	if entry.ID != "kandev-plugin-hello" {
		t.Fatalf("entry.ID = %q, want %q", entry.ID, "kandev-plugin-hello")
	}
	if entry.BundleURL != "/api/plugins/kandev-plugin-hello/bundle" {
		t.Fatalf("entry.BundleURL = %q, want %q", entry.BundleURL, "/api/plugins/kandev-plugin-hello/bundle")
	}
	if len(entry.StyleURLs) != 1 || entry.StyleURLs[0] != "/api/plugins/kandev-plugin-hello/ui/ui/style.css" {
		t.Fatalf("entry.StyleURLs = %v, want [/api/plugins/kandev-plugin-hello/ui/ui/style.css]", entry.StyleURLs)
	}
}

func TestBootActivePluginsNoServiceReturnsNil(t *testing.T) {
	got := bootActivePlugins(routeParams{features: config.FeaturesConfig{Plugins: true}, services: &Services{}})
	if got != nil {
		t.Fatalf("bootActivePlugins() with nil Plugins service = %v, want nil", got)
	}
}

// --- pluginActivePluginsAdapter ---

func TestPluginActivePluginsAdapterIncludesActiveAndErrorOnly(t *testing.T) {
	svc := newTestPluginsService(t)

	installTestPluginForBoot(t, svc, "kandev-plugin-active", false) // active after install

	installTestPluginForBoot(t, svc, "kandev-plugin-error", false)
	if err := svc.SetStatus("kandev-plugin-error", plugins.StatusError); err != nil {
		t.Fatalf("SetStatus(error): %v", err)
	}

	installTestPluginForBoot(t, svc, "kandev-plugin-disabled", false)
	if err := svc.Disable("kandev-plugin-disabled"); err != nil {
		t.Fatalf("Disable(disabled): %v", err)
	}

	adapter := pluginActivePluginsAdapter{svc: svc}
	records := adapter.ActivePlugins()

	byID := make(map[string]delivery.PluginRecord, len(records))
	for _, rec := range records {
		byID[rec.ID] = rec
	}
	if len(byID) != 2 {
		t.Fatalf("ActivePlugins() len = %d, want 2: %+v", len(byID), records)
	}
	if _, ok := byID["kandev-plugin-disabled"]; ok {
		t.Fatal("ActivePlugins() must not include a StatusDisabled plugin")
	}
	rec, ok := byID["kandev-plugin-active"]
	if !ok {
		t.Fatal("ActivePlugins() missing kandev-plugin-active")
	}
	if len(rec.EventSubjects) != 1 || rec.EventSubjects[0] != "task.created" {
		t.Fatalf("ActivePlugins() record = %+v, want EventSubjects=[task.created]", rec)
	}
}
