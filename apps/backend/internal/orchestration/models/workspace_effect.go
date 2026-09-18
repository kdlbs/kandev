package models

import "context"

type workspaceEffectKey struct{}

// The server installs this guard after resolving an explicit linked target.
// Native adapters invoke it immediately before their first external mutation.
func WithWorkspaceEffectGuard(ctx context.Context, guard func(context.Context) error) context.Context {
	return context.WithValue(ctx, workspaceEffectKey{}, guard)
}

func CheckWorkspaceEffect(ctx context.Context) error {
	if guard, ok := ctx.Value(workspaceEffectKey{}).(func(context.Context) error); ok {
		return guard(ctx)
	}
	return nil
}
