package acp

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestWakeupLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: "stderr",
	})
	if err != nil {
		t.Fatalf("logger init failed: %v", err)
	}
	return log
}

func TestExtractScheduleWakeup_NotAWakeup(t *testing.T) {
	cases := []struct {
		name string
		meta any
	}{
		{"nil", nil},
		{"non-map", "string"},
		{"empty map", map[string]any{}},
		{"missing claudeCode", map[string]any{"other": "x"}},
		{"claudeCode not a map", map[string]any{"claudeCode": "x"}},
		{"different tool", map[string]any{
			"claudeCode": map[string]any{"toolName": "Bash"},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms, ok := extractScheduleWakeup(c.meta)
			if ok {
				t.Errorf("expected isWakeup=false for %s, got true (ms=%d)", c.name, ms)
			}
		})
	}
}

func TestExtractScheduleWakeup_NoResponseYet(t *testing.T) {
	meta := map[string]any{
		"claudeCode": map[string]any{
			"toolName": "ScheduleWakeup",
		},
	}
	ms, ok := extractScheduleWakeup(meta)
	if !ok {
		t.Fatal("expected isWakeup=true")
	}
	if ms != 0 {
		t.Errorf("expected ms=0 when toolResponse missing, got %d", ms)
	}
}

func TestExtractScheduleWakeup_WithResponse(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want int64
	}{
		{"float64", float64(1700000000000), 1700000000000},
		{"int", int(1700000000000), 1700000000000},
		{"int64", int64(1700000000000), 1700000000000},
		{"unsupported type", "1700000000000", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			meta := map[string]any{
				"claudeCode": map[string]any{
					"toolName": "ScheduleWakeup",
					"toolResponse": map[string]any{
						"scheduledFor": c.val,
					},
				},
			}
			ms, ok := extractScheduleWakeup(meta)
			if !ok {
				t.Fatal("expected isWakeup=true")
			}
			if ms != c.want {
				t.Errorf("ms=%d, want %d", ms, c.want)
			}
		})
	}
}

func TestExtractWakeupPrompt(t *testing.T) {
	cases := []struct {
		name     string
		input    any
		wantOK   bool
		wantText string
	}{
		{"nil input", nil, false, ""},
		{"non-map", "x", false, ""},
		{"empty map", map[string]any{}, false, ""},
		{"empty prompt", map[string]any{"prompt": ""}, false, ""},
		{"non-string prompt", map[string]any{"prompt": 42}, false, ""},
		{"valid prompt", map[string]any{"prompt": "hello"}, true, "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := extractWakeupPrompt(c.input)
			if ok != c.wantOK || got != c.wantText {
				t.Errorf("got (%q, %v), want (%q, %v)", got, ok, c.wantText, c.wantOK)
			}
		})
	}
}

type wakeupCapture struct {
	mu        sync.Mutex
	calls     int32
	lastSess  string
	lastPrmpt string
}

func newWakeupCapture() *wakeupCapture {
	return &wakeupCapture{}
}

func (c *wakeupCapture) fire(sessionID, prompt string) {
	c.mu.Lock()
	c.lastSess = sessionID
	c.lastPrmpt = prompt
	c.mu.Unlock()
	atomic.AddInt32(&c.calls, 1)
}

// Timer-driven tests use testing/synctest so time.AfterFunc is intercepted by
// synctest's fake clock. To advance fake time past a scheduled timer we
// time.Sleep in the test goroutine — under synctest, all bubble goroutines
// are then blocked and the runtime jumps fake time to the next pending event.
// synctest.Wait() afterwards ensures any goroutine the timer spawned has
// finished its work before the assertion.

func TestWakeupScheduler_FiresAfterDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestWakeupLogger(t)
		cap := newWakeupCapture()
		sched := newWakeupScheduler(log, cap.fire)

		at := time.Now().Add(40 * time.Millisecond).UnixMilli()
		sched.schedule("sess-1", "wake-prompt", at)

		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		if got := atomic.LoadInt32(&cap.calls); got != 1 {
			t.Fatalf("expected 1 fire, got %d", got)
		}
		cap.mu.Lock()
		defer cap.mu.Unlock()
		if cap.lastSess != "sess-1" || cap.lastPrmpt != "wake-prompt" {
			t.Errorf("unexpected fire args: sess=%q prompt=%q", cap.lastSess, cap.lastPrmpt)
		}
	})
}

func TestWakeupScheduler_CancelPreventsFire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestWakeupLogger(t)
		cap := newWakeupCapture()
		sched := newWakeupScheduler(log, cap.fire)

		at := time.Now().Add(50 * time.Millisecond).UnixMilli()
		sched.schedule("sess-1", "p", at)
		sched.cancel()

		// Advance fake time past the would-be fire instant; nothing should run.
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		if got := atomic.LoadInt32(&cap.calls); got != 0 {
			t.Errorf("expected 0 fires, got %d", got)
		}
	})
}

func TestWakeupScheduler_StaleTimestampDropped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestWakeupLogger(t)
		cap := newWakeupCapture()
		sched := newWakeupScheduler(log, cap.fire)

		// Past timestamp: should not arm a timer at all.
		sched.schedule("sess-1", "p", time.Now().Add(-time.Hour).UnixMilli())

		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		if got := atomic.LoadInt32(&cap.calls); got != 0 {
			t.Errorf("expected 0 fires, got %d", got)
		}
	})
}

func TestWakeupScheduler_RescheduleReplacesPrior(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestWakeupLogger(t)
		cap := newWakeupCapture()
		sched := newWakeupScheduler(log, cap.fire)

		// First schedule far in the future, then reschedule soon — only the
		// second should fire, and only once.
		sched.schedule("sess-1", "first", time.Now().Add(time.Hour).UnixMilli())
		sched.schedule("sess-1", "second", time.Now().Add(40*time.Millisecond).UnixMilli())

		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		if got := atomic.LoadInt32(&cap.calls); got != 1 {
			t.Fatalf("expected 1 fire, got %d", got)
		}
		cap.mu.Lock()
		defer cap.mu.Unlock()
		if cap.lastPrmpt != "second" {
			t.Errorf("expected last prompt %q, got %q", "second", cap.lastPrmpt)
		}
	})
}

// TestWakeupScheduler_StaleFireOnceDoesNotConsumeNewState reproduces the race
// where time.AfterFunc has already launched a fireOnce goroutine for the old
// timer before schedule() runs. The stale fireOnce must observe the gen
// mismatch and bail out, not consume the newly scheduled wakeup.
func TestWakeupScheduler_StaleFireOnceDoesNotConsumeNewState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestWakeupLogger(t)
		cap := newWakeupCapture()
		sched := newWakeupScheduler(log, cap.fire)

		// Manually drive the race: arm a timer, capture its gen, then simulate
		// a stale fire by calling fireOnce with the captured gen *after* a
		// rescheduling has bumped gen.
		sched.mu.Lock()
		sched.gen++
		staleGen := sched.gen
		sched.sessionID = "old-sess"
		sched.prompt = "old-prompt"
		sched.mu.Unlock()

		// Reschedule before stale fire executes.
		sched.schedule("new-sess", "new-prompt", time.Now().Add(time.Hour).UnixMilli())

		// Stale fire arrives now — should be a no-op.
		sched.fireOnce(staleGen)

		if got := atomic.LoadInt32(&cap.calls); got != 0 {
			t.Errorf("stale fireOnce fired anyway: calls=%d", got)
		}

		// New schedule should still fire normally when its timer elapses.
		sched.cancel()
		sched.schedule("new-sess", "new-prompt", time.Now().Add(40*time.Millisecond).UnixMilli())

		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		if got := atomic.LoadInt32(&cap.calls); got != 1 {
			t.Fatalf("expected 1 fire after reschedule, got %d", got)
		}
		cap.mu.Lock()
		defer cap.mu.Unlock()
		if cap.lastSess != "new-sess" || cap.lastPrmpt != "new-prompt" {
			t.Errorf("fired with wrong args: sess=%q prompt=%q", cap.lastSess, cap.lastPrmpt)
		}
	})
}

func TestWakeupScheduler_IgnoresMissingFields(t *testing.T) {
	log := newTestWakeupLogger(t)
	cap := newWakeupCapture()
	sched := newWakeupScheduler(log, cap.fire)

	// Missing each required field in turn — none should arm a timer.
	sched.schedule("", "p", time.Now().Add(time.Minute).UnixMilli())
	sched.schedule("sess", "", time.Now().Add(time.Minute).UnixMilli())
	sched.schedule("sess", "p", 0)

	if got := atomic.LoadInt32(&cap.calls); got != 0 {
		t.Errorf("expected 0 fires, got %d", got)
	}
}

func TestHandleWakeupEvent_SchedulesOnceBothFieldsArrive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := newTestAdapter()
		t.Cleanup(func() { _ = a.Close() })

		cap := newWakeupCapture()
		a.wakeup = newWakeupScheduler(a.logger, cap.fire)

		at := time.Now().Add(40 * time.Millisecond).UnixMilli()

		// Initial tool_call: only the toolName is known.
		metaInitial := map[string]any{
			"claudeCode": map[string]any{"toolName": "ScheduleWakeup"},
		}
		a.handleWakeupEvent("sess-1", "tc-1", metaInitial, nil, false)

		// Mid-update: rawInput arrives with the prompt.
		a.handleWakeupEvent("sess-1", "tc-1", metaInitial, map[string]any{
			"prompt":       "wake-now",
			"delaySeconds": 60,
		}, false)

		// Verify nothing fired yet — scheduledFor still missing.
		if got := atomic.LoadInt32(&cap.calls); got != 0 {
			t.Fatalf("fired before scheduledFor known: calls=%d", got)
		}

		// Final update: scheduledFor arrives in meta.
		metaWithResp := map[string]any{
			"claudeCode": map[string]any{
				"toolName":     "ScheduleWakeup",
				"toolResponse": map[string]any{"scheduledFor": float64(at)},
			},
		}
		a.handleWakeupEvent("sess-1", "tc-1", metaWithResp, nil, false)

		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		if got := atomic.LoadInt32(&cap.calls); got != 1 {
			t.Fatalf("expected 1 fire, got %d", got)
		}
		cap.mu.Lock()
		defer cap.mu.Unlock()
		if cap.lastPrmpt != "wake-now" {
			t.Errorf("prompt=%q, want %q", cap.lastPrmpt, "wake-now")
		}
	})
}

func TestHandleWakeupEvent_NonWakeupIgnored(t *testing.T) {
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	cap := newWakeupCapture()
	a.wakeup = newWakeupScheduler(a.logger, cap.fire)

	// Bash tool — should be ignored entirely.
	meta := map[string]any{
		"claudeCode": map[string]any{
			"toolName":     "Bash",
			"toolResponse": map[string]any{"scheduledFor": float64(time.Now().Add(time.Hour).UnixMilli())},
		},
	}
	a.handleWakeupEvent("sess-1", "tc-1", meta, map[string]any{"prompt": "x"}, false)

	a.mu.Lock()
	if _, present := a.pendingWakeups["tc-1"]; present {
		t.Error("non-wakeup tool should not be tracked")
	}
	a.mu.Unlock()

	if got := atomic.LoadInt32(&cap.calls); got != 0 {
		t.Errorf("expected 0 fires, got %d", got)
	}
}

func TestHandleWakeupEvent_TerminalCleansUpPending(t *testing.T) {
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	cap := newWakeupCapture()
	a.wakeup = newWakeupScheduler(a.logger, cap.fire)

	meta := map[string]any{
		"claudeCode": map[string]any{"toolName": "ScheduleWakeup"},
	}
	a.handleWakeupEvent("sess-1", "tc-1", meta, nil, false)

	a.mu.Lock()
	_, present := a.pendingWakeups["tc-1"]
	a.mu.Unlock()
	if !present {
		t.Fatal("expected pending wakeup before terminal")
	}

	// Terminal arrives without prompt or scheduledFor — should clean up.
	a.handleWakeupEvent("sess-1", "tc-1", meta, nil, true)

	a.mu.Lock()
	_, present = a.pendingWakeups["tc-1"]
	a.mu.Unlock()
	if present {
		t.Error("expected pending wakeup cleared on terminal")
	}
	if got := atomic.LoadInt32(&cap.calls); got != 0 {
		t.Errorf("terminal-without-data should not fire, got %d calls", got)
	}
}

// TestFireWakeup_SkipsOnSessionChange exercises fireWakeup's session-guard
// branch — when the adapter's current session has rotated away from the one
// the wakeup was scheduled for, the wakeup must drop instead of trying to
// drive an unrelated session.
// TestFireWakeup_SkipsOnSessionChange exercises fireWakeup's session-guard
// branch — when the adapter's current session has rotated away from the one
// the wakeup was scheduled for, the wakeup must drop instead of trying to
// drive an unrelated session.
//
// We can't observe Prompt directly (it returns early on nil acpConn without
// any side effect on the updates channel) so we use a.pendingContext as a
// canary: Prompt's first action under a.mu is to read-and-clear it. If the
// guard works, fireWakeup never spawns the goroutine that calls Prompt and
// the canary survives. If the guard regresses, Prompt runs and clears it.
func TestFireWakeup_SkipsOnSessionChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := newTestAdapter()
		t.Cleanup(func() { _ = a.Close() })

		// Adapter currently tracks a different session than the one the wakeup
		// was scheduled against, and pendingContext holds a canary string.
		const canary = "guard-canary"
		a.mu.Lock()
		a.sessionID = "new-sess"
		a.pendingContext = canary
		a.mu.Unlock()

		a.fireWakeup("old-sess", "should-not-fire")

		// Let any spawned goroutine run to completion. Inside synctest, after
		// Wait() returns the bubble has settled and we can inspect state.
		synctest.Wait()

		a.mu.Lock()
		got := a.pendingContext
		a.mu.Unlock()
		if got != canary {
			t.Errorf("pendingContext=%q, want %q — Prompt was reached, guard regressed", got, canary)
		}
	})
}
