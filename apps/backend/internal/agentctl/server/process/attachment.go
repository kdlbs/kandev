package process

// attachedCount tracks how many live backend event-stream connections
// (/agent/stream) this instance currently has. It is a count rather than a
// single flag so an overlapping reconnect -- a new connection arriving
// before the old one's teardown has run -- cannot make IsAttached report
// false while a connection is still actually live.
//
// This is the agentctl-local "instance is attached" signal design 03
// requires: an instance is attached when its event stream to the owning
// backend is established, evaluated at INSTANCE granularity (not "the
// server has an owning backend", which is true during the window after
// adoption and before this instance's own stream reconnects -- exactly the
// window a permission request must not be auto-denied in).

// MarkAttached records that a backend event-stream connection is now live
// for this instance. Called by the /agent/stream handler once the
// WebSocket upgrade succeeds.
func (m *Manager) MarkAttached() {
	m.attachedCount.Add(1)
}

// MarkDetached records that a backend event-stream connection for this
// instance has ended. Called by the /agent/stream handler when the
// connection closes, for symmetry with MarkAttached.
func (m *Manager) MarkDetached() {
	m.attachedCount.Add(-1)
}

// IsAttached reports whether this instance currently has at least one live
// backend event-stream connection.
func (m *Manager) IsAttached() bool {
	return m.attachedCount.Load() > 0
}
