package handlers

import (
	"context"
	"errors"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
)

type officeRuntimeMessageAuthorityKey struct{}

type officeRuntimeMessageAuthority struct {
	taskID, sessionID string
}

// WithOfficeRuntimeMessageAuthority is installed only by the signed Office
// runtime gateway after checking its live run and source task/session. It does
// not change the real principal or advertise operation tools to Office MCP.
func WithOfficeRuntimeMessageAuthority(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.Surface != mcpprofile.SurfaceOfficeTask ||
		principal.CallerTaskID != taskID || principal.CallerSessionID != sessionID {
		return nil, errors.New("office runtime message authority requires its actual task session")
	}
	return context.WithValue(ctx, officeRuntimeMessageAuthorityKey{}, officeRuntimeMessageAuthority{taskID, sessionID}), nil
}

func officeRuntimeMessageAuthorized(ctx context.Context, principal mcpscope.Principal) bool {
	authority, ok := ctx.Value(officeRuntimeMessageAuthorityKey{}).(officeRuntimeMessageAuthority)
	return ok && principal.Surface == mcpprofile.SurfaceOfficeTask &&
		authority.taskID == principal.CallerTaskID && authority.sessionID == principal.CallerSessionID
}

type sendOperationReplayOnlyKey struct{}

// WithSendOperationReplayOnly marks an identified send as a pure replay read:
// a retained record is returned as usual, and a missing one is answered with
// SendOperationNotRetainedCode instead of reaching target lookup or dispatch.
// The Office gateway uses it to recover a receipt before re-checking a target
// that may since have been deleted or lost its prompt grant.
func WithSendOperationReplayOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, sendOperationReplayOnlyKey{}, true)
}

func sendOperationReplayOnly(ctx context.Context) bool {
	replayOnly, _ := ctx.Value(sendOperationReplayOnlyKey{}).(bool)
	return replayOnly
}
