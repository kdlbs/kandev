package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionRetirementFairJobs(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "fair-large", 20, 0)
			initializeRetentionDeletionQueue(t, repo)
			ctx := context.Background()
			session, err := repo.GetTaskSession(ctx, events[0].SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.DeleteTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			now := repo.nowUTC()
			_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cleanup_jobs
				(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
				VALUES (?, ?, ?, 'pending', ?, ?)`), "zz-small-deleted-session", "small-inc", now, now, now)
			if err != nil {
				t.Fatal(err)
			}
			repo.clockNow = func() time.Time { return now.Add(time.Second) }
			first, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 1})
			if err != nil || first.RowsPurged != 1 || first.Completed != 0 {
				t.Fatalf("first large job quantum: %+v %v", first, err)
			}
			// Recreate the repository facade to ensure scheduling uses durable job state.
			restarted := &Repository{db: repo.db, ro: repo.ro, clockNow: repo.clockNow}
			second, err := restarted.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 1})
			if err != nil || second.Completed != 1 || second.RowsPurged != 0 {
				t.Fatalf("small job must complete before large history drains: %+v %v", second, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "session_id=?", session.ID, 19)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id=?", "zz-small-deleted-session", 0)
		})
	}
}
