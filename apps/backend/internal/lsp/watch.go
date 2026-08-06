package lsp

import (
	"context"
	"errors"
	"time"
)

var recoveryBackoffs = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second}

const (
	readyRecoveryReset  = 5 * time.Minute
	watchReconnectDelay = time.Second
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
	attempts   int
	timer      ScheduledTimer
	timerEpoch uint64
	readyTimer ScheduledTimer
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
	c.lifecycleCtx = workerCtx
	c.lifecycleCancel = cancel
	c.lifecycleWG.Add(1)
	c.lifecycleMu.Unlock()

	settings, settingsErr := c.loadSettings(workerCtx)
	if settingsErr == nil {
		c.rememberSettings(settings)
	}
	go c.runSettingsWorker(workerCtx)
	reconcileErr := c.ReconcileAll(workerCtx)
	states, listErr := c.store.ListAllTaskLSPLanguages(workerCtx)
	if listErr == nil {
		for _, state := range states {
			if phaseHasServer(state.Phase) && state.Generation > 0 {
				c.ensureWatch(TaskLanguageKey{TaskID: state.TaskID, Language: state.Language})
			}
		}
	}
	return errors.Join(settingsErr, reconcileErr, listErr)
}

func (c *Controller) Close(ctx context.Context) error {
	c.lifecycleMu.Lock()
	cancel := c.lifecycleCancel
	if cancel == nil {
		c.lifecycleMu.Unlock()
		return nil
	}
	c.lifecycleCancel = nil
	for key, watchCancel := range c.watches {
		watchCancel()
		delete(c.watches, key)
	}
	for key, recovery := range c.recoveries {
		stopRecoveryTimers(recovery)
		delete(c.recoveries, key)
	}
	c.lifecycleMu.Unlock()
	cancel()

	done := make(chan struct{})
	go func() {
		c.lifecycleWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		c.lifecycleMu.Lock()
		c.lifecycleCtx = nil
		c.lifecycleMu.Unlock()
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (c *Controller) ensureWatch(key TaskLanguageKey) {
	c.lifecycleMu.Lock()
	if c.lifecycleCtx == nil || c.lifecycleCtx.Err() != nil || c.watches[key] != nil {
		c.lifecycleMu.Unlock()
		return
	}
	watchCtx, cancel := context.WithCancel(c.lifecycleCtx)
	c.watches[key] = cancel
	c.lifecycleWG.Add(1)
	c.lifecycleMu.Unlock()
	go c.watchTaskLanguage(watchCtx, key)
}

func (c *Controller) watchTaskLanguage(ctx context.Context, key TaskLanguageKey) {
	defer c.lifecycleWG.Done()
	defer func() {
		c.lifecycleMu.Lock()
		delete(c.watches, key)
		c.lifecycleMu.Unlock()
	}()
	for {
		host, err := c.resolveExistingHost(ctx, key.TaskID)
		if err == nil && host != nil {
			err = host.WatchTaskLSP(ctx, key.Language, func(snapshot RuntimeSnapshot) error {
				return c.observeRuntimeSnapshot(ctx, key, snapshot)
			})
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			state, _, stateErr := c.store.GetTaskLSPLanguage(ctx, key.TaskID, key.Language)
			if stateErr == nil {
				settings, _ := c.loadSettings(ctx)
				_, _ = c.transition(ctx, *state, settings, PhaseError, "task_host_watch_lost", err.Error())
			}
		}
		timer := time.NewTimer(watchReconnectDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func (c *Controller) resolveExistingHost(ctx context.Context, taskID string) (TaskHost, error) {
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !readyTaskEnvironment(environment) || !ExecutorSupportsLSP(environment.ExecutorType) {
		return nil, ErrTaskNotReady
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, environment.ID)
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
	if runtime.Phase == PhaseOff {
		c.releaseCapacity(ctx, key, runtime.Generation)
	}
	stored, err := c.adoptRuntime(ctx, *state, runtime)
	if err != nil {
		return err
	}
	if runtimeHasProcess(&runtime) {
		c.capacity.Adopt(key, runtime.Generation)
	}
	settings, settingsErr := c.loadSettings(ctx)
	if settingsErr != nil {
		return settingsErr
	}
	desired := effectivePolicy(*stored, settings) == PolicyKeepWarm
	switch runtime.Phase {
	case PhaseReady:
		c.scheduleReadyReset(key, runtime.Generation)
	case PhaseError, PhaseOff:
		if desired {
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
	recovery.timer = scheduler.AfterFunc(delay, func() { c.runRecovery(key, epoch) })
}

func (c *Controller) runRecovery(key TaskLanguageKey, epoch uint64) {
	c.lifecycleMu.Lock()
	recovery := c.recoveries[key]
	if recovery == nil || recovery.timer == nil || recovery.timerEpoch != epoch ||
		c.lifecycleCtx == nil || c.lifecycleCtx.Err() != nil {
		c.lifecycleMu.Unlock()
		return
	}
	recovery.timer = nil
	ctx := c.lifecycleCtx
	c.lifecycleMu.Unlock()

	state, _, err := c.store.GetTaskLSPLanguage(ctx, key.TaskID, key.Language)
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
	candidate, err := c.inspectReconcileState(ctx, state)
	if err != nil || candidate == nil {
		return false
	}
	snapshot, err := c.commands.submit(
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
	lifecycleCtx := c.lifecycleCtx
	recovery.readyTimer = scheduler.AfterFunc(readyRecoveryReset, func() {
		state, _, err := c.store.GetTaskLSPLanguage(lifecycleCtx, key.TaskID, key.Language)
		c.lifecycleMu.Lock()
		defer c.lifecycleMu.Unlock()
		current := c.recoveries[key]
		if err == nil && current != nil && state.Generation == generation && state.Phase == PhaseReady {
			current.attempts = 0
			current.readyTimer = nil
		}
	})
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
	if cancel := c.watches[key]; cancel != nil {
		cancel()
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
	settings, err := c.loadSettings(ctx)
	if err != nil {
		return
	}
	snapshot := c.languageSnapshot(state, settings, runtime)
	_ = c.publisher.PublishTaskLSP(context.WithoutCancel(ctx), snapshot)
}

func (s LanguageSnapshot) GetTaskID() string { return s.TaskID }
