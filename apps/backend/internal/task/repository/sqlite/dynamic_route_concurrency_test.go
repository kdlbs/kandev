package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
)

// TestConcurrentResumePendingClaimsExactlyOneGeneration reproduces the
// duplicate-launch race: two callers that both observe a due retry_wait state
// at the same generation must not both receive a decision. The durable claim
// predicate is the authority, so exactly one caller succeeds and the other
// gets ErrStaleGeneration.
func TestConcurrentResumePendingClaimsExactlyOneGeneration(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-race", Title: "Race"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-race", TaskID: "task-race", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	elapsedDeadline := time.Now().UTC().Add(-time.Minute)
	state := dynamicruntime.RouteState{
		SessionID: "session-race", LogicalProfileID: "dynamic-1",
		ExecutionProfileID: "concrete-1", Generation: 4, ProfileVersion: 1,
		Status:          "retry_wait",
		PolicyStateJSON: `{"deadline":"` + elapsedDeadline.Format(time.RFC3339Nano) + `"}`,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.SaveRouteState(ctx, state); err != nil {
		t.Fatalf("SaveRouteState: %v", err)
	}

	const callers = 2
	engines := make([]*dynamicruntime.Engine, callers)
	for i := range engines {
		engines[i] = dynamicruntime.NewEngine(
			dynamicruntime.WithPersistence(repo),
			dynamicruntime.WithStateLoader(repo),
		)
		if _, _, err := engines[i].LoadState(ctx, "session-race"); err != nil {
			t.Fatalf("warm engine cache %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	decisions := make([]dynamicruntime.RouteDecision, callers)
	errs := make([]error, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			decisions[i], errs[i] = engines[i].ResumePending(ctx, "session-race", 4)
		}(i)
	}
	wg.Wait()

	successes := 0
	staleCount := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, dynamicruntime.ErrStaleGeneration) || errors.Is(err, dynamicruntime.ErrRecoveryPending) {
			staleCount++
			continue
		}
		t.Fatalf("unexpected ResumePending error: %v", err)
	}
	if successes != 1 {
		t.Fatalf("successful claims = %d, errors = %v (want exactly 1 success)", successes, errs)
	}
	if staleCount != callers-1 {
		t.Fatalf("stale/rejected claims = %d, want %d", staleCount, callers-1)
	}
}
