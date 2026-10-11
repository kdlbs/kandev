package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestDeliveryRetentionSerialization(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task schema: %v", err)
	}
	ctx := context.Background()
	const sessionID = "delivery-retention-serialization-session"
	seedAgentDeliveryTestSession(t, repo, sessionID, "delivery-retention-serialization-incarnation")

	blocker, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin blocker transaction: %v", err)
	}
	defer func() { _ = blocker.Rollback() }()
	if _, err := blocker.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"agent-delivery-session:"+sessionID,
	); err != nil {
		t.Fatalf("hold delivery session lock: %v", err)
	}

	type result struct {
		inserted bool
		err      error
	}
	started := make(chan struct{})
	completed := make(chan result, 1)
	go func() {
		close(started)
		inserted, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
			ID: "delivery-retention-serialization-submission", SessionID: sessionID,
			IncarnationID:     "delivery-retention-serialization-incarnation",
			HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "serialization-hash",
			Payload: []byte("retained prompt"), State: models.DeliverySubmissionPrepared,
		})
		completed <- result{inserted: inserted, err: err}
	}()
	<-started
	waitForPostgresLockWait(t, ctx, repo, "pg_advisory_xact_lock")

	var submissionExists bool
	if err := db.Get(&submissionExists, `SELECT EXISTS (
		SELECT 1 FROM agent_delivery_submissions
		WHERE id = 'delivery-retention-serialization-submission')`); err != nil {
		t.Fatalf("inspect blocked submission: %v", err)
	}
	if submissionExists {
		t.Fatal("submission committed while its delivery session lock was held")
	}

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release delivery session lock: %v", err)
	}
	select {
	case got := <-completed:
		if got.err != nil || !got.inserted {
			t.Fatalf("submission after lock release = inserted %v, err %v", got.inserted, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("submission did not resume after delivery session lock release")
	}
}
