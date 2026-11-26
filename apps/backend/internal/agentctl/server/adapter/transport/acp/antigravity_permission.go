package acp

import (
	"context"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

type permissionToolNameKey struct {
	sessionID  string
	toolCallID string
}

type permissionToolName struct {
	name      string
	ambiguous bool
}

func (a *Adapter) usesAntigravityCLI() bool {
	return a != nil && a.cfg != nil && a.agentID == acpcompat.AntigravityAgentID && a.cfg.AuthPolicy != nil &&
		a.cfg.AuthPolicy.RuntimeVariant == acpcompat.AntigravityRuntimeCLI
}

func (a *Adapter) trackAntigravityPermissionToolNameLocked(sessionID string, call *acp.SessionUpdateToolCall) {
	if !a.usesAntigravityCLI() || sessionID == "" || call == nil {
		return
	}
	toolCallID := string(call.ToolCallId)
	if toolCallID == "" {
		return
	}
	key := permissionToolNameKey{sessionID: sessionID, toolCallID: toolCallID}
	if a.antigravityPermissionToolNames == nil {
		a.antigravityPermissionToolNames = make(map[permissionToolNameKey]permissionToolName)
	}
	if existing, ok := a.antigravityPermissionToolNames[key]; ok {
		if existing.ambiguous {
			return
		}
		if call.Name == nil || strings.TrimSpace(*call.Name) == "" {
			return
		}
		name := *call.Name
		if existing.name == "" {
			a.antigravityPermissionToolNames[key] = permissionToolName{name: name}
			return
		}
		if existing.name != name {
			a.antigravityPermissionToolNames[key] = permissionToolName{ambiguous: true}
			return
		}
		return
	}
	if call.Name == nil || strings.TrimSpace(*call.Name) == "" {
		a.antigravityPermissionToolNames[key] = permissionToolName{}
		return
	}
	a.antigravityPermissionToolNames[key] = permissionToolName{name: *call.Name}
}

func (a *Adapter) antigravityPermissionToolName(sessionID, toolCallID string) (*string, bool) {
	if !a.usesAntigravityCLI() || sessionID == "" || toolCallID == "" {
		return nil, false
	}
	key := permissionToolNameKey{sessionID: sessionID, toolCallID: toolCallID}
	a.mu.Lock()
	defer a.mu.Unlock()
	identity, ok := a.antigravityPermissionToolNames[key]
	if !ok {
		return nil, false
	}
	if identity.ambiguous || identity.name == "" {
		return nil, identity.ambiguous
	}
	name := identity.name
	return &name, false
}

func (a *Adapter) waitForAntigravityPermissionToolCall(
	ctx context.Context,
	sessionID, toolCallID string,
	timeout time.Duration,
) (bool, *string, bool) {
	if !a.usesAntigravityCLI() || sessionID == "" || toolCallID == "" {
		return false, nil, false
	}
	const pollInterval = 10 * time.Millisecond
	key := permissionToolNameKey{sessionID: sessionID, toolCallID: toolCallID}
	deadline := time.Now().Add(timeout)
	for {
		a.mu.RLock()
		identity, tracked := a.antigravityPermissionToolNames[key]
		a.mu.RUnlock()
		if tracked && identity.ambiguous {
			return true, nil, true
		}
		if tracked && identity.name != "" {
			name := identity.name
			return true, &name, false
		}
		if time.Now().After(deadline) {
			return tracked, nil, false
		}
		select {
		case <-ctx.Done():
			return false, nil, false
		case <-time.After(pollInterval):
		}
	}
}
