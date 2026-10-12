package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionRejectsLateWritesAfterPurge(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			session := createRetentionSession(t, repo, ctx, "late-task", "late-session", false, "")
			seedRetentionDeliveryEvidence(t, repo, ctx, session)
			if err := repo.DeleteTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			purgeDeliveryRetirementJobs(t, repo)
			assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", session.ID, 0)
			check := func() {
				t.Helper()
				prepared, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
					ID: "late-prompt", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
					HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "late-hash", Payload: []byte("late prompt"),
				})
				if prepared || !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
					t.Errorf("late prepare = %v, %v; want identity refusal", prepared, err)
				}
				inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{
					SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
					StreamID: "late-stream", Sequence: 1, EventType: "assistant_message", Payload: []byte("late output"),
				}, 1)
				if inserted || !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
					t.Errorf("late receive = %v, %v; want identity refusal", inserted, err)
				}
				assertDeliveryRetirementEvidencePurged(t, repo, session.ID)
			}
			check()
			replacement := &models.TaskSession{ID: session.ID, TaskID: session.TaskID}
			if err := repo.CreateTaskSession(ctx, replacement); err != nil {
				t.Fatal(err)
			}
			check()
			seedRetentionDeliveryEvidence(t, repo, ctx, replacement)
			assertRetentionDeliveryEvidence(t, repo, replacement.ID)
		})
	}
}
