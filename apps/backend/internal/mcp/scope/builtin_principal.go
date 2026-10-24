package scope

import (
	"context"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// BuiltinPrincipalMatches prevents retained bindings from following a task into
// another workspace or a different session surface.
func BuiltinPrincipalMatches(ctx context.Context, manifest *p.BuiltinManifest) bool {
	principal, ok := PrincipalFromContext(ctx)
	return ok && manifest != nil && principal.WorkspaceID == manifest.WorkspaceID && principal.CallerTaskID == manifest.TaskID && principal.Surface == manifest.Surface
}
