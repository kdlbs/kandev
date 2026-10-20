package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/deliveryretention"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.1 AC-PLATFORM-DELIVERY-RETENTION-005.5 AC-PLATFORM-DELIVERY-RETENTION-005.6
func TestDeliveryRetentionProtectedSweepRepository(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "protected-sweep", 8, 1)
			// Pending consumers protect raw bodies; the unsettled terminal pins receipts.
			if _, err := repo.db.Exec(`UPDATE agent_delivery_effects SET state = 'pending', completed_at = NULL`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET payload = ?, payload_pruned_at = ? WHERE sequence > 5`), []byte{}, repo.nowUTC()); err != nil {
				t.Fatal(err)
			}
			now := repo.nowUTC()
			timed := &protectedSweepRepository{Repository: repo, now: &now}
			timers := make(chan time.Duration, 1)
			snapshots := make(chan deliveryretention.Snapshot, 32)
			worker := deliveryretention.New(timed, db.NewPool(repo.db, repo.db), deliveryretention.Options{
				BatchRows: 2, Now: func() time.Time { return now },
				After:      func(d time.Duration) <-chan time.Time { timers <- d; return make(chan time.Time) },
				OnSnapshot: func(s deliveryretention.Snapshot) { snapshots <- s },
			})
			if err := worker.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(worker.Stop)
			select {
			case delay := <-timers:
				worker.Stop()
				if delay != time.Minute {
					t.Fatalf("protected sweep must reach idle, got retry %s after %d visits", delay, timed.calls)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("protected sweep did not reach idle")
			}
			if timed.calls > 50 {
				t.Fatalf("protected sweep revisited inventory %d times", timed.calls)
			}
			for len(snapshots) > 0 {
				s := <-snapshots
				if s.EventPayloadsPruned+s.PromptPayloadsPruned+s.EventRowsRetired != 0 || s.FailedBatches != 0 {
					t.Fatalf("protected data changed or cleanup failed: %+v", s)
				}
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", events[0].StreamID, 8)
			verifyProtectedSweepRecheck(t, repo, events[0], worker, timers, snapshots)
		})
	}
}

type protectedSweepRepository struct {
	*Repository
	now   *time.Time
	calls int
}

func (r *protectedSweepRepository) RunAgentDeliveryRetentionBatch(ctx context.Context, o models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	r.calls++
	if r.calls > 100 {
		return models.AgentDeliveryRetentionBatchResult{}, fmt.Errorf("protected sweep never exhausted")
	}
	result, err := r.Repository.RunAgentDeliveryRetentionBatch(ctx, o)
	*r.now = r.now.Add(600 * time.Millisecond)
	return result, err
}

// @covers AC-PLATFORM-DELIVERY-RETENTION-006.3 AC-PLATFORM-DELIVERY-RETENTION-006.6
func TestDeliveryRetentionDiscoveryVectorCommit(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "discovery-vector", 3, 1)
			seedProtectedSweepPrompt(t, repo, events[0])
			if _, err := repo.db.Exec(`UPDATE agent_delivery_effects SET state = 'pending', completed_at = NULL`); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("rollback discovery vector")
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "after_progress" {
					return injected
				}
				return nil
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1})
			if !errors.Is(err, injected) || result != (models.AgentDeliveryRetentionBatchResult{}) {
				t.Fatalf("rollback published progress: %+v %v", result, err)
			}
			repo.agentDeliveryRetentionFailpoint = nil
			before := readDiscoveryVector(t, repo)
			result, err = repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1})
			if err != nil || result.DiscoveryBefore != before || result.DiscoveryAfter != readDiscoveryVector(t, repo) || result.DiscoveryAfter.SubmissionID != "protected-prompt" {
				t.Fatalf("committed vector mismatch: %+v %v", result, err)
			}
			reopened := NewWithInitializedDB(repo.db, repo.ro, repo.log)
			t.Cleanup(func() {
				if err := reopened.Close(); err != nil {
					t.Error(err)
				}
			})
			reopened.clockNow = repo.clockNow
			before = result.DiscoveryAfter
			result, err = reopened.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1})
			if err != nil || result.DiscoveryBefore != before || result.DiscoveryAfter != readDiscoveryVector(t, reopened) || !result.DiscoveryAfter.CompactionValid || result.DiscoveryAfter.CompactionSequence != 1 {
				t.Fatalf("reopen lost progress: %+v %v", result, err)
			}
			if _, err := repo.db.Exec(`DELETE FROM agent_delivery_submissions`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.db.Exec(`UPDATE agent_delivery_retention_state SET last_stream_id = 'missing', last_sequence = 9, last_submission_id = 'missing'`); err != nil {
				t.Fatal(err)
			}
			result, err = reopened.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.DiscoveryAfter.ReceiptStreamID != "" || result.DiscoveryAfter.SubmissionID != "" || result.DiscoveryAfter != readDiscoveryVector(t, reopened) {
				t.Fatalf("empty wrap did not match committed reset: %+v %v", result, err)
			}
		})
	}
}

func readDiscoveryVector(t *testing.T, repo *Repository) models.AgentDeliveryRetentionDiscovery {
	t.Helper()
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	progress, err := loadAgentDeliveryRetentionProgress(t.Context(), tx, repo.db.Rebind)
	if err != nil {
		t.Fatal(err)
	}
	return agentDeliveryDiscoveryVector(progress)
}

func verifyProtectedSweepRecheck(t *testing.T, repo *Repository, event *models.AgentDeliveryEvent, worker *deliveryretention.Worker, timers <-chan time.Duration, snapshots <-chan deliveryretention.Snapshot) {
	t.Helper()
	// A lower-key raw row becomes eligible after idle. Restart the same owner.
	seedProtectedSweepPrompt(t, repo, event)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_effects SET state = 'completed', completed_at = ? WHERE sequence = 3`), repo.nowUTC()); err != nil {
		t.Fatal(err)
	}
	pruned := 0
	for cycle := 0; cycle < 3 && pruned == 0; cycle++ {
		if err := worker.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		select {
		case delay := <-timers:
			if delay != time.Minute {
				t.Fatalf("mixed sweep failed: %s", delay)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("mixed sweep did not reach idle")
		}
		worker.Stop()
		for len(snapshots) > 0 {
			s := <-snapshots
			pruned += s.EventPayloadsPruned
			if s.FailedBatches != 0 {
				t.Fatalf("mixed sweep failure: %+v", s)
			}
		}
	}
	if pruned != 1 {
		t.Fatalf("newly eligible lower-key row was not pruned: %d", pruned)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_submissions", "id = ?", "protected-prompt", 1)
	var terminalBytes int
	if err := repo.db.QueryRow(`SELECT LENGTH(payload) FROM agent_delivery_inbox WHERE sequence = 1`).Scan(&terminalBytes); err != nil || terminalBytes != len(event.Payload) {
		t.Fatalf("protected terminal changed: %d %v", terminalBytes, err)
	}
}

func seedProtectedSweepPrompt(t *testing.T, repo *Repository, event *models.AgentDeliveryEvent) {
	t.Helper()
	_, err := repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
		ID: "protected-prompt", SessionID: event.SessionID, IncarnationID: event.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1,
		PayloadHash: "protected-hash", Payload: []byte("protected prompt"), State: models.DeliverySubmissionDispatching,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Legacy settlement time alone cannot authorize pruning without terminal proofs.
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_submissions SET fully_settled_at = ?`), repo.nowUTC().Add(-26*time.Hour)); err != nil {
		t.Fatal(err)
	}
}
