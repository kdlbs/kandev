package sqlite

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestIncrementalShellIdentityChangeRequiresFreshCheckpoint(t *testing.T) {
	for _, checkpoint := range []bool{false, true} {
		t.Run(strconv.FormatBool(checkpoint), func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			seedIncrementalShellMessage(t, repo)
			patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "before"}, "first", true)
			require.NoError(t, err)
			first := incrementalShellTestEvent(t, 1, patch)
			_, err = repo.ReceiveAgentDeliveryEvent(ctx, first, 1)
			require.NoError(t, err)
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, first, completedRetentionEventEffect(first))
			require.NoError(t, err)
			base, err := streams.ApplyShellOutputPatch(nil, patch)
			require.NoError(t, err)
			epoch := "first"
			if checkpoint {
				base, epoch = nil, "successor"
			}
			patch, err = streams.EncodeShellOutputPatch(base, streams.ShellExecOutput{Stdout: "after"}, epoch, checkpoint)
			require.NoError(t, err)
			next := incrementalShellTestEvent(t, 2, patch)
			var agent streams.AgentEvent
			require.NoError(t, json.Unmarshal(next.Payload, &agent))
			agent.PromptGeneration = 2
			next.Payload, err = json.Marshal(agent)
			require.NoError(t, err)
			_, err = repo.ReceiveAgentDeliveryEvent(ctx, next, 2)
			require.NoError(t, err)
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, next, completedRetentionEventEffect(next))
			expected := "before"
			if checkpoint {
				require.NoError(t, err, "a fresh checkpoint must replace the predecessor's identity and chunks")
				expected = "after"
			} else {
				require.ErrorIs(t, err, streams.ErrShellOutputBaseline)
			}
			message, err := repo.GetMessage(ctx, "shell-message")
			require.NoError(t, err)
			require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
			output, ok := models.ExtractShellExecOutput(message.Metadata)
			require.True(t, ok)
			require.Equal(t, expected, output.Stdout)
		})
	}
}
