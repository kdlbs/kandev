package websocket

import "sync/atomic"

func (h *Hub) nextSessionSeq(sessionID string) int64 {
	if sessionID == "" {
		return 0
	}
	value, ok := h.sessionSeqs.Load(sessionID)
	if !ok {
		value, _ = h.sessionSeqs.LoadOrStore(sessionID, &atomic.Int64{})
	}
	return value.(*atomic.Int64).Add(1)
}

func (h *Hub) incSessionSubscribers(sessionID string) {
	if sessionID == "" {
		return
	}
	value, ok := h.sessionSubscriberCounts.Load(sessionID)
	if !ok {
		value, _ = h.sessionSubscriberCounts.LoadOrStore(sessionID, &atomic.Int64{})
	}
	value.(*atomic.Int64).Add(1)
	if _, exists := h.sessionSeqs.Load(sessionID); !exists {
		h.sessionSeqs.LoadOrStore(sessionID, &atomic.Int64{})
	}
}

func (h *Hub) decSessionSubscribers(sessionID string) {
	if sessionID == "" {
		return
	}
	value, ok := h.sessionSubscriberCounts.Load(sessionID)
	if !ok {
		return
	}
	if value.(*atomic.Int64).Add(-1) <= 0 {
		h.sessionSubscriberCounts.Delete(sessionID)
	}
}

func (h *Hub) deleteSessionSeqIfIdleLocked(sessionID string) {
	if sessionID == "" {
		return
	}
	if len(h.sessionSubscribers[sessionID]) > 0 {
		return
	}
	if len(h.sessionMode.focusByClient[sessionID]) > 0 {
		return
	}
	h.sessionSeqs.Delete(sessionID)
}
