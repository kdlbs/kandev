package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
func TestDeliverySessionStatePostgresConvergesConcurrently(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	fixture := seedSettledSessionStateFixture(t, repo, "pg-concurrent")

	const callers = 4
	results := make(chan *models.AgentDeliverySessionStateResult, callers)
	errors := make(chan error, callers)
	var wg sync.WaitGroup
	for index := 0; index < callers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	changed, consistent := 0, 0
	for result := range results {
		switch result.Outcome {
		case models.AgentDeliverySessionStateChanged:
			changed++
		case models.AgentDeliverySessionStateAlreadyConsistent:
			consistent++
		default:
			t.Fatalf("concurrent correction outcome = %q", result.Outcome)
		}
	}
	require.Equal(t, 1, changed)
	require.Equal(t, callers-1, consistent)
	require.Equal(t, models.TaskSessionStateWaitingForInput, deliverySessionStateOf(t, repo, fixture.sessionID))
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestDeliverySessionStatePostgresRejectsLiveSuccessor(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	fixture := seedSettledSessionStateFixture(t, repo, "pg-successor")
	if _, err := repo.PrepareAgentDeliverySubmission(context.Background(), &models.AgentDeliverySubmission{
		ID: "submission-session-state-pg-successor-live", SessionID: fixture.sessionID,
		IncarnationID: fixture.incarnation, HarnessGeneration: 1, OwnerGeneration: 1,
		PayloadHash: "hash-pg-successor-live", Payload: []byte("next prompt"),
		State: models.DeliverySubmissionPrepared,
	}); err != nil {
		t.Fatalf("prepare successor: %v", err)
	}
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	require.NoError(t, err)
	require.Equal(t, models.AgentDeliverySessionStateSuperseded, result.Outcome)
	require.Equal(t, models.TaskSessionStateRunning, deliverySessionStateOf(t, repo, fixture.sessionID))
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestDeliverySessionStatePostgresRejectsUnattemptedSuccessorTurn(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	fixture := seedSettledSessionStateFixture(t, repo, "pg-reserved-successor")
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(context.Background(), &models.Turn{
		ID: "turn-session-state-pg-reserved-successor-next", TaskID: fixture.taskID,
		TaskSessionID: fixture.sessionID, StartedAt: now, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{models.TurnMetaKeyPromptDispatchPending: true},
	}))
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	require.NoError(t, err)
	require.Equal(t, models.AgentDeliverySessionStateSuperseded, result.Outcome)
	require.Equal(t, models.TaskSessionStateRunning, deliverySessionStateOf(t, repo, fixture.sessionID))
}
