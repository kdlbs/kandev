package controller

import (
	"context"
	"github.com/kandev/kandev/internal/agent/agents"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.2
func TestRuntimeBackgroundOwnsOneCadenceAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ag := agents.NewGemini()
		c := newTestController(map[string]agents.Agent{ag.ID(): ag})
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "9.0.0", nil })
		notices := &runtimeNoticeCapture{}
		c.SetRuntimeUpdateNotifier(notices)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		synctest.Wait()
		if notices.count() != 1 {
			t.Fatalf("startup pass notices=%d", notices.count())
		}
		time.Sleep(15 * time.Minute)
		synctest.Wait()
		if notices.count() != 2 {
			t.Fatalf("cadence notices=%d", notices.count())
		}
		stop()
		time.Sleep(15 * time.Minute)
		synctest.Wait()
		if notices.count() != 2 {
			t.Fatal("disposed background loop published")
		}
	})
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.2
func TestRuntimeBackgroundWaitsForHostProbeBootstrap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestController(map[string]agents.Agent{"gemini": agents.NewGemini()})
		var calls atomic.Int32
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { calls.Add(1); return "9.0.0", nil })
		ready := make(chan struct{})
		stop := c.StartRuntimeUpdateBackground(context.Background(), ready)
		synctest.Wait()
		if calls.Load() != 0 {
			t.Fatal("source lookup started before host probes")
		}
		close(ready)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("bootstrap completion did not start immediate pass")
		}
		stop()
	})
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.4
func TestRuntimeSubscriberReplayCannotStartAutomaticJobsAfterWorkerStops(t *testing.T) {
	previous := agents.NewGemini().ManagedNPMRuntime().DefaultVersionOrPinned()
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", previous}}}
	c, _ := autoController(t, updater, newRecoverySelectionStore(), &autoMemorySettings{})
	ctx := context.Background()
	stop := c.StartRuntimeUpdateBackground(ctx)
	stop()
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	notices := &runtimeNoticeCapture{}
	c.SetRuntimeUpdateNotifier(notices)
	if err := c.ReplayRuntimeUpdateNotices(ctx); err != nil {
		t.Fatal(err)
	}
	if len(c.ListAgentUpdateJobs()) != 0 {
		t.Fatal("subscriber replay restarted mutation after shutdown/restore quiesce")
	}
	if notices.count() != 1 {
		t.Fatalf("replayed notices=%d, want available runtime", notices.count())
	}
}
