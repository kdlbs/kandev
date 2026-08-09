package lifecycle

import (
	"context"
)

// BackgroundProbe is the injectable port for querying whether background
// processes spawned by a session are still alive. The argument is the Kandev
// task-session id; the production implementation (Manager) translates it to
// the ACP session id before delegating to the agentctl client.
//
// Return values: "live" (≥1 qualifying descendant alive), "settled" (none
// found), "unknown" (session not found, transport error, or any failure).
// Every error path must map to "unknown" — callers treat it as a neutral
// non-park value and the feature degrades silently rather than false-parking.
type BackgroundProbe interface {
	ProbeBackgroundWorkloads(ctx context.Context, kandevSessionID string) (string, error)
}

// ProbeBackgroundWorkloads implements BackgroundProbe on *Manager.
// It resolves the Kandev task-session id to the ACP session id via the
// execution store, then delegates to the agentctl Client. Every error maps to
// "unknown" so the feature degrades gracefully — a missing or unroutable
// session never causes a false-park.
func (m *Manager) ProbeBackgroundWorkloads(ctx context.Context, kandevSessionID string) (string, error) {
	// AC-46's ninth and tenth conditions: an empty Kandev session id, and a
	// caller context already done on entry, both resolve to unknown with
	// nothing put on the wire — checked before any lookup.
	if kandevSessionID == "" {
		return sshStatusUnknown, nil
	}
	if ctx.Err() != nil {
		return sshStatusUnknown, nil
	}
	// F4 (Review round 2): this resolves the execution via a bare
	// *BySessionID lookup, which per apps/backend/CLAUDE.md's documented
	// convention skips the GetOrEnsure* chokepoint where the lifecycle
	// access check normally runs — so it must call CheckSessionAccess
	// itself. The spec's own Permissions section requires the same guard
	// RespondToPermission uses. A denial maps to "unknown" like every other
	// failure path (AC-46) rather than surfacing an auth error to a probe
	// caller.
	if err := m.CheckSessionAccess(ctx, kandevSessionID); err != nil {
		return sshStatusUnknown, nil
	}
	execution, exists := m.executionStore.GetBySessionID(kandevSessionID)
	if !exists || execution == nil {
		return sshStatusUnknown, nil
	}
	if execution.ACPSessionID == "" {
		return sshStatusUnknown, nil
	}
	client := execution.GetAgentCtlClient()
	if client == nil {
		return sshStatusUnknown, nil
	}
	result, err := client.ProbeBackgroundWorkloads(ctx, execution.ACPSessionID)
	if err != nil {
		return sshStatusUnknown, nil
	}
	return result, nil
}
