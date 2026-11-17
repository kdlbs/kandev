package process

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementNativeQuestionKeepsOriginScope(t *testing.T) {
	source := p.ProviderMCPScope{BindingID: "original", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 1, ProfileDigest: strings.Repeat("a", 64)}
	mgr := &Manager{cfg: &config.InstanceConfig{ProviderMCP: &config.ProviderMCPConfig{Scope: source}}}
	var observed p.ProviderMCPScope
	var scoped bool
	mgr.userInputRequestHandler = func(ctx context.Context, _ *types.UserInputRequest) (*types.UserInputResponse, error) {
		observed, scoped = streams.ProviderMCPScopeFromContext(ctx)
		return &types.UserInputResponse{}, nil
	}
	factory, ok := any(mgr).(interface {
		providerUserInputHandler() types.UserInputRequestHandler
	})
	require.True(t, ok, "native questions need generation-bound backend context")
	originalHandler := factory.providerUserInputHandler()
	next := source
	next.BindingID = "replacement"
	next.ProcessGeneration++
	mgr.cfg.ProviderMCP = &config.ProviderMCPConfig{Scope: next}
	_, err := originalHandler(context.Background(), &types.UserInputRequest{})
	require.NoError(t, err)
	require.True(t, scoped)
	require.Equal(t, source, observed, "a late question must never inherit successor authority")
}
