package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func TestGenericWrapperPassthroughBuilders(t *testing.T) {
	for _, agentName := range []string{"pi-acp", "claude-acp"} {
		for _, path := range []string{"initial", "fresh", "resume"} {
			t.Run(agentName+"/"+path, func(t *testing.T) {
				manager, execution, profile := newPassthroughMCPTestManager(t, agentName)
				profile.CommandPrefix = `independent-wrapper "{{kandev.work_dir}}"`
				manager.profileResolver = &mockPassthroughProfileResolver{profile: profile}
				var cmd passthroughCommand
				var err error
				switch path {
				case "initial":
					_, _, _, cmd, err = manager.passthroughAgentCommand(context.Background(), execution, profile)
				case "fresh":
					_, _, cmd, err = manager.freshPassthroughCommand(context.Background(), execution)
				case "resume":
					var resolved *resolvedPassthrough
					resolved, err = manager.resolvePassthroughAgent(context.Background(), execution)
					require.NoError(t, err)
					cmd, err = manager.resumePassthroughCommand(context.Background(), execution, resolved, true)
				}
				require.NoError(t, err)
				require.NotContains(t, cmd.Args(), "independent-wrapper")
				execution.LaunchWrapper = launchwrapper.Profile([]string{"different-wrapper"})
				request := buildInteractiveStartRequest(execution.SessionID, execution, agents.PassthroughConfig{}, nil, cmd, nil, true)
				require.Equal(t, []string{"independent-wrapper", "{{kandev.work_dir}}"}, request.LaunchWrapper.Argv)
			})
		}
	}
}
