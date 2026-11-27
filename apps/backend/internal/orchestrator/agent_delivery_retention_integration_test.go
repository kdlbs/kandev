package orchestrator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-003.1 AC-PLATFORM-DELIVERY-RETENTION-003.3
func TestDeliveryRetentionIntegrationTerminalFirstRPC(t *testing.T) {
	fixture := newDeliveryWorkflowOutcomeFixture(t, "pruned-terminal-first", "")
	fixture.run(t, "not_required", turnCompletionCauseAgentTurn)
	old := time.Now().UTC().Add(-48 * time.Hour)
	effect := &models.AgentDeliveryEffect{
		EffectKey: "agent_delivery.event:" + fixture.effect.StreamID + ":1",
		StreamID:  fixture.effect.StreamID, Sequence: 1, EffectType: "agent_delivery.event",
		State: models.DeliveryEffectCompleted, CreatedAt: old, CompletedAt: &old,
	}
	if _, err := fixture.repo.PutAgentDeliveryEffect(fixture.ctx, effect); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.DB().ExecContext(fixture.ctx,
		`UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ?`, old, fixture.effect.StreamID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.DB().ExecContext(fixture.ctx,
		`UPDATE agent_delivery_submissions SET fully_settled_at = ? WHERE id = ?`, old, fixture.effect.SubmissionID); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.repo.RunAgentDeliveryRetentionBatch(fixture.ctx, models.AgentDeliveryRetentionBatchOptions{})
	if err != nil || result.EventRowsRetired != 1 || result.PromptPayloadsPruned != 1 {
		t.Fatalf("actual terminal/prompt cleanup: %+v %v", result, err)
	}
	before, err := fixture.repo.GetAgentDeliverySubmissionMetadata(fixture.ctx, fixture.effect.SubmissionID)
	if err != nil || before.PayloadPrunedAt == nil {
		t.Fatalf("compact prompt: %+v %v", before, err)
	}
	runtime := &agentDeliverySubmissionRuntime{store: fixture.repo, id: before.ID, sessionID: before.SessionID}
	if err := runtime.markCompleted(fixture.ctx); err != nil {
		t.Fatalf("late prompt RPC completion after cleanup: %v", err)
	}
	if !runtime.completed {
		t.Fatal("late RPC did not observe the committed terminal outcome")
	}
	if err := fixture.service.reconcileAgentDeliverySettlements(fixture.ctx, before.SessionID); err != nil {
		t.Fatal(err)
	}
	after, err := fixture.repo.GetAgentDeliverySubmissionMetadata(fixture.ctx, before.ID)
	if err != nil || after.State != before.State || after.Outcome != before.Outcome || after.PayloadHash != before.PayloadHash || after.FullySettledAt == nil || !after.FullySettledAt.Equal(*before.FullySettledAt) {
		t.Fatalf("late completion changed settled compact evidence: %+v %v", after, err)
	}
	workflow, err := fixture.repo.GetAgentDeliveryEffect(fixture.ctx, fixture.effect.EffectKey)
	if err != nil || workflow.Outcome != "not_required" || workflow.State != models.DeliveryEffectCompleted {
		t.Fatalf("retained workflow receipt: %+v %v", workflow, err)
	}
}
