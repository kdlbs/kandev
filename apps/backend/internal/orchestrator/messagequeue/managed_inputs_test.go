package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagedInputFIFOBoundary(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("fifo")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)

			first, replayed, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("input-1", "occurrence-1", "first", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)
			require.False(t, replayed)
			second, replayed, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("input-2", "occurrence-2", "second", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)
			require.False(t, replayed)
			require.Greater(t, second.Sequence, first.Sequence)

			entries, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, entries, 2)
			require.Equal(t, []string{first.ID, second.ID}, []string{entries[0].ID, entries[1].ID})
			require.Equal(t, true, entries[0].Metadata[MetadataManagedInput])
			require.Equal(t, first.ID, entries[0].Metadata[MetadataManagedInputID])

			receipts, err := storage.ListManagedInputs(ctx, identity)
			require.NoError(t, err)
			require.Len(t, receipts, 2)
			require.Equal(t, first.Sequence, receipts[0].Sequence)
			require.Equal(t, second.Sequence, receipts[1].Sequence)

			const concurrentCount = 8
			concurrentReceipts := make([]ManagedInputReceipt, concurrentCount)
			concurrentErrors := make([]error, concurrentCount)
			var wait sync.WaitGroup
			for index := range concurrentReceipts {
				wait.Add(1)
				go func(index int) {
					defer wait.Done()
					request := managedInputRequest(
						fmt.Sprintf("parallel-input-%d", index), fmt.Sprintf("parallel-occurrence-%d", index),
						fmt.Sprintf("parallel-%d", index), ManagedInputOriginAutomation, "",
					)
					concurrentReceipts[index], _, concurrentErrors[index] = storage.AdmitManagedInput(
						ctx, identity, request, 20,
					)
				}(index)
			}
			wait.Wait()
			for _, admissionErr := range concurrentErrors {
				require.NoError(t, admissionErr)
			}
			entries, err = repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			receipts, err = storage.ListManagedInputs(ctx, identity)
			require.NoError(t, err)
			require.Len(t, entries, 2+concurrentCount)
			require.Len(t, receipts, 2+concurrentCount)
			for index := range entries {
				require.Equal(t, entries[index].ID, receipts[index].ID)
				require.Equal(t, entries[index].Position, receipts[index].Sequence)
			}
		})
	}
}

func TestManagedInputOccurrenceRetryReturnsReceipt(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("retry")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			request := managedInputRequest("host-input-1", "occurrence-1", "same payload", ManagedInputOriginHuman, "")

			first, replayed, err := storage.AdmitManagedInput(ctx, identity, request, 10)
			require.NoError(t, err)
			require.False(t, replayed)
			retry := request
			retry.ID = "new-host-retry-id"
			got, replayed, err := storage.AdmitManagedInput(ctx, identity, retry, 10)
			require.NoError(t, err)
			require.True(t, replayed)
			require.Equal(t, first, got)

			entries, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, first.ID, entries[0].ID)
		})
	}
}

func TestManagedInputOccurrencePayloadConflict(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("conflict")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			_, _, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("input-1", "occurrence-1", "original", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)
			_, _, err = storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("input-2", "occurrence-1", "changed", ManagedInputOriginHuman, ""), 10,
			)
			require.ErrorIs(t, err, ErrQueueIDConflict)
		})
	}
}

func TestManagedInputListAndGetAreIdentityScoped(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			firstIdentity := managedInputIdentity("scope-a")
			secondIdentity := managedInputIdentity("scope-b")
			seedQueueSessionIdentity(t, repository, firstIdentity)
			seedQueueSessionIdentity(t, repository, secondIdentity)
			storage := requireManagedInputStorage(t, repository)
			first, _, err := storage.AdmitManagedInput(
				ctx, firstIdentity, managedInputRequest("input-a", "occurrence-a", "a", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)
			second, _, err := storage.AdmitManagedInput(
				ctx, secondIdentity, managedInputRequest("input-b", "occurrence-b", "b", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)

			firstList, err := storage.ListManagedInputs(ctx, firstIdentity)
			require.NoError(t, err)
			require.Equal(t, []string{first.ID}, managedInputIDs(firstList))
			secondList, err := storage.ListManagedInputs(ctx, secondIdentity)
			require.NoError(t, err)
			require.Equal(t, []string{second.ID}, managedInputIDs(secondList))
			got, err := storage.GetManagedInput(ctx, firstIdentity, first.ID)
			require.NoError(t, err)
			require.Equal(t, first, got)
			_, err = storage.GetManagedInput(ctx, secondIdentity, first.ID)
			require.ErrorIs(t, err, ErrEntryNotFound)
		})
	}
}

func TestManagedInputCancellationOnlyCancelsAccepted(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("cancel")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			accepted, _, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("input-accepted", "occ-accepted", "pending", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)
			cancelled, changed, err := storage.CancelManagedInput(ctx, identity, accepted.ID)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, ManagedInputStateCancelled, cancelled.State)
			entries, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Empty(t, entries)

			running, _, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("input-running", "occ-running", "started", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)
			running, changed, err = storage.MarkManagedInputRunning(
				ctx, identity, running.ID, "turn-1", "execution-1",
			)
			require.NoError(t, err)
			require.True(t, changed)
			waiting, _, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("input-waiting", "occ-waiting", "next", ManagedInputOriginHuman, ""), 10,
			)
			require.NoError(t, err)
			_, changed, err = storage.MarkManagedInputRunning(
				ctx, identity, waiting.ID, "turn-2", "execution-2",
			)
			require.ErrorIs(t, err, ErrManagedInputBusy)
			require.False(t, changed)
			stillRunning, changed, err := storage.CancelManagedInput(ctx, identity, running.ID)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, ManagedInputStateRunning, stillRunning.State)
			stopped, changed, err := storage.SettleManagedInput(
				ctx, identity, running.ID, "turn-1", "execution-1", ManagedInputStateCancelled, "stop confirmed",
			)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, ManagedInputStateCancelled, stopped.State)
			replayedStop, changed, err := storage.SettleManagedInput(
				ctx, identity, running.ID, "turn-1", "execution-1", ManagedInputStateCancelled, "stop confirmed",
			)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, stopped, replayedStop)
			_, changed, err = storage.MarkManagedInputRunning(
				ctx, identity, waiting.ID, "turn-2", "execution-2",
			)
			require.NoError(t, err)
			require.True(t, changed)
		})
	}
}

func TestManagedInputAcceptedUncertainSettlement(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("accepted-uncertain")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			accepted, _, err := storage.AdmitManagedInput(
				ctx, identity,
				managedInputRequest("input-accepted-uncertain", "occ-accepted-uncertain", "dispatch", ManagedInputOriginHuman, ""),
				10,
			)
			require.NoError(t, err)

			uncertain, changed, err := storage.SettleManagedInput(
				ctx, identity, accepted.ID, "", "", ManagedInputStateUncertain, "dispatch outcome unknown",
			)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, ManagedInputStateUncertain, uncertain.State)
			require.Empty(t, uncertain.TurnID)
			require.Empty(t, uncertain.ExecutionID)
			require.Equal(t, "dispatch outcome unknown", uncertain.Outcome)

			entries, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Empty(t, entries)

			replayed, changed, err := storage.SettleManagedInput(
				ctx, identity, accepted.ID, "", "", ManagedInputStateUncertain, "dispatch outcome unknown",
			)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, uncertain, replayed)
		})
	}
}

func TestManagedInputSettlementRejectsPartialExecutionIdentity(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("partial-identity")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			accepted, _, err := storage.AdmitManagedInput(
				ctx, identity,
				managedInputRequest("input-partial-identity", "occ-partial-identity", "dispatch", ManagedInputOriginHuman, ""),
				10,
			)
			require.NoError(t, err)

			for _, pair := range [][2]string{{"turn-only", ""}, {"", "execution-only"}} {
				_, changed, err := storage.SettleManagedInput(
					ctx, identity, accepted.ID, pair[0], pair[1], ManagedInputStateUncertain, "dispatch outcome unknown",
				)
				require.Error(t, err)
				require.False(t, changed)
			}
			_, changed, err := storage.SettleManagedInput(
				ctx, identity, accepted.ID, "", "", ManagedInputStateCompleted, "done",
			)
			require.Error(t, err)
			require.False(t, changed)
			_, changed, err = storage.SettleManagedInput(
				ctx, identity, accepted.ID, "", "", ManagedInputStateUncertain, strings.Repeat("x", 2049),
			)
			require.Error(t, err)
			require.False(t, changed)

			unchanged, err := storage.GetManagedInput(ctx, identity, accepted.ID)
			require.NoError(t, err)
			require.Equal(t, ManagedInputStateAccepted, unchanged.State)
			entries, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, entries, 1)
		})
	}
}

func TestManagedInputRecovery(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("recovery")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			request := managedInputRequest("input-1", "occurrence-1", "payload", ManagedInputOriginHuman, "")
			accepted, _, err := storage.AdmitManagedInput(
				ctx, identity, request, 10,
			)
			require.NoError(t, err)

			_, storage = reopenManagedInputStorage(t, repository)
			recovered, err := storage.GetManagedInput(ctx, identity, accepted.ID)
			require.NoError(t, err)
			require.Equal(t, ManagedInputStateAccepted, recovered.State)
			queued, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, queued, 1)
			require.Equal(t, accepted.ID, queued[0].ID)
			replayed, wasReplay, err := storage.AdmitManagedInput(ctx, identity, request, 10)
			require.NoError(t, err)
			require.True(t, wasReplay)
			require.Equal(t, accepted.ID, replayed.ID)

			running, changed, err := storage.MarkManagedInputRunning(ctx, identity, accepted.ID, "turn-1", "execution-1")
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, ManagedInputStateRunning, running.State)
			require.Equal(t, "turn-1", running.TurnID)
			require.Equal(t, "execution-1", running.ExecutionID)
			replayedStart, changed, err := storage.MarkManagedInputRunning(
				ctx, identity, accepted.ID, "turn-1", "execution-1",
			)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, running, replayedStart)

			_, storage = reopenManagedInputStorage(t, repository)
			recovered, err = storage.GetManagedInput(ctx, identity, accepted.ID)
			require.NoError(t, err)
			require.Equal(t, ManagedInputStateRunning, recovered.State)
			require.Equal(t, "execution-1", recovered.ExecutionID)
			byExecution, err := storage.GetManagedInputByExecution(ctx, identity, "turn-1", "execution-1")
			require.NoError(t, err)
			require.Equal(t, accepted.ID, byExecution.ID)
			_, changed, err = storage.SettleManagedInput(
				ctx, identity, accepted.ID, "turn-1", "different-execution", ManagedInputStateCompleted, "done",
			)
			require.ErrorIs(t, err, ErrManagedInputTransition)
			require.False(t, changed)
			uncertain, changed, err := storage.SettleManagedInput(
				ctx, identity, accepted.ID, "turn-1", "execution-1", ManagedInputStateUncertain, "completion not observed",
			)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, ManagedInputStateUncertain, uncertain.State)
			require.Equal(t, "completion not observed", uncertain.Outcome)
			_, storage = reopenManagedInputStorage(t, repository)
			recovered, err = storage.GetManagedInput(ctx, identity, accepted.ID)
			require.NoError(t, err)
			require.Equal(t, ManagedInputStateUncertain, recovered.State)
			require.Equal(t, "completion not observed", recovered.Outcome)
		})
	}
}

func TestManagedInputCoalescesOnlyExplicitPendingPeriodicInputs(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("coalesce")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			for index, origin := range []ManagedInputOrigin{
				ManagedInputOriginHuman, ManagedInputOriginAutomation, ManagedInputOriginInteraction,
			} {
				suffix := fmt.Sprintf("%d", index)
				first, _, err := storage.AdmitManagedInput(
					ctx, identity, managedInputRequest("nonperiodic-1-"+suffix,
						"nonperiodic-occ-1-"+suffix, "first", origin, "shared-key"), 10,
				)
				require.NoError(t, err)
				second, _, err := storage.AdmitManagedInput(
					ctx, identity, managedInputRequest("nonperiodic-2-"+suffix,
						"nonperiodic-occ-2-"+suffix, "second", origin, "shared-key"), 10,
				)
				require.NoError(t, err)
				require.NotEqual(t, first.ID, second.ID)
			}
			periodicWithoutKey, _, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("periodic-no-key-1", "periodic-no-key-occ-1",
					"first", ManagedInputOriginPeriodic, ""), 10,
			)
			require.NoError(t, err)
			periodicWithoutKey2, _, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("periodic-no-key-2", "periodic-no-key-occ-2",
					"second", ManagedInputOriginPeriodic, ""), 10,
			)
			require.NoError(t, err)
			require.NotEqual(t, periodicWithoutKey.ID, periodicWithoutKey2.ID)
		})
	}

	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name+"-periodic", func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("periodic")
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			first, _, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("periodic-1", "periodic-occ-1", "old status", ManagedInputOriginPeriodic, "status"), 1,
			)
			require.NoError(t, err)
			second, replayed, err := storage.AdmitManagedInput(
				ctx, identity, managedInputRequest("periodic-2", "periodic-occ-2", "new status", ManagedInputOriginPeriodic, "status"), 1,
			)
			require.NoError(t, err)
			require.False(t, replayed)
			require.Greater(t, second.Sequence, first.Sequence)
			old, err := storage.GetManagedInput(ctx, identity, first.ID)
			require.NoError(t, err)
			require.Equal(t, ManagedInputStateSuperseded, old.State)
			require.Equal(t, second.ID, old.SupersededBy)
			entries, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, second.ID, entries[0].ID)
			require.Equal(t, "new status", entries[0].Content)
		})
	}
}

func TestManagedInputPeriodicCoalescingPreservesFIFOAndCursorOrder(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("periodic-cursor-" + factory.name)
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			_, _, err := storage.AdmitManagedInput(ctx, identity,
				managedInputRequest("periodic-first-"+factory.name, "periodic-first-occ-"+factory.name,
					"old status", ManagedInputOriginPeriodic, "status"), 10)
			require.NoError(t, err)
			middle, _, err := storage.AdmitManagedInput(ctx, identity,
				managedInputRequest("human-middle-"+factory.name, "human-middle-occ-"+factory.name,
					"human request", ManagedInputOriginHuman, ""), 10)
			require.NoError(t, err)
			latest, _, err := storage.AdmitManagedInput(ctx, identity,
				managedInputRequest("periodic-latest-"+factory.name, "periodic-latest-occ-"+factory.name,
					"new status", ManagedInputOriginPeriodic, "status"), 10)
			require.NoError(t, err)

			require.Greater(t, latest.Sequence, middle.Sequence)
			entries, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, []string{middle.ID, latest.ID}, []string{entries[0].ID, entries[1].ID})
			receipts, err := storage.ListManagedInputs(ctx, identity)
			require.NoError(t, err)
			var afterCursor []string
			for _, receipt := range receipts {
				if receipt.Sequence > middle.Sequence && receipt.State == ManagedInputStateAccepted {
					afterCursor = append(afterCursor, receipt.ID)
				}
			}
			require.Equal(t, []string{latest.ID}, afterCursor)
		})
	}
}

func TestManagedInputDoesNotCoalesceReservedPeriodicInput(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("reserved-periodic-" + factory.name)
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			first, _, err := storage.AdmitManagedInput(
				ctx, identity,
				managedInputRequest("periodic-reserved-1-"+factory.name,
					"periodic-reserved-occ-1-"+factory.name, "first", ManagedInputOriginPeriodic, "status"),
				10,
			)
			require.NoError(t, err)
			reserved, _, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.NotNil(t, reserved)

			second, replayed, err := storage.AdmitManagedInput(
				ctx, identity,
				managedInputRequest("periodic-reserved-2-"+factory.name,
					"periodic-reserved-occ-2-"+factory.name, "second", ManagedInputOriginPeriodic, "status"),
				10,
			)
			require.NoError(t, err)
			require.False(t, replayed)
			require.NotEqual(t, first.Sequence, second.Sequence)
			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 2)
			require.Equal(t, first.ID, stored[0].ID)
			require.True(t, stored[0].IsReservedInFlight())
			require.Equal(t, second.ID, stored[1].ID)
		})
	}
}

func TestManagedInputCannotAutoMergeWithOrdinaryMessages(t *testing.T) {
	managed := &QueuedMessage{
		TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser,
		Metadata: map[string]interface{}{MetadataManagedInput: true},
	}
	ordinary := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser}
	require.False(t, autoMergeAllowed(managed, ordinary))
	require.False(t, autoMergeAllowed(ordinary, managed))
}

func TestManagedInputReservationRetainsRowUntilStartOrAcknowledge(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repository := factory.new(t)
			identity := managedInputIdentity("reservation-" + factory.name)
			seedQueueSessionIdentity(t, repository, identity)
			storage := requireManagedInputStorage(t, repository)
			accepted, _, err := storage.AdmitManagedInput(
				ctx, identity,
				managedInputRequest("input-reserved-"+factory.name, "occ-reserved-"+factory.name,
					"retained until start", ManagedInputOriginHuman, ""),
				10,
			)
			require.NoError(t, err)

			reserved, autoRun, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, autoRun)
			require.NotNil(t, reserved)
			require.True(t, reserved.IsDurableDelivery())
			require.False(t, reserved.IsDurableLifecycle())
			stored, err := repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.True(t, stored[0].IsReservedInFlight())
			require.NoError(t, repository.ReleaseDeliveryReservationForSession(ctx, identity, reserved))
			stored, err = repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			require.False(t, stored[0].IsReservedInFlight())
			reserved, autoRun, err = repository.ReserveHeadIfAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, autoRun)
			require.NotNil(t, reserved)

			running, changed, err := storage.MarkManagedInputRunning(
				ctx, identity, accepted.ID, "turn-reserved", "execution-reserved",
			)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, ManagedInputStateRunning, running.State)
			stored, err = repository.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Empty(t, stored)

			ackRepository := factory.new(t)
			ackIdentity := managedInputIdentity("ack-" + factory.name)
			seedQueueSessionIdentity(t, ackRepository, ackIdentity)
			ackStorage := requireManagedInputStorage(t, ackRepository)
			ackInput, _, err := ackStorage.AdmitManagedInput(
				ctx, ackIdentity,
				managedInputRequest("input-ack-"+factory.name, "occ-ack-"+factory.name,
					"retained until explicit acknowledgement", ManagedInputOriginHuman, ""),
				10,
			)
			require.NoError(t, err)
			ackReserved, _, err := ackRepository.ReserveHeadIfAutoRunForSession(ctx, ackIdentity)
			require.NoError(t, err)
			require.NotNil(t, ackReserved)
			require.NoError(t, ackRepository.AcknowledgeByIDForSession(ctx, ackIdentity, ackReserved))
			stored, err = ackRepository.ListBySession(ctx, ackIdentity.SessionID)
			require.NoError(t, err)
			require.Empty(t, stored)
			stillAccepted, err := ackStorage.GetManagedInput(ctx, ackIdentity, ackInput.ID)
			require.NoError(t, err)
			require.Equal(t, ManagedInputStateAccepted, stillAccepted.State)
		})
	}
}

func TestManagedInputAttemptedReservationSurvivesRestartAndReserve(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	identity := managedInputIdentity("attempt-recovery")
	seedQueueSessionIdentity(t, repository, identity)
	storage := requireManagedInputStorage(t, repository)
	accepted, _, err := storage.AdmitManagedInput(
		ctx, identity,
		managedInputRequest("input-attempted", "occ-attempted", "dispatch attempt", ManagedInputOriginHuman, ""),
		10,
	)
	require.NoError(t, err)

	reserved, _, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.NotNil(t, reserved)
	require.True(t, reserved.IsDurableDelivery())
	require.NoError(t, repository.MarkDeliveryAttemptedForSession(
		ctx, identity, []QueuedMessage{*reserved},
	))

	repository, _ = reopenManagedInputStorage(t, repository)
	recovered, autoRun, err := repository.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, autoRun)
	require.NotNil(t, recovered)
	require.True(t, recovered.IsDeliveryAttempted())
	require.True(t, recovered.IsDurableDelivery())
	require.False(t, recovered.IsDurableLifecycle())
	require.Equal(t, accepted.ID, recovered.ID)
	stored, err := repository.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.True(t, stored[0].IsReservedInFlight())
	require.True(t, stored[0].IsDeliveryAttempted())
}

func requireManagedInputStorage(t *testing.T, repository Repository) ManagedInputStorage {
	t.Helper()
	storage, ok := repository.(ManagedInputStorage)
	require.True(t, ok, "%T must implement ManagedInputStorage", repository)
	return storage
}

func reopenManagedInputStorage(t *testing.T, repository Repository) (Repository, ManagedInputStorage) {
	t.Helper()
	sqlRepository, ok := repository.(*sqliteRepository)
	if !ok {
		return repository, requireManagedInputStorage(t, repository)
	}
	restarted, err := NewSQLiteRepository(sqlRepository.db, sqlRepository.ro)
	require.NoError(t, err)
	return restarted, requireManagedInputStorage(t, restarted)
}

func managedInputIdentity(suffix string) QueueSessionIdentity {
	return QueueSessionIdentity{
		TaskID:               "managed-task-" + suffix,
		SessionID:            "managed-session-" + suffix,
		SessionIncarnationID: "managed-incarnation-" + suffix,
	}
}

func managedInputRequest(id, occurrence, payload string, origin ManagedInputOrigin, coalesceKey string) ManagedInputRequest {
	digest := sha256.Sum256([]byte(payload))
	return ManagedInputRequest{
		ID: id, OccurrenceKey: occurrence, Payload: payload,
		PayloadDigest: hex.EncodeToString(digest[:]), Origin: origin,
		CoalesceKey: coalesceKey, ConversationRevision: 1,
	}
}

func managedInputIDs(receipts []ManagedInputReceipt) []string {
	ids := make([]string, len(receipts))
	for index, receipt := range receipts {
		ids[index] = receipt.ID
	}
	return ids
}
