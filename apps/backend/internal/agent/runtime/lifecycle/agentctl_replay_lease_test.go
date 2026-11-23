package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// leaseFree reports whether no client lease is held, which is the state a
// terminal stop needs before it can take the write side.
func leaseFree(execution *AgentExecution) bool {
	if !execution.agentctlLifecycleMu.TryLock() {
		return false
	}
	execution.agentctlLifecycleMu.Unlock()
	return true
}

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.4
func TestRecoveredDeliveryReplayReleasesLeaseBeforeLifecycleCallbacks(t *testing.T) {
	mgr := newTestManager(t)
	execution, repository, _, _ := newRecoveredReplayLeaseFixture(t, mgr)
	callbacks := 0
	leaseFreeInCallback := true
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {
		callbacks++
		leaseFreeInCallback = leaseFreeInCallback && leaseFree(execution)
		mgr.handleAgentEvent(execution, event)
	}}, nil, nil)
	t.Cleanup(sm.Wait)
	// The ACK worker takes its own lease, which is outside this test's callback.
	sm.ackStopped = true
	sm.setAgentDeliveryRepository(repository)

	require.NoError(t, sm.ReplayRecoveredDelivery(t.Context(), execution))
	require.Positive(t, callbacks)
	require.True(t, leaseFreeInCallback, "the replay must release its lease before lifecycle callbacks")
}

func newRecoveredReplayLeaseFixture(t *testing.T, mgr *Manager) (*AgentExecution, *tasksqlite.Repository, *sqlx.DB, <-chan uint64) {
	t.Helper()
	ctx := t.Context()
	payload, err := json.Marshal(agentctl.AgentEvent{Type: "complete"})
	require.NoError(t, err)
	repository, database := runOwnerTestRepository(t)
	client, _, stream, acknowledged := newIdleActivityJournalPeer(t, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "stream", Type: "complete", Payload: payload, CreatedAt: time.Now().UTC(), Terminal: true,
	}, repository)
	t.Cleanup(client.Close)
	require.NoError(t, repository.CreateTask(ctx, &models.Task{ID: "task", Title: "Replay lease"}))
	require.NoError(t, repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session", TaskID: "task", QueueIncarnationID: "incarnation",
	}))
	require.NoError(t, repository.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 2, NativeSessionID: "native", CreationReason: "initial",
	}))
	e := createTestExecution("execution", "task", "session")
	e.DeliveryMode, e.DeliveryStreamID, e.DeliveryIncarnationID, e.DeliveryHarnessGeneration = DurableDeliveryV1, "stream", "incarnation", 2
	e.DeliveryDescriptor, e.agentctl = &agentctl.DeliveryStatus{Stream: &stream}, client
	e.Status = v1.AgentStatusRunning
	require.NoError(t, mgr.executionStore.Add(e))
	mgr.SetExecutorRunningWriter(repository)
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, e))
	return e, repository, database, acknowledged
}

type replayLeaseRepository struct {
	*tasksqlite.Repository
	afterReceive    func()
	afterCursorRead func()
}

func (r *replayLeaseRepository) ReceiveAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, highWater int64) (bool, error) {
	inserted, err := r.Repository.ReceiveAgentDeliveryEvent(ctx, event, highWater)
	if r.afterReceive != nil {
		r.afterReceive()
	}
	return inserted, err
}

func (r *replayLeaseRepository) GetAgentDeliveryCursor(ctx context.Context, streamID string) (*models.AgentDeliveryCursor, error) {
	cursor, err := r.Repository.GetAgentDeliveryCursor(ctx, streamID)
	if r.afterCursorRead != nil {
		r.afterCursorRead()
	}
	return cursor, err
}

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.4
func TestRecoveredDeliveryReplayRejectsRetiredClientBeforeFollowUp(t *testing.T) {
	for _, tc := range []struct {
		name, request, clientState               string
		expired, replaceAfterStatus, closeClient bool
	}{
		{name: "duplicate acknowledgement", request: "acknowledge duplicate durable event", clientState: "changed"},
		{name: "expired status", request: "read authenticated peer delivery state", clientState: "changed", expired: true},
		{name: "expired acknowledgement", request: "acknowledge projected delivery cursor", clientState: "changed", expired: true, replaceAfterStatus: true},
		{name: "closed client before detach", request: "acknowledge duplicate durable event", clientState: "closed", closeClient: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			execution, repository, database, acknowledged := newRecoveredReplayLeaseFixture(t, newTestManager(t))
			original := execution.agentctl
			replacement := createTestClient(t, original.BaseURL())
			t.Cleanup(replacement.Close)
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			sm.ackStopped = true
			sm.setAgentDeliveryRepository(repository)
			require.NoError(t, sm.ReplayRecoveredDelivery(ctx, execution))
			if tc.expired {
				_, err := database.ExecContext(ctx, `UPDATE agent_delivery_cursors SET retired_sequence = 1 WHERE stream_id = ?`, execution.DeliveryStreamID)
				require.NoError(t, err)
				_, err = database.ExecContext(ctx, `DELETE FROM agent_delivery_inbox WHERE stream_id = ?`, execution.DeliveryStreamID)
				require.NoError(t, err)
			}

			hooks := &replayLeaseRepository{Repository: repository}
			retire := func() {
				require.True(t, execution.agentctlLifecycleMu.TryLock(), "SQL must run outside the client lease")
				defer execution.agentctlLifecycleMu.Unlock()
				if tc.closeClient {
					original.Close()
					return
				}
				execution.replaceAgentctlClient(replacement)
			}
			if tc.replaceAfterStatus {
				hooks.afterCursorRead = retire
			} else {
				hooks.afterReceive = retire
			}
			sm.setAgentDeliveryRepository(hooks)
			execution.DeliveryReplayCursor = 0
			err := sm.ReplayRecoveredDelivery(ctx, execution)
			require.ErrorContains(t, err, tc.request)
			require.ErrorContains(t, err, "agentctl client "+tc.clientState+" during replay")
			require.Zero(t, execution.DeliveryReplayCursor)
			require.Empty(t, acknowledged, "neither client may send the previous replay's acknowledgement")

			// The saved cursor can still be acknowledged by a new replay.
			execution.agentctlLifecycleMu.Lock()
			execution.replaceAgentctlClient(replacement)
			execution.agentctlLifecycleMu.Unlock()
			sm.setAgentDeliveryRepository(repository)
			err = sm.ReplayRecoveredDelivery(ctx, execution)
			if tc.expired {
				require.ErrorIs(t, err, errDeliveryReplayReconciled)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, uint64(1), execution.DeliveryReplayCursor)
			select {
			case sequence := <-acknowledged:
				require.Equal(t, uint64(1), sequence)
			default:
				t.Fatal("the current replay did not acknowledge the projected event")
			}
		})
	}
}
