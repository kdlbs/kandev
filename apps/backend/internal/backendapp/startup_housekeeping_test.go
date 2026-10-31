package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kdlbs/kandev/pluginsdk"
	"testing"
	"time"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.2
func TestHousekeepingDefersRealReachabilityPass(t *testing.T) {
	reporter := startup.New(nil)
	ctx := startup.WithReporter(t.Context(), reporter)
	repo := &sshReachabilityWiringRepo{listed: make(chan struct{}, 1)}
	var stop func() error
	startSSHReachabilityPoller(ctx, repo, 3600, logger.Default(), nil, func(fn func() error) func() error { stop = fn; return fn })
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	// This poller has an immediate first pass and an hour-long recurring interval.
	select {
	case <-repo.listed:
		t.Fatal("remote pass admitted before housekeeping finished")
	case <-time.After(100 * time.Millisecond):
	}
	reporter.Set(startup.Ready)
	if err := startup.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.listed:
	case <-time.After(time.Second):
		t.Fatal("activated poller did not run its immediate pass")
	}
}

type housekeepingPluginRuntime struct {
	alwaysUpRuntime
	entered chan struct{}
	release chan struct{}
}

func (r *housekeepingPluginRuntime) Running(string) bool { return false }
func (r *housekeepingPluginRuntime) Start(ctx context.Context, _ *store.Record, _ func(string) pluginsdk.Host) error {
	close(r.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

func TestHousekeepingPluginStartupDoesNotBlockOtherActivation(t *testing.T) {
	svc := newTestPluginsService(t)
	record := installTestPluginForBoot(t, svc, "startup-housekeeping", false)
	if err := svc.Enable(record.ID); err != nil {
		t.Fatal(err)
	}
	runtime := &housekeepingPluginRuntime{entered: make(chan struct{}), release: make(chan struct{})}
	repaired := make(chan struct{})
	lifecycle := housekeepingPluginLifecycle{pluginToolLifecycleFake: pluginToolLifecycleFake{calls: make(chan plugintools.Snapshot, 10)}, repaired: repaired}
	svc.SetRuntime(runtime)
	ctx, cancel := context.WithCancel(t.Context())
	var cleanups []func() error
	defer func() {
		cancel()
		for i := len(cleanups) - 1; i >= 0; i-- {
			if err := cleanups[i](); err != nil {
				t.Error(err)
			}
		}
	}()
	reporter := startup.New(nil)
	ctx = startup.WithReporter(ctx, reporter)
	startPluginsSubsystems(ctx, svc, lifecycle, bus.NewMemoryEventBus(logger.Default()), logger.Default(), func(fn func() error) { cleanups = append(cleanups, fn) })
	later := make(chan struct{})
	startup.AfterReady(ctx, func() { close(later) })
	reporter.Set(startup.Ready)
	activated := make(chan error, 1)
	go func() { activated <- startup.Activate(ctx) }()
	select {
	case <-runtime.entered:
	case <-time.After(time.Second):
		t.Fatal("active plugin never started")
	}
	select {
	case <-later:
	case <-time.After(time.Second):
		cancel()
		<-activated
		t.Fatal("a blocked plugin serialized unrelated runtime activation")
	}
	if err := <-activated; err != nil {
		t.Fatal(err)
	}
	select {
	case <-repaired:
		t.Fatal("inventory repaired before plugin boot completed")
	default:
	}
	close(runtime.release)
	select {
	case <-repaired:
	case <-time.After(time.Second):
		t.Fatal("unfinished plugin inventory never repaired after boot")
	}
}

type housekeepingPluginLifecycle struct {
	pluginToolLifecycleFake
	repaired chan struct{}
}

func (l housekeepingPluginLifecycle) ReconcileStartupPluginInventory(context.Context) error {
	close(l.repaired)
	return nil
}
