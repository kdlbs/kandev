package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type projectionWaitRepository struct {
	*adoptionDeliveryRepository
	reads  int
	onRead func(context.Context, int) error
}

func (r *projectionWaitRepository) GetAgentDeliveryCursor(ctx context.Context, streamID string) (*models.AgentDeliveryCursor, error) {
	r.reads++
	if r.onRead != nil {
		if err := r.onRead(ctx, r.reads); err != nil {
			return nil, err
		}
	}
	cursor, err := r.adoptionDeliveryRepository.GetAgentDeliveryCursor(ctx, streamID)
	if cursor != nil && r.reads >= 3 {
		cursor.ProjectedSequence = cursor.ReceivedSequence
	}
	return cursor, err
}

func newProjectionWaitFixture(t *testing.T) (*Manager, *AgentExecution, *projectionWaitRepository, *journal.Journal) {
	t.Helper()
	ctx := context.Background()
	store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	for range 2 {
		_, err := store.Append(ctx, journal.Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`),
		})
		require.NoError(t, err)
	}
	require.NoError(t, store.Acknowledge(ctx, "stream", 2))
	repository := &projectionWaitRepository{adoptionDeliveryRepository: &adoptionDeliveryRepository{
		cursor: &models.AgentDeliveryCursor{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "stream", ReceivedSequence: 3, ProjectedSequence: 2,
		},
		submissions: make(map[string]*models.AgentDeliverySubmission),
	}}
	repository.onRead = func(ctx context.Context, read int) error {
		if read == 1 {
			_, err := store.Append(ctx, journal.Event{
				SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
				StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`),
			})
			return err
		}
		if read == 2 {
			return store.Acknowledge(ctx, "stream", 3)
		}
		return nil
	}
	manager := newTestManager(t)
	manager.SetAgentDeliveryRepository(repository)
	execution := &AgentExecution{
		ID: "execution", SessionID: "session", ACPSessionID: "native-conversation",
		DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream",
		agentctl: nativeResumeJournalPeer(t, store, false),
	}
	execution.setSessionInitialized(true)
	require.NoError(t, manager.executionStore.Add(execution))
	return manager, execution, repository, store
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
func TestNativeResumeProjectionWaitSucceedsFromOneRequest(t *testing.T) {
	manager, execution, repository, _ := newProjectionWaitFixture(t)
	proof, err := manager.GetNativeResumeDeliveryEvidence(t.Context(), execution.SessionID)
	require.NoError(t, err, "one Resume must wait for verified projection lag")
	require.Equal(t, int64(3), proof.Cursor.ProjectedSequence)
	require.Equal(t, execution.ACPSessionID, proof.NativeSessionID)
	require.GreaterOrEqual(t, repository.reads, 4, "rebuild the complete proof after the lag clears")
}

func TestNativeResumeProjectionWaitRejectsUnresolvedJournal(t *testing.T) {
	manager, execution, repository, store := newProjectionWaitFixture(t)
	repository.onRead = nil
	_, err := store.Append(t.Context(), journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	proof, err := manager.GetNativeResumeDeliveryEvidence(t.Context(), execution.SessionID)
	require.Nil(t, proof)
	require.ErrorIs(t, err, ErrDurableAdoptionBlocked)
	require.LessOrEqual(t, repository.reads, 1, "unresolved work must not schedule a projection retry")
}

func TestNativeResumeProjectionWaitRejectsCancellationAndChangedOwner(t *testing.T) {
	for _, scenario := range []string{"cancelled", "deadline", "owner", "replacement", "storage"} {
		t.Run(scenario, func(t *testing.T) {
			manager, execution, repository, _ := newProjectionWaitFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
			if scenario != "deadline" {
				cancel()
				ctx, cancel = context.WithCancel(t.Context())
			}
			defer cancel()
			baseRead := repository.onRead
			storageErr := errors.New("SQL unavailable")
			repository.onRead = func(ctx context.Context, read int) error {
				if scenario == "cancelled" && read == 2 {
					cancel()
					return nil
				}
				if scenario == "owner" && read == 4 {
					execution.ACPSessionID = "successor-native"
				}
				if scenario == "replacement" && read == 3 {
					manager.executionStore.Remove(execution.ID)
					require.NoError(t, manager.executionStore.Add(&AgentExecution{ID: "successor", SessionID: execution.SessionID}))
				}
				if scenario == "storage" && read == 3 {
					return storageErr
				}
				return baseRead(ctx, read)
			}
			proof, err := manager.GetNativeResumeDeliveryEvidence(ctx, execution.SessionID)
			require.Nil(t, proof)
			switch scenario {
			case "cancelled":
				require.ErrorIs(t, err, context.Canceled)
			case "deadline":
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.ErrorIs(t, err, ErrDurableAdoptionBlocked)
			case "owner", "replacement":
				require.ErrorIs(t, err, ErrDurableAdoptionIdentityMismatch)
			case "storage":
				require.ErrorIs(t, err, storageErr)
			}
		})
	}
}

func TestNativeResumeProjectionWaitReleasesClientBetweenAttempts(t *testing.T) {
	manager, execution, repository, _ := newProjectionWaitFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	writerDone := make(chan struct{})
	baseRead := repository.onRead
	repository.onRead = func(ctx context.Context, read int) error {
		if read == 2 {
			go func() {
				execution.agentctlLifecycleMu.Lock()
				cancel()
				execution.agentctlLifecycleMu.Unlock()
				close(writerDone)
			}()
		}
		if read >= 3 {
			<-ctx.Done()
			return ctx.Err()
		}
		return baseRead(ctx, read)
	}
	proof, err := manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.Nil(t, proof)
	require.ErrorIs(t, err, context.Canceled, "replacement must not wait for the projection budget")
	<-writerDone
}

func TestNativeResumeProjectionWaitCapsLongCallerDeadline(t *testing.T) {
	manager, _, repository, _ := newProjectionWaitFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	baseRead := repository.onRead
	repository.onRead = func(ctx context.Context, read int) error {
		if read == 3 {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			remaining := time.Until(deadline)
			require.Positive(t, remaining)
			require.LessOrEqual(t, remaining, 10*time.Second)
			cancel()
			return ctx.Err()
		}
		return baseRead(ctx, read)
	}
	_, err := manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 3, repository.reads)
}
