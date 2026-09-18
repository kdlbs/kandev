package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenanceInterruptedReceipt(t *testing.T) {
	s, b, candidate, sandbox, manager := maintenanceFixture(t)
	base, _ := prepareMaintenanceFixture(t, s, b, candidate)
	ctx := context.Background()
	operation, _, err := s.Repo.BeginOperation(ctx, models.Operation{BindingID: b.ID, ConversationID: b.ConversationID, BindingVersion: b.Version, OperationID: "lost-commit", RunID: "interrupted", Target: base + "/maintenance", RequestHash: "native"})
	require.NoError(t, err)
	require.NoError(t, s.Repo.DispatchOperation(ctx, operation))
	require.NoError(t, s.RecoverInterrupted(ctx))
	current, err := s.Repo.ImprovementCandidate(ctx, b.ID, candidate.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", current.State)
	require.EqualValues(t, 1, manager.creates.Load())
	require.Zero(t, sandbox.commits)
	sandbox.artifact = &models.MaintenanceReviewArtifact{MaintenanceArtifact: models.MaintenanceArtifact{BaseOID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TreeOID: "tree", CommitOID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, Patch: "synthetic patch", Validation: &models.MaintenanceValidation{TreeOID: "tree", GrantRevision: 1, Passed: true, Checks: []models.MaintenanceCheck{{Kind: "positive"}, {Kind: "negative"}}}}
	response := runtimeRequest(t, assistantRouter(s), "POST", base+"/reconcile", "", "", map[string]any{"expected_binding_version": b.Version, "expected_revision": current.Revision})
	require.Equal(t, 200, response.Code, response.Body.String())
	current, err = s.Repo.ImprovementCandidate(ctx, b.ID, candidate.ID)
	require.NoError(t, err)
	require.Equal(t, "prepared", current.State)
	require.Equal(t, sandbox.artifact.CommitOID, current.CommitOID)
	require.Nil(t, current.ResolvedAt)
	require.Zero(t, sandbox.commits, "recovery records an existing artifact without repeating the commit")
}

func TestAssistantFrictionRetentionDuringReconciliation(t *testing.T) {
	s, _, b, _ := assistantAttentionFixture(t)
	now := time.Now().UTC()
	s.Now = func() time.Time { return now }
	row := models.Friction{TaskID: "worker", SessionID: "session", OccurrenceID: "old", ProfileID: "profile", AccountRevision: "account", Origin: "native", Operation: "question", Reason: "pending_approval", PolicyVersion: "unknown", Outcome: "blocked"}
	ctx := context.Background()
	require.NoError(t, s.Repo.RecordFriction(ctx, b, row, now.Add(-31*24*time.Hour)))
	row.WorkspaceID = b.WorkspaceID
	before, err := s.Repo.ImprovementEvidence(ctx, b.ID, row.ScopeFingerprint(), "", 10)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.NoError(t, s.ReconcileAttention(ctx))
	after, err := s.Repo.ImprovementEvidence(ctx, b.ID, row.ScopeFingerprint(), "", 10)
	require.NoError(t, err)
	require.Empty(t, after)
}
