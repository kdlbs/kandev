package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresQueueRun_SelfTriggerAllowanceHoldsUnderConcurrency pins
// AC-OFFICE-LAUNCH-SAFETY-003.8: the self-trigger window count, the
// causation-depth check, the idempotency check, and the insert must be
// serialized against every other concurrent enqueue for the same agent
// profile. Before BeginEnqueueTx's per-agent-profile
// pg_advisory_xact_lock, N concurrent self-triggered QueueRun calls each
// read the self-trigger count on their own connection before any of them
// committed a row, so all N observed "0 so far" and all N were admitted —
// the exact race this test drives against a real multi-connection
// PostgreSQL backend, which SQLite's single-writer pool cannot exercise.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresQueueRun_SelfTriggerAllowanceHoldsUnderConcurrency(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	const agentProfileID = "agent-self-race"
	seedAgentProfile(t, db, agentProfileID)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eb := bus.NewMemoryEventBus(log)
	svc := runsservice.New(officeRepo.RunsRepository(), eb, log, nil)
	svc.SetLaunchSafetyLimits(runsservice.DefaultMaxCausationDepth, 1)

	const concurrency = 8
	var (
		start   sync.WaitGroup
		wg      sync.WaitGroup
		mu      sync.Mutex
		queued  int
		refused int
	)
	start.Add(1)
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			start.Wait()
			outcome, err := svc.QueueRun(context.Background(), runsservice.QueueRunRequest{
				AgentProfileID: agentProfileID,
				Reason:         "self_trigger_reason",
				ActorKind:      models.ActorKindAgent,
				ActorID:        agentProfileID,
				IdempotencyKey: fmt.Sprintf("task_comment:self-race-%d", i),
			})
			var refusal *runsservice.RefusalError
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && outcome == runsservice.QueueOutcomeQueued:
				queued++
			case errors.As(err, &refusal) && refusal.Gate == runsservice.RefusalSelfTrigger:
				refused++
			case err != nil:
				t.Errorf("queue self-trigger %d: unexpected error: %v", i, err)
			default:
				t.Errorf("queue self-trigger %d: unexpected outcome %q with no error", i, outcome)
			}
		}(i)
	}
	start.Done()
	wg.Wait()

	if queued != 1 {
		t.Fatalf("queued = %d, want 1 (allowance=1, requests=%d, refused=%d)", queued, concurrency, refused)
	}
	if refused != concurrency-1 {
		t.Fatalf("refused = %d, want %d (allowance=1, requests=%d, queued=%d)", refused, concurrency-1, concurrency, queued)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`
		SELECT COUNT(*) FROM runs
		WHERE agent_profile_id = ? AND reason = ? AND actor_kind = ? AND actor_id = ?
	`), agentProfileID, "self_trigger_reason", string(models.ActorKindAgent), agentProfileID); err != nil {
		t.Fatalf("count persisted runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted self-triggered runs = %d, want 1", count)
	}
}
