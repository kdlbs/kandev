package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// successorBootstrapPreflightFixture models the retained successor stream of a
// fresh start that failed before committing its replacement native session.
type successorBootstrapPreflightFixture struct {
	repo       *tasksqlite.Repository
	delivery   AgentDeliveryRepository
	descriptor *agentctl.DeliveryStatus
	client     *agentctl.Client
	sm         *StreamManager
	execution  *AgentExecution
}

func newSuccessorBootstrapPreflightFixture(
	t *testing.T, generation uint64, commitSelected bool,
) *successorBootstrapPreflightFixture {
	t.Helper()
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Uncommitted successor"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
	for committed := int64(1); committed <= int64(generation); committed++ {
		if committed == int64(generation) && !commitSelected {
			break
		}
		predecessor := committed
		generationRecord := &models.HarnessSessionGeneration{
			SessionID: "session", IncarnationID: "incarnation", Generation: committed,
			NativeSessionID: fmt.Sprintf("native-%d", committed), CreationReason: "native_started",
		}
		if predecessor > 1 {
			generationRecord.PredecessorGeneration = predecessor - 1
			generationRecord.CreationReason = "native_replaced"
		}
		require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, generationRecord))
	}
	streamID := fmt.Sprintf("incarnation:g%d", generation)
	descriptor := &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: generation,
		StreamID: streamID,
		Stream: &journal.Stream{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: generation,
			StreamID: streamID, HighWater: 2, FirstRetained: 1,
		},
	}
	client := ackTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(descriptor))
	})
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(sm.Wait)
	execution := &AgentExecution{
		SessionID: "session", DeliveryIncarnationID: "incarnation",
		DeliveryHarnessGeneration: generation, DeliveryStreamID: streamID,
		ReplacesNativeConversation: generation > 1,
	}
	if generation == 1 {
		execution.deliveryBootstrap.Store(true)
	}
	return &successorBootstrapPreflightFixture{
		repo: repo, delivery: repo, descriptor: descriptor, client: client, sm: sm, execution: execution,
	}
}

func (f *successorBootstrapPreflightFixture) preflight() (uint64, error) {
	return f.sm.initialDeliveryReplayCursor(
		context.Background(), f.execution, f.delivery, f.client, f.execution.DeliveryStreamID,
	)
}

func (f *successorBootstrapPreflightFixture) requireNoCursor(t *testing.T, message string) {
	t.Helper()
	_, err := f.repo.GetAgentDeliveryCursor(context.Background(), f.execution.DeliveryStreamID)
	require.Error(t, err, message)
}

// TestDeliveryPreflightUncommittedFreshStartSuccessor proves that a retained
// successor stream can replay from zero after a previous attempt failed before
// its SQL generation commit. The generation-one bootstrap control stays intact.
func TestDeliveryPreflightUncommittedFreshStartSuccessor(t *testing.T) {
	for _, generation := range []uint64{2, 10} {
		for _, resumed := range []bool{false, true} {
			t.Run(fmt.Sprintf("g%d/resumed=%v", generation, resumed), func(t *testing.T) {
				f := newSuccessorBootstrapPreflightFixture(t, generation, false)
				f.execution.isResumedSession = resumed
				after, err := f.preflight()
				require.NoError(t, err, "an uncommitted fresh-start successor must replay its retained setup events")
				require.Zero(t, after, "replay the retained successor events from the beginning")
				f.requireNoCursor(t, "preflight must not fabricate a SQL checkpoint for the uncommitted successor")
			})
		}
	}
	t.Run("generation one control", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 1, false)
		after, err := f.preflight()
		require.NoError(t, err, "the generation-one bootstrap exception must keep its current behavior")
		require.Zero(t, after)
		f.requireNoCursor(t, "generation-one bootstrap must not create a SQL receipt")
	})
}

type staticHarnessGenerationRepository struct {
	AgentDeliveryRepository
	generation *models.HarnessSessionGeneration
	err        error
}

func (r *staticHarnessGenerationRepository) GetCurrentHarnessSessionGeneration(
	context.Context, string, string,
) (*models.HarnessSessionGeneration, error) {
	return r.generation, r.err
}

type deliveryOnlyRepository struct {
	AgentDeliveryRepository
}

func TestDeliveryPreflightUncommittedFreshStartRefusesUnsafeState(t *testing.T) {
	t.Run("absent replacement intent", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.execution.ReplacesNativeConversation = false
		_, err := f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "an unmarked successor must not create a checkpoint")
	})
	t.Run("no predecessor", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		_, err := f.repo.DB().Exec(`DELETE FROM harness_session_generations`)
		require.NoError(t, err)
		_, err = f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "a missing predecessor must not create a checkpoint")
	})
	t.Run("non-adjacent predecessor", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 10, false)
		_, err := f.repo.DB().Exec(`DELETE FROM harness_session_generations WHERE generation > 1`)
		require.NoError(t, err)
		_, err = f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "g1 must not authorize a stray g10 stream")
	})
	t.Run("already committed selected generation", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, true)
		_, err := f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "a committed selected generation must not use the bootstrap exception")
	})
	t.Run("foreign predecessor identity", func(t *testing.T) {
		for _, foreign := range []string{"session", "incarnation"} {
			t.Run(foreign, func(t *testing.T) {
				f := newSuccessorBootstrapPreflightFixture(t, 2, false)
				predecessor := &models.HarnessSessionGeneration{
					SessionID: "session", IncarnationID: "incarnation", Generation: 1,
					NativeSessionID: "native-1", CreationReason: "native_started",
				}
				if foreign == "session" {
					predecessor.SessionID = "foreign-session"
				} else {
					predecessor.IncarnationID = "foreign-incarnation"
				}
				f.delivery = &staticHarnessGenerationRepository{AgentDeliveryRepository: f.repo, generation: predecessor}
				_, err := f.preflight()
				require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
				f.requireNoCursor(t, "a foreign predecessor must not create a checkpoint")
			})
		}
	})
	t.Run("generation-one bootstrap flag on successor", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.execution.deliveryBootstrap.Store(true)
		_, err := f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "the generation-one bootstrap flag must not widen to a successor")
	})
	t.Run("native identity", func(t *testing.T) {
		for _, native := range []string{"acp", "required"} {
			t.Run(native, func(t *testing.T) {
				f := newSuccessorBootstrapPreflightFixture(t, 2, false)
				if native == "acp" {
					f.execution.ACPSessionID = "existing-native"
				} else {
					f.execution.RequiredNativeConversationID = "required-native"
				}
				_, err := f.preflight()
				require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
				f.requireNoCursor(t, "a native identity must not use the bootstrap exception")
			})
		}
	})
	t.Run("nonzero acknowledgement", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.descriptor.Stream.Acknowledged = 1
		_, err := f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "an acknowledged stream must not be replayed from zero")
	})
	t.Run("pruned prefix", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.descriptor.Stream.Acknowledged = 1
		f.descriptor.Stream.FirstRetained = 2
		_, err := f.preflight()
		require.ErrorIs(t, err, ErrAgentDeliveryHistoryGap)
		f.requireNoCursor(t, "pruned history must not be replayed")
	})
	t.Run("submission evidence", func(t *testing.T) {
		cases := map[string]func(*agentctl.DeliveryStatus){
			"unresolved": func(status *agentctl.DeliveryStatus) { status.Unresolved = true },
			"count":      func(status *agentctl.DeliveryStatus) { status.SubmissionCount = 1 },
			"summaries": func(status *agentctl.DeliveryStatus) {
				status.Submissions = []journal.SubmissionSummary{{ID: "submission"}}
			},
			"truncated": func(status *agentctl.DeliveryStatus) { status.SubmissionsTruncated = true },
		}
		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				f := newSuccessorBootstrapPreflightFixture(t, 2, false)
				mutate(f.descriptor)
				_, err := f.preflight()
				require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
				f.requireNoCursor(t, "submission evidence must remain a hard refusal")
			})
		}
	})
	t.Run("authority read failure", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.delivery = &staticHarnessGenerationRepository{AgentDeliveryRepository: f.repo, err: errors.New("authority unavailable")}
		_, err := f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "a failed authority read must not create a checkpoint")
	})
	t.Run("unsupported generation reader", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.delivery = &deliveryOnlyRepository{AgentDeliveryRepository: f.repo}
		_, err := f.preflight()
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryCheckpointMissing)
		f.requireNoCursor(t, "an unsupported reader must not create a checkpoint")
	})
	t.Run("unavailable storage descriptor", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.execution.DeliveryMode = DurableDeliveryV1
		f.descriptor.Durable = false
		f.descriptor.Reason = "storage_unavailable"
		_, err := f.preflight()
		require.Error(t, err)
		f.requireNoCursor(t, "an unavailable storage descriptor must not create a checkpoint")
	})
	t.Run("ordinary cursor keeps ordinary validation", func(t *testing.T) {
		f := newSuccessorBootstrapPreflightFixture(t, 2, false)
		f.descriptor.Stream.HighWater = 1
		event := &models.AgentDeliveryEvent{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: f.execution.DeliveryStreamID, Sequence: 1, EventType: "agent_capabilities",
			Payload: []byte(`{}`),
		}
		inserted, err := f.repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1)
		require.NoError(t, err)
		require.True(t, inserted)
		_, err = f.repo.ProjectAgentDeliveryEvent(context.Background(), event, nil)
		require.NoError(t, err)
		after, err := f.preflight()
		require.NoError(t, err)
		require.EqualValues(t, 1, after, "an existing cursor must use ordinary replay, not the bootstrap exception")
	})
}
