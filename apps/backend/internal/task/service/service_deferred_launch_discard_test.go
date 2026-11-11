package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type deferredLaunchDiscardProbe struct {
	calls int
}

func (p *deferredLaunchDiscardProbe) DiscardDeferredCeilingLaunch(
	context.Context, string,
) (models.DeferredLaunchDiscardResult, error) {
	p.calls++
	return models.DeferredLaunchDiscardResult{Outcome: models.DeferredLaunchDiscardDiscarded}, nil
}

func TestDiscardDeferredLaunchRejectsUnauthorizedTaskBeforeDelegating(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	probe := &deferredLaunchDiscardProbe{}
	svc.SetDeferredLaunchDiscarder(probe)

	result, err := svc.DiscardDeferredLaunch(ctxAs("user-a"), "task-b")

	require.ErrorIs(t, err, repoerrors.ErrTaskNotFound)
	require.Zero(t, probe.calls, "an unauthorized caller must not reach the settlement delegate")
	require.Empty(t, result.Outcome)
}
