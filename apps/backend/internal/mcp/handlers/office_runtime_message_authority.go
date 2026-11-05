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
