package process

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

// parkThenAssertRelease is the shared shape for the three "must park, not
// discard" site tests below: fill updatesCh, run the site's send in a
// goroutine, confirm it has not returned while the channel stays full, then
// release it (by draining the channel or closing stopCh) and confirm the
// goroutine finishes promptly afterward.
func parkThenAssertRelease(t *testing.T, run func(), release func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		run()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("send returned before the channel had room or a stop was signalled -- it must park, not discard")
	case <-time.After(50 * time.Millisecond):
	}

	release()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("send did not return promptly after release")
	}
}

// TestSendErrorEventWithProviderErrorParksOnAFullChannel pins
// AC-EXECUTORS-SURVIVAL-001.5/.6 for the agent-ERROR site: a full channel
// must park the caller rather than silently drop the error, and must
// release once a consumer drains a slot.
func TestSendErrorEventWithProviderErrorParksOnAFullChannel(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	m.stopChSnapshot.Store(make(chan struct{}))
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete} // fill the buffer

	parkThenAssertRelease(t,
		func() { m.SendErrorEventWithProviderError("boom", 1, nil) },
		func() { <-m.updatesCh },
	)

	select {
	case event := <-m.updatesCh:
		if event.Error != "boom" {
			t.Fatalf("Error = %q, want %q", event.Error, "boom")
		}
	default:
		t.Fatal("the parked error event was never delivered")
	}
}

// TestSendErrorEventWithProviderErrorReleasesOnStop pins the pause-not-a-hang
// half: a parked send must release the instant stop fires, without ever
// delivering the event.
func TestSendErrorEventWithProviderErrorReleasesOnStop(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	stopCh := make(chan struct{})
	m.stopChSnapshot.Store(stopCh)
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete}

	parkThenAssertRelease(t,
		func() { m.SendErrorEventWithProviderError("boom", 1, nil) },
		func() { close(stopCh) },
	)
}

// TestSendErrorEventWithProviderErrorReleasesOnRealManagerStop pins the same
// release guarantee driven through the real Manager.Stop path, not a
// hand-closed stopCh standing in for it: a full channel must still park a
// send in flight when Stop is called on a started manager, and Stop's own
// teardown (closeAdapterAndStdin closing stopCh) must release it and return
// within a bounded time, not hang waiting on the parked goroutine.
func TestSendErrorEventWithProviderErrorReleasesOnRealManagerStop(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	m.stopCh = make(chan struct{})
	m.stopChSnapshot.Store(m.stopCh)
	m.status.Store(StatusRunning)
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete}

	parkThenAssertRelease(t,
		func() { m.SendErrorEventWithProviderError("boom", 1, nil) },
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := m.Stop(ctx); err != nil {
				t.Errorf("Stop: %v", err)
			}
		},
	)
}

// TestSendPermissionCancelledNotificationParksOnAFullChannel pins
// AC-EXECUTORS-SURVIVAL-001.5/.6 for the permission-cancelled site.
func TestSendPermissionCancelledNotificationParksOnAFullChannel(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	m.stopChSnapshot.Store(make(chan struct{}))
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete}
	pending := &PendingPermission{ID: "pending-1", RequestID: "req-1", Request: &adapter.PermissionRequest{}}

	parkThenAssertRelease(t,
		func() { m.sendPermissionCancelledNotification(pending) },
		func() { <-m.updatesCh },
	)

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypePermissionCancelled || event.PendingID != "pending-1" {
			t.Fatalf("event = %+v, want a permission-cancelled event for pending-1", event)
		}
	default:
		t.Fatal("the parked permission-cancelled event was never delivered")
	}
}

// newPendingPermissionForNotificationTest builds a PendingPermission with a
// populated Snapshot, matching the shape sendPermissionNotification reads.
func newPendingPermissionForNotificationTest(m *Manager) *PendingPermission {
	pending := &PendingPermission{
		ID:        "pending-1",
		RequestID: "req-1",
		Request: &adapter.PermissionRequest{
			SessionID:  "session-1",
			ToolCallID: "tool-1",
			Title:      "Run command",
			Options: []adapter.PermissionOption{
				{OptionID: "allow-once", Kind: "allow_once"},
			},
		},
		ResponseCh: make(chan *adapter.PermissionResponse, 1),
	}
	pending.Snapshot = m.permissionSnapshot(pending)
	return pending
}

// TestSendPermissionNotificationParksWhenDetachedOnAFullChannel pins the
// behaviour change design 03 requires for the eighth COVERED site: while
// DETACHED, a full channel must park the permission-request notification
// rather than auto-cancelling after five seconds -- auto-cancelling while
// detached would silently deny every permission the agent asks for.
func TestSendPermissionNotificationParksWhenDetachedOnAFullChannel(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	m.stopChSnapshot.Store(make(chan struct{}))
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete} // fill the buffer
	pending := newPendingPermissionForNotificationTest(m)

	parkThenAssertRelease(t,
		func() { m.sendPermissionNotification(pending) },
		func() { <-m.updatesCh },
	)

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypePermissionRequest || event.PendingID != "pending-1" {
			t.Fatalf("event = %+v, want a permission-request event for pending-1", event)
		}
	default:
		t.Fatal("the parked permission-request event was never delivered")
	}
	select {
	case resp := <-pending.ResponseCh:
		t.Fatalf("ResponseCh received %+v, want no auto-cancel while parked", resp)
	default:
	}
}

// TestSendPermissionNotificationReleasesOnStopWithoutAutoCancellingWhenDetached
// pins the "pause not a hang" half for the detached branch: a parked
// permission-request notification releases on instance stop like every
// other COVERED site, and -- unlike the attached five-second timer path --
// does not synthesize a Cancelled response, since nobody is waiting to be
// denied and the instance is being torn down anyway.
func TestSendPermissionNotificationReleasesOnStopWithoutAutoCancellingWhenDetached(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	stopCh := make(chan struct{})
	m.stopChSnapshot.Store(stopCh)
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete}
	pending := newPendingPermissionForNotificationTest(m)

	parkThenAssertRelease(t,
		func() { m.sendPermissionNotification(pending) },
		func() { close(stopCh) },
	)

	select {
	case resp := <-pending.ResponseCh:
		t.Fatalf("ResponseCh received %+v, want no auto-cancel on a stop release", resp)
	default:
	}
}

// TestSendPermissionNotificationSendsImmediatelyWhenAttachedAndRoomAvailable
// pins that the attached branch is unchanged for the common case: with room
// on the channel, the event is delivered the same way it always was,
// regardless of the new attached/detached gate.
func TestSendPermissionNotificationSendsImmediatelyWhenAttachedAndRoomAvailable(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	m.MarkAttached()
	pending := newPendingPermissionForNotificationTest(m)

	m.sendPermissionNotification(pending)

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypePermissionRequest || event.PendingID != "pending-1" {
			t.Fatalf("event = %+v, want a permission-request event for pending-1", event)
		}
	default:
		t.Fatal("attached send with room available did not deliver the event")
	}
}

// TestManagerFailingProcessHelper is a self-exec subprocess helper that
// exits with a nonzero status, used by
// TestWaitForExitParksTheExitErrorEventOnAFullChannel below to drive
// waitForExit's real error branch through a real process exit rather than a
// fabricated exec.Cmd state.
func TestManagerFailingProcessHelper(t *testing.T) {
	if os.Getenv("KANDEV_MANAGER_FAILING_PROCESS_HELPER") != "1" {
		return
	}
	os.Exit(3)
}

// TestWaitForExitParksTheExitErrorEventOnAFullChannel pins
// AC-EXECUTORS-SURVIVAL-001.5/.6 for the agent-process-exit-error site: the
// exit error event carrying exit code and recent stderr must park on a full
// channel rather than being dropped, since it is the only report that the
// agent process died at all.
func TestWaitForExitParksTheExitErrorEventOnAFullChannel(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestManagerFailingProcessHelper")
	cmd.Env = append(os.Environ(), "KANDEV_MANAGER_FAILING_PROCESS_HELPER=1")
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	cmd.Stderr = stderrWriter
	m := &Manager{
		cmd:          cmd,
		stderr:       stderr,
		logger:       newTestLogger(t),
		doneCh:       make(chan struct{}),
		updatesCh:    make(chan adapter.AgentEvent, 1),
		groupAliveFn: func(int) bool { return false },
	}
	m.status.Store(StatusRunning)
	// sendUpdateBlocking treats an unestablished stopChSnapshot as "already
	// stopped" (see its doc comment) so a genuinely never-started Manager
	// doesn't park forever. Since this test builds a Manager by struct
	// literal instead of through Start(), it must seed the snapshot itself
	// the same way Start() does, or the send below would return immediately
	// on a fast cmd.Wait() instead of parking -- a real flake this test hit
	// before the snapshot was added here.
	m.stopChSnapshot.Store(make(chan struct{}))
	m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeComplete} // fill the buffer
	t.Cleanup(func() {
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = stderrWriter.Close()
		_ = stderr.Close()
		m.wg.Wait()
	})
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("close parent stderr pipe: %v", err)
	}

	stderrDone := make(chan struct{})
	m.wg.Add(2)
	go m.readStderr(stderrDone)

	parkThenAssertRelease(t,
		func() { m.waitForExit(stderrDone) },
		func() { <-m.updatesCh },
	)

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypeError {
			t.Fatalf("event type = %q, want error", event.Type)
		}
		if data, ok := event.Data["exit_code"]; !ok || data != 3 {
			t.Fatalf("exit_code = %v, want 3", data)
		}
	default:
		t.Fatal("the parked exit-error event was never delivered")
	}
}
