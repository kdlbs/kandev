package process

import "testing"

// TestManagerStartsDetached pins that a freshly constructed Manager has no
// attached backend event stream yet -- matching design 03's "the coarser
// [server-owned] granularity would auto-deny exactly [in the window] before
// this instance's streams are up" caveat: a permission request in that
// window must be treated as detached, not attached.
func TestManagerStartsDetached(t *testing.T) {
	m := &Manager{}
	if m.IsAttached() {
		t.Fatal("IsAttached() = true for a freshly constructed Manager, want false")
	}
}

// TestMarkAttachedAndMarkDetachedToggleIsAttached pins the basic connect/
// disconnect transition the WS stream handler drives.
func TestMarkAttachedAndMarkDetachedToggleIsAttached(t *testing.T) {
	m := &Manager{}
	m.AttachStream("s1")
	if !m.IsAttached() {
		t.Fatal("IsAttached() = false after MarkAttached, want true")
	}
	m.DetachStream("s1")
	if m.IsAttached() {
		t.Fatal("IsAttached() = true after MarkDetached, want false")
	}
}

// TestIsAttachedStaysTrueAcrossAnOverlappingReconnect pins that a second,
// overlapping stream connection (e.g. a reconnect racing the old
// connection's teardown) does not make the instance look detached the
// moment the FIRST connection's defer runs -- attachment is a count, not a
// single flag, so it only reaches zero once every live connection has
// disconnected.
func TestIsAttachedStaysTrueAcrossAnOverlappingReconnect(t *testing.T) {
	m := &Manager{}
	m.AttachStream("s1")
	m.AttachStream("s2")

	m.DetachStream("s1")
	if !m.IsAttached() {
		t.Fatal("IsAttached() = false with one connection still live, want true")
	}

	m.DetachStream("s2")
	if m.IsAttached() {
		t.Fatal("IsAttached() = true after every connection disconnected, want false")
	}
}

func TestSnapshotReadyOnlyAfterMarkWriterReady(t *testing.T) {
	m := &Manager{}
	m.AttachStream("s1")
	snap := m.AttachmentSnapshot()
	if !snap.Attached || snap.Ready {
		t.Fatalf("snapshot after attach = %+v, want attached and not ready", snap)
	}
	m.MarkWriterReady("s1")
	snap = m.AttachmentSnapshot()
	if !snap.Ready || snap.Epoch != 1 {
		t.Fatalf("snapshot after ready = %+v, want ready at epoch 1", snap)
	}
}

func TestSnapshotChangeChClosesOnReadinessChangeAndIsReplaced(t *testing.T) {
	m := &Manager{}
	before := m.AttachmentSnapshot().ChangeCh
	m.AttachStream("s1")
	select {
	case <-before:
	default:
		t.Fatal("ChangeCh not closed by attach")
	}
	mid := m.AttachmentSnapshot().ChangeCh
	select {
	case <-mid:
		t.Fatal("replacement ChangeCh is already closed")
	default:
	}
	m.MarkWriterReady("s1")
	select {
	case <-mid:
	default:
		t.Fatal("ChangeCh not closed by writer ready")
	}
}

func TestEpochIncrementsEachTimeReadinessReturns(t *testing.T) {
	m := &Manager{}
	m.AttachStream("s1")
	m.MarkWriterReady("s1")
	m.DetachStream("s1")
	m.AttachStream("s2")
	m.MarkWriterReady("s2")
	if got := m.AttachmentSnapshot().Epoch; got != 2 {
		t.Fatalf("epoch = %d, want 2", got)
	}
}

func TestReadyStreamEndingWhileAnotherReplaysClearsReadiness(t *testing.T) {
	m := &Manager{}
	m.AttachStream("s1")
	m.MarkWriterReady("s1")
	m.AttachStream("s2") // replaying, not ready
	m.DetachStream("s1")
	snap := m.AttachmentSnapshot()
	if !snap.Attached || snap.Ready {
		t.Fatalf("snapshot = %+v, want attached and not ready", snap)
	}
}

func TestMarkWriterReadyForUnknownStreamIsIgnored(t *testing.T) {
	m := &Manager{}
	m.MarkWriterReady("ghost")
	if m.AttachmentSnapshot().Ready {
		t.Fatal("ready without an attached stream")
	}
}
