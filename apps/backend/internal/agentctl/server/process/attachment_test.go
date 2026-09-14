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
	m.MarkAttached()
	if !m.IsAttached() {
		t.Fatal("IsAttached() = false after MarkAttached, want true")
	}
	m.MarkDetached()
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
	m.MarkAttached() // first connection
	m.MarkAttached() // second, overlapping connection

	m.MarkDetached() // first connection's defer fires
	if !m.IsAttached() {
		t.Fatal("IsAttached() = false with one connection still live, want true")
	}

	m.MarkDetached() // second connection's defer fires
	if m.IsAttached() {
		t.Fatal("IsAttached() = true after every connection disconnected, want false")
	}
}
