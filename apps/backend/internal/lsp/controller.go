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
	AcquireTaskLSPAdmission(ctx context.Context, taskID string) (release func(), err error)
	GetTask(ctx context.Context, taskID string) (*taskmodels.Task, error)
	GetTaskEnvironmentForTaskLSP(ctx context.Context, taskID string) (*taskmodels.TaskEnvironment, error)
}

type SettingsProvider interface {
	TaskLSPSettings(ctx context.Context, taskID string) (TaskSettings, error)
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
	PurgeTaskLSP(ctx context.Context) error
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
	EnsureTaskHost(ctx context.Context, taskID, taskEnvironmentID string) (TaskHost, error)
	ExistingTaskHost(ctx context.Context, taskID, taskEnvironmentID string) (TaskHost, bool, error)
	RecoverTaskHost(ctx context.Context, taskEnvironmentID string) (bool, error)
	CleanupTaskHost(ctx context.Context, taskID, taskEnvironmentID, reason string) (TaskHostCleanupResult, error)
	DiscoverTaskLanguages(ctx context.Context, taskID, taskEnvironmentID string) (*DiscoveryResult, error)
}

// TaskHostCleanupResult distinguishes a successful physical process-tree reap
// from an intentional no-op for a task borrowing another task's shared host.
type TaskHostCleanupResult struct {
	ProcessTreeGone bool
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

	lifecycleMu      sync.Mutex
	lifecycleCtx     context.Context
	lifecycleCancel  context.CancelFunc
	lifecycleDone    chan struct{}
	startupReady     chan struct{}
	startupComplete  bool
	startupErr       error
	lifecycleWG      sync.WaitGroup
	watches          map[TaskLanguageKey]*taskLanguageWatch
	recoveries       map[TaskLanguageKey]*recoveryState
	settingsApplyMu  sync.Mutex
	appliedSettings  map[string]TaskSettings
	settingsSignal   chan struct{}
	discoveryMu      sync.Mutex
	discoveries      map[string]*discoveryCall
	discoveryRetries map[string]*discoveryRetryState
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
		watches:          make(map[TaskLanguageKey]*taskLanguageWatch),
		recoveries:       make(map[TaskLanguageKey]*recoveryState),
		appliedSettings:  make(map[string]TaskSettings),
		settingsSignal:   make(chan struct{}, 1),
		discoveries:      make(map[string]*discoveryCall),
		discoveryRetries: make(map[string]*discoveryRetryState),
	}
}

func (c *Controller) Snapshot(ctx context.Context, taskID string) (*TaskSnapshot, error) {
	if err := c.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if err := c.waitForStartup(ctx); err != nil {
		return nil, err
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	active := taskAllowsLSPRuntime(task)
	if active {
		// Bounded discovery is opportunistic and never starts an LSP or project
		// import. Docker discovery may ensure the task host to inspect its filesystem.
		_ = c.discoverTask(ctx, taskID, false)
		// Converge already-persisted task policy after reload/resume. Errors are
		// represented on the language rows so the status surface remains usable.
		_ = c.ReconcileTask(ctx, taskID)
	}
	settings, err := c.loadSettings(ctx, taskID)
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
	runtimeByLanguage := make(map[string]RuntimeSnapshot)
	if active {
		runtimeByLanguage = c.liveRuntimeSnapshots(ctx, taskID, states)
	}
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
	environment, err := c.tasks.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil || !readyTaskEnvironment(environment) || !ExecutorSupportsLSP(environment.ExecutorType) {
		return result
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, taskID, environment.ID)
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
	if err := c.waitForStartup(ctx); err != nil {
		return nil, err
	}
	if policy == PolicyDisabled {
		return c.commands.submitInterrupting(
			ctx, TaskLanguageKey{TaskID: taskID, Language: language}, ActionSetPolicy, string(policy),
			func(workCtx context.Context) (*LanguageSnapshot, error) {
				return c.setPolicy(workCtx, taskID, language, policy, origin)
			},
		)
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
	if err := c.waitForStartup(ctx); err != nil {
		return nil, err
	}
	key := TaskLanguageKey{TaskID: taskID, Language: language}
	c.cancelRecovery(key)
	return c.commands.submit(ctx, key, ActionStart, "",
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
	if err := c.waitForStartup(ctx); err != nil {
		return nil, err
	}
	c.cancelRecovery(TaskLanguageKey{TaskID: taskID, Language: language})
	return c.commands.submitInterrupting(ctx, TaskLanguageKey{TaskID: taskID, Language: language}, ActionStop, "",
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.stop(workCtx, taskID, language, origin, ActionStop)
		})
}

// CancelTaskOperations interrupts accepted task-language commands before a
// terminal task mutation waits for exclusive runtime admission. The terminal
// cleanup commands remain the final process-tree proof.
func (c *Controller) CancelTaskOperations(taskID string) {
	c.commands.cancelTask(taskID)
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
	if err := c.waitForStartup(ctx); err != nil {
		return nil, err
	}
	key := TaskLanguageKey{TaskID: taskID, Language: language}
	c.cancelRecovery(key)
	return c.commands.submit(ctx, key, ActionRestart, "",
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.start(workCtx, taskID, language, origin, ActionRestart)
		})
}

func (c *Controller) ResolveAttachment(
	ctx context.Context,
	taskID, language string,
) (*AttachmentTarget, error) {
	if err := c.validateAttachmentRequest(ctx, taskID, language); err != nil {
		return nil, err
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !taskAllowsLSPRuntime(task) {
		return nil, ErrTaskNotReady
	}
	state, _, err := c.store.GetTaskLSPLanguage(ctx, taskID, language)
	if err != nil {
		return nil, err
	}
	if state.Policy == PolicyDisabled || state.Generation == 0 || state.Phase != PhaseReady {
		return nil, ErrAttachmentNotReady
	}
	environment, err := c.tasks.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !readyTaskEnvironment(environment) {
		return nil, ErrTaskNotReady
	}
	if !ExecutorSupportsLSP(environment.ExecutorType) {
		return nil, ErrExecutorUnsupported
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, taskID, environment.ID)
	if err != nil {
		return nil, err
	}
	if !exists || host == nil {
		return nil, ErrAttachmentNotReady
	}
	return &AttachmentTarget{Host: host, Language: language, Generation: state.Generation}, nil
}

func (c *Controller) validateAttachmentRequest(ctx context.Context, taskID, language string) error {
	if err := c.authorize(ctx, taskID); err != nil {
		return err
	}
	if !installer.IsSupported(language) {
		return fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
	}
	return c.waitForStartup(ctx)
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
	settings, err := c.loadSettings(ctx, taskID)
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
	releaseAdmission, err := c.tasks.AcquireTaskLSPAdmission(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTaskNotReady, err)
	}
	defer releaseAdmission()
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	settings, err := c.loadSettings(ctx, taskID)
	if err != nil {
		return nil, err
	}
	current, _, err := c.store.GetTaskLSPLanguage(ctx, taskID, language)
	if err != nil {
		return nil, err
	}
	if action == ActionReconcile && effectivePolicy(*current, settings) != PolicyKeepWarm {
		snapshot := c.languageSnapshot(*current, settings, nil)
		return &snapshot, nil
	}
	if restartRequiresRunningServer(action, *current) {
		return nil, ErrServerDisabled
	}
	previousMayHaveProcess := action == ActionRestart && stateMayHaveProcess(*current)
	if !taskAllowsLSPRuntime(task) {
		return c.waitForTaskWithoutAllocation(ctx, *current, settings, origin, action)
	}
	environment, err := c.tasks.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !readyTaskEnvironment(environment) {
		return c.waitForTaskWithoutAllocation(ctx, *current, settings, origin, action)
	}
	if snapshot, handled, reconnectErr := c.reconnectBeforeStart(
		ctx, *current, settings, environment, action, origin,
	); handled {
		return snapshot, reconnectErr
	}
	acceptedAt := c.clock()
	state, err := c.store.AllocateTaskLSPGeneration(
		ctx, taskID, language, action, origin.Initiator, origin.Reason, acceptedAt,
	)
	if err != nil {
		return nil, err
	}
	return c.startAllocatedWithEnvironment(
		ctx, task, *state, settings, acceptedAt, action, environment, previousMayHaveProcess,
	)
}

func restartRequiresRunningServer(action Action, state TaskLanguageState) bool {
	return action == ActionRestart &&
		(state.Policy == PolicyDisabled || state.Generation == 0 || !phaseHasServer(state.Phase))
}

func (c *Controller) waitForTaskWithoutAllocation(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	origin Origin,
	action Action,
) (*LanguageSnapshot, error) {
	switch action {
	case ActionStart:
		updated, updateErr := c.recordExplicitStartIntent(ctx, state, origin)
		if updateErr != nil {
			return nil, updateErr
		}
		state = *updated
	case ActionRestart:
		now := c.clock()
		updated, updateErr := c.updateState(ctx, state.TaskID, state.Language, func(next *TaskLanguageState) {
			next.LastAction = ActionRestart
			next.LastActionAt = &now
			next.LastInitiator = origin.Initiator
			next.LastRestartReason = origin.Reason
			next.LastTransitionAt = now
		})
		if updateErr != nil {
			return nil, updateErr
		}
		state = *updated
	}
	return c.transition(ctx, state, settings, PhaseWaitingForTask, "", "")
}

func (c *Controller) reconnectBeforeStart(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	environment *taskmodels.TaskEnvironment,
	action Action,
	origin Origin,
) (*LanguageSnapshot, bool, error) {
	if action == ActionRestart || state.Generation == 0 || !readyTaskEnvironment(environment) ||
		!ExecutorSupportsLSP(environment.ExecutorType) {
		return nil, false, nil
	}
	snapshot, adopted, err := c.adoptExistingRuntime(
		ctx, state, settings, environment, action, origin,
	)
	if adopted {
		return snapshot, true, err
	}
	if err == nil {
		return nil, false, nil
	}
	return c.handleReconnectFailure(ctx, state, settings, action, origin, err)
}

func (c *Controller) handleReconnectFailure(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	action Action,
	origin Origin,
	reconnectErr error,
) (*LanguageSnapshot, bool, error) {
	if action == ActionStart {
		updated, err := c.recordExplicitStartIntent(ctx, state, origin)
		if err != nil {
			return nil, true, err
		}
		state = *updated
	}
	snapshot, err := c.transition(
		ctx, state, settings, PhaseError, "task_host_unavailable", reconnectErr.Error(),
	)
	c.scheduleDesiredRecovery(TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}, snapshot)
	return snapshot, true, err
}

func (c *Controller) recordExplicitStartIntent(
	ctx context.Context,
	state TaskLanguageState,
	origin Origin,
) (*TaskLanguageState, error) {
	now := c.clock()
	return c.updateState(ctx, state.TaskID, state.Language, func(next *TaskLanguageState) {
		next.Policy = PolicyKeepWarm
		next.LastAction = ActionStart
		next.LastActionAt = &now
		next.LastInitiator = origin.Initiator
		next.LastTransitionAt = now
	})
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
		if phase == PhaseOff && next.Generation > 0 {
			next.ProcessAbsentGeneration = next.Generation
		}
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
) (*TaskLanguageState, bool, error) {
	if runtime.Generation > state.Generation {
		return nil, false, fmt.Errorf("task-host generation %d does not match controller generation %d", runtime.Generation, state.Generation)
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
	state, _, err := c.updateStateWithRuntime(ctx, taskID, language, nil, mutate)
	return state, err
}

func (c *Controller) updateStateWithRuntime(
	ctx context.Context,
	taskID, language string,
	runtime *RuntimeSnapshot,
	mutate func(*TaskLanguageState),
) (*TaskLanguageState, bool, error) {
	for range 5 {
		state, _, err := c.store.GetTaskLSPLanguage(ctx, taskID, language)
		if err != nil {
			return nil, false, err
		}
		if runtime != nil && staleRuntimeObservation(*state, *runtime) {
			return state, false, nil
		}
		expected := state.Revision
		previousGeneration := state.Generation
		mutate(state)
		if runtime != nil {
			recordRuntimeObservation(state, previousGeneration, *runtime)
		}
		updated, err := c.store.CompareAndUpdateTaskLSPLanguage(ctx, *state, expected)
		if errors.Is(err, ErrRevisionConflict) {
			continue
		}
		if err == nil && updated != nil {
			c.publishState(ctx, *updated, runtime)
		}
		return updated, err == nil, err
	}
	return nil, false, ErrRevisionConflict
}

func staleRuntimeObservation(state TaskLanguageState, runtime RuntimeSnapshot) bool {
	if runtime.Generation < state.Generation {
		return true
	}
	if runtime.Generation > state.Generation || runtime.Incarnation == "" || state.RuntimeIncarnation == "" {
		return false
	}
	if runtime.Incarnation == state.RuntimeIncarnation {
		return runtime.Revision <= state.RuntimeRevision
	}
	if runtime.RuntimeStartedAt.IsZero() || state.RuntimeStartedAt == nil {
		return false
	}
	return !runtime.RuntimeStartedAt.After(*state.RuntimeStartedAt)
}

func recordRuntimeObservation(state *TaskLanguageState, previousGeneration uint64, runtime RuntimeSnapshot) {
	if runtimeFailureProvesNoProcess(&runtime, state.Generation) {
		state.ProcessAbsentGeneration = runtime.Generation
	} else if runtimeHasProcess(&runtime) {
		state.ProcessAbsentGeneration = 0
	}
	if state.Generation != previousGeneration {
		state.RuntimeIncarnation = ""
		state.RuntimeStartedAt = nil
		state.RuntimeRevision = 0
	}
	if runtime.Incarnation == "" {
		return
	}
	state.RuntimeIncarnation = runtime.Incarnation
	state.RuntimeRevision = runtime.Revision
	if runtime.RuntimeStartedAt.IsZero() {
		state.RuntimeStartedAt = nil
		return
	}
	startedAt := runtime.RuntimeStartedAt
	state.RuntimeStartedAt = &startedAt
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

func taskAllowsLSPRuntime(task *taskmodels.Task) bool {
	return task != nil && task.ArchivedAt == nil
}

func (c *Controller) loadSettings(ctx context.Context, taskID string) (TaskSettings, error) {
	if c.settings == nil {
		return TaskSettings{}, nil
	}
	return c.settings.TaskLSPSettings(ctx, taskID)
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
