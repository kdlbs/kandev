package websocket

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/auth/authn"
)

// Per-user scoping for dispatched WS RPC actions.
//
// Subscriptions and HTTP routes were scoped when opt-in auth landed, but
// dispatched actions were left to authorize themselves at the service layer.
// That is per-method opt-in across ~74 actions in four packages, and it was
// missed repeatedly: session stop/delete/rename/launch, agent.cancel,
// permission.respond and agent.stdin all reached another user's session with
// nothing but a guessed ID. agent.stdin writes arbitrary bytes into the agent
// process, so the gap was equivalent to running commands in someone else's
// worktree.
//
// This is the structural backstop. Rather than trusting every present and
// future handler to remember, any action naming a task or session in its
// payload is checked here, before dispatch. Handlers still scope themselves
// (defense in depth, and they cover non-WS callers) — this makes a *newly
// added* action safe by default instead of only when its author remembers.

// scopedActionRefs are the payload fields that name a per-user resource.
//
// All three names must stay in sync with what handlers actually send. The
// user-shell actions key off task_environment_id and treat task_id as
// optional, so checking only task_id let `user_shell.stop` tear down another
// user's terminal by naming their environment with task_id empty. An action
// that invents a fourth name for the same kind of resource silently opts out
// of this backstop — see AGENTS.md.
type scopedActionRefs struct {
	TaskID            string `json:"task_id"`
	SessionID         string `json:"session_id"`
	TaskEnvironmentID string `json:"task_environment_id"`
}

// authorizeAction reports why a dispatched action must be refused, or nil when
// the caller may proceed.
//
// No-op for connections without a real identity: identity-less internal
// clients and the synthetic identity injected while auth is disabled keep the
// pre-auth see-everything behavior, exactly as callerScope does server-side.
func (c *Client) authorizeAction(ctx context.Context, payload json.RawMessage) error {
	if c.identity.UserID == "" || c.identity.Synthetic {
		return nil
	}
	refs, ok := parseScopedActionRefs(payload)
	if !ok {
		return nil
	}
	policy := c.hub.authPolicy
	if refs.SessionID != "" && policy.Subscriptions.Session != nil {
		if err := policy.Subscriptions.Session(ctx, refs.SessionID); err != nil {
			return err
		}
	}
	if refs.TaskID != "" && policy.Subscriptions.Task != nil {
		if err := policy.Subscriptions.Task(ctx, refs.TaskID); err != nil {
			return err
		}
	}
	if refs.TaskEnvironmentID != "" && policy.ActionEnvironment != nil {
		if err := policy.ActionEnvironment(ctx, refs.TaskEnvironmentID); err != nil {
			return err
		}
	}
	return nil
}

// parseScopedActionRefs pulls the scoping IDs out of a payload. ok=false when
// the payload carries neither (or is not a JSON object at all), which means
// there is nothing to authorize.
func parseScopedActionRefs(payload json.RawMessage) (scopedActionRefs, bool) {
	if len(payload) == 0 {
		return scopedActionRefs{}, false
	}
	var refs scopedActionRefs
	if err := json.Unmarshal(payload, &refs); err != nil {
		// Non-object payloads (arrays, bare values) name no resource.
		return scopedActionRefs{}, false
	}
	return refs, refs.TaskID != "" || refs.SessionID != "" || refs.TaskEnvironmentID != ""
}

// identityIsScoped reports whether this client's identity participates in
// per-user scoping. Exposed for tests and for symmetry with callerScope.
func identityIsScoped(identity authn.Identity) bool {
	return identity.UserID != "" && !identity.Synthetic
}
