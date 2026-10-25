package scope

import (
	"context"
	"fmt"
)

// GuardSource reports whether a task holds a sandbox task guard in a
// workspace. An error means the answer is unknown.
type GuardSource interface {
	TaskGuarded(ctx context.Context, workspaceID, taskID string) (bool, error)
}

// SetGuardSource wires task-guard resolution. A resolver without a source
// resolves every principal as unguarded.
func (r *Resolver) SetGuardSource(src GuardSource) {
	r.guards = src
}

// resolveGuard fails closed: an unreadable guard denies the dispatch rather
// than yielding an unguarded principal.
func (r *Resolver) resolveGuard(ctx context.Context, p *Principal) error {
	if r.guards == nil {
		return nil
	}
	guarded, err := r.guards.TaskGuarded(ctx, p.WorkspaceID, p.CallerTaskID)
	if err != nil {
		return fmt.Errorf("resolve MCP principal task %s guard: %w", p.CallerTaskID, err)
	}
	p.Guarded = guarded
	return nil
}
