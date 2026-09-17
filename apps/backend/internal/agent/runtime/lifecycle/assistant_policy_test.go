package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestAssistantReadOnlyLifecycleAdmission(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceAssistantBroker, nil, nil)
	req := LaunchRequest{ExecutorType: "local", McpProfile: &profile}
	require.NoError(t, validateAssistantLaunch(&req))
	for _, mutate := range []func(*LaunchRequest){
		func(r *LaunchRequest) { r.ExecutorType = "ssh" },
		func(r *LaunchRequest) { r.RepositoryID = "repo" },
		func(r *LaunchRequest) { r.SetupScript = "mutate" },
		func(r *LaunchRequest) { r.IsPassthrough = true },
	} {
		copy := req
		mutate(&copy)
		require.Error(t, validateAssistantLaunch(&copy))
	}
	info := &AgentProfileInfo{}
	require.NoError(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), "0.75.1", nil, nil))
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), "0.75.2", nil, nil))
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), "0.75.1", []string{"--tools=Bash"}, nil))
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), "0.75.1", nil, []string{"sh"}))
}
