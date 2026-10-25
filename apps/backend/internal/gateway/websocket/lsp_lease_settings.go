package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/kandev/kandev/internal/events/bus"
)

func (h *LSPHandler) handleUserSettingsUpdated(_ context.Context, event *bus.Event) error {
	if h.leases == nil || event == nil {
		return nil
	}
	userID, payload := routingUserIDAndPayload(event.Data)
	if userID == "" {
		return nil
	}
	encoded, err := json.Marshal(payload["lsp_server_configs"])
	if err != nil {
		return nil
	}
	var configurations map[string]map[string]any
	if err := json.Unmarshal(encoded, &configurations); err != nil || configurations == nil {
		configurations = make(map[string]map[string]any)
	}
	h.leases.updateUserConfiguration(userID, configurations)
	return nil
}

func configurationSection(configuration map[string]any, section, language string) any {
	if section == "" {
		return configuration
	}
	if section == language {
		return configuration
	}
	if prefix := language + "."; language != "" && strings.HasPrefix(section, prefix) {
		section = strings.TrimPrefix(section, prefix)
	}
	var value any = configuration
	for _, part := range strings.Split(section, ".") {
		current, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value, ok = current[part]
		if !ok {
			return nil
		}
	}
	return value
}

func (l *lspLease) updateConfiguration(configuration map[string]any, notify bool) error {
	configuration = cloneStringAnyMap(configuration)
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return err
	}
	l.mu.Lock()
	current, _ := json.Marshal(l.configuration)
	ready, closed := l.ready, l.closed
	if bytes.Equal(current, encoded) {
		l.mu.Unlock()
		return l.notifyConfiguration(configuration, notify, ready, closed)
	}
	l.configuration = configuration
	l.configurationSnapshotBytes = len(encoded)
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.mu.Unlock()
		return err
	}
	l.mu.Unlock()
	return l.notifyConfiguration(configuration, notify, ready, closed)
}

func (l *lspLease) notifyConfiguration(configuration map[string]any, notify, ready, closed bool) error {
	if notify && ready && !closed {
		return l.writeUpstream(jsonRPCNotification("workspace/didChangeConfiguration", map[string]any{"settings": configuration}))
	}
	return nil
}
