package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionOrphansBoundedLiveTraversal(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx := context.Background()
			for i := 0; i < 3; i++ {
				session := createRetentionSession(t, repo, ctx, fmt.Sprintf("scan-task-%d", i), fmt.Sprintf("scan-session-%d", i), false, "")
				seedRetentionDeliveryEvidence(t, repo, ctx, session)
				if i == 2 {
					if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), session.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			for i := 0; i < 3; i++ {
				result, err := repo.RunAgentDeliveryOrphanBatch(ctx, models.AgentDeliveryOrphanBatchOptions{MaxCandidates: 1})
				if err != nil {
					t.Fatal(err)
				}
				wantJobs := 0
				if i == 2 {
					wantJobs = 1
				}
				if result.Candidates != 1 || result.JobsScheduled != wantJobs {
					t.Fatalf("pass %d = %+v, want one inspected identity and %d jobs", i, result, wantJobs)
				}
				var cursor string
				if err := repo.db.Get(&cursor, `SELECT last_session_id FROM agent_delivery_retention_state WHERE id = 1`); err != nil {
					t.Fatal(err)
				}
				if want := fmt.Sprintf("scan-session-%d", i); cursor != want {
					t.Fatalf("orphan cursor %q, want %q", cursor, want)
				}
				if _, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{}); err != nil {
					t.Fatal(err)
				}
				var after string
				if err := repo.db.Get(&after, `SELECT last_session_id FROM agent_delivery_retention_state WHERE id = 1`); err != nil {
					t.Fatal(err)
				}
				if after != cursor {
					t.Fatalf("event sweep overwrote orphan cursor: %q -> %q", cursor, after)
				}
			}
			result, err := repo.RunAgentDeliveryOrphanBatch(ctx, models.AgentDeliveryOrphanBatchOptions{MaxCandidates: 1})
			if err != nil || result.JobsScheduled != 0 || result.Candidates != 1 {
				t.Fatalf("wrapped scan = %+v, %v", result, err)
			}
		})
	}
}
