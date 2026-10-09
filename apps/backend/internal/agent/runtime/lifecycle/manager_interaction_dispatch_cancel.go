package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const dispatchCancelLockPollInterval = 5 * time.Millisecond

type capturedDispatchReadyState struct {
	completionAccepted bool
	ownershipLost      bool
	changed            bool
	payload            AgentEventPayload
}

type cancelPromptSnapshot struct {
	finished          <-chan struct{}
	prompt            promptLifecycleSnapshot
	dispatchOnly      bool
	ownershipConflict bool
}

func (m *Manager) captureCancelPromptSnapshot(execution *AgentExecution) cancelPromptSnapshot {
	execution.promptFinishedMu.Lock()
	finished := execution.promptFinished
	prompt, exists := m.executionStore.promptLifecycleSnapshot(execution.ID)
	execution.promptFinishedMu.Unlock()

	if !exists {
		return cancelPromptSnapshot{
			finished:          finished,
			ownershipConflict: execution.dispatchedPromptPending.Load(),
		}
	}
	if prompt.execution != execution {
		return cancelPromptSnapshot{finished: finished, ownershipConflict: true}
	}
	pending := execution.dispatchedPromptPending.Load()
	activePrompt := prompt.generation != 0
	dispatchOnly := activePrompt &&
		(pending || (!promptBarrierOpen(finished) && prompt.completedGeneration != prompt.generation))
	return cancelPromptSnapshot{
		finished:          finished,
		prompt:            prompt,
		dispatchOnly:      dispatchOnly,
		ownershipConflict: pending && !activePrompt,
	}
}

func promptBarrierOpen(finished <-chan struct{}) bool {
	if finished == nil {
		return false
	}
	select {
	case <-finished:
		return false
	default:
		return true
	}
}

func (m *Manager) cancelDispatchOnlyPrompt(
	ctx context.Context,
	execution *AgentExecution,
	snapshot cancelPromptSnapshot,
	immediate bool,
) error {
	if immediate {
		if !execution.promptMu.TryLock() {
			return m.escalateCapturedDispatchWithoutPromptLock(ctx, execution, snapshot)
		}
		defer execution.promptMu.Unlock()
		if !m.ownsCapturedPrompt(execution, snapshot.prompt.generation) {
			return ErrPromptActivityNotOwned
		}
		if m.promptCompletionAccepted(execution, snapshot.prompt.generation) {
			execution.dispatchedPromptPending.Store(false)
			return nil
		}
		return m.escalateStuckCancel(ctx, execution, snapshot.finished)
	}

	deadline := time.Now().Add(cancelWaitTimeout)
	locked, err := lockPromptUntil(ctx, &execution.promptMu, deadline)
	if err != nil {
		return err
	}
	if !locked {
		return m.escalateCapturedDispatchWithoutPromptLock(ctx, execution, snapshot)
	}
	defer execution.promptMu.Unlock()
	return m.waitForCapturedDispatchCompletion(ctx, execution, snapshot.prompt.generation, deadline)
}

func lockPromptUntil(ctx context.Context, mutex *sync.Mutex, deadline time.Time) (bool, error) {
	ticker := time.NewTicker(dispatchCancelLockPollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if mutex.TryLock() {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-timer.C:
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if mutex.TryLock() {
				return true, nil
			}
			return false, nil
		case <-ticker.C:
		}
	}
}

// escalateCapturedDispatchWithoutPromptLock releases a contended predecessor
// through its existing completion consumer without taking promptMu or consuming
// the shared completion channel itself. It takes the startup callback lease
// before promptLifecycleMu, matching the stream-completion lock order.
func (m *Manager) escalateCapturedDispatchWithoutPromptLock(
	ctx context.Context,
	execution *AgentExecution,
	snapshot cancelPromptSnapshot,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	execution.startupCallbackMu.RLock()
	defer execution.startupCallbackMu.RUnlock()
	startupGeneration := execution.startupAttemptSnapshot()
	attemptID := execution.currentStartupAttemptID()

	generation := snapshot.prompt.generation
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	current, exists := m.executionStore.promptLifecycleSnapshot(execution.ID)
	if !exists || current.execution != execution || current.generation != generation {
		return ErrPromptActivityNotOwned
	}
	if current.completedGeneration == generation {
		return nil
	}
	state, storeErr := m.prepareCapturedDispatchReady(execution, generation, attemptID)
	if state.ownershipLost {
		return ErrPromptActivityNotOwned
	}
	if storeErr != nil {
		return storeErr
	}
	if state.completionAccepted {
		return nil
	}
	if state.changed {
		persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), cancelEscalationTimeout)
		m.persistExecutorRunning(persistCtx, execution)
		cancelPersist()
		go m.eventPublisher.publishAgentEventPayload(context.Background(), events.AgentReady, state.payload)
	}

	// Keep the generation fence until the wake is queued. The existing consumer
	// receives before taking promptLifecycleMu, so this bounded send can release it.
	execution.dispatchedPromptPending.Store(true)
	signalQueued := sendPromptCompletionSignalBounded(
		execution,
		PromptCompletionSignal{
			IsError:           true,
			Error:             "cancel escalated: agent did not complete turn within timeout",
			PromptGeneration:  generation,
			StartupGeneration: startupGeneration,
		},
		cancelEscalationTimeout,
	)
	if !signalQueued {
		m.logger.Warn("dispatch cancellation escalation could not enqueue completion signal",
			zap.String("execution_id", execution.ID),
			zap.Uint64("prompt_generation", generation))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrCancelEscalated
}

func (m *Manager) prepareCapturedDispatchReady(
	execution *AgentExecution,
	generation uint64,
	attemptID string,
) (capturedDispatchReadyState, error) {
	state := capturedDispatchReadyState{}
	err := m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		if current != execution || current.promptGeneration != generation {
			state.ownershipLost = true
			return
		}
		if current.promptCompletionGeneration == generation {
			state.completionAccepted = true
			return
		}
		if current.Status == v1.AgentStatusReady {
			return
		}
		current.Status = v1.AgentStatusReady
		state.payload = newAgentEventPayloadWithTurnID(current, current.promptTurnID)
		state.payload.AttemptID = attemptID
		state.changed = true
	})
	return state, err
}

func sendPromptCompletionSignalBounded(
	execution *AgentExecution,
	signal PromptCompletionSignal,
	budget time.Duration,
) bool {
	if budget <= 0 {
		select {
		case execution.promptDoneCh <- signal:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(budget)
	defer timer.Stop()
	ticker := time.NewTicker(dispatchCancelLockPollInterval)
	defer ticker.Stop()
	for {
		select {
		case execution.promptDoneCh <- signal:
			return true
		case <-ticker.C:
		case <-timer.C:
			return false
		}
	}
}

func (m *Manager) waitForPromptFinishedAfterCancel(
	ctx context.Context,
	execution *AgentExecution,
	snapshot cancelPromptSnapshot,
) error {
	deadline := time.Now().Add(cancelWaitTimeout)
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-snapshot.finished:
		return m.validatePromptAfterBarrier(execution, snapshot.prompt.generation)
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		select {
		case <-snapshot.finished:
			return m.validatePromptAfterBarrier(execution, snapshot.prompt.generation)
		default:
		}
		if snapshot.prompt.generation != 0 && !m.ownsCapturedPrompt(execution, snapshot.prompt.generation) {
			return ErrPromptActivityNotOwned
		}
		return m.escalateStuckCancel(ctx, execution, snapshot.finished)
	}
}

func (m *Manager) validatePromptAfterBarrier(execution *AgentExecution, generation uint64) error {
	if generation == 0 {
		return nil
	}
	if !m.ownsCapturedPrompt(execution, generation) {
		return ErrPromptActivityNotOwned
	}
	return nil
}

func (m *Manager) ownsCapturedPrompt(execution *AgentExecution, generation uint64) bool {
	current, exists := m.executionStore.promptLifecycleSnapshot(execution.ID)
	return exists && current.execution == execution && current.generation == generation
}

func (m *Manager) waitForCapturedDispatchCompletion(
	ctx context.Context,
	execution *AgentExecution,
	generation uint64,
	deadline time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.confirmCapturedPromptOwnership(execution, generation); err != nil {
		return err
	}
	if m.promptCompletionAccepted(execution, generation) {
		execution.dispatchedPromptPending.Store(false)
		return nil
	}
	if !execution.dispatchedPromptPending.Load() {
		return m.escalateStuckCancel(ctx, execution, nil)
	}

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	var transportFailure error
	for {
		select {
		case signal := <-execution.promptDoneCh:
			if err := m.confirmCapturedPromptOwnership(execution, generation); err != nil {
				return err
			}
			if m.promptCompletionAccepted(execution, generation) {
				execution.dispatchedPromptPending.Store(false)
				return nil
			}
			if signal.PromptGeneration == generation && signal.IsError {
				transportFailure = errors.New(signal.Error)
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return m.finishDispatchCancelTimeout(ctx, execution, generation, transportFailure)
		}
	}
}

func (m *Manager) finishDispatchCancelTimeout(
	ctx context.Context,
	execution *AgentExecution,
	generation uint64,
	transportFailure error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.confirmCapturedPromptOwnership(execution, generation); err != nil {
		return err
	}
	if m.promptCompletionAccepted(execution, generation) {
		execution.dispatchedPromptPending.Store(false)
		return nil
	}
	if signal, ok := tryReceivePromptSignal(execution.promptDoneCh); ok {
		if signal.PromptGeneration == generation && signal.IsError && signal.Error != "" && transportFailure == nil {
			transportFailure = errors.New(signal.Error)
		}
		if m.promptCompletionAccepted(execution, generation) {
			execution.dispatchedPromptPending.Store(false)
			return nil
		}
	}
	escalationErr := m.escalateStuckCancel(ctx, execution, nil)
	if transportFailure == nil {
		return escalationErr
	}
	return errors.Join(escalationErr, fmt.Errorf("dispatch completion transport failure: %w", transportFailure))
}

func tryReceivePromptSignal(channel <-chan PromptCompletionSignal) (PromptCompletionSignal, bool) {
	select {
	case signal := <-channel:
		return signal, true
	default:
		return PromptCompletionSignal{}, false
	}
}

func (m *Manager) confirmCapturedPromptOwnership(execution *AgentExecution, generation uint64) error {
	if !m.ownsCapturedPrompt(execution, generation) {
		return ErrPromptActivityNotOwned
	}
	return nil
}

func (m *Manager) promptCompletionAccepted(execution *AgentExecution, generation uint64) bool {
	current, exists := m.executionStore.promptLifecycleSnapshot(execution.ID)
	return exists && current.execution == execution &&
		current.generation == generation && current.completedGeneration == generation
}
