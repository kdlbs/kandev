package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// ownerMismatchRecoverer reports the evidenced delivery failure for every
// recovery attempt, the way lifecycle surfaces a detached turn after restart.
type ownerMismatchRecoverer struct {
	*mockAgentManager
	result lifecycle.DeliveryReconciliationResult
	calls  int
}

func (m *ownerMismatchRecoverer) RecoverAgentPromptStreamWithIdentity(
	context.Context, lifecycle.AgentDeliveryRecoveryIdentity,
) lifecycle.DeliveryReconciliationResult {
	m.calls++
	return m.result
}

func seedDetachedTurn(t *testing.T, svc *Service, fixture *silentRestoreCheckpointFixture) {
	t.Helper()
	svc.turnService = &repoTurnService{repo: fixture.repo}
	_, err := svc.turnService.StartTurn(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = fixture.repo.PrepareAgentDeliverySubmission(fixture.ctx, &models.AgentDeliverySubmission{
		ID: "prompt:detached-pass", SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "detached-pass", PayloadHash: "detached-hash",
		Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
}

func TestSilentRestorePassSettlesDetachedTurnOnEvidencedOwnerMismatch(t *testing.T) {
	service, repo, manager, session, store := newSilentRestoreEligibilityFixture(t, "provider-session", "mismatch")
	fixture := &silentRestoreCheckpointFixture{ctx: context.Background(), repo: repo, session: session}
	seedDetachedTurn(t, service, fixture)
	recoverer := &ownerMismatchRecoverer{mockAgentManager: manager, result: lifecycle.DeliveryReconciliationResult{
		Outcome: lifecycle.DeliveryReconciliationBlocked, Reason: "delivery_evidence_unavailable",
		Err: evidencedOwnerMismatchError(),
	}}
	service.agentManager = recoverer

	service.runSilentRestoreRecoveryPass(context.Background(), store)

	require.NotZero(t, recoverer.calls, "the pass must reach delivery recovery for the candidate")
	stored, err := repo.GetAgentDeliverySubmission(fixture.ctx, "prompt:detached-pass")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State,
		"the in-flight submission is settled as interrupted")
	require.Zero(t, openTurnCount(t, repo, session.ID), "the session is free for new input")

	// The candidate is re-listed every pass; settling again changes nothing.
	service.runSilentRestoreRecoveryPass(context.Background(), store)
	again, err := repo.GetAgentDeliverySubmission(fixture.ctx, "prompt:detached-pass")
	require.NoError(t, err)
	require.Equal(t, stored.State, again.State)
	require.Zero(t, openTurnCount(t, repo, session.ID))
}

func TestSilentRestorePassKeepsTransientEvidenceChangeRetrying(t *testing.T) {
	service, repo, manager, session, store := newSilentRestoreEligibilityFixture(t, "provider-session", "transient")
	fixture := &silentRestoreCheckpointFixture{ctx: context.Background(), repo: repo, session: session}
	seedDetachedTurn(t, service, fixture)
	service.agentManager = &ownerMismatchRecoverer{mockAgentManager: manager, result: lifecycle.DeliveryReconciliationResult{
		Outcome: lifecycle.DeliveryReconciliationBlocked, Reason: "delivery_evidence_unavailable",
		Err: transientStaleEvidenceError(),
	}}

	service.runSilentRestoreRecoveryPass(context.Background(), store)

	stored, err := repo.GetAgentDeliverySubmission(fixture.ctx, "prompt:detached-pass")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, stored.State, "a transient change must not settle the turn")
	require.Equal(t, 1, openTurnCount(t, repo, session.ID))
}
