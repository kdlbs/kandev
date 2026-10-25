package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionAuthenticatedRetirementOrder(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			session := createRetentionBenchmarkSession(t, repo)
			prepareRetentionBenchmarkOwner(t, repo)
			now := time.Now().UTC()
			if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cleanup_jobs
			 (session_id, incarnation_id, retired_at, phase, created_at, updated_at)
			 VALUES (?, ?, ?, 'pending', ?, ?)`), session.ID, session.QueueIncarnationID, now, now, now); err != nil {
				t.Fatal(err)
			}
			valid := models.AgentDeliveryEvent{SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
				HarnessGeneration: 1, StreamID: "retirement-order-stream", Sequence: 1,
				EventType: "message_chunk", Payload: []byte(`{"type":"message_chunk","text":"output"}`)}
			setRetentionBenchmarkEventOwner(&valid)
			mismatched := valid
			mismatched.ExecutionID = "replaced-execution"
			legacy := valid
			legacy.TaskID, legacy.ExecutionID, legacy.ExpectedStreamID, legacy.OwnerGeneration = "", "", "", 0
			for _, test := range []struct {
				name  string
				event models.AgentDeliveryEvent
				want  error
			}{
				{"valid-owner", valid, repoerrors.ErrAgentDeliveryIdentityRetiring},
				{"mismatched-owner", mismatched, repoerrors.ErrAgentDeliveryIdentityMismatch},
				{"legacy-owner", legacy, repoerrors.ErrAgentDeliveryIdentityRetiring},
			} {
				t.Run(test.name, func(t *testing.T) {
					if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), &test.event, 1); inserted || !errors.Is(err, test.want) {
						t.Fatalf("retiring admission: inserted=%t err=%v want=%v", inserted, err, test.want)
					}
					assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", valid.StreamID, 0)
					assertRetentionRowCount(t, repo, "agent_delivery_cursors", "stream_id = ?", valid.StreamID, 0)
				})
			}
		})
	}
}
