package runtime

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestScheduledDeliveryReusesConversationAndSelectedAccount(t *testing.T) {
	svc, _, task := newRuntime(t)
	ctx := context.Background()
	require.Error(t, svc.Validate(ctx, "foreign", "chief"))
	conversation, err := svc.Send(ctx, "ws", "chief", "delivery", "Daily PR review")
	require.NoError(t, err)
	require.Equal(t, task, conversation)
	_, err = svc.Send(ctx, "ws", "chief", "delivery", "Daily PR review")
	require.NoError(t, err)
	comments, err := svc.Repo.ListComments(ctx, task, 10)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	require.Equal(t, "automation", comments[0].Source)
	run, err := svc.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	svc.Start = func(_ context.Context, launch Launch) error {
		require.Equal(t, "personal", launch.ProfileID)
		require.Contains(t, launch.Prompt, "Daily PR review")
		return nil
	}
	handled, err := svc.Process(ctx, run)
	require.NoError(t, err)
	require.True(t, handled)
	_, err = svc.Personas.UpdateAgentStatus(ctx, "chief", "paused", "")
	require.NoError(t, err)
	_, err = svc.Send(ctx, "ws", "chief", "next-delivery", "Daily PR review")
	require.ErrorContains(t, err, "paused")
}
