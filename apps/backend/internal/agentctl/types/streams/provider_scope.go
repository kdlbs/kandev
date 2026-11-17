package streams

import (
	"context"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

type providerScopeContextKey struct{}

// WithProviderMCPScope attaches helper-owned scope identity. MCP tool arguments
// cannot populate this context value.
func WithProviderMCPScope(ctx context.Context, scope p.ProviderMCPScope) context.Context {
	return context.WithValue(ctx, providerScopeContextKey{}, scope)
}

func ProviderMCPScopeFromContext(ctx context.Context) (p.ProviderMCPScope, bool) {
	value, ok := ctx.Value(providerScopeContextKey{}).(p.ProviderMCPScope)
	return value, ok
}
