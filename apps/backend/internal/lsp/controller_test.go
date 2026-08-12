package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestLanguageSnapshotSerializesEmptyProgressAsArray(t *testing.T) {
	controller := NewController(ControllerConfig{})
	state := DefaultTaskLanguageState("task-1", "kotlin")
	state.Generation = 1
	runtime := &RuntimeSnapshot{Generation: 1, Work: nil}

	snapshot := controller.languageSnapshot(state, TaskSettings{}, runtime)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if !strings.Contains(string(encoded), `"progress":[]`) {
		t.Fatalf("snapshot progress must serialize as an array: %s", encoded)
	}
}

func TestLanguageSnapshotSeparatesCompletedWorkFromActiveProgress(t *testing.T) {
	controller := NewController(ControllerConfig{})
	state := DefaultTaskLanguageState("task-1", "kotlin")
	state.Generation = 1
	completedAt := time.Unix(102, 0).UTC()
	runtime := &RuntimeSnapshot{
		Generation: 1,
		LastCompletedWork: &CompletedWorkItem{
			Token: "gradle", Title: "Importing Kotlin project", Message: "Project model loaded",
			StartedAt: time.Unix(101, 0).UTC(), CompletedAt: completedAt,
		},
	}

	snapshot := controller.languageSnapshot(state, TaskSettings{}, runtime)
	if len(snapshot.Progress) != 0 || snapshot.LastCompletedWork == nil ||
		snapshot.LastCompletedWork.CompletedAt != completedAt {
		t.Fatalf("completed work projection = %#v", snapshot)
	}
}

func TestLSPControllerAuthorizationRunsBeforeEveryUserOperation(t *testing.T) {
	denied := errors.New("hidden task")
	tasks := &fakeControllerTasks{authErr: denied}
	controller := NewController(ControllerConfig{
		Tasks: tasks, Store: newMemoryLSPStore(), Settings: &fakeLSPSettings{},
		Runtimes: &fakeLSPRuntimes{}, Capacity: NewCapacity(2),
	})
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}
	operations := []struct {
		name string
		run  func() error
	}{
		{name: "snapshot", run: func() error { _, err := controller.Snapshot(context.Background(), "hidden"); return err }},
		{name: "policy", run: func() error {
			_, err := controller.SetPolicy(context.Background(), "hidden", "kotlin", PolicyKeepWarm, origin)
			return err
		}},
		{name: "start", run: func() error {
			_, err := controller.Start(context.Background(), "hidden", "kotlin", origin)
			return err
		}},
		{name: "stop", run: func() error {
			_, err := controller.Stop(context.Background(), "hidden", "kotlin", origin)
			return err
		}},
		{name: "restart", run: func() error {
			_, err := controller.Restart(context.Background(), "hidden", "kotlin", origin)
			return err
		}},
		{name: "attachment", run: func() error {
			_, err := controller.ResolveAttachment(context.Background(), "hidden", "kotlin")
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			tasks.resetCalls()
			if err := operation.run(); !errors.Is(err, denied) {
				t.Fatalf("error = %v, want authorization error", err)
			}
			if got := tasks.callsSnapshot(); len(got) != 1 || got[0] != "authorize:hidden" {
				t.Fatalf("calls before denial = %v", got)
			}
		})
	}
}

func TestSnapshotDiscoversLanguagesWithoutEnsuringTaskHostOrOpeningFile(t *testing.T) {
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{discovery: &DiscoveryResult{
		Languages: []string{"kotlin", "typescript"}, State: DetectionComplete,
		ScannedAt: time.Unix(150, 0).UTC(),
	}}
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, runtimes)

	snapshot, err := controller.Snapshot(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !languageFromSnapshot(t, snapshot, "kotlin").Detected ||
		!languageFromSnapshot(t, snapshot, "typescript").Detected ||
		languageFromSnapshot(t, snapshot, "go").Detected {
		t.Fatalf("discovery snapshot = %#v", snapshot.Languages)
	}
	if runtimes.discoveryCalls != 1 || runtimes.ensureCalls != 0 {
		t.Fatalf("discovery calls=%d existing=%d ensure=%d",
			runtimes.discoveryCalls, runtimes.existingCalls, runtimes.ensureCalls)
	}

	if _, err := controller.Snapshot(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if runtimes.discoveryCalls != 1 {
		t.Fatalf("recorded discovery reran: calls=%d", runtimes.discoveryCalls)
	}
}

func TestSnapshotOverlaysLiveTaskHostProgressForReconnect(t *testing.T) {
	store := newMemoryLSPStore()
	state := DefaultTaskLanguageState("task-1", "kotlin")
	state.Policy = PolicyKeepWarm
	state.Detected = true
	state.DetectionState = DetectionComplete
	state.Phase = PhaseInitializing
	state.Generation = 1
	if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), state, 0); err != nil {
		t.Fatal(err)
	}
	percentage := 42.0
	host := newFakeLSPHost()
	host.snapshots["kotlin"] = RuntimeSnapshot{
		Language: "kotlin", Generation: 1, Phase: PhaseInitializing,
		Activity: ActivityServerWork,
		Work: []WorkItem{{
			Token: "gradle", Title: "Importing Kotlin project", Message: "Resolving Gradle",
			Percentage: &percentage, StartedAt: time.Unix(101, 0).UTC(),
		}},
	}
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)

	snapshot, err := controller.Snapshot(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	kotlin := languageFromSnapshot(t, snapshot, "kotlin")
	if kotlin.Activity != ActivityServerWork || len(kotlin.Progress) != 1 ||
		kotlin.Progress[0].Title != "Importing Kotlin project" {
		t.Fatalf("reconnect snapshot lost live progress: %#v", kotlin)
	}
}

func TestDiscoveryFailureIsTruthfulAndNeverEnsuresResources(t *testing.T) {
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{discoveryErr: errors.New("scanner unavailable")}
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, runtimes)

	snapshot, err := controller.Snapshot(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range snapshot.Languages {
		if language.DetectionState != DetectionUnavailable || !language.DetectionTruncated {
			t.Fatalf("language discovery state = %#v", language)
		}
	}
	if runtimes.ensureCalls != 0 {
		t.Fatalf("failed discovery launched task-host resources: ensure=%d", runtimes.ensureCalls)
	}
}

func TestSnapshotRetriesStaleUnavailableDiscoveryWithoutHotLooping(t *testing.T) {
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{discoveryErr: errors.New("task host still materializing")}
	now := time.Unix(100, 0).UTC()
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, runtimes)
	controller.clock = func() time.Time { return now }

	if _, err := controller.Snapshot(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Snapshot(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if runtimes.discoveryCalls != 1 {
		t.Fatalf("fresh unavailable discovery calls = %d, want 1", runtimes.discoveryCalls)
	}

	now = now.Add(discoveryRetryDelay)
	runtimes.discoveryErr = nil
	runtimes.discovery = &DiscoveryResult{
		Languages: []string{"kotlin"}, State: DetectionComplete, ScannedAt: now,
	}
	snapshot, err := controller.Snapshot(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !languageFromSnapshot(t, snapshot, "kotlin").Detected || runtimes.discoveryCalls != 2 {
		t.Fatalf("retried discovery snapshot/calls = %#v/%d", snapshot.Languages, runtimes.discoveryCalls)
	}
}

func TestArchivedTaskCannotDiscoverReconnectOrAttach(t *testing.T) {
	archivedAt := time.Unix(90, 0).UTC()
	tasks := &fakeControllerTasks{task: &models.Task{ID: "task-1", ArchivedAt: &archivedAt}}
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 3,
		LastInitiator: InitiatorUser,
	})
	host := newFakeLSPHost()
	host.snapshots["kotlin"] = RuntimeSnapshot{Language: "kotlin", Generation: 3, Phase: PhaseReady}
	runtimes := &fakeLSPRuntimes{host: host, discovery: &DiscoveryResult{
		Languages: []string{"kotlin"}, State: DetectionComplete,
	}}
	controller := newTestController(tasks, store, &fakeLSPSettings{}, runtimes)

	snapshot, err := controller.Snapshot(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := languageFromSnapshot(t, snapshot, "kotlin"); got.Activity != ActivityIdle {
		t.Fatalf("archived snapshot overlaid live runtime: %#v", got)
	}
	if err := controller.RefreshDiscovery(context.Background(), "task-1"); !errors.Is(err, ErrTaskNotReady) {
		t.Fatalf("archived discovery error = %v, want task not ready", err)
	}
	if _, err := controller.ResolveAttachment(context.Background(), "task-1", "kotlin"); !errors.Is(err, ErrTaskNotReady) {
		t.Fatalf("archived attachment error = %v, want task not ready", err)
	}
	started, err := controller.Start(context.Background(), "task-1", "kotlin", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Phase != PhaseWaitingForTask || started.Generation != 3 || started.Policy != PolicyKeepWarm {
		t.Fatalf("archived start = %#v", started)
	}
	if runtimes.ensureCalls != 0 || runtimes.existingCalls != 0 || runtimes.discoveryCalls != 0 {
		t.Fatalf("archived task touched runtime: ensure=%d existing=%d discovery=%d",
			runtimes.ensureCalls, runtimes.existingCalls, runtimes.discoveryCalls)
	}
}

func TestStoppedTaskStartRecordsIntentWithoutAllocatingOrReconnecting(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"task-1": {
			ID: "env-task-1", TaskID: "task-1", ExecutorType: executorTypeLocalPC,
			Status: models.TaskEnvironmentStatusStopped,
		},
	}}
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyDisabled,
		DetectionState: DetectionComplete, Phase: PhaseOff, Generation: 2,
		LastInitiator: InitiatorUser,
	})
	runtimes := &fakeLSPRuntimes{host: newFakeLSPHost()}
	controller := newTestController(tasks, store, &fakeLSPSettings{}, runtimes)

	snapshot, err := controller.Start(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Phase != PhaseWaitingForTask || snapshot.Generation != 2 || snapshot.Policy != PolicyKeepWarm {
		t.Fatalf("stopped task start = %#v", snapshot)
	}
	if runtimes.ensureCalls != 0 || runtimes.existingCalls != 0 {
		t.Fatalf("stopped task touched runtime: ensure=%d existing=%d", runtimes.ensureCalls, runtimes.existingCalls)
	}
}

func TestWorkspaceSourceChangeUpdatesDynamicServerAndMarksStaticServer(t *testing.T) {
	store := newMemoryLSPStore()
	for _, language := range []string{"go", "kotlin"} {
		state := DefaultTaskLanguageState("task-1", language)
		state.Policy = PolicyKeepWarm
		state.Phase = PhaseReady
		state.Generation = 1
		if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), state, 0); err != nil {
			t.Fatal(err)
		}
	}
	host := newFakeLSPHost()
	host.workspaceResult = &WorkspaceUpdateResult{
		DynamicLanguages: []string{"go"}, RestartRequiredLanguages: []string{"kotlin"},
	}
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})

	if err := controller.WorkspaceSourcesChanged(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	goState, _, _ := store.GetTaskLSPLanguage(context.Background(), "task-1", "go")
	kotlinState, _, _ := store.GetTaskLSPLanguage(context.Background(), "task-1", "kotlin")
	if goState.RestartRequired || !kotlinState.RestartRequired ||
		kotlinState.RestartRequiredReason != "workspace_roots_changed" {
		t.Fatalf("go=%#v kotlin=%#v", goState, kotlinState)
	}
	if host.workspaceRefreshes != 1 || host.startCalls != 0 {
		t.Fatalf("refreshes=%d starts=%d", host.workspaceRefreshes, host.startCalls)
	}
}

func TestControllerPublishesFullMonotonicLanguageSnapshots(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	publisher := &recordingLSPPublisher{}
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.publisher = publisher
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}
	started, err := controller.Start(context.Background(), "task-1", "kotlin", origin)
	if err != nil {
		t.Fatal(err)
	}
	percentage := 42.0
	runtime := RuntimeSnapshot{
		Language: "kotlin", Generation: started.Generation, Phase: PhaseReady,
		Activity: ActivityServerWork, Work: []WorkItem{{
			Token: "gradle", Title: "Importing project", Percentage: &percentage, StartedAt: time.Unix(160, 0).UTC(),
		}},
	}
	if err := controller.observeRuntimeSnapshot(context.Background(), TaskLanguageKey{
		TaskID: "task-1", Language: "kotlin",
	}, runtime); err != nil {
		t.Fatal(err)
	}

	snapshots := publisher.all()
	if len(snapshots) == 0 {
		t.Fatal("no semantic task LSP snapshots published")
	}
	var previous uint64
	for _, snapshot := range snapshots {
		if snapshot.TaskID != "task-1" || snapshot.Language != "kotlin" {
			t.Fatalf("ownership missing from snapshot: %#v", snapshot)
		}
		if previous != 0 && snapshot.Revision <= previous {
			t.Fatalf("revision was not strictly monotonic: %d after %d", snapshot.Revision, previous)
		}
		previous = snapshot.Revision
	}
	last := snapshots[len(snapshots)-1]
	if last.Activity != ActivityServerWork || len(last.Progress) != 1 || last.Progress[0].Token != "gradle" {
		t.Fatalf("runtime evidence missing from event: %#v", last)
	}
}

func TestPolicyExplicitStartStopAndRestartUseOneTaskHostGeneration(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	runtimes := &fakeLSPRuntimes{host: host}
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{
		settings: TaskSettings{
			AutoInstallLanguages: []string{"typescript"},
			ServerConfigs:        map[string]json.RawMessage{"typescript": json.RawMessage(`{"tsserver":{"log":"off"}}`)},
		},
	}, runtimes)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	started, err := controller.Start(context.Background(), "task-1", "typescript", origin)
	if err != nil {
		t.Fatal(err)
	}
	if started.Policy != PolicyKeepWarm || started.Generation != 1 || started.Phase != PhaseReady {
		t.Fatalf("started = %#v", started)
	}
	if host.startCalls != 1 || runtimes.ensureCalls != 1 {
		t.Fatalf("start calls host=%d ensure=%d", host.startCalls, runtimes.ensureCalls)
	}
	if !host.lastStart.AutoInstall || string(host.lastStart.Configuration) != `{"tsserver":{"log":"off"}}` {
		t.Fatalf("start settings = %#v", host.lastStart)
	}

	restarted, err := controller.Restart(context.Background(), "task-1", "typescript", origin)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Policy != PolicyKeepWarm || restarted.Generation != 2 {
		t.Fatalf("restarted = %#v", restarted)
	}
	if host.restartCalls != 1 {
		t.Fatalf("restart calls = %d, want 1", host.restartCalls)
	}

	stopped, err := controller.Stop(context.Background(), "task-1", "typescript", origin)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Policy != PolicyDisabled || stopped.Phase != PhaseOff || stopped.Generation != 2 {
		t.Fatalf("stopped = %#v", stopped)
	}
	if host.stopCalls != 1 || controller.capacity.Active() != 0 {
		t.Fatalf("stop calls=%d active=%d", host.stopCalls, controller.capacity.Active())
	}
	if _, err := controller.Restart(context.Background(), "task-1", "typescript", origin); !errors.Is(err, ErrServerDisabled) {
		t.Fatalf("restart after stop error = %v", err)
	}
}

func TestDelayedRestartErrorCannotRegressNewerWatchState(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorUser,
	})
	host := newFakeLSPHost()
	runtimeStartedAt := time.Unix(200, 0).UTC()
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)
	host.restartBeforeReturn = func(request TaskHostStartRequest) {
		if err := controller.observeRuntimeSnapshot(context.Background(), TaskLanguageKey{
			TaskID: "task-1", Language: request.Language,
		}, RuntimeSnapshot{
			Language: request.Language, Generation: request.Generation, Phase: PhaseReady,
			Revision: 2, Incarnation: "task-host-new", RuntimeStartedAt: runtimeStartedAt,
		}); err != nil {
			t.Errorf("observe newer watch state: %v", err)
		}
	}
	host.restartResponse = &RuntimeSnapshot{
		Phase: PhaseError, ErrorCode: errorCodeProcessStartFailed, Revision: 1,
		Incarnation: "task-host-new", RuntimeStartedAt: runtimeStartedAt,
	}
	host.restartErr = errors.New("delayed restart failure")

	restarted, err := controller.Restart(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Phase != PhaseReady || restarted.Generation != 2 || restarted.RuntimeRevision != 2 {
		t.Fatalf("delayed restart response regressed newer watch state: %#v", restarted)
	}
	if host.restartCalls != 1 || controller.capacity.Active() != 1 {
		t.Fatalf("restart calls=%d active=%d", host.restartCalls, controller.capacity.Active())
	}
}

func TestStartDoesNotDuplicateRuntimeWhenSnapshotTrailsWatch(t *testing.T) {
	store := newMemoryLSPStore()
	runtimeStartedAt := time.Unix(200, 0).UTC()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		RuntimeIncarnation: "task-host", RuntimeStartedAt: &runtimeStartedAt, RuntimeRevision: 2,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["go"] = RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseProcessStarted,
		Revision: 1, Incarnation: "task-host", RuntimeStartedAt: runtimeStartedAt,
	}
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)

	started, err := controller.Start(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Phase != PhaseReady || started.Generation != 1 || host.startCalls != 0 {
		t.Fatalf("stale reconnect snapshot launched a duplicate: snapshot=%#v starts=%d", started, host.startCalls)
	}
}

func newTestController(
	tasks *fakeControllerTasks,
	store *memoryLSPStore,
	settings *fakeLSPSettings,
	runtimes *fakeLSPRuntimes,
) *Controller {
	return NewController(ControllerConfig{
		Tasks: tasks, Store: store, Settings: settings, Runtimes: runtimes,
		Capacity: NewCapacity(8), Clock: func() time.Time { return time.Unix(100, 0).UTC() },
	})
}

func readyEnvironment(taskID, executorType string) *models.TaskEnvironment {
	return &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID, ExecutorType: executorType,
		Status: models.TaskEnvironmentStatusReady,
	}
}

func languageFromSnapshot(t *testing.T, snapshot *TaskSnapshot, language string) LanguageSnapshot {
	t.Helper()
	for _, candidate := range snapshot.Languages {
		if candidate.Language == language {
			return candidate
		}
	}
	t.Fatalf("language %q absent from %#v", language, snapshot)
	return LanguageSnapshot{}
}

type fakeLSPSettings struct {
	settings TaskSettings
}

type recordingLSPPublisher struct {
	mu        sync.Mutex
	snapshots []LanguageSnapshot
}

func (p *recordingLSPPublisher) PublishTaskLSP(_ context.Context, snapshot LanguageSnapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshots = append(p.snapshots, snapshot)
	return nil
}

func (p *recordingLSPPublisher) all() []LanguageSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]LanguageSnapshot(nil), p.snapshots...)
}

func (f *fakeLSPSettings) TaskLSPSettings(context.Context, string) (TaskSettings, error) {
	return f.settings, nil
}

type fakeLSPRuntimes struct {
	mu             sync.Mutex
	host           *fakeLSPHost
	ensureCalls    int
	existingCalls  int
	cleanupCalls   int
	discovery      *DiscoveryResult
	discoveryErr   error
	discoveryCalls int
	recoverCalls   int
}

func (f *fakeLSPRuntimes) EnsureTaskHost(context.Context, string, string) (TaskHost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureCalls++
	if f.host == nil {
		f.host = newFakeLSPHost()
	}
	return f.host, nil
}

func (f *fakeLSPRuntimes) ExistingTaskHost(context.Context, string, string) (TaskHost, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.existingCalls++
	if f.host == nil {
		return nil, false, nil
	}
	return f.host, true, nil
}

func (f *fakeLSPRuntimes) CleanupTaskHost(
	context.Context, string, string, string,
) (TaskHostCleanupResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanupCalls++
	f.host = nil
	return TaskHostCleanupResult{ProcessTreeGone: true}, nil
}

func (f *fakeLSPRuntimes) RecoverTaskHost(context.Context, string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recoverCalls++
	if f.host == nil {
		return false, nil
	}
	f.host = nil
	return true, nil
}

func (f *fakeLSPRuntimes) DiscoverTaskLanguages(context.Context, string, string) (*DiscoveryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discoveryCalls++
	if f.discovery == nil {
		return nil, f.discoveryErr
	}
	result := *f.discovery
	result.Languages = append([]string(nil), f.discovery.Languages...)
	return &result, f.discoveryErr
}

type fakeLSPHost struct {
	mu                      sync.Mutex
	startCalls              int
	restartCalls            int
	stopCalls               int
	configurationCalls      int
	lastStart               TaskHostStartRequest
	lastStop                TaskHostStopRequest
	lastConfiguration       TaskHostConfigurationRequest
	snapshots               map[string]RuntimeSnapshot
	startEntered            chan struct{}
	startRelease            chan struct{}
	startErr                error
	startErrorSnapshot      *RuntimeSnapshot
	restartBeforeReturn     func(TaskHostStartRequest)
	restartResponse         *RuntimeSnapshot
	restartErr              error
	stopErr                 error
	purgeCalls              int
	purgeErr                error
	discovery               *DiscoveryResult
	discoveryErr            error
	discoveries             int
	workspaceResult         *WorkspaceUpdateResult
	workspaceErr            error
	workspaceRefreshes      int
	workspaceRefreshEntered chan struct{}
	workspaceRefreshRelease chan struct{}
	snapshotErr             error
	watchErr                error
	watchSnapshotEntered    chan struct{}
	watchSnapshotRelease    chan struct{}
	watchSnapshotDone       chan struct{}
}

func (f *fakeLSPHost) RefreshTaskLSPWorkspace(context.Context) (*WorkspaceUpdateResult, error) {
	f.mu.Lock()
	f.workspaceRefreshes++
	entered := f.workspaceRefreshEntered
	release := f.workspaceRefreshRelease
	f.mu.Unlock()
	if entered != nil {
		close(entered)
	}
	if release != nil {
		<-release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.workspaceResult == nil {
		return &WorkspaceUpdateResult{}, f.workspaceErr
	}
	result := *f.workspaceResult
	result.DynamicLanguages = append([]string(nil), result.DynamicLanguages...)
	result.RestartRequiredLanguages = append([]string(nil), result.RestartRequiredLanguages...)
	return &result, f.workspaceErr
}

func newFakeLSPHost() *fakeLSPHost {
	return &fakeLSPHost{snapshots: make(map[string]RuntimeSnapshot)}
}

func (f *fakeLSPHost) DiscoverLSP(context.Context) (*DiscoveryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discoveries++
	if f.discovery == nil {
		return &DiscoveryResult{State: DetectionComplete, ScannedAt: time.Unix(102, 0).UTC()}, f.discoveryErr
	}
	result := *f.discovery
	result.Languages = append([]string(nil), f.discovery.Languages...)
	return &result, f.discoveryErr
}

func (f *fakeLSPHost) StartTaskLSP(_ context.Context, request TaskHostStartRequest) (*RuntimeSnapshot, error) {
	f.mu.Lock()
	f.startCalls++
	f.lastStart = request
	entered := f.startEntered
	release := f.startRelease
	f.mu.Unlock()
	if entered != nil {
		select {
		case <-entered:
		default:
			close(entered)
		}
	}
	if release != nil {
		<-release
	}
	f.mu.Lock()
	startErr := f.startErr
	startErrorSnapshot := f.startErrorSnapshot
	if startErr != nil {
		f.snapshots[request.Language] = RuntimeSnapshot{
			Language: request.Language, Generation: request.Generation,
			Phase: PhaseError, ErrorCode: errorCodeProcessStartFailed, ErrorMessage: startErr.Error(),
		}
	}
	f.mu.Unlock()
	if startErr != nil {
		if startErrorSnapshot != nil {
			snapshot := *startErrorSnapshot
			snapshot.Language = request.Language
			snapshot.Generation = request.Generation
			return &snapshot, startErr
		}
		return nil, startErr
	}
	return f.setReady(request.Language, request.Generation), nil
}

func (f *fakeLSPHost) RestartTaskLSP(_ context.Context, request TaskHostStartRequest) (*RuntimeSnapshot, error) {
	f.mu.Lock()
	f.restartCalls++
	f.lastStart = request
	beforeReturn := f.restartBeforeReturn
	response := f.restartResponse
	restartErr := f.restartErr
	f.mu.Unlock()
	if beforeReturn != nil {
		beforeReturn(request)
	}
	if response != nil {
		snapshot := *response
		if snapshot.Language == "" {
			snapshot.Language = request.Language
		}
		if snapshot.Generation == 0 {
			snapshot.Generation = request.Generation
		}
		return &snapshot, restartErr
	}
	if restartErr != nil {
		return nil, restartErr
	}
	return f.setReady(request.Language, request.Generation), nil
}

func (f *fakeLSPHost) UpdateTaskLSPConfiguration(
	_ context.Context,
	request TaskHostConfigurationRequest,
) (*RuntimeSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.configurationCalls++
	f.lastConfiguration = request
	snapshot := f.snapshots[request.Language]
	return &snapshot, nil
}

func (f *fakeLSPHost) StopTaskLSP(_ context.Context, request TaskHostStopRequest) (*RuntimeSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	f.lastStop = request
	if f.stopErr != nil {
		return nil, f.stopErr
	}
	snapshot := RuntimeSnapshot{Language: request.Language, Generation: request.Generation, Phase: PhaseOff}
	f.snapshots[request.Language] = snapshot
	return &snapshot, nil
}

func (f *fakeLSPHost) TaskLSPSnapshot(_ context.Context, language string) (*RuntimeSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	snapshot := f.snapshots[language]
	snapshot.Language = language
	return &snapshot, nil
}

func (f *fakeLSPHost) WatchTaskLSP(ctx context.Context, language string, onSnapshot func(RuntimeSnapshot) error) error {
	f.mu.Lock()
	watchErr := f.watchErr
	entered := f.watchSnapshotEntered
	release := f.watchSnapshotRelease
	done := f.watchSnapshotDone
	f.mu.Unlock()
	if watchErr != nil {
		return watchErr
	}
	snapshot, err := f.TaskLSPSnapshot(ctx, language)
	if err != nil {
		return err
	}
	if entered != nil {
		select {
		case <-entered:
		default:
			close(entered)
		}
	}
	if release != nil {
		<-release
	}
	if done != nil {
		defer func() {
			select {
			case <-done:
			default:
				close(done)
			}
		}()
	}
	if err := onSnapshot(*snapshot); err != nil {
		return err
	}
	<-ctx.Done()
	return context.Cause(ctx)
}

func (f *fakeLSPHost) setReady(language string, generation uint64) *RuntimeSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Unix(101, 0).UTC()
	snapshot := RuntimeSnapshot{
		Language: language, Generation: generation, Phase: PhaseReady,
		Activity: ActivityIdle, ProcessStartedAt: &now, InitializeStartedAt: &now,
		ReadyAt: &now, LastTransitionAt: now,
	}
	f.snapshots[language] = snapshot
	return &snapshot
}
