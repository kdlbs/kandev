package lsp

import (
	"context"
	"errors"
	"fmt"
	"sort"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type reconcileCandidate struct {
	state    TaskLanguageState
	settings TaskSettings
}

// ReconcileAll adopts every proven-live task-host generation before it admits
// any missing desired server. This ordering rebuilds real capacity without a
// backend restart creating duplicate imports.
func (c *Controller) ReconcileAll(ctx context.Context) error {
	states, err := c.store.ListAllTaskLSPLanguages(ctx)
	if err != nil {
		return err
	}
	sort.Slice(states, func(i, j int) bool {
		if states[i].TaskID != states[j].TaskID {
			return states[i].TaskID < states[j].TaskID
		}
		return states[i].Language < states[j].Language
	})
	missing := make([]reconcileCandidate, 0)
	var reconcileErrors []error
	for _, state := range states {
		candidate, inspectErr := c.inspectReconcileState(ctx, state, true)
		if inspectErr != nil {
			reconcileErrors = append(reconcileErrors, inspectErr)
			continue
		}
		if candidate != nil {
			missing = append(missing, *candidate)
		}
	}
	for _, candidate := range missing {
		if _, startErr := c.reconcileMissing(ctx, candidate); startErr != nil {
			reconcileErrors = append(reconcileErrors, startErr)
		}
	}
	return errors.Join(reconcileErrors...)
}

// ReconcileTask converges durable languages for one real task. It is an
// internal lifecycle hook: callers resolve the task record rather than
// accepting an agent-selected ownership identifier.
func (c *Controller) ReconcileTask(ctx context.Context, taskID string) error {
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return err
	}
	discoveryErr := c.discoverTask(ctx, taskID, false)
	states, err := c.store.ListTaskLSPLanguages(ctx, taskID)
	if err != nil {
		return errors.Join(discoveryErr, err)
	}
	var reconcileErrors []error
	for _, state := range states {
		key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
		_, commandErr := c.commands.submit(ctx, key, ActionReconcile, "",
			func(workCtx context.Context) (*LanguageSnapshot, error) {
				candidate, inspectErr := c.inspectReconcileState(workCtx, state, true)
				if inspectErr != nil || candidate == nil {
					return nil, inspectErr
				}
				return c.reconcileMissing(workCtx, *candidate)
			})
		if commandErr != nil {
			reconcileErrors = append(reconcileErrors, commandErr)
		}
	}
	return errors.Join(discoveryErr, errors.Join(reconcileErrors...))
}

func (c *Controller) inspectReconcileState(
	ctx context.Context,
	state TaskLanguageState,
	scheduleFailure bool,
) (*reconcileCandidate, error) {
	releaseAdmission, err := c.tasks.AcquireTaskLSPAdmission(ctx, state.TaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTaskNotReady, err)
	}
	defer releaseAdmission()
	task, err := c.tasks.GetTask(ctx, state.TaskID)
	if err != nil {
		return nil, err
	}
	settings, err := c.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	desired := effectivePolicy(state, settings) == PolicyKeepWarm && task.ArchivedAt == nil
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, state.TaskID)
	if err != nil {
		return nil, err
	}
	if handled, stateErr := c.reconcileEnvironmentState(ctx, state, settings, desired, environment); handled {
		return nil, stateErr
	}
	return c.inspectReconcileRuntime(ctx, state, settings, environment, desired, scheduleFailure)
}

func (c *Controller) inspectReconcileRuntime(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	environment *taskmodels.TaskEnvironment,
	desired bool,
	scheduleFailure bool,
) (*reconcileCandidate, error) {
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, environment.ID)
	if err != nil {
		c.recordReconcileHostFailure(ctx, state, settings, desired, scheduleFailure, err)
		return nil, fmt.Errorf("inspect task host for %s/%s: %w", state.TaskID, state.Language, err)
	}
	if !exists || host == nil {
		return c.inspectAbsentTaskHost(ctx, state, settings, environment, desired, scheduleFailure)
	}
	runtime, err := host.TaskLSPSnapshot(ctx, state.Language)
	if err != nil {
		c.recordReconcileHostFailure(ctx, state, settings, desired, scheduleFailure, err)
		return nil, fmt.Errorf("snapshot task host for %s/%s: %w", state.TaskID, state.Language, err)
	}
	if !runtimeHasProcess(runtime) {
		return c.reconcileAbsentRuntime(ctx, state, settings, desired)
	}
	return c.reconcileLiveRuntime(ctx, state, settings, desired, host, *runtime)
}

func (c *Controller) inspectAbsentTaskHost(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	environment *taskmodels.TaskEnvironment,
	desired bool,
	scheduleFailure bool,
) (*reconcileCandidate, error) {
	if !desired || state.Generation == 0 {
		return c.reconcileAbsentRuntime(ctx, state, settings, desired)
	}
	_, adopted, err := c.adoptExistingRuntime(
		ctx, state, settings, environment, ActionReconcile,
		Origin{Initiator: InitiatorAutomatic, Reason: "reconcile_existing_runtime"},
	)
	if err != nil {
		c.recordReconcileHostFailure(ctx, state, settings, desired, scheduleFailure, err)
		return nil, err
	}
	if adopted {
		return nil, nil
	}
	return c.reconcileAbsentRuntime(ctx, state, settings, desired)
}

func (c *Controller) recordReconcileHostFailure(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	desired bool,
	scheduleFailure bool,
	cause error,
) {
	snapshot, _ := c.transition(ctx, state, settings, PhaseError, "task_host_unreachable", cause.Error())
	if scheduleFailure && desired {
		c.scheduleDesiredRecovery(TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}, snapshot)
	}
}

func (c *Controller) adoptExistingRuntime(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	environment *taskmodels.TaskEnvironment,
	action Action,
	origin Origin,
) (*LanguageSnapshot, bool, error) {
	host, err := c.runtimes.EnsureTaskHost(ctx, environment.ID)
	if err != nil {
		return nil, false, err
	}
	if host == nil {
		return nil, false, errors.New("task host ensure returned no host")
	}
	runtime, err := host.TaskLSPSnapshot(ctx, state.Language)
	if err != nil {
		return nil, false, fmt.Errorf("snapshot reconnected task host for %s/%s: %w",
			state.TaskID, state.Language, err)
	}
	if !runtimeHasProcess(runtime) {
		return nil, false, nil
	}
	stored, err := c.adoptRuntime(ctx, state, *runtime)
	if err != nil {
		return nil, false, err
	}
	if action == ActionStart {
		stored, err = c.recordExplicitStartIntent(ctx, *stored, origin)
		if err != nil {
			return nil, false, err
		}
	}
	key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
	c.capacity.Adopt(key, runtime.Generation)
	c.ensureWatch(key)
	snapshot := c.languageSnapshot(*stored, settings, runtime)
	return &snapshot, true, nil
}

func (c *Controller) reconcileEnvironmentState(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	desired bool,
	environment *taskmodels.TaskEnvironment,
) (bool, error) {
	if !readyTaskEnvironment(environment) {
		if desired && state.Phase != PhaseWaitingForTask {
			_, err := c.transition(ctx, state, settings, PhaseWaitingForTask, "", "")
			return true, err
		}
		return true, nil
	}
	if ExecutorSupportsLSP(environment.ExecutorType) {
		return false, nil
	}
	if desired && (state.Phase != PhaseUnsupported || state.ErrorCode != "unsupported_executor") {
		_, err := c.transition(ctx, state, settings, PhaseUnsupported, "unsupported_executor", "")
		return true, err
	}
	return true, nil
}

func (c *Controller) reconcileAbsentRuntime(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	desired bool,
) (*reconcileCandidate, error) {
	if desired {
		return &reconcileCandidate{state: state, settings: settings}, nil
	}
	if state.Phase == PhaseOff && state.ErrorCode == "" {
		return nil, nil
	}
	_, err := c.transition(ctx, state, settings, PhaseOff, "", "")
	return nil, err
}

func (c *Controller) reconcileLiveRuntime(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	desired bool,
	host TaskHost,
	runtime RuntimeSnapshot,
) (*reconcileCandidate, error) {
	if !desired {
		return nil, c.stopReconciledRuntime(ctx, state, settings, host, runtime.Generation, "policy_disabled")
	}
	if runtime.Generation < state.Generation {
		if err := c.stopReconciledRuntime(ctx, state, settings, host, runtime.Generation, "stale_generation"); err != nil {
			return nil, err
		}
		return &reconcileCandidate{state: state, settings: settings}, nil
	}
	if _, err := c.adoptRuntime(ctx, state, runtime); err != nil {
		return nil, err
	}
	key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
	c.capacity.Adopt(key, runtime.Generation)
	c.ensureWatch(key)
	return nil, nil
}

func (c *Controller) reconcileMissing(
	ctx context.Context,
	candidate reconcileCandidate,
) (*LanguageSnapshot, error) {
	origin := Origin{Initiator: InitiatorAutomatic, Reason: "reconcile_missing_runtime"}
	return c.start(ctx, candidate.state.TaskID, candidate.state.Language, origin, ActionReconcile)
}

func (c *Controller) adoptRuntime(
	ctx context.Context,
	state TaskLanguageState,
	runtime RuntimeSnapshot,
) (*TaskLanguageState, error) {
	return c.updateStateWithRuntime(ctx, state.TaskID, state.Language, &runtime, func(next *TaskLanguageState) {
		if runtime.Generation < next.Generation {
			return
		}
		next.Generation = runtime.Generation
		next.Phase = runtime.Phase
		next.ProcessStartedAt = runtime.ProcessStartedAt
		next.InitializeStartedAt = runtime.InitializeStartedAt
		next.ReadyAt = runtime.ReadyAt
		next.LastTransitionAt = runtime.LastTransitionAt
		next.ErrorCode = runtime.ErrorCode
		next.ErrorMessage = runtime.ErrorMessage
		next.LastAction = ActionReconcile
		next.LastInitiator = InitiatorAutomatic
	})
}

func (c *Controller) stopReconciledRuntime(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	host TaskHost,
	generation uint64,
	reason string,
) error {
	runtime, err := host.StopTaskLSP(ctx, TaskHostStopRequest{
		Language: state.Language, Generation: generation, Reason: reason,
	})
	if err != nil {
		_, _ = c.transition(ctx, state, settings, PhaseError, "task_host_stop_failed", err.Error())
		return err
	}
	c.releaseCapacity(
		ctx,
		TaskLanguageKey{TaskID: state.TaskID, Language: state.Language},
		generation,
	)
	_, err = c.updateState(ctx, state.TaskID, state.Language, func(next *TaskLanguageState) {
		next.Phase = PhaseOff
		if runtime != nil {
			next.Generation = runtime.Generation
		}
		next.LastAction = ActionReconcile
		next.LastInitiator = InitiatorAutomatic
		next.LastStopReason = reason
		next.LastTransitionAt = c.clock()
		next.ErrorCode = ""
		next.ErrorMessage = ""
	})
	return err
}

// CleanupTask is the task-lifecycle ownership hook. It cancels task-level
// recovery and reaps every language before the environment cleanup backstop;
// policy/history remain durable for archive/stop resume.
func (c *Controller) CleanupTask(ctx context.Context, taskID, reason string) error {
	states, err := c.store.ListTaskLSPLanguages(ctx, taskID)
	if err != nil {
		return err
	}
	// Local task-owned work must stop even when the environment record is
	// temporarily unavailable. Otherwise recovery can relaunch after teardown.
	for _, state := range states {
		key := TaskLanguageKey{TaskID: taskID, Language: state.Language}
		c.cancelRecovery(key)
		c.cancelWatch(key)
		c.capacity.CancelQueued(key)
	}
	environment, envErr := c.tasks.GetTaskEnvironmentByTaskID(ctx, taskID)
	if envErr != nil {
		return envErr
	}
	cleanupErrors := make([]error, 0, len(states)+1)
	stopAttempts := make(chan struct{}, len(states))
	commandResults := make(chan error, len(states))
	taskHostDone := make(chan struct{})
	var taskHostCleanupErr error
	// Each language lane stays held from its refreshed generation snapshot
	// through the task-host backstop. A Start/Restart accepted on either side
	// of cleanup therefore cannot be mistaken for the generation that cleanup
	// actually proved dead.
	for _, listed := range states {
		listed := listed
		go func() {
			key := TaskLanguageKey{TaskID: taskID, Language: listed.Language}
			_, commandErr := c.commands.submitExclusive(
				context.WithoutCancel(ctx), key, ActionReconcile,
				func(workCtx context.Context) (*LanguageSnapshot, error) {
					current, _, currentErr := c.store.GetTaskLSPLanguage(
						workCtx, taskID, listed.Language,
					)
					result := taskLanguageCleanupResult{state: listed, err: currentErr}
					if currentErr == nil {
						host, hostExists, hostErr := c.cleanupTaskHost(workCtx, environment)
						result = c.cleanupTaskLanguage(
							workCtx, *current, host, hostExists, hostErr, reason,
						)
					}
					stopAttempts <- struct{}{}
					<-taskHostDone
					return nil, c.finalizeTaskLanguageCleanup(
						workCtx, result, taskHostCleanupErr, reason,
					)
				},
			)
			commandResults <- commandErr
		}()
	}
	for range states {
		<-stopAttempts
	}
	if environment != nil && ExecutorSupportsLSP(environment.ExecutorType) {
		taskHostCleanupErr = c.runtimes.CleanupTaskHost(ctx, environment.ID, reason)
	}
	close(taskHostDone)
	for range states {
		cleanupErrors = append(cleanupErrors, <-commandResults)
	}
	cleanupErrors = append(cleanupErrors, taskHostCleanupErr)
	return errors.Join(cleanupErrors...)
}

type taskLanguageCleanupResult struct {
	state       TaskLanguageState
	processGone bool
	err         error
}

func (c *Controller) cleanupTaskHost(
	ctx context.Context,
	environment *taskmodels.TaskEnvironment,
) (TaskHost, bool, error) {
	if environment == nil || !ExecutorSupportsLSP(environment.ExecutorType) {
		return nil, false, nil
	}
	return c.runtimes.ExistingTaskHost(ctx, environment.ID)
}

func (c *Controller) cleanupTaskLanguage(
	ctx context.Context,
	state TaskLanguageState,
	host TaskHost,
	hostExists bool,
	hostErr error,
	reason string,
) taskLanguageCleanupResult {
	if hostErr != nil {
		return taskLanguageCleanupResult{
			state: state,
			err:   c.recordTaskLanguageCleanupFailure(ctx, state, reason, "task_host_unreachable", hostErr),
		}
	}
	if hostExists && host != nil && state.Generation > 0 {
		_, stopErr := host.StopTaskLSP(ctx, TaskHostStopRequest{
			Language: state.Language, Generation: state.Generation, Reason: reason,
		})
		if stopErr != nil {
			return taskLanguageCleanupResult{
				state: state,
				err: c.recordTaskLanguageCleanupFailure(
					ctx, state, reason, "task_host_stop_failed", stopErr,
				),
			}
		}
	}
	return taskLanguageCleanupResult{
		state:       state,
		processGone: true,
	}
}

func (c *Controller) finalizeTaskLanguageCleanup(
	ctx context.Context,
	result taskLanguageCleanupResult,
	taskHostCleanupErr error,
	reason string,
) error {
	if !result.processGone && taskHostCleanupErr != nil {
		return result.err
	}
	current, _, err := c.store.GetTaskLSPLanguage(ctx, result.state.TaskID, result.state.Language)
	if err != nil {
		return err
	}
	return c.finishTaskLanguageCleanup(ctx, *current, reason)
}

func (c *Controller) finishTaskLanguageCleanup(
	ctx context.Context,
	state TaskLanguageState,
	reason string,
) error {
	c.releaseCapacity(ctx, TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}, state.Generation)
	_, updateErr := c.updateState(ctx, state.TaskID, state.Language, func(next *TaskLanguageState) {
		next.Phase = PhaseOff
		next.LastAction = ActionReconcile
		next.LastInitiator = InitiatorAutomatic
		next.LastStopReason = reason
		next.LastTransitionAt = c.clock()
		next.ErrorCode = ""
		next.ErrorMessage = ""
	})
	return updateErr
}

func (c *Controller) recordTaskLanguageCleanupFailure(
	ctx context.Context,
	state TaskLanguageState,
	reason, errorCode string,
	cause error,
) error {
	_, updateErr := c.updateState(ctx, state.TaskID, state.Language, func(next *TaskLanguageState) {
		next.Phase = PhaseError
		next.LastAction = ActionReconcile
		next.LastInitiator = InitiatorAutomatic
		next.LastStopReason = reason
		next.LastTransitionAt = c.clock()
		next.ErrorCode = errorCode
		next.ErrorMessage = cause.Error()
	})
	return errors.Join(cause, updateErr)
}

func runtimeHasProcess(snapshot *RuntimeSnapshot) bool {
	if snapshot == nil || snapshot.Generation == 0 {
		return false
	}
	switch snapshot.Phase {
	case PhaseInstalling, PhaseStarting, PhaseProcessStarted, PhaseInitializing, PhaseReady, PhaseStopping:
		return true
	default:
		return false
	}
}

func runtimeFailureProvesNoProcess(snapshot *RuntimeSnapshot, generation uint64) bool {
	if snapshot == nil || snapshot.Generation != generation {
		return false
	}
	if snapshot.Phase == PhaseOff {
		return true
	}
	if snapshot.Phase != PhaseError {
		return false
	}
	switch snapshot.ErrorCode {
	case "binary_unavailable", errorCodeProcessStartFailed, "start_canceled", errorCodeProcessExited:
		return true
	default:
		return false
	}
}
