package process

import "sync"

// attachTracker records the live backend event-stream connections
// (/agent/stream) of one instance. It is a set rather than a flag so an
// overlapping reconnect -- a new connection arriving before the old one's
// teardown has run -- cannot make IsAttached report false while a connection
// is still live. Attachment is evaluated at INSTANCE granularity, not "the
// server has an owning backend", which is true after adoption and before this
// instance's own stream reconnects, exactly the window a permission request
// must not be auto-denied in.
//
// Zero value is a detached instance. All fields are guarded by mu.
type attachTracker struct {
	mu       sync.Mutex
	streams  map[string]bool // stream ID -> writer ready
	ready    int
	epoch    uint64
	changeCh chan struct{}
}

// AttachmentSnapshot is the instance's attach state at one instant. ChangeCh
// is closed, and replaced, on every change of Attached or Ready.
type AttachmentSnapshot struct {
	Attached bool
	Ready    bool
	Epoch    uint64
	ChangeCh <-chan struct{}
}

func (t *attachTracker) changeLocked() {
	if t.changeCh != nil {
		close(t.changeCh)
	}
	t.changeCh = make(chan struct{})
}

func (t *attachTracker) snapshotLocked() AttachmentSnapshot {
	if t.changeCh == nil {
		t.changeCh = make(chan struct{})
	}
	return AttachmentSnapshot{
		Attached: len(t.streams) > 0,
		Ready:    t.ready > 0,
		Epoch:    t.epoch,
		ChangeCh: t.changeCh,
	}
}

// AttachStream records that a backend event-stream connection is now live
// for this instance. It is not writer-ready until MarkWriterReady.
func (m *Manager) AttachStream(streamID string) {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.streams == nil {
		t.streams = make(map[string]bool)
	}
	t.streams[streamID] = false
	t.changeLocked()
}

// MarkWriterReady records that the stream's writer is serving Kandev calls
// for the current attachment, after the journal replay of that attach.
func (m *Manager) MarkWriterReady(streamID string) {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	ready, ok := t.streams[streamID]
	if !ok || ready {
		return
	}
	t.streams[streamID] = true
	t.ready++
	if t.ready == 1 {
		t.epoch++
	}
	t.changeLocked()
}

// DetachStream records that a backend event-stream connection for this
// instance has ended.
func (m *Manager) DetachStream(streamID string) {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	ready, ok := t.streams[streamID]
	if !ok {
		return
	}
	delete(t.streams, streamID)
	if ready {
		t.ready--
	}
	t.changeLocked()
}

// IsAttached reports whether this instance currently has at least one live
// backend event-stream connection.
func (m *Manager) IsAttached() bool {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.streams) > 0
}

// AttachmentSnapshot returns the current attach state.
func (m *Manager) AttachmentSnapshot() AttachmentSnapshot {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}
