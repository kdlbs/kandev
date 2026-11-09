package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type bootstrapPreflightFixture struct {
	repo       *tasksqlite.Repository
	descriptor *agentctl.DeliveryStatus
	client     *agentctl.Client
	sm         *StreamManager
	execution  *AgentExecution
}

func newBootstrapPreflightFixture(t *testing.T) *bootstrapPreflightFixture {
	t.Helper()
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Managed startup"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
	descriptor := &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1",
		Stream: &journal.Stream{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1", HighWater: 1, FirstRetained: 1},
	}
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/agent/delivery", r.URL.Path)
		require.NoError(t, json.NewEncoder(w).Encode(descriptor))
	})
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(sm.Wait)
	execution := &AgentExecution{SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliveryStreamID: "incarnation:g1"}
	execution.deliveryBootstrap.Store(true)
	return &bootstrapPreflightFixture{repo: repo, descriptor: descriptor, client: client, sm: sm, execution: execution}
}

func (f *bootstrapPreflightFixture) preflight() (uint64, error) {
	return f.sm.initialDeliveryReplayCursor(context.Background(), f.execution, f.repo, f.client, f.execution.DeliveryStreamID)
}

func (f *bootstrapPreflightFixture) requireNoCursor(t *testing.T, msg string) {
	t.Helper()
	_, err := f.repo.GetAgentDeliveryCursor(context.Background(), f.execution.DeliveryStreamID)
	require.Error(t, err, msg)
}

func TestDeliveryPreflightRetainedStartupEventBeforeNativeInitialization(t *testing.T) {
	f := newBootstrapPreflightFixture(t)
	after, err := f.preflight()
	require.NoError(t, err, "a retained pre-initialize child exit must not prevent ACP initialization and its managed-runtime retry")
	require.Zero(t, after, "replay the real startup event; do not fabricate a receipt")
	f.requireNoCursor(t, "preflight must leave the absent SQL receipt absent")

	f.execution.isResumedSession = true
	after, err = f.preflight()
	require.NoError(t, err, "a relaunch after a first launch that never committed a native conversation is a generation-one bootstrap")
	require.Zero(t, after, "replay the retained startup events from the beginning")
	f.requireNoCursor(t, "relaunch preflight must leave the absent SQL receipt absent")
}

func TestDeliveryPreflightRelaunchKeepsDurableRefusals(t *testing.T) {
	for _, resumed := range []bool{false, true} {
		f := newBootstrapPreflightFixture(t)
		f.execution.isResumedSession = resumed

		f.execution.ACPSessionID = "existing-native"
		_, err := f.preflight()
		require.Error(t, err, "a restored native conversation is not a fresh bootstrap (resumed=%v)", resumed)
		f.execution.ACPSessionID = ""

		f.execution.RequiredNativeConversationID = "required-native"
		_, err = f.preflight()
		require.Error(t, err, "a pinned native conversation is not a fresh bootstrap (resumed=%v)", resumed)
		f.execution.RequiredNativeConversationID = ""

		f.descriptor.SubmissionCount = 1
		_, err = f.preflight()
		require.Error(t, err, "retained prompt work must be reconciled (resumed=%v)", resumed)
		f.descriptor.SubmissionCount = 0

		f.descriptor.Stream.Acknowledged = 1
		f.descriptor.Stream.FirstRetained = 2
		_, err = f.preflight()
		require.ErrorIs(t, err, ErrAgentDeliveryHistoryGap, "fresh bootstrap cannot bypass pruned history (resumed=%v)", resumed)
		f.descriptor.Stream.Acknowledged = 0
		f.descriptor.Stream.FirstRetained = 1

		require.NoError(t, f.repo.CreateHarnessSessionGeneration(context.Background(), &models.HarnessSessionGeneration{
			SessionID: "session", IncarnationID: "incarnation", Generation: 1, NativeSessionID: "committed-native", CreationReason: "native_started",
		}))
		_, err = f.preflight()
		require.Error(t, err, "an existing SQL native generation cannot bypass a missing checkpoint (resumed=%v)", resumed)
		f.requireNoCursor(t, "refused preflight must not create a SQL receipt")
	}
}
