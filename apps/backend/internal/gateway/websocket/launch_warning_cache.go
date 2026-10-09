package websocket

import (
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// rememberAndBroadcastSessionLaunchWarning stores the latest warning before
// the live fan-out. This ordering closes the subscribe-versus-publish race:
// a client either receives the live notification or replays the cached frame
// when it joins later.
func (h *Hub) rememberAndBroadcastSessionLaunchWarning(sessionID string, message *ws.Message) {
	if sessionID == "" || message == nil {
		return
	}
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	h.mu.Lock()
	if h.sessionLaunchWarnings == nil {
		h.sessionLaunchWarnings = make(map[string][]byte)
	}
	h.sessionLaunchWarnings[sessionID] = append([]byte(nil), data...)
	h.mu.Unlock()
	h.BroadcastToSession(sessionID, message)
}

func (h *Hub) clearSessionLaunchWarning(sessionID string) {
	if sessionID == "" {
		return
	}
	h.mu.Lock()
	delete(h.sessionLaunchWarnings, sessionID)
	h.mu.Unlock()
}

func (h *Hub) replaySessionLaunchWarning(client *Client, sessionID string) {
	if client == nil || sessionID == "" {
		return
	}
	h.mu.RLock()
	data := append([]byte(nil), h.sessionLaunchWarnings[sessionID]...)
	h.mu.RUnlock()
	if len(data) == 0 {
		return
	}
	client.sendNotificationFrame(newOutboundNotification(data, ws.ActionSessionLaunchWarning))
}
