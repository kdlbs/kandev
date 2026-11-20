package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

var errDeliveryStartupInitialization = errors.New("native initialization failed")

func deliveryStartupTestExecution(t *testing.T) (*AgentExecution, *deliveryStartupAttempt) {
	t.Helper()
	execution, _ := retentionBootstrapOwner()
	attempt := execution.beginDeliveryStartupAttempt(context.Background())
	return execution, attempt
}

func deliveryStartupTestPrepare(
	t *testing.T,
	manager *StreamManager,
	ctx context.Context,
	execution *AgentExecution,
	repository AgentDeliveryRepository,
) chan error {
	t.Helper()
	_, event := retentionBootstrapOwner()
	result := make(chan error, 1)
	go func() {
		_, err := manager.prepareAgentEvent(ctx, execution, nil, repository, event)
		result <- err
	}()
	return result
}

func TestDeliveryStartupNegotiationBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		execution, attempt := deliveryStartupTestExecution(t)
		execution.DeliveryMode = ""
		repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
		repository.generation.Store(retentionBootstrapGeneration())
		manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
		result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)

		time.Sleep(35 * time.Second)
		select {
		case err := <-result:
			t.Fatalf("negotiation expired before the caller deadline: %v", err)
		default:
		}

		execution.DeliveryMode = DurableDeliveryV1
		attempt.finishNegotiation(nil)
		attempt.finishNative(nil)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if repository.receipts.Load() != 1 {
			t.Fatalf("admitted after slow negotiation: receipts=%d", repository.receipts.Load())
		}
	})
}

func TestDeliveryStartupNativeBudget(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AgentExecution)
	}{
		{"fresh generation one", func(*AgentExecution) {}},
		{"native restore", func(execution *AgentExecution) { execution.ACPSessionID = "existing-native" }},
		{"uncommitted first launch retry", func(execution *AgentExecution) { execution.isResumedSession = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				execution, attempt := deliveryStartupTestExecution(t)
				execution.DeliveryMode = DurableDeliveryV1
				tc.mutate(execution)
				repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
				manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
				result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
				attempt.finishNegotiation(nil)

				time.Sleep(35 * time.Second)
				select {
				case err := <-result:
					t.Fatalf("event expired before native initialization: %v", err)
				case <-repository.firstRead:
					t.Fatal("SQL authority was read before native initialization completed")
				default:
				}
				attempt.finishNative(nil)
				select {
				case <-repository.firstRead:
				case <-time.After(time.Second):
					t.Fatal("native success did not start the SQL authority wait")
				}
				repository.generation.Store(retentionBootstrapGeneration())
				if err := <-result; err != nil {
					t.Fatal(err)
				}
				if repository.receipts.Load() != 1 {
					t.Fatalf("admitted after slow native startup: receipts=%d", repository.receipts.Load())
				}
			})
		})
	}
}

func TestDeliveryStartupMissingAuthority(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		execution, attempt := deliveryStartupTestExecution(t)
		execution.DeliveryMode = DurableDeliveryV1
		repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
		manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
		result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
		attempt.finishNegotiation(nil)
		attempt.finishNative(nil)
		<-repository.firstRead

		time.Sleep(29 * time.Second)
		select {
		case err := <-result:
			t.Fatalf("missing authority failed before its bounded budget: %v", err)
		default:
		}
		time.Sleep(2 * time.Second)
		select {
		case err := <-result:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("missing authority error = %v", err)
			}
		default:
			t.Fatal("missing authority did not exhaust its bounded budget")
		}
		if repository.receipts.Load() != 0 || repository.projections.Load() != 0 {
			t.Fatal("missing authority admitted delivery evidence")
		}
	})
}

type blockingAuthorityRepository struct {
	retentionBootstrapRepository
	readStarted chan time.Time
}

func (r *blockingAuthorityRepository) GetCurrentHarnessSessionGeneration(
	ctx context.Context, _, _ string,
) (*models.HarnessSessionGeneration, error) {
	deadline, _ := ctx.Deadline()
	select {
	case r.readStarted <- deadline:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestDeliveryStartupAuthorityQueryIsBounded(t *testing.T) {
	t.Run("initial read uses the native-anchored budget", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, attempt := deliveryStartupTestExecution(t)
			execution.DeliveryMode = DurableDeliveryV1
			attempt.finishNegotiation(nil)
			attempt.finishNative(nil)
			repository := &blockingAuthorityRepository{
				retentionBootstrapRepository: retentionBootstrapRepository{firstRead: make(chan struct{})},
				readStarted:                  make(chan time.Time, 1),
			}
			callbacks := make(chan struct{}, 1)
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{
				OnAgentEventWithGeneration: func(*AgentExecution, agentctl.AgentEvent, uint64) {
					callbacks <- struct{}{}
				},
			}, nil, nil)
			_, event := retentionBootstrapOwner()
			result := make(chan error, 1)
			go func() {
				result <- manager.processAgentEvent(context.Background(), execution, nil, repository, event, 0)
			}()
			select {
			case deadline := <-repository.readStarted:
				if deadline.IsZero() {
					t.Fatal("initial authority query has no deadline")
				}
			case <-time.After(time.Second):
				t.Fatal("initial authority query did not run")
			}
			time.Sleep(31 * time.Second)
			if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("bounded initial authority query = %v", err)
			}
			if repository.receipts.Load() != 0 || repository.projections.Load() != 0 {
				t.Fatal("bounded initial authority query admitted delivery evidence")
			}
			select {
			case <-callbacks:
				t.Fatal("bounded initial authority query notified a callback")
			default:
			}
			manager.ackMu.Lock()
			ackWorkers := len(manager.ackWorkers)
			manager.ackMu.Unlock()
			if ackWorkers != 0 {
				t.Fatal("bounded initial authority query scheduled an ACK")
			}
		})
	})

	t.Run("released successful attempt keeps its native anchor", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, attempt := deliveryStartupTestExecution(t)
			execution.DeliveryMode = DurableDeliveryV1
			attempt.finishNegotiation(nil)
			attempt.finishNative(nil)
			execution.releaseDeliveryStartupAttempt(attempt)
			if execution.deliveryStartupSnapshot() != attempt {
				t.Fatal("successful attempt anchor was cleared on release")
			}
			repository := &blockingAuthorityRepository{
				retentionBootstrapRepository: retentionBootstrapRepository{firstRead: make(chan struct{})},
				readStarted:                  make(chan time.Time, 1),
			}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			time.Sleep(20 * time.Second)
			result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
			select {
			case deadline := <-repository.readStarted:
				remaining := time.Until(deadline)
				if remaining <= 8*time.Second || remaining > 11*time.Second {
					t.Fatalf("released attempt deadline is %v away, want the original native anchor", remaining)
				}
			case <-time.After(time.Second):
				t.Fatal("released attempt did not query authority")
			}
			time.Sleep(11 * time.Second)
			if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("released attempt budget = %v", err)
			}
			if repository.receipts.Load() != 0 || repository.projections.Load() != 0 {
				t.Fatal("released attempt budget admitted delivery evidence")
			}
		})
	})

	t.Run("expired attempt probe has its own bound", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, _ := retentionBootstrapOwner()
			repository := &blockingAuthorityRepository{
				retentionBootstrapRepository: retentionBootstrapRepository{firstRead: make(chan struct{})},
				readStarted:                  make(chan time.Time, 1),
			}
			result := make(chan error, 1)
			go func() {
				_, err := awaitBootstrapGeneration(
					context.Background(), execution, nil, repository,
					execution.SessionID, execution.DeliveryIncarnationID,
					time.Now().Add(-deliveryStartupAuthorityBudget),
				)
				result <- err
			}()
			select {
			case deadline := <-repository.readStarted:
				remaining := time.Until(deadline)
				if remaining <= 0 || remaining > deliveryStartupAuthorityProbe+time.Second {
					t.Fatalf("expired attempt probe deadline is %v away", remaining)
				}
			case <-time.After(time.Second):
				t.Fatal("expired attempt probe did not run")
			}
			time.Sleep(deliveryStartupAuthorityProbe + time.Second)
			if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expired attempt probe = %v", err)
			}
			if repository.receipts.Load() != 0 || repository.projections.Load() != 0 {
				t.Fatal("expired attempt probe admitted delivery evidence")
			}
		})
	})
}

var errDeliveryStartupAuthorityRead = errors.New("authority read failed")

type transientAuthorityRepository struct {
	retentionBootstrapRepository
	err error
}

func (r *transientAuthorityRepository) GetCurrentHarnessSessionGeneration(
	context.Context, string, string,
) (*models.HarnessSessionGeneration, error) {
	return nil, r.err
}

func TestDeliveryStartupAuthorityReadFailureKeepsBootstrap(t *testing.T) {
	execution, attempt := deliveryStartupTestExecution(t)
	execution.DeliveryMode = DurableDeliveryV1
	attempt.finishNegotiation(nil)
	attempt.finishNative(nil)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	failure := &transientAuthorityRepository{
		retentionBootstrapRepository: retentionBootstrapRepository{firstRead: make(chan struct{})},
		err:                          errDeliveryStartupAuthorityRead,
	}
	result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, failure)
	if err := <-result; !errors.Is(err, errDeliveryStartupAuthorityRead) {
		t.Fatalf("transient authority read = %v", err)
	}
	if !execution.deliveryBootstrap.Load() {
		t.Fatal("transient authority read cleared bootstrap admission")
	}
	recovered := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	recovered.generation.Store(retentionBootstrapGeneration())
	result = deliveryStartupTestPrepare(t, manager, context.Background(), execution, recovered)
	if err := <-result; err != nil {
		t.Fatalf("retry after transient authority read = %v", err)
	}
	if execution.deliveryBootstrap.Load() {
		t.Fatal("successful retry left bootstrap admission open")
	}
	if recovered.receipts.Load() != 1 {
		t.Fatalf("successful retry admitted %d events", recovered.receipts.Load())
	}
}

type finalCommitAuthorityRepository struct {
	retentionBootstrapRepository
	calls atomic.Int32
}

func (r *finalCommitAuthorityRepository) GetCurrentHarnessSessionGeneration(
	ctx context.Context, _, _ string,
) (*models.HarnessSessionGeneration, error) {
	if r.calls.Add(1) == 1 {
		return nil, models.ErrTaskSessionNotFound
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > deliveryStartupAuthorityProbe+time.Second {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return retentionBootstrapGeneration(), nil
}

func TestDeliveryStartupFinalAuthorityReadCatchesDeadlineCommit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		execution, _ := retentionBootstrapOwner()
		repository := &finalCommitAuthorityRepository{
			retentionBootstrapRepository: retentionBootstrapRepository{firstRead: make(chan struct{})},
		}
		type outcome struct {
			generation *models.HarnessSessionGeneration
			err        error
		}
		result := make(chan outcome, 1)
		go func() {
			generation, err := awaitBootstrapGeneration(
				context.Background(), execution, nil, repository,
				execution.SessionID, execution.DeliveryIncarnationID, time.Now(),
			)
			result <- outcome{generation: generation, err: err}
		}()
		time.Sleep(deliveryStartupAuthorityBudget + deliveryStartupPollInterval)
		got := <-result
		if got.err != nil {
			t.Fatalf("deadline commit was missed: %v", got.err)
		}
		if got.generation == nil || got.generation.Generation != 1 {
			t.Fatalf("final authority read returned %+v", got.generation)
		}
		if execution.deliveryBootstrap.Load() {
			t.Fatal("successful final authority read left bootstrap admission open")
		}
	})
}

func TestDeliveryStartupCancellationAndReplacement(t *testing.T) {
	t.Run("initialization failure releases pending work with its cause", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, attempt := deliveryStartupTestExecution(t)
			execution.DeliveryMode = DurableDeliveryV1
			repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
			attempt.finishNegotiation(nil)
			synctest.Wait()
			attempt.fail(errDeliveryStartupInitialization)
			if err := <-result; !errors.Is(err, errDeliveryStartupInitialization) {
				t.Fatalf("initialization failure surfaced as %v", err)
			}
			if repository.receipts.Load() != 0 {
				t.Fatal("initialization failure admitted delivery evidence")
			}
		})
	})

	t.Run("caller deadline terminates its own attempt", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, _ := retentionBootstrapOwner()
			startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			execution.beginDeliveryStartupAttempt(startupCtx)
			execution.DeliveryMode = DurableDeliveryV1
			repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
			time.Sleep(11 * time.Second)
			if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("caller deadline surfaced as %v", err)
			}
			if repository.receipts.Load() != 0 {
				t.Fatal("caller deadline admitted delivery evidence")
			}
		})
	})

	t.Run("stream cancellation releases its own attempt", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, attempt := deliveryStartupTestExecution(t)
			execution.DeliveryMode = DurableDeliveryV1
			attempt.finishNegotiation(nil)
			repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			streamCtx, cancel := context.WithCancel(context.Background())
			result := deliveryStartupTestPrepare(t, manager, streamCtx, execution, repository)
			synctest.Wait()
			cancel()
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("stream cancellation surfaced as %v", err)
			}
			if repository.receipts.Load() != 0 {
				t.Fatal("stream cancellation admitted delivery evidence")
			}
		})
	})

	t.Run("manager shutdown releases its own attempt", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, attempt := deliveryStartupTestExecution(t)
			execution.DeliveryMode = DurableDeliveryV1
			attempt.finishNegotiation(nil)
			repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			stopCh := make(chan struct{})
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, stopCh)
			result := deliveryStartupTestPrepare(t, manager, manager.streamContext(execution), execution, repository)
			synctest.Wait()
			close(stopCh)
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("manager shutdown surfaced as %v", err)
			}
			manager.Wait()
			if repository.receipts.Load() != 0 {
				t.Fatal("manager shutdown admitted delivery evidence")
			}
		})
	})

	t.Run("stale completion cannot release a replacement attempt", func(t *testing.T) {
		execution, _ := retentionBootstrapOwner()
		execution.beginStartupAttempt()
		previous := execution.beginDeliveryStartupAttempt(context.Background())
		execution.beginStartupAttempt()
		replacement := execution.beginDeliveryStartupAttempt(context.Background())
		execution.deliveryBootstrap.Store(true)
		previous.finishNative(nil)
		previous.fail(errDeliveryStartupInitialization)
		select {
		case <-replacement.nativeDone:
			t.Fatal("stale completion released the replacement attempt")
		default:
		}
		execution.clearDeliveryBootstrapForAttempt(previous)
		if !execution.deliveryBootstrap.Load() {
			t.Fatal("stale attempt cleared the replacement bootstrap admission")
		}
		execution.releaseDeliveryStartupAttempt(previous)
		if execution.deliveryStartupSnapshot() != replacement {
			t.Fatal("stale release cleared the replacement attempt")
		}
		execution.clearDeliveryBootstrapForAttempt(replacement)
		if execution.deliveryBootstrap.Load() {
			t.Fatal("current attempt did not clear bootstrap admission")
		}
	})

	t.Run("successor advancing during reset keeps its bootstrap wait", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, _ := retentionBootstrapOwner()
			execution.beginStartupAttempt()
			previous := execution.beginDeliveryStartupAttempt(context.Background())
			execution.deliveryBootstrap.Store(true)

			observed := make(chan struct{})
			release := make(chan struct{})
			resetDone := make(chan struct{})
			go func() {
				defer close(resetDone)
				// The reset callback runs inside the same ownership lease that
				// clearDeliveryBootstrapForAttempt uses, so the successor cannot
				// advance between observation and mutation.
				execution.applyDeliveryBootstrapReset(previous, func() {
					close(observed)
					<-release
					execution.deliveryBootstrap.Store(false)
				})
			}()
			<-observed
			if execution.startupCallbackMu.TryLock() {
				execution.startupCallbackMu.Unlock()
				close(release)
				t.Fatal("bootstrap reset did not hold the startup ownership lease")
			}
			advanced := make(chan struct{})
			go func() {
				execution.beginStartupAttempt()
				execution.deliveryBootstrap.Store(true)
				close(advanced)
			}()
			close(release)
			<-resetDone
			<-advanced
			if !execution.deliveryBootstrap.Load() {
				t.Fatal("old reset cleared the successor bootstrap flag")
			}

			// The replacement must still wait for its own native result and
			// then its own SQL authority.
			replacement := execution.beginDeliveryStartupAttempt(context.Background())
			replacement.finishNegotiation(nil)
			execution.DeliveryMode = DurableDeliveryV1
			repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
			synctest.Wait()
			select {
			case <-repository.firstRead:
				t.Fatal("replacement read SQL authority before its native result")
			default:
			}
			replacement.finishNative(nil)
			<-repository.firstRead
			repository.generation.Store(retentionBootstrapGeneration())
			if err := <-result; err != nil {
				t.Fatal(err)
			}
			if repository.receipts.Load() != 1 {
				t.Fatalf("replacement admitted %d events after its own authority", repository.receipts.Load())
			}
		})
	})

	t.Run("startup cleanup does not cancel a later authority commit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, _ := retentionBootstrapOwner()
			startupCtx, cancel := context.WithCancel(context.Background())
			attempt := execution.beginDeliveryStartupAttempt(startupCtx)
			execution.DeliveryMode = DurableDeliveryV1
			attempt.finishNegotiation(nil)
			attempt.finishNative(nil)
			cancel()
			repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
			<-repository.firstRead
			time.Sleep(5 * time.Second)
			repository.generation.Store(retentionBootstrapGeneration())
			if err := <-result; err != nil {
				t.Fatalf("late authority commit after startup cleanup: %v", err)
			}
			if repository.receipts.Load() != 1 {
				t.Fatalf("late commit evidence: receipts=%d", repository.receipts.Load())
			}
		})
	})

	t.Run("authority committed after the budget is never admitted", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			execution, attempt := deliveryStartupTestExecution(t)
			execution.DeliveryMode = DurableDeliveryV1
			attempt.finishNegotiation(nil)
			repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			result := deliveryStartupTestPrepare(t, manager, context.Background(), execution, repository)
			attempt.finishNative(nil)
			<-repository.firstRead
			time.Sleep(31 * time.Second)
			repository.generation.Store(retentionBootstrapGeneration())
			if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("late authority error = %v", err)
			}
			if repository.receipts.Load() != 0 || repository.projections.Load() != 0 {
				t.Fatal("authority committed after the budget admitted delivery evidence")
			}
		})
	})
}
