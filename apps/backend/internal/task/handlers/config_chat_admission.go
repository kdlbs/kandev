package handlers

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/task/service"
)

type configChatAdmission struct {
	mu          sync.Mutex
	operations  map[string]string
	generations map[string]uint64
}

// An empty session denotes ordinary creation; a nonempty session denotes its
// retirement. Only live operations are retained.
func (a *configChatAdmission) begin(workspaceID, retiringSessionID string) (func(), bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.operations[workspaceID]; exists {
		return nil, false
	}
	if a.operations == nil {
		a.operations = make(map[string]string)
		a.generations = make(map[string]uint64)
	}
	a.operations[workspaceID] = retiringSessionID
	a.generations[workspaceID]++
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		delete(a.operations, workspaceID)
		a.generations[workspaceID]++
	}, true
}

func (a *configChatAdmission) snapshot(workspaceID string) (uint64, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.generations[workspaceID], a.operations[workspaceID]
}

func (h *TaskHandlers) listQuickChatsWithRestart(ctx context.Context, workspaceID string) ([]service.QuickChatSession, string, error) {
	for {
		generation, _ := h.configChatAdmission.snapshot(workspaceID)
		items, err := h.service.ListQuickChatSessions(ctx, workspaceID)
		if err != nil {
			return nil, "", err
		}
		after, retiringSessionID := h.configChatAdmission.snapshot(workspaceID)
		if after == generation {
			return items, retiringSessionID, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
	}
}
