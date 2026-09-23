package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	if bytes.Equal(current, encoded) {
		l.mu.Unlock()
		return nil
	}
	l.configuration = configuration
	ready, closed := l.ready, l.closed
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.mu.Unlock()
		return err
	}
	l.mu.Unlock()
	if notify && ready && !closed {
		return l.writeUpstream(jsonRPCNotification("workspace/didChangeConfiguration", map[string]any{"settings": configuration}))
	}
	return nil
}

func (l *lspLease) checkSnapshotLimitLocked() error {
	snapshot := map[string]any{
		"workspacePath":      l.workspacePath,
		"workspaceUri":       l.workspaceURI,
		"repoSubpaths":       l.repoSubpaths,
		"initializeResult":   json.RawMessage(l.initializeResult),
		"initializeServerID": json.RawMessage(l.initializeServerID),
		"initializeResponse": json.RawMessage(l.initializeResponse),
		"initializeWaiter":   l.initializeWaiter,
		"initialized":        l.initializedReceived,
		"registrations":      l.registrations,
		"progressTokens":     l.progressTokens,
		"progress":           l.progress,
		"configuration":      l.configuration,
		"documentVersions":   l.documentVersions,
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(encoded) > lspLeaseSnapshotLimit {
		return fmt.Errorf("LSP resume state is %d bytes; maximum is %d", len(encoded), lspLeaseSnapshotLimit)
	}
	return nil
}
