package usagehistory

import "context"

type workspaceAccessKey struct{}

// WithWorkspaceAccess carries the workspace reach resolved by the existing
// task service. It is set by the Host, never decoded from a plugin payload.
func WithWorkspaceAccess(ctx context.Context, ids []string) context.Context {
	allowed := make(map[string]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}
	return context.WithValue(ctx, workspaceAccessKey{}, allowed)
}

func WorkspaceAllowed(ctx context.Context, id string) bool {
	allowed, scoped := ctx.Value(workspaceAccessKey{}).(map[string]bool)
	return !scoped || allowed[id]
}
