package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"testing"

	builtinintent "github.com/kandev/kandev/internal/mcp/intent"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/stretchr/testify/require"
)

func TestMessageOrchestratorWrapperCarriesBuiltinAdmission(t *testing.T) {
	wrapper := &orchestratorWrapper{svc: &orchestrator.Service{}}
	preparer, ok := any(wrapper).(interface {
		PrepareBuiltinIntent(context.Context, string, string, string) (context.Context, error)
		PrepareTaskSessionMCPPrompt(context.Context, string, string) (context.Context, error)
	})
	require.True(t, ok, "the production message adapter must expose built-in admission and prompt preparation")
	ctx := context.Background()
	prepared, err := preparer.PrepareBuiltinIntent(ctx, "task", "session", "")
	require.NoError(t, err)
	require.Equal(t, ctx, prepared)
	_, err = preparer.PrepareBuiltinIntent(ctx, "task", "session", "invalid")
	require.ErrorIs(t, err, builtinintent.ErrInvalid)
}

func TestLifecycleAdapterCarriesBuiltinWorkspaceAdmission(t *testing.T) {
	adapter := &lifecycleAdapter{mgr: &lifecycle.Manager{}}
	_, ok := any(adapter).(interface {
		PrepareBuiltinWorkspace(context.Context, string, p.Context) error
	})
	require.True(t, ok, "prepared-workspace starts must reach lifecycle binding admission")
}
