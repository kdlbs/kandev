package lsp

import (
	"context"
	"errors"
	"time"
)

var recoveryBackoffs = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second}

const (
	readyRecoveryReset = 5 * time.Minute
)

type ScheduledTimer interface {
	Stop() bool
}

type Scheduler interface {
	AfterFunc(delay time.Duration, callback func()) ScheduledTimer
}

type StatePublisher interface {
	PublishTaskLSP(ctx context.Context, snapshot LanguageSnapshot) error
}

type realScheduler struct{}

func (realScheduler) AfterFunc(delay time.Duration, callback func()) ScheduledTimer {
	return time.AfterFunc(delay, callback)
}

type recoveryState struct {
	attempts        int
	timer           ScheduledTimer
	timerEpoch      uint64
	readyTimer      ScheduledTimer
	readyTimerEpoch uint64
}

type taskLanguageWatch struct {
	cancel context.CancelFunc
}

// StartReconciler owns startup adoption, task-host watches, and bounded
// recovery until Close. It is safe to call once.
func (c *Controller) StartReconciler(ctx context.Context) error {
	c.lifecycleMu.Lock()
	if c.lifecycleCtx != nil {
		c.lifecycleMu.Unlock()
		return errors.New("task LSP reconciler already started")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	startupReady := make(chan struct{})
	c.lifecycleCtx = workerCtx
	c.lifecycleCancel = cancel
	c.lifecycleDone = make(chan struct{})
	c.startupReady = startupReady
	c.startupComplete = false
	c.startupErr = nil
	c.lifecycleWG.Add(1)
	c.lifecycleMu.Unlock()
	go c.runSettingsWorker(workerCtx)

	settingsErr := c.rememberCurrentTaskSettings(workerCtx)
	states, inventoryReady, reconcileErr := c.reconcileAllWithInventory(workerCtx)
	if inventoryReady {
		c.completeStartup(states, nil)
	} else {
		c.completeStartup(nil, reconcileErr)
	}
	return errors.Join(settingsErr, reconcileErr)
}

// waitForStartup prevents controls from observing an empty capacity ledger
// while durable live generations are still being adopted. Controllers that
// have not started their reconciler retain their lightweight unit-test and
// embedding behavior.
func (c *Controller) waitForStartup(ctx context.Context) error {
	c.lifecycleMu.Lock()
	ready := c.startupReady
	lifecycleCtx := c.lifecycleCtx
	running := c.lifecycleCancel != nil
	c.lifecycleMu.Unlock()
	if ready == nil {
		return nil
	}
	if !running || lifecycleCtx == nil {
		return context.Canceled
	}
	select {
	case <-ready:
		if lifecycleCtx.Err() != nil {
			return context.Cause(lifecycleCtx)
		}
		c.lifecycleMu.Lock()
		startupErr := c.startupErr
		c.lifecycleMu.Unlock()
		return startupErr
	case <-lifecycleCtx.Done():
		return context.Cause(lifecycleCtx)
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (c *Controller) Close(ctx context.Context) error {
	c.lifecycleMu.Lock()
	cancel := c.lifecycleCancel
	done := c.lifecycleDone
	if cancel != nil {
		if done == nil {
			done = make(chan struct{})
			c.lifecycleDone = done
		}
		c.lifecycleCancel = nil
		cancel()
		for key, watch := range c.watches {
			watch.cancel()
			delete(c.watches, key)
		}
		for key, recovery := range c.recoveries {
			stopRecoveryTimers(recovery)
			delete(c.recoveries, key)
		}
		for taskID, retry := range c.discoveryRetries {
			if retry.timer != nil {
				retry.timer.Stop()
			}
			delete(c.discoveryRetries, taskID)
		}
		go c.finishLifecycle(done)
	}
	c.lifecycleMu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (c *Controller) finishLifecycle(done chan struct{}) {
	c.lifecycleWG.Wait()
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.lifecycleDone != done {
		return
	}
	c.lifecycleCtx = nil
	close(done)
}

func (c *Controller) completeStartup(states []TaskLanguageState, startupErr error) {
	for _, state := range states {
		if phaseHasServer(state.Phase) && state.Generation > 0 {
			c.ensureWatch(TaskLanguageKey{TaskID: state.TaskID, Language: state.Language})
		}
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.startupComplete || c.startupReady == nil || c.lifecycleCancel == nil ||
		c.lifecycleCtx == nil || c.lifecycleCtx.Err() != nil {
		return
	}
	c.startupErr = startupErr
	c.startupComplete = true
	close(c.startupReady)
}

func (c *Controller) beginLifecycleCallback() (context.Context, func(), bool) {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.lifecycleCtx == nil || c.lifecycleCtx.Err() != nil || c.lifecycleCancel == nil {
		return nil, nil, false
	}
	c.lifecycleWG.Add(1)
	return c.lifecycleCtx, c.lifecycleWG.Done, true
}

func (c *Controller) ensureWatch(key TaskLanguageKey) {
	c.lifecycleMu.Lock()
	if c.lifecycleCtx == nil || c.lifecycleCtx.Err() != nil || c.watches[key] != nil {
		c.lifecycleMu.Unlock()
		return
	}
	watchCtx, cancel := context.WithCancel(c.lifecycleCtx)
	watch := &taskLanguageWatch{cancel: cancel}
	c.watches[key] = watch
	c.lifecycleWG.Add(1)
	c.lifecycleMu.Unlock()
	go c.watchTaskLanguage(watchCtx, key, watch)
}

func (c *Controller) watchTaskLanguage(
	ctx context.Context,
	key TaskLanguageKey,
	watch *taskLanguageWatch,
) {
	defer c.lifecycleWG.Done()
	defer func() {
		c.lifecycleMu.Lock()
		if c.watches[key] == watch {
			delete(c.watches, key)
		}
		c.lifecycleMu.Unlock()
	}()
	host, err := c.resolveExistingHost(ctx, key.TaskID)
	if err == nil && host != nil {
		err = host.WatchTaskLSP(ctx, key.Language, func(snapshot RuntimeSnapshot) error {
			_, observeErr := c.commands.submitExclusive(
				ctx,
				key,
				ActionReconcile,
				func(workCtx context.Context) (*LanguageSnapshot, error) {
					if !c.watchIsCurrent(key, watch) {
						return nil, nil
					}
					return nil, c.observeRuntimeSnapshot(workCtx, key, snapshot)
				},
			)
			return observeErr
		})
		if err == nil {
			err = errors.New("task host watch ended unexpectedly")
		}
	}
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		snapshot, _ := c.recordWatchLoss(ctx, key, err)
		c.scheduleDesiredRecovery(key, snapshot)
	}
}

func (c *Controller) recordWatchLoss(
	ctx context.Context,
	key TaskLanguageKey,
	watchErr error,
) (*LanguageSnapshot, error) {
	for range 5 {
		state, _, err := c.store.GetTaskLSPLanguage(ctx, key.TaskID, key.Language)
		if err != nil {
			return nil, err
		}
		if !phaseHasServer(state.Phase) || state.Generation == 0 {
			return nil, nil
		}
		settings, err := c.loadSettings(ctx, key.TaskID)
		if err != nil {
			return nil, err
		}
		expected := state.Revision
		state.Phase = PhaseError
		state.ErrorCode = "task_host_watch_lost"
		state.ErrorMessage = watchErr.Error()
		state.LastTransitionAt = c.clock()
		updated, err := c.store.CompareAndUpdateTaskLSPLanguage(ctx, *state, expected)
		if errors.Is(err, ErrRevisionConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		c.publishState(ctx, *updated, nil)
		snapshot := c.languageSnapshot(*updated, settings, nil)
		return &snapshot, nil
	}
	return nil, ErrRevisionConflict
}

func (c *Controller) watchIsCurrent(key TaskLanguageKey, watch *taskLanguageWatch) bool {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	return c.lifecycleCtx != nil && c.lifecycleCtx.Err() == nil && c.watches[key] == watch
}

func (c *Controller) resolveExistingHost(ctx context.Context, taskID string) (TaskHost, error) {
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !taskAllowsLSPRuntime(task) {
		return nil, ErrTaskNotReady
	}
	environment, err := c.tasks.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !readyTaskEnvironment(environment) || !ExecutorSupportsLSP(environment.ExecutorType) {
		return nil, ErrTaskNotReady
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, taskID, environment.ID)
	if err != nil {
		return nil, err
	}
	if !exists || host == nil {
		return nil, ErrAttachmentNotReady
	}
	return host, nil
}

func (c *Controller) observeRuntimeSnapshot(
	ctx context.Context,
	key TaskLanguageKey,
	runtime RuntimeSnapshot,
) error {
	state, _, err := c.store.GetTaskLSPLanguage(ctx, key.TaskID, key.Language)
	if err != nil {
		return err
	}
	if runtime.Generation < state.Generation {
		return nil
	}
	stored, accepted, err := c.adoptRuntime(ctx, *state, runtime)
	if err != nil {
		return err
	}
	if !accepted {
		return nil
	}
	processGone := runtimeFailureProvesNoProcess(&runtime, runtime.Generation)
	if processGone {
		c.releaseCapacity(ctx, key, runtime.Generation)
	}
	if runtimeHasProcess(&runtime) {
		c.capacity.Adopt(key, runtime.Generation)
	}
	settings, settingsErr := c.loadSettings(ctx, key.TaskID)
	if settingsErr != nil {
		return settingsErr
	}
	desired := effectivePolicy(*stored, settings) == PolicyKeepWarm
	switch runtime.Phase {
	case PhaseReady:
		c.scheduleReadyReset(key, runtime.Generation)
	case PhaseError, PhaseOff:
		if desired && processGone {
			c.scheduleRecovery(key)
		}
	}
	return nil
}

func (c *Controller) scheduleRecovery(key TaskLanguageKey) {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.lifecycleCtx == nil || c.lifecycleCtx.Err() != nil {
		return
	}
	recovery := c.recoveries[key]
	if recovery == nil {
		recovery = &recoveryState{}
		c.recoveries[key] = recovery
	}
	if recovery.readyTimer != nil {
		recovery.readyTimer.Stop()
		recovery.readyTimer = nil
	}
	if recovery.timer != nil || recovery.attempts >= len(recoveryBackoffs) {
		return
	}
	delay := recoveryBackoffs[recovery.attempts]
	recovery.attempts++
	scheduler := c.scheduler
	if scheduler == nil {
		scheduler = realScheduler{}
	}
	recovery.timerEpoch++
	epoch := recovery.timerEpoch
	recovery.timer = scheduler.AfterFunc(delay, func() { c.runRecovery(key, recovery, epoch) })
}

func (c *Controller) runRecovery(
	key TaskLanguageKey,
	expected *recoveryState,
	epoch uint64,
) {
	ctx, done, ok := c.beginLifecycleCallback()
	if !ok {
		return
	}
	defer done()

	c.lifecycleMu.Lock()
	recovery := c.recoveries[key]
	if recovery != expected || recovery.timer == nil || recovery.timerEpoch != epoch ||
		c.lifecycleCtx != ctx || ctx.Err() != nil {
		c.lifecycleMu.Unlock()
		return
	}
	recovery.timer = nil
	c.lifecycleMu.Unlock()

	state, _, err := c.store.GetTaskLSPLanguage(ctx, key.TaskID, key.Language)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		c.scheduleRecovery(key)
		return
	}
	if c.attemptRecovery(ctx, key, *state) {
		return
	}
	c.scheduleRecovery(key)
}

func (c *Controller) attemptRecovery(
	ctx context.Context,
	key TaskLanguageKey,
	state TaskLanguageState,
) bool {
	candidate, err := c.inspectReconcileState(ctx, state, false)
	if err != nil {
		if !c.recoverDeadTaskHost(ctx, key.TaskID) {
			return false
		}
		candidate = &reconcileCandidate{state: state}
		settings, settingsErr := c.loadSettings(ctx, key.TaskID)
		if settingsErr != nil {
			return false
		}
		candidate.settings = settings
	}
	if candidate == nil {
		// A nil candidate is a successful convergence result: inspection either
		// adopted the live runtime or established a non-starting terminal state.
		// Do not spend another recovery attempt merely because no start command
		// was required.
		current, _, stateErr := c.store.GetTaskLSPLanguage(ctx, key.TaskID, key.Language)
		if stateErr == nil && current.Phase == PhaseReady {
			c.scheduleReadyReset(key, current.Generation)
		}
		return true
	}
	snapshot, err := c.commands.submitOwnedExclusive(
		ctx,
		key,
		ActionReconcile,
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.reconcileMissing(workCtx, *candidate)
		},
	)
	if err != nil || snapshot == nil {
		return false
	}
	c.ensureWatch(key)
	if snapshot.Phase == PhaseReady {
		c.scheduleReadyReset(key, snapshot.Generation)
		return true
	}
	return snapshot.Phase != PhaseError && snapshot.Phase != PhaseOff
}

func (c *Controller) recoverDeadTaskHost(ctx context.Context, taskID string) bool {
	environment, err := c.tasks.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil || !readyTaskEnvironment(environment) || !ExecutorSupportsLSP(environment.ExecutorType) {
		return false
	}
	recovered, err := c.runtimes.RecoverTaskHost(ctx, environment.ID)
	return err == nil && recovered
}

func (c *Controller) scheduleDesiredRecovery(key TaskLanguageKey, snapshot *LanguageSnapshot) {
	if snapshot != nil && snapshot.EffectivePolicy == PolicyKeepWarm {
		c.scheduleRecovery(key)
	}
}

func (c *Controller) scheduleReadyReset(key TaskLanguageKey, generation uint64) {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.lifecycleCtx == nil || c.lifecycleCtx.Err() != nil {
		return
	}
	recovery := c.recoveries[key]
	if recovery == nil {
		recovery = &recoveryState{}
		c.recoveries[key] = recovery
	}
	if recovery.timer != nil {
		recovery.timer.Stop()
		recovery.timer = nil
	}
	if recovery.readyTimer != nil {
		recovery.readyTimer.Stop()
	}
	scheduler := c.scheduler
	if scheduler == nil {
		scheduler = realScheduler{}
	}
	recovery.readyTimerEpoch++
	epoch := recovery.readyTimerEpoch
	recovery.readyTimer = scheduler.AfterFunc(readyRecoveryReset, func() {
		c.runReadyReset(key, recovery, generation, epoch)
	})
}

func (c *Controller) runReadyReset(
	key TaskLanguageKey,
	expected *recoveryState,
	generation, epoch uint64,
) {
	ctx, done, ok := c.beginLifecycleCallback()
	if !ok {
		return
	}
	defer done()

	c.lifecycleMu.Lock()
	current := c.recoveries[key]
	if current != expected || current.readyTimer == nil || current.readyTimerEpoch != epoch {
		c.lifecycleMu.Unlock()
		return
	}
	current.readyTimer = nil
	c.lifecycleMu.Unlock()

	state, _, err := c.store.GetTaskLSPLanguage(ctx, key.TaskID, key.Language)
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	current = c.recoveries[key]
	if err == nil && current == expected && current.readyTimerEpoch == epoch &&
		state.Generation == generation && state.Phase == PhaseReady {
		current.attempts = 0
	}
}

func (c *Controller) cancelRecovery(key TaskLanguageKey) {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if recovery := c.recoveries[key]; recovery != nil {
		stopRecoveryTimers(recovery)
		delete(c.recoveries, key)
	}
}

func (c *Controller) cancelWatch(key TaskLanguageKey) {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if watch := c.watches[key]; watch != nil {
		watch.cancel()
		delete(c.watches, key)
	}
}

func stopRecoveryTimers(recovery *recoveryState) {
	if recovery == nil {
		return
	}
	if recovery.timer != nil {
		recovery.timer.Stop()
	}
	if recovery.readyTimer != nil {
		recovery.readyTimer.Stop()
	}
}

func (c *Controller) publishState(ctx context.Context, state TaskLanguageState, runtime *RuntimeSnapshot) {
	if c.publisher == nil {
		return
	}
	settings, err := c.loadSettings(ctx, state.TaskID)
	if err != nil {
		return
	}
	snapshot := c.languageSnapshot(state, settings, runtime)
	_ = c.publisher.PublishTaskLSP(context.WithoutCancel(ctx), snapshot)
}

func (s LanguageSnapshot) GetTaskID() string { return s.TaskID }
