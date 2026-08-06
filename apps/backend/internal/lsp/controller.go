package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/lsp/installer"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

var (
	ErrUnsupportedLanguage = errors.New("unsupported LSP language")
	ErrInvalidPolicy       = errors.New("invalid task LSP policy")
	ErrServerDisabled      = errors.New("task language server is disabled or off")
	ErrTaskNotReady        = errors.New("task environment is not ready")
	ErrExecutorUnsupported = errors.New("task executor does not support LSP")
	ErrAttachmentNotReady  = errors.New("task language server attachment is not ready")
)

const (
	errorCodeProcessExited      = "process_exited"
	errorCodeProcessStartFailed = "process_start_failed"
)

type Origin struct {
	Initiator Initiator
	Reason    string
}

type TaskSettings struct {
	AutoStartLanguages   []string
	AutoInstallLanguages []string
	ServerConfigs        map[string]json.RawMessage
}

type TaskService interface {
	AuthorizeTaskAccess(ctx context.Context, taskID string) error
	GetTask(ctx context.Context, taskID string) (*taskmodels.Task, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskEnvironment, error)
}

type SettingsProvider interface {
	TaskLSPSettings(ctx context.Context) (TaskSettings, error)
}

type TaskHost interface {
	DiscoverLSP(ctx context.Context) (*DiscoveryResult, error)
	RefreshTaskLSPWorkspace(ctx context.Context) (*WorkspaceUpdateResult, error)
	StartTaskLSP(ctx context.Context, request TaskHostStartRequest) (*RuntimeSnapshot, error)
	RestartTaskLSP(ctx context.Context, request TaskHostStartRequest) (*RuntimeSnapshot, error)
	UpdateTaskLSPConfiguration(
		ctx context.Context,
		request TaskHostConfigurationRequest,
	) (*RuntimeSnapshot, error)
	StopTaskLSP(ctx context.Context, request TaskHostStopRequest) (*RuntimeSnapshot, error)
	TaskLSPSnapshot(ctx context.Context, language string) (*RuntimeSnapshot, error)
	WatchTaskLSP(
		ctx context.Context,
		language string,
		onSnapshot func(RuntimeSnapshot) error,
	) error
	DialTaskLSPAttach(
		ctx context.Context,
		language string,
		generation uint64,
	) (*websocket.Conn, *http.Response, error)
}

type RuntimeProvider interface {
	EnsureTaskHost(ctx context.Context, taskEnvironmentID string) (TaskHost, error)
	ExistingTaskHost(ctx context.Context, taskEnvironmentID string) (TaskHost, bool, error)
	CleanupTaskHost(ctx context.Context, taskEnvironmentID, reason string) error
	DiscoverTaskLanguages(ctx context.Context, taskEnvironmentID string) (*DiscoveryResult, error)
}

type LanguageSnapshot struct {
	TaskLanguageState
	EffectivePolicy   Policy             `json:"effective_policy"`
	Activity          Activity           `json:"activity"`
	Progress          []WorkItem         `json:"progress"`
	LastCompletedWork *CompletedWorkItem `json:"last_completed_work,omitempty"`
	Capacity          CapacitySnapshot   `json:"capacity"`
}

type TaskSnapshot struct {
	TaskID    string             `json:"task_id"`
	Languages []LanguageSnapshot `json:"languages"`
	Capacity  CapacitySnapshot   `json:"capacity"`
}

type CapacitySnapshot struct {
	Active   int    `json:"active"`
	Queued   int    `json:"queued"`
	Limit    int    `json:"limit"`
	Epoch    string `json:"epoch"`
	Revision uint64 `json:"revision"`
}

type AttachmentTarget struct {
	Host       TaskHost
	Language   string
	Generation uint64
}

type ControllerConfig struct {
	Tasks     TaskService
	Store     Store
	Settings  SettingsProvider
	Runtimes  RuntimeProvider
	Capacity  *Capacity
	Clock     func() time.Time
	Scheduler Scheduler
	Publisher StatePublisher
}

type Controller struct {
	tasks     TaskService
	store     Store
	settings  SettingsProvider
	runtimes  RuntimeProvider
	capacity  *Capacity
	clock     func() time.Time
	commands  commandCoordinator
	scheduler Scheduler
	publisher StatePublisher

	lifecycleMu     sync.Mutex
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	lifecycleWG     sync.WaitGroup
	watches         map[TaskLanguageKey]context.CancelFunc
	recoveries      map[TaskLanguageKey]*recoveryState
	settingsApplyMu sync.Mutex
	appliedSettings *TaskSettings
	settingsSignal  chan struct{}
	discoveryMu     sync.Mutex
	discoveries     map[string]*discoveryCall
}

func NewController(config ControllerConfig) *Controller {
	capacity := config.Capacity
	if capacity == nil {
		capacity = NewCapacityFromEnv()
	}
	clock := config.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Controller{
		tasks: config.Tasks, store: config.Store, settings: config.Settings,
		runtimes: config.Runtimes, capacity: capacity, clock: clock,
		scheduler: config.Scheduler, publisher: config.Publisher,
		watches:        make(map[TaskLanguageKey]context.CancelFunc),
		recoveries:     make(map[TaskLanguageKey]*recoveryState),
		settingsSignal: make(chan struct{}, 1),
		discoveries:    make(map[string]*discoveryCall),
	}
}

func (c *Controller) Snapshot(ctx context.Context, taskID string) (*TaskSnapshot, error) {
	if err := c.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	// Read-only, bounded discovery is opportunistic and never creates a task
	// host. A failed/absent runtime does not hide the controller.
	_ = c.discoverTask(ctx, taskID, false)
	// Converge already-persisted task policy after reload/resume. Errors are
	// represented on the language rows so the status surface remains usable.
	_ = c.ReconcileTask(ctx, taskID)
	settings, err := c.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	states, err := c.store.ListTaskLSPLanguages(ctx, taskID)
	if err != nil {
		return nil, err
	}
	byLanguage := make(map[string]TaskLanguageState, len(states))
	for _, state := range states {
		if installer.IsSupported(state.Language) {
			byLanguage[state.Language] = state
		}
	}
	runtimeByLanguage := c.liveRuntimeSnapshots(ctx, taskID, states)
	languages := registeredLanguages()
	result := &TaskSnapshot{
		TaskID:    taskID,
		Languages: make([]LanguageSnapshot, 0, len(languages)),
		Capacity:  c.capacity.Snapshot(),
	}
	for _, language := range languages {
		state, ok := byLanguage[language]
		if !ok {
			state = DefaultTaskLanguageState(taskID, language)
		}
		var runtime *RuntimeSnapshot
		if live, ok := runtimeByLanguage[language]; ok {
			runtime = &live
		}
		result.Languages = append(result.Languages, c.languageSnapshot(state, settings, runtime))
	}
	return result, nil
}

// liveRuntimeSnapshots overlays generation-scoped task-host evidence for REST
// hydration. It never creates or resumes a task host; reconnecting browsers
// must see the same progress without becoming lifecycle owners.
func (c *Controller) liveRuntimeSnapshots(
	ctx context.Context,
	taskID string,
	states []TaskLanguageState,
) map[string]RuntimeSnapshot {
	result := make(map[string]RuntimeSnapshot)
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || !readyTaskEnvironment(environment) || !ExecutorSupportsLSP(environment.ExecutorType) {
		return result
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, environment.ID)
	if err != nil || !exists || host == nil {
		return result
	}
	for _, state := range states {
		if state.Generation == 0 || !phaseHasServer(state.Phase) {
			continue
		}
		runtime, snapshotErr := host.TaskLSPSnapshot(ctx, state.Language)
		if snapshotErr != nil || runtime == nil || runtime.Generation != state.Generation {
			continue
		}
		result[state.Language] = *runtime
	}
	return result
}

func (c *Controller) SetPolicy(
	ctx context.Context,
	taskID, language string,
	policy Policy,
	origin Origin,
) (*LanguageSnapshot, error) {
	if err := c.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if err := validateLanguageAndOrigin(language, origin); err != nil {
		return nil, err
	}
	if !validPolicy(policy) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPolicy, policy)
	}
	return c.commands.submit(ctx, TaskLanguageKey{TaskID: taskID, Language: language}, ActionSetPolicy, string(policy),
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.setPolicy(workCtx, taskID, language, policy, origin)
		})
}

func (c *Controller) Start(
	ctx context.Context,
	taskID, language string,
	origin Origin,
) (*LanguageSnapshot, error) {
	if err := c.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if err := validateLanguageAndOrigin(language, origin); err != nil {
		return nil, err
	}
	return c.commands.submit(ctx, TaskLanguageKey{TaskID: taskID, Language: language}, ActionStart, "",
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.start(workCtx, taskID, language, origin, ActionStart)
		})
}

func (c *Controller) Stop(
	ctx context.Context,
	taskID, language string,
	origin Origin,
) (*LanguageSnapshot, error) {
	if err := c.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if err := validateLanguageAndOrigin(language, origin); err != nil {
		return nil, err
	}
	c.cancelRecovery(TaskLanguageKey{TaskID: taskID, Language: language})
	return c.commands.submit(ctx, TaskLanguageKey{TaskID: taskID, Language: language}, ActionStop, "",
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.stop(workCtx, taskID, language, origin, ActionStop)
		})
}

func (c *Controller) Restart(
	ctx context.Context,
	taskID, language string,
	origin Origin,
) (*LanguageSnapshot, error) {
	if err := c.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if err := validateLanguageAndOrigin(language, origin); err != nil {
		return nil, err
	}
	return c.commands.submit(ctx, TaskLanguageKey{TaskID: taskID, Language: language}, ActionRestart, "",
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.start(workCtx, taskID, language, origin, ActionRestart)
		})
}

func (c *Controller) ResolveAttachment(
	ctx context.Context,
	taskID, language string,
) (*AttachmentTarget, error) {
	if err := c.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if !installer.IsSupported(language) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
	}
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	state, _, err := c.store.GetTaskLSPLanguage(ctx, taskID, language)
	if err != nil {
		return nil, err
	}
	if state.Policy == PolicyDisabled || state.Generation == 0 || state.Phase != PhaseReady {
		return nil, ErrAttachmentNotReady
	}
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !readyTaskEnvironment(environment) {
		return nil, ErrTaskNotReady
	}
	if !ExecutorSupportsLSP(environment.ExecutorType) {
		return nil, ErrExecutorUnsupported
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, environment.ID)
	if err != nil {
		return nil, err
	}
	if !exists || host == nil {
		return nil, ErrAttachmentNotReady
	}
	return &AttachmentTarget{Host: host, Language: language, Generation: state.Generation}, nil
}

func (c *Controller) setPolicy(
	ctx context.Context,
	taskID, language string,
	policy Policy,
	origin Origin,
) (*LanguageSnapshot, error) {
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	now := c.clock()
	state, err := c.updateState(ctx, taskID, language, func(next *TaskLanguageState) {
		next.Policy = policy
		next.LastAction = ActionSetPolicy
		next.LastActionAt = &now
		next.LastInitiator = origin.Initiator
		next.LastTransitionAt = now
	})
	if err != nil {
		return nil, err
	}
	settings, err := c.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	effective := effectivePolicy(*state, settings)
	if effective == PolicyDisabled {
		return c.stop(ctx, taskID, language, origin, ActionSetPolicy)
	}
	if phaseHasServer(state.Phase) {
		snapshot := c.languageSnapshot(*state, settings, nil)
		return &snapshot, nil
	}
	return c.start(ctx, taskID, language, origin, ActionStart)
}

func (c *Controller) start(
	ctx context.Context,
	taskID, language string,
	origin Origin,
	action Action,
) (*LanguageSnapshot, error) {
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	settings, err := c.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	current, _, err := c.store.GetTaskLSPLanguage(ctx, taskID, language)
	if err != nil {
		return nil, err
	}
	if snapshot, handled, validateErr := c.validateStart(*current, settings, action); handled {
		return snapshot, validateErr
	}
	acceptedAt := c.clock()
	state, err := c.store.AllocateTaskLSPGeneration(
		ctx, taskID, language, action, origin.Initiator, origin.Reason, acceptedAt,
	)
	if err != nil {
		return nil, err
	}
	return c.startAllocated(ctx, task, *state, settings, acceptedAt, action)
}

func (c *Controller) validateStart(
	current TaskLanguageState,
	settings TaskSettings,
	action Action,
) (*LanguageSnapshot, bool, error) {
	if action == ActionReconcile && effectivePolicy(current, settings) != PolicyKeepWarm {
		snapshot := c.languageSnapshot(current, settings, nil)
		return &snapshot, true, nil
	}
	if action == ActionStart && current.Policy == PolicyKeepWarm && phaseHasServer(current.Phase) {
		snapshot := c.languageSnapshot(current, settings, nil)
		return &snapshot, true, nil
	}
	if action == ActionRestart &&
		(current.Policy == PolicyDisabled || current.Generation == 0 || !phaseHasServer(current.Phase)) {
		return nil, true, ErrServerDisabled
	}
	return nil, false, nil
}

func (c *Controller) startAllocated(
	ctx context.Context,
	task *taskmodels.Task,
	state TaskLanguageState,
	settings TaskSettings,
	acceptedAt time.Time,
	action Action,
) (*LanguageSnapshot, error) {
	if task.ArchivedAt != nil {
		return c.transition(ctx, state, settings, PhaseWaitingForTask, "", "")
	}
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, state.TaskID)
	if err != nil {
		return nil, err
	}
	if !readyTaskEnvironment(environment) {
		return c.transition(ctx, state, settings, PhaseWaitingForTask, "", "")
	}
	if !ExecutorSupportsLSP(environment.ExecutorType) {
		return c.transition(ctx, state, settings, PhaseUnsupported, "unsupported_executor", "")
	}
	key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
	if !c.capacity.Admit(key, state.Generation, acceptedAt) {
		return c.transition(ctx, state, settings, PhaseQueued, "", "")
	}
	return c.launchReserved(ctx, state, settings, environment, action)
}

func (c *Controller) launchReserved(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	environment *taskmodels.TaskEnvironment,
	action Action,
) (*LanguageSnapshot, error) {
	key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
	host, err := c.runtimes.EnsureTaskHost(ctx, environment.ID)
	if err != nil {
		c.releaseCapacity(ctx, key, state.Generation)
		return c.transition(ctx, state, settings, PhaseError, "task_host_unavailable", err.Error())
	}
	request := TaskHostStartRequest{
		Language: state.Language, Generation: state.Generation,
		AutoInstall: contains(settings.AutoInstallLanguages, state.Language) &&
			installer.SupportsAutoInstall(state.Language),
		Configuration: append(json.RawMessage(nil), settings.ServerConfigs[state.Language]...),
	}
	var runtimeSnapshot *RuntimeSnapshot
	if action == ActionRestart {
		runtimeSnapshot, err = host.RestartTaskLSP(ctx, request)
	} else {
		runtimeSnapshot, err = host.StartTaskLSP(ctx, request)
	}
	if err != nil {
		// Transport failures remain ambiguous unless the task host returned
		// generation-scoped evidence that no process was created.
		if runtimeFailureProvesNoProcess(runtimeSnapshot, state.Generation) {
			c.releaseCapacity(ctx, key, state.Generation)
		}
		if runtimeSnapshot != nil {
			persisted, persistErr := c.persistRuntime(ctx, state, *runtimeSnapshot)
			if persistErr == nil {
				state = *persisted
			}
		}
		return c.transition(ctx, state, settings, PhaseError, "task_host_control_failed", err.Error())
	}
	stored, err := c.persistRuntime(ctx, state, *runtimeSnapshot)
	if err != nil {
		return nil, err
	}
	c.ensureWatch(key)
	snapshot := c.languageSnapshot(*stored, settings, runtimeSnapshot)
	return &snapshot, nil
}

func (c *Controller) transition(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	phase Phase,
	errorCode, errorMessage string,
) (*LanguageSnapshot, error) {
	stored, err := c.updateState(ctx, state.TaskID, state.Language, func(next *TaskLanguageState) {
		next.Phase = phase
		next.ErrorCode = errorCode
		next.ErrorMessage = errorMessage
		next.LastTransitionAt = c.clock()
	})
	if err != nil {
		return nil, err
	}
	snapshot := c.languageSnapshot(*stored, settings, nil)
	return &snapshot, nil
}

func (c *Controller) persistRuntime(
	ctx context.Context,
	state TaskLanguageState,
	runtime RuntimeSnapshot,
) (*TaskLanguageState, error) {
	if runtime.Generation != state.Generation {
		return nil, fmt.Errorf("task-host generation %d does not match controller generation %d", runtime.Generation, state.Generation)
	}
	return c.updateStateWithRuntime(ctx, state.TaskID, state.Language, &runtime, func(next *TaskLanguageState) {
		if next.Generation != runtime.Generation {
			return
		}
		next.Phase = runtime.Phase
		next.ProcessStartedAt = runtime.ProcessStartedAt
		next.InitializeStartedAt = runtime.InitializeStartedAt
		next.ReadyAt = runtime.ReadyAt
		next.LastTransitionAt = runtime.LastTransitionAt
		next.ErrorCode = runtime.ErrorCode
		next.ErrorMessage = runtime.ErrorMessage
	})
}

func (c *Controller) updateState(
	ctx context.Context,
	taskID, language string,
	mutate func(*TaskLanguageState),
) (*TaskLanguageState, error) {
	return c.updateStateWithRuntime(ctx, taskID, language, nil, mutate)
}

func (c *Controller) updateStateWithRuntime(
	ctx context.Context,
	taskID, language string,
	runtime *RuntimeSnapshot,
	mutate func(*TaskLanguageState),
) (*TaskLanguageState, error) {
	for range 5 {
		state, _, err := c.store.GetTaskLSPLanguage(ctx, taskID, language)
		if err != nil {
			return nil, err
		}
		expected := state.Revision
		mutate(state)
		updated, err := c.store.CompareAndUpdateTaskLSPLanguage(ctx, *state, expected)
		if errors.Is(err, ErrRevisionConflict) {
			continue
		}
		if err == nil && updated != nil {
			c.publishState(ctx, *updated, runtime)
		}
		return updated, err
	}
	return nil, ErrRevisionConflict
}

func (c *Controller) languageSnapshot(
	state TaskLanguageState,
	settings TaskSettings,
	runtime *RuntimeSnapshot,
) LanguageSnapshot {
	snapshot := LanguageSnapshot{
		TaskLanguageState: state,
		EffectivePolicy:   effectivePolicy(state, settings),
		Activity:          ActivityIdle,
		Progress:          []WorkItem{},
		Capacity:          c.capacity.Snapshot(),
	}
	if runtime != nil && runtime.Generation == state.Generation {
		snapshot.Activity = runtime.Activity
		snapshot.Progress = append([]WorkItem{}, runtime.Work...)
		if runtime.LastCompletedWork != nil {
			completed := *runtime.LastCompletedWork
			snapshot.LastCompletedWork = &completed
		}
	}
	return snapshot
}

func (c *Controller) authorize(ctx context.Context, taskID string) error {
	if c.tasks == nil {
		return errors.New("task LSP controller task service is not configured")
	}
	return c.tasks.AuthorizeTaskAccess(ctx, taskID)
}

func (c *Controller) loadSettings(ctx context.Context) (TaskSettings, error) {
	if c.settings == nil {
		return TaskSettings{}, nil
	}
	return c.settings.TaskLSPSettings(ctx)
}

func effectivePolicy(state TaskLanguageState, settings TaskSettings) Policy {
	switch state.Policy {
	case PolicyKeepWarm:
		return PolicyKeepWarm
	case PolicyDisabled:
		return PolicyDisabled
	default:
		if state.Detected && contains(settings.AutoStartLanguages, state.Language) {
			return PolicyKeepWarm
		}
		return PolicyDisabled
	}
}

func validateLanguageAndOrigin(language string, origin Origin) error {
	if !installer.IsSupported(language) {
		return fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
	}
	if !validInitiator(origin.Initiator) {
		return fmt.Errorf("invalid task LSP initiator %q", origin.Initiator)
	}
	if origin.Reason == "" {
		return errors.New("task LSP origin reason is required")
	}
	return nil
}

func registeredLanguages() []string {
	registered := installer.SupportedLanguages()
	languages := make([]string, 0, len(registered))
	for language := range registered {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}

func readyTaskEnvironment(environment *taskmodels.TaskEnvironment) bool {
	return environment != nil && environment.ID != "" && environment.Status == taskmodels.TaskEnvironmentStatusReady
}

func phaseHasServer(phase Phase) bool {
	switch phase {
	case PhaseInstalling, PhaseStarting, PhaseProcessStarted, PhaseInitializing, PhaseReady, PhaseStopping, PhaseError:
		return true
	default:
		return false
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type commandResult struct {
	snapshot *LanguageSnapshot
	err      error
}

type commandBatch struct {
	action      Action
	coalesceKey string
	ctx         context.Context
	run         func(context.Context) (*LanguageSnapshot, error)
	done        chan struct{}
	result      commandResult
}

type commandLane struct {
	running *commandBatch
	queued  []*commandBatch
}

type commandCoordinator struct {
	mu    sync.Mutex
	lanes map[TaskLanguageKey]*commandLane
}

func (c *commandCoordinator) submit(
	ctx context.Context,
	key TaskLanguageKey,
	action Action,
	coalesceKey string,
	run func(context.Context) (*LanguageSnapshot, error),
) (*LanguageSnapshot, error) {
	c.mu.Lock()
	if c.lanes == nil {
		c.lanes = make(map[TaskLanguageKey]*commandLane)
	}
	lane := c.lanes[key]
	if lane == nil {
		lane = &commandLane{}
		c.lanes[key] = lane
	}
	batch := coalescibleBatch(lane, action, coalesceKey)
	if batch == nil {
		batch = &commandBatch{
			action: action, coalesceKey: coalesceKey,
			ctx: context.WithoutCancel(ctx), run: run, done: make(chan struct{}),
		}
		if lane.running == nil {
			lane.running = batch
			go c.execute(key, lane, batch)
		} else {
			lane.queued = append(lane.queued, batch)
		}
	}
	c.mu.Unlock()

	select {
	case <-batch.done:
		return batch.result.snapshot, batch.result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func coalescibleBatch(lane *commandLane, action Action, coalesceKey string) *commandBatch {
	if len(lane.queued) > 0 {
		last := lane.queued[len(lane.queued)-1]
		if last.action == action && last.coalesceKey == coalesceKey {
			return last
		}
		return nil
	}
	if lane.running != nil && lane.running.action == action && lane.running.coalesceKey == coalesceKey {
		return lane.running
	}
	return nil
}

func (c *commandCoordinator) execute(key TaskLanguageKey, lane *commandLane, batch *commandBatch) {
	batch.result.snapshot, batch.result.err = batch.run(batch.ctx)

	c.mu.Lock()
	close(batch.done)
	if len(lane.queued) == 0 {
		delete(c.lanes, key)
		c.mu.Unlock()
		return
	}
	next := lane.queued[0]
	lane.queued = lane.queued[1:]
	lane.running = next
	go c.execute(key, lane, next)
	c.mu.Unlock()
}
