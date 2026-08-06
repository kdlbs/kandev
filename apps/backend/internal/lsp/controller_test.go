package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

func TestCapacityAndUnsupportedExecutorDoNotEnsureTaskResources(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"unsupported": readyEnvironment("unsupported", "ssh"),
		"first":       readyEnvironment("first", "local_docker"),
		"queued":      readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{host: newFakeLSPHost()}
	controller := newTestController(tasks, store, &fakeLSPSettings{}, runtimes)
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	unsupported, err := controller.Start(context.Background(), "unsupported", "go", origin)
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Phase != PhaseUnsupported || runtimes.ensureCalls != 0 || controller.capacity.Active() != 0 {
		t.Fatalf("unsupported=%#v ensure=%d active=%d", unsupported, runtimes.ensureCalls, controller.capacity.Active())
	}
	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Phase != PhaseQueued || runtimes.ensureCalls != 1 || controller.capacity.Queued() != 1 {
		t.Fatalf("queued=%#v ensure=%d queued-count=%d", queued, runtimes.ensureCalls, controller.capacity.Queued())
	}
}

func TestCapacityReleaseStartsQueuedAcceptedGeneration(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"queued": readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil || queued.Phase != PhaseQueued || queued.Generation != 1 {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	if _, err := controller.Stop(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}

	promoted, _, err := store.GetTaskLSPLanguage(context.Background(), "queued", "kotlin")
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Phase != PhaseReady || promoted.Generation != 1 ||
		controller.capacity.Active() != 1 || controller.capacity.Queued() != 0 {
		t.Fatalf("promoted=%#v active=%d queued=%d", promoted, controller.capacity.Active(), controller.capacity.Queued())
	}
	if host.startCalls != 2 || store.allocations != 2 {
		t.Fatalf("start calls=%d allocations=%d, want accepted generation reused", host.startCalls, store.allocations)
	}
}

func TestProvenStartFailureReleasesCapacity(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"failed": readyEnvironment("failed", "local_pc"),
		"next":   readyEnvironment("next", "local_pc"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startErr = errors.New("process did not start")
	host.startErrorSnapshot = &RuntimeSnapshot{Phase: PhaseError, ErrorCode: errorCodeProcessStartFailed}
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "failed", "go", origin); err != nil {
		t.Fatalf("failed start: %v", err)
	}
	if active := controller.capacity.Active(); active != 0 {
		t.Fatalf("capacity active after proven process-start failure = %d, want 0", active)
	}
	host.mu.Lock()
	host.startErr = nil
	host.startErrorSnapshot = nil
	host.mu.Unlock()
	next, err := controller.Start(context.Background(), "next", "kotlin", origin)
	if err != nil {
		t.Fatalf("next start: %v", err)
	}
	if next.Phase != PhaseReady {
		t.Fatalf("next phase = %q, want ready", next.Phase)
	}
}

func TestSuccessfulStopReleasesCapacityWhenPersistenceFails(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}
	if _, err := controller.Start(context.Background(), "task-1", "go", origin); err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	store.compareErrAt = store.compareCalls + 2 // mark stopping succeeds; runtime persistence fails.
	store.compareErr = errors.New("persistence unavailable")
	store.mu.Unlock()
	if _, err := controller.Stop(context.Background(), "task-1", "go", origin); err == nil {
		t.Fatal("stop unexpectedly succeeded")
	}
	if active := controller.capacity.Active(); active != 0 {
		t.Fatalf("capacity active after successful task-host stop = %d, want 0", active)
	}
}

func TestConcurrentDuplicateStartCoalescesOneGeneration(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startEntered = make(chan struct{})
	host.startRelease = make(chan struct{})
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	const callers = 20
	results := make(chan *LanguageSnapshot, callers)
	errorsSeen := make(chan error, callers)
	go func() {
		result, err := controller.Start(context.Background(), "task-1", "go", origin)
		results <- result
		errorsSeen <- err
	}()
	<-host.startEntered
	for range callers - 1 {
		go func() {
			result, err := controller.Start(context.Background(), "task-1", "go", origin)
			results <- result
			errorsSeen <- err
		}()
	}
	close(host.startRelease)
	for range callers {
		if err := <-errorsSeen; err != nil {
			t.Fatal(err)
		}
		if result := <-results; result == nil || result.Generation != 1 {
			t.Fatalf("coalesced result = %#v", result)
		}
	}
	if host.startCalls != 1 || store.allocations != 1 {
		t.Fatalf("start calls=%d allocations=%d", host.startCalls, store.allocations)
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

type fakeControllerTasks struct {
	mu             sync.Mutex
	authErr        error
	environmentErr error
	calls          []string
	environments   map[string]*models.TaskEnvironment
}

func (f *fakeControllerTasks) AuthorizeTaskAccess(_ context.Context, taskID string) error {
	f.record("authorize:" + taskID)
	return f.authErr
}

func (f *fakeControllerTasks) GetTask(_ context.Context, taskID string) (*models.Task, error) {
	f.record("task:" + taskID)
	return &models.Task{ID: taskID}, nil
}

func (f *fakeControllerTasks) GetTaskEnvironmentByTaskID(_ context.Context, taskID string) (*models.TaskEnvironment, error) {
	f.record("environment:" + taskID)
	if f.environmentErr != nil {
		return nil, f.environmentErr
	}
	if f.environments != nil {
		return f.environments[taskID], nil
	}
	return readyEnvironment(taskID, "local_pc"), nil
}

func (f *fakeControllerTasks) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *fakeControllerTasks) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeControllerTasks) resetCalls() {
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
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

func (f *fakeLSPSettings) TaskLSPSettings(context.Context) (TaskSettings, error) {
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
}

func (f *fakeLSPRuntimes) EnsureTaskHost(context.Context, string) (TaskHost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureCalls++
	if f.host == nil {
		f.host = newFakeLSPHost()
	}
	return f.host, nil
}

func (f *fakeLSPRuntimes) ExistingTaskHost(context.Context, string) (TaskHost, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.existingCalls++
	if f.host == nil {
		return nil, false, nil
	}
	return f.host, true, nil
}

func (f *fakeLSPRuntimes) CleanupTaskHost(context.Context, string, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanupCalls++
	f.host = nil
	return nil
}

func (f *fakeLSPRuntimes) DiscoverTaskLanguages(context.Context, string) (*DiscoveryResult, error) {
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
	mu                 sync.Mutex
	startCalls         int
	restartCalls       int
	stopCalls          int
	configurationCalls int
	lastStart          TaskHostStartRequest
	lastConfiguration  TaskHostConfigurationRequest
	snapshots          map[string]RuntimeSnapshot
	startEntered       chan struct{}
	startRelease       chan struct{}
	startErr           error
	startErrorSnapshot *RuntimeSnapshot
	stopErr            error
	discovery          *DiscoveryResult
	discoveryErr       error
	discoveries        int
	workspaceResult    *WorkspaceUpdateResult
	workspaceErr       error
	workspaceRefreshes int
}

func (f *fakeLSPHost) RefreshTaskLSPWorkspace(context.Context) (*WorkspaceUpdateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workspaceRefreshes++
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
	f.mu.Unlock()
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
	snapshot := f.snapshots[language]
	snapshot.Language = language
	return &snapshot, nil
}

func (f *fakeLSPHost) WatchTaskLSP(ctx context.Context, language string, onSnapshot func(RuntimeSnapshot) error) error {
	snapshot, err := f.TaskLSPSnapshot(ctx, language)
	if err != nil {
		return err
	}
	if err := onSnapshot(*snapshot); err != nil {
		return err
	}
	<-ctx.Done()
	return context.Cause(ctx)
}

func (f *fakeLSPHost) DialTaskLSPAttach(context.Context, string, uint64) (*websocket.Conn, *http.Response, error) {
	return nil, nil, errors.New("not dialed by controller tests")
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
