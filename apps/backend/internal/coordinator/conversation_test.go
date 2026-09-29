package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// fakeConversationTasks is a test double for ConversationTaskManager backed by
// an in-memory map. onCreate, when set, runs after a task is created and
// before CreateTask returns, letting a test inject a concurrent write (e.g.
// another SetConversationTaskID winning, or the coordinator being deleted)
// deterministically instead of with real goroutines.
type fakeConversationTasks struct {
	tasks map[string]*taskmodels.Task

	createErr  error
	getErr     error
	archiveErr error
	deleteErr  error
	listErr    error
	updateErr  error

	onCreate func(task *taskmodels.Task)

	createdIDs  []string
	deletedIDs  []string
	archivedIDs []string
}

func newFakeConversationTasks() *fakeConversationTasks {
	return &fakeConversationTasks{tasks: map[string]*taskmodels.Task{}}
}

func (f *fakeConversationTasks) CreateTask(_ context.Context, req *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error) {
	if f.createErr != nil {
		return taskservice.CreateTaskResult{}, f.createErr
	}
	id := "task-" + time.Now().UTC().Format("150405.000000000") + "-" + req.Title
	task := &taskmodels.Task{
		ID:          id,
		WorkspaceID: req.WorkspaceID,
		Title:       req.Title,
		IsEphemeral: req.IsEphemeral,
		Metadata:    req.Metadata,
		CreatedAt:   time.Now().UTC(),
	}
	f.tasks[id] = task
	f.createdIDs = append(f.createdIDs, id)
	if f.onCreate != nil {
		f.onCreate(task)
	}
	return taskservice.CreateTaskResult{Task: task}, nil
}

func (f *fakeConversationTasks) UpdateTask(_ context.Context, id string, req *taskservice.UpdateTaskRequest) (*taskmodels.Task, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return nil, taskrepo.ErrTaskNotFound
	}
	if req.Metadata != nil {
		task.Metadata = req.Metadata
	}
	return task, nil
}

func (f *fakeConversationTasks) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return nil, taskrepo.ErrTaskNotFound
	}
	return task, nil
}

func (f *fakeConversationTasks) ArchiveTask(_ context.Context, id string) error {
	if f.archiveErr != nil {
		return f.archiveErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return taskrepo.ErrTaskNotFound
	}
	if task.ArchivedAt != nil {
		return fmt.Errorf("%w: %s", taskservice.ErrTaskAlreadyArchived, id)
	}
	now := time.Now().UTC()
	task.ArchivedAt = &now
	f.archivedIDs = append(f.archivedIDs, id)
	return nil
}

func (f *fakeConversationTasks) DeleteTask(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.tasks[id]; !ok {
		return taskrepo.ErrTaskNotFound
	}
	delete(f.tasks, id)
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func (f *fakeConversationTasks) ListCoordinatorOriginTasks(_ context.Context, workspaceID string) ([]*taskmodels.Task, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []*taskmodels.Task
	for _, task := range f.tasks {
		if workspaceID != "" && task.WorkspaceID != workspaceID {
			continue
		}
		out = append(out, task)
	}
	return out, nil
}

// fakeSessionEnsurer is a test double for SessionEnsurer.
type fakeSessionEnsurer struct {
	err      error
	sessions map[string]string // taskID -> sessionID
	states   map[string]string // taskID -> session state reported by EnsureSession
	calls    []string
	lastOpts orchestrator.EnsureSessionOptions
}

func newFakeSessionEnsurer() *fakeSessionEnsurer {
	return &fakeSessionEnsurer{sessions: map[string]string{}, states: map[string]string{}}
}

func (f *fakeSessionEnsurer) EnsureSession(_ context.Context, taskID string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	f.calls = append(f.calls, taskID)
	if len(opts) > 0 {
		f.lastOpts = opts[0]
	}
	if f.err != nil {
		return nil, f.err
	}
	sessionID, ok := f.sessions[taskID]
	if !ok {
		sessionID = "session-" + taskID
		f.sessions[taskID] = sessionID
	}
	return &orchestrator.EnsureSessionResponse{TaskID: taskID, SessionID: sessionID, State: f.states[taskID]}, nil
}

// conversationTestDeps bundles a real Store-backed Service with fake
// conversation dependencies, and the coordinator row OpenConversation acts
// on.
type conversationTestDeps struct {
	svc         *Service
	tasks       *fakeConversationTasks
	sessions    *fakeSessionEnsurer
	coordinator *Coordinator
}

func newConversationTestDeps(t *testing.T) *conversationTestDeps {
	t.Helper()
	const workspaceID = "ws-1"
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}
	svc := newServiceForTest(t, agents, executors, nil)

	coord := &Coordinator{WorkspaceID: workspaceID, Name: "Release Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1"}
	if err := svc.store.CreateCoordinator(context.Background(), coord); err != nil {
		t.Fatalf("seed coordinator: %v", err)
	}

	tasks := newFakeConversationTasks()
	sessions := newFakeSessionEnsurer()
	svc.SetConversationDeps(tasks, sessions)

	return &conversationTestDeps{svc: svc, tasks: tasks, sessions: sessions, coordinator: coord}
}

func TestSetConversationDepsAndReady(t *testing.T) {
	svc := newServiceForTest(t, nil, nil, nil)
	if svc.ConversationDepsReady() {
		t.Fatal("ConversationDepsReady() = true before SetConversationDeps, want false")
	}
	svc.SetConversationDeps(newFakeConversationTasks(), newFakeSessionEnsurer())
	if !svc.ConversationDepsReady() {
		t.Fatal("ConversationDepsReady() = false after SetConversationDeps, want true")
	}
}

func TestOpenConversationCreatesAndCommitsNewTask(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	result, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("OpenConversation() unexpected error: %v", err)
	}
	if result.TaskID == "" || result.SessionID == "" {
		t.Fatalf("OpenConversation() result = %+v, want non-empty task and session ids", result)
	}
	if len(deps.tasks.createdIDs) != 1 {
		t.Fatalf("created %d tasks, want exactly 1", len(deps.tasks.createdIDs))
	}
	created := deps.tasks.tasks[result.TaskID]
	if created.Metadata[taskmodels.MetaKeyCoordinatorID] != deps.coordinator.ID {
		t.Errorf("created task coordinator_id metadata = %v, want %q", created.Metadata[taskmodels.MetaKeyCoordinatorID], deps.coordinator.ID)
	}
	if created.Metadata[taskmodels.MetaKeyAgentProfileID] != "ap-1" || created.Metadata[taskmodels.MetaKeyExecutorProfileID] != "ep-1" {
		t.Errorf("created task carries wrong profile metadata: %+v", created.Metadata)
	}

	reread, err := deps.svc.store.GetCoordinatorByID(ctx, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("reread coordinator: %v", err)
	}
	if reread.ConversationTaskID == nil || *reread.ConversationTaskID != result.TaskID {
		t.Errorf("coordinator conversation_task_id = %v, want %q", reread.ConversationTaskID, result.TaskID)
	}
}

func TestOpenConversationReusesLiveUnarchivedTask(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("first OpenConversation() unexpected error: %v", err)
	}

	second, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("second OpenConversation() unexpected error: %v", err)
	}
	if second.TaskID != first.TaskID {
		t.Errorf("second open task = %q, want reuse of %q", second.TaskID, first.TaskID)
	}
	if len(deps.tasks.createdIDs) != 1 {
		t.Errorf("created %d tasks across two opens, want exactly 1 (reuse)", len(deps.tasks.createdIDs))
	}
}

// A conversation task whose session has ended (failed, cancelled or
// completed) can never take another message, so reopening must archive it and
// start a fresh conversation task rather than hand the dead session back.
func TestOpenConversationReplacesTaskWithTerminalSession(t *testing.T) {
	for _, state := range []taskmodels.TaskSessionState{
		taskmodels.TaskSessionStateFailed,
		taskmodels.TaskSessionStateCancelled,
		taskmodels.TaskSessionStateCompleted,
	} {
		t.Run(string(state), func(t *testing.T) {
			deps := newConversationTestDeps(t)
			ctx := context.Background()

			first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
			if err != nil {
				t.Fatalf("first OpenConversation() unexpected error: %v", err)
			}
			deps.sessions.states[first.TaskID] = string(state)

			second, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
			if err != nil {
				t.Fatalf("second OpenConversation() unexpected error: %v", err)
			}
			if second.TaskID == first.TaskID {
				t.Fatalf("second open reused task %q whose session is %s", first.TaskID, state)
			}
			if len(deps.tasks.archivedIDs) != 1 || deps.tasks.archivedIDs[0] != first.TaskID {
				t.Errorf("archived = %v, want [%s]", deps.tasks.archivedIDs, first.TaskID)
			}
			reread, err := deps.svc.store.GetCoordinatorByID(ctx, deps.coordinator.ID)
			if err != nil {
				t.Fatalf("GetCoordinatorByID: %v", err)
			}
			if reread.ConversationTaskID == nil || *reread.ConversationTaskID != second.TaskID {
				t.Errorf("current conversation task = %v, want %q", reread.ConversationTaskID, second.TaskID)
			}
		})
	}
}

// TestOpenConversationReusesNonTerminalSessionStates covers copilot.md
// #conversation-task step 2's closing sentence: CREATED, STARTING, RUNNING,
// IDLE and WAITING_FOR_INPUT are all reusable, not just the empty/zero-value
// state the other reuse tests happen to exercise.
func TestOpenConversationReusesNonTerminalSessionStates(t *testing.T) {
	for _, state := range []taskmodels.TaskSessionState{
		taskmodels.TaskSessionStateWaitingForInput,
		taskmodels.TaskSessionStateCreated,
		taskmodels.TaskSessionStateStarting,
		taskmodels.TaskSessionStateRunning,
		taskmodels.TaskSessionStateIdle,
	} {
		t.Run(string(state), func(t *testing.T) {
			deps := newConversationTestDeps(t)
			ctx := context.Background()

			first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
			if err != nil {
				t.Fatalf("first OpenConversation() unexpected error: %v", err)
			}
			deps.sessions.states[first.TaskID] = string(state)

			second, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
			if err != nil {
				t.Fatalf("second OpenConversation() unexpected error: %v", err)
			}
			if second.TaskID != first.TaskID {
				t.Errorf("second open task = %q, want reuse of %q (session state %s)", second.TaskID, first.TaskID, state)
			}
			if len(deps.tasks.archivedIDs) != 0 {
				t.Errorf("archivedIDs = %v for a reusable session state %s, want none", deps.tasks.archivedIDs, state)
			}
		})
	}
}

// TestOpenConversationTerminalSessionArchiveFailureStillReplacesTask covers
// copilot.md#conversation-task step 2: a failed archive (logged warn with the
// coordinator id, task id and session state) does not block the open from
// continuing at step 3 with a fresh task.
func TestOpenConversationTerminalSessionArchiveFailureStillReplacesTask(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("first OpenConversation() unexpected error: %v", err)
	}
	deps.sessions.states[first.TaskID] = string(taskmodels.TaskSessionStateFailed)
	deps.tasks.archiveErr = errors.New("archive boom")

	second, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("second OpenConversation() unexpected error: %v", err)
	}
	if second.TaskID == first.TaskID {
		t.Fatalf("second open reused task %q despite a terminal session", first.TaskID)
	}
	if len(deps.tasks.archivedIDs) != 0 {
		t.Errorf("archivedIDs = %v, want none: ArchiveTask always fails in this test", deps.tasks.archivedIDs)
	}
	reread, err := deps.svc.store.GetCoordinatorByID(ctx, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("GetCoordinatorByID: %v", err)
	}
	if reread.ConversationTaskID == nil || *reread.ConversationTaskID != second.TaskID {
		t.Errorf("current conversation task = %v, want %q", reread.ConversationTaskID, second.TaskID)
	}
}

// TestOpenConversationRacingOpensOverEndedTaskConverge covers copilot.md
// #conversation-task step 2's race note: two opens racing over the same
// ended task both reach step 4 with the same stale value (the ended task's
// id), so exactly one new conversation task wins and the loser deletes its
// own and converges on it.
func TestOpenConversationRacingOpensOverEndedTaskConverge(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("first OpenConversation() unexpected error: %v", err)
	}
	deps.sessions.states[first.TaskID] = string(taskmodels.TaskSessionStateFailed)

	var winnerTaskID, loserTaskID string
	raceInjected := false
	deps.tasks.onCreate = func(task *taskmodels.Task) {
		if raceInjected {
			return // only race the second open's own create
		}
		raceInjected = true
		loserTaskID = task.ID
		winner, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
			WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: concurrent winner",
		})
		if err != nil {
			t.Fatalf("seed winner task: %v", err)
		}
		winnerTaskID = winner.Task.ID
		// The concurrent winner races over the same ended task: its stale
		// value is also first.TaskID.
		ok, err := deps.svc.store.SetConversationTaskID(ctx, deps.coordinator.ID, winnerTaskID, first.TaskID, deps.coordinator.ConfigRevision)
		if err != nil || !ok {
			t.Fatalf("commit winner task: ok=%v err=%v", ok, err)
		}
	}

	second, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("second OpenConversation() unexpected error: %v", err)
	}
	if second.TaskID != winnerTaskID {
		t.Errorf("OpenConversation() task = %q, want the race winner %q", second.TaskID, winnerTaskID)
	}
	if _, stillExists := deps.tasks.tasks[loserTaskID]; stillExists {
		t.Errorf("losing task %q was not deleted", loserTaskID)
	}
	if len(deps.tasks.archivedIDs) != 1 || deps.tasks.archivedIDs[0] != first.TaskID {
		t.Errorf("archivedIDs = %v, want [%s] (the original ended task, archived exactly once)", deps.tasks.archivedIDs, first.TaskID)
	}
}

// TestOpenConversationReuseArchiveCollisionSurfacesAlreadyArchived covers
// copilot.md #conversation-task step 2's race note directly: two callers
// that both independently observe the same terminal task each call
// ArchiveTask on it. The first succeeds; the second must hit
// ErrTaskAlreadyArchived, exactly like the real task service, and that
// error must be logged and swallowed identically to any other archive
// failure rather than surfaced or treated as a second successful archive.
func TestOpenConversationReuseArchiveCollisionSurfacesAlreadyArchived(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("first OpenConversation() unexpected error: %v", err)
	}
	deps.sessions.states[first.TaskID] = string(taskmodels.TaskSessionStateFailed)

	resultA, reusableA, errA := deps.svc.reuseConversationTask(ctx, deps.coordinator.ID, first.TaskID)
	if errA != nil || reusableA || resultA != nil {
		t.Fatalf("caller A: result=%v reusable=%v err=%v, want (nil, false, nil)", resultA, reusableA, errA)
	}
	if len(deps.tasks.archivedIDs) != 1 || deps.tasks.archivedIDs[0] != first.TaskID {
		t.Fatalf("archivedIDs after caller A = %v, want [%s]", deps.tasks.archivedIDs, first.TaskID)
	}

	resultB, reusableB, errB := deps.svc.reuseConversationTask(ctx, deps.coordinator.ID, first.TaskID)
	if errB != nil || reusableB || resultB != nil {
		t.Fatalf("caller B: result=%v reusable=%v err=%v, want (nil, false, nil): the archive collision must be logged and swallowed, not surfaced", resultB, reusableB, errB)
	}
	if len(deps.tasks.archivedIDs) != 1 {
		t.Errorf("archivedIDs after caller B = %v, want still exactly one entry: B's ArchiveTask must fail as already-archived, not succeed a second time", deps.tasks.archivedIDs)
	}
}

func TestOpenConversationProfileUnavailableCreatesNoTask(t *testing.T) {
	const workspaceID = "ws-1"
	// No agent/executor profiles registered: ProfileStatus resolves to
	// missing for both.
	svc := newServiceForTest(t, nil, nil, nil)
	coord := &Coordinator{WorkspaceID: workspaceID, Name: "Coordinator", AgentProfileID: "ap-missing", ExecutorProfileID: "ep-missing"}
	if err := svc.store.CreateCoordinator(context.Background(), coord); err != nil {
		t.Fatalf("seed coordinator: %v", err)
	}
	tasks := newFakeConversationTasks()
	svc.SetConversationDeps(tasks, newFakeSessionEnsurer())

	_, err := svc.OpenConversation(context.Background(), workspaceID, coord.ID)
	var profileErr *ProfileUnavailableError
	if !errors.As(err, &profileErr) {
		t.Fatalf("OpenConversation() error = %v, want *ProfileUnavailableError", err)
	}
	if len(tasks.createdIDs) != 0 {
		t.Errorf("created %d tasks despite unavailable profile, want 0", len(tasks.createdIDs))
	}
}

// TestOpenConversationCreateRaceConverges simulates step 4's zero-rows branch:
// while this call's CreateTask is "in flight", a concurrent request commits
// its own task first. The losing call must delete the task it just created
// and converge on the winner.
func TestOpenConversationCreateRaceConverges(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	var winnerTaskID string
	raceInjected := false
	deps.tasks.onCreate = func(task *taskmodels.Task) {
		if raceInjected {
			return // only race the first create; avoid recursing into the winner's own onCreate
		}
		raceInjected = true
		winner, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
			WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: concurrent winner",
			Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: deps.coordinator.ID},
		})
		if err != nil {
			t.Fatalf("seed winner task: %v", err)
		}
		winnerTaskID = winner.Task.ID
		ok, err := deps.svc.store.SetConversationTaskID(ctx, deps.coordinator.ID, winnerTaskID, "", deps.coordinator.ConfigRevision)
		if err != nil || !ok {
			t.Fatalf("commit winner task: ok=%v err=%v", ok, err)
		}
	}

	result, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("OpenConversation() unexpected error: %v", err)
	}
	if result.TaskID != winnerTaskID {
		t.Errorf("OpenConversation() task = %q, want the race winner %q", result.TaskID, winnerTaskID)
	}
	loserTaskID := deps.tasks.createdIDs[0]
	if loserTaskID == winnerTaskID {
		t.Fatalf("test setup bug: loser id equals winner id")
	}
	if _, stillExists := deps.tasks.tasks[loserTaskID]; stillExists {
		t.Errorf("losing task %q was not deleted", loserTaskID)
	}
}

// TestOpenConversationCreateRaceCoordinatorDeleted covers step 5: the
// coordinator was deleted between step 1's read and step 4's CAS.
func TestOpenConversationCreateRaceCoordinatorDeleted(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	deps.tasks.onCreate = func(*taskmodels.Task) {
		if err := deps.svc.store.DeleteCoordinator(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID); err != nil {
			t.Fatalf("delete coordinator mid-create: %v", err)
		}
	}

	_, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenConversation() error = %v, want ErrNotFound", err)
	}
	if len(deps.tasks.tasks) != 0 {
		t.Errorf("%d tasks remain after coordinator-deleted race, want 0", len(deps.tasks.tasks))
	}
}

// TestResolveConversationCreateRaceReferenceCleared covers step 4's
// zero-rows branch landing on a coordinator whose conversation_task_id is
// NULL by the time of the reread (a concurrent context/profile change won
// the race and cleared the reference): resolveConversationCreateRace finds no
// task to converge on and reports ErrConversationConflict, having deleted the
// task this call created.
func TestResolveConversationCreateRaceReferenceCleared(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	created, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: mine"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	// The coordinator's conversation_task_id is already NULL (never set),
	// simulating the reread landing after a concurrent clear.

	_, err = deps.svc.resolveConversationCreateRace(ctx, deps.coordinator.ID, created.Task.ID, deps.coordinator.ConfigRevision)
	if !errors.Is(err, ErrConversationConflict) {
		t.Fatalf("resolveConversationCreateRace() error = %v, want ErrConversationConflict", err)
	}
	if _, exists := deps.tasks.tasks[created.Task.ID]; exists {
		t.Error("the task created by the losing call was not deleted")
	}
}

// TestResolveConversationCreateRaceConfigRevisionMismatch covers step 4's
// zero-rows branch when the reread's config_revision differs from the value
// read in step 1: a context/profile change was saved while this call created
// its task under the earlier configuration, so it must report
// ErrConversationConflict and delete the task it created, even when the
// reference happens to be NULL on both sides (checked before the
// reference-cleared branch).
func TestResolveConversationCreateRaceConfigRevisionMismatch(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	created, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: mine"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	newContext := "changed context"
	if _, _, err := deps.svc.store.PatchCoordinator(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID, CoordinatorPatch{Context: &newContext}, nil); err != nil {
		t.Fatalf("PatchCoordinator: %v", err)
	}

	staleConfigRevision := deps.coordinator.ConfigRevision
	_, err = deps.svc.resolveConversationCreateRace(ctx, deps.coordinator.ID, created.Task.ID, staleConfigRevision)
	if !errors.Is(err, ErrConversationConflict) {
		t.Fatalf("resolveConversationCreateRace() error = %v, want ErrConversationConflict", err)
	}
	if _, exists := deps.tasks.tasks[created.Task.ID]; exists {
		t.Error("the task created by the losing call was not deleted")
	}
}

func TestOpenConversationEnsureSessionFailureLeavesTaskCurrent(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()
	deps.sessions.err = errors.New("ensure session boom")

	_, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if !errors.Is(err, ErrConversationSessionUnavailable) {
		t.Fatalf("OpenConversation() error = %v, want ErrConversationSessionUnavailable", err)
	}
	reread, err := deps.svc.store.GetCoordinatorByID(ctx, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("reread coordinator: %v", err)
	}
	if reread.ConversationTaskID == nil {
		t.Fatal("conversation_task_id was cleared after a session-ensure failure, want it left as current")
	}
	taskID := *reread.ConversationTaskID

	// The next open retries only the session step against the same task.
	deps.sessions.err = nil
	result, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("retry OpenConversation() unexpected error: %v", err)
	}
	if result.TaskID != taskID {
		t.Errorf("retry task = %q, want the same task %q", result.TaskID, taskID)
	}
	if len(deps.tasks.createdIDs) != 1 {
		t.Errorf("created %d tasks across the failure and retry, want exactly 1", len(deps.tasks.createdIDs))
	}
}

func TestOpenConversationEnsureSessionFailureCoordinatorGoneIsNotFound(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()
	deps.sessions.err = errors.New("ensure session boom")

	// Delete the coordinator once CreateTask has run but before EnsureSession
	// is evaluated, by racing it inside onCreate.
	deps.tasks.onCreate = func(*taskmodels.Task) {
		if err := deps.svc.store.DeleteCoordinator(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID); err != nil {
			t.Fatalf("delete coordinator: %v", err)
		}
	}

	_, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenConversation() error = %v, want ErrNotFound (not %v)", err, ErrConversationSessionUnavailable)
	}
	if len(deps.tasks.tasks) != 0 {
		t.Errorf("%d tasks remain after coordinator-gone ensure failure, want the task deleted", len(deps.tasks.tasks))
	}
}

func TestOpenConversationConfirmMismatchIsConflict(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	// After EnsureSession succeeds but before step 7's re-read, a concurrent
	// context change repoints the coordinator at a different task.
	other, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: other"})
	if err != nil {
		t.Fatalf("create other task: %v", err)
	}

	origEnsure := deps.sessions
	origEnsure.err = nil
	// Wrap EnsureSession via a small shim that repoints the coordinator once,
	// simulating the race landing between step 6 and step 7.
	repointed := false
	deps.svc.SetConversationDeps(deps.tasks, ensureSessionFunc(func(ctx context.Context, taskID string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
		resp, err := origEnsure.EnsureSession(ctx, taskID, opts...)
		if err == nil && !repointed {
			repointed = true
			if ok, casErr := deps.svc.store.SetConversationTaskID(ctx, deps.coordinator.ID, other.Task.ID, taskID, deps.coordinator.ConfigRevision); casErr != nil || !ok {
				t.Fatalf("repoint coordinator: ok=%v err=%v", ok, casErr)
			}
		}
		return resp, err
	}))

	_, err = deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if !errors.Is(err, ErrConversationConflict) {
		t.Fatalf("OpenConversation() error = %v, want ErrConversationConflict", err)
	}
}

// ensureSessionFunc adapts a function to SessionEnsurer.
type ensureSessionFunc func(ctx context.Context, taskID string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error)

func (f ensureSessionFunc) EnsureSession(ctx context.Context, taskID string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return f(ctx, taskID, opts...)
}

// TestOpenConversationConfirmDetectsConcurrentArchive covers Review round 2
// Finding A: EnsureSession's per-task lock (session_ensure.go) is released as
// soon as EnsureSession returns, so it does not span the caller's subsequent
// confirmConversationTask re-read. If a concurrent opener (caller B) archives
// this same task in that window -- because it independently observed a
// terminal session state before caller A's own create+CAS (if any) would
// otherwise have changed ConversationTaskID -- confirmConversationTask's
// ConversationTaskID-only check still matches and must not hand the
// now-archived task back to caller A as a success.
func TestOpenConversationConfirmDetectsConcurrentArchive(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("first OpenConversation() unexpected error: %v", err)
	}

	origEnsure := deps.sessions
	raced := false
	deps.svc.SetConversationDeps(deps.tasks, ensureSessionFunc(func(ctx context.Context, taskID string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
		resp, err := origEnsure.EnsureSession(ctx, taskID, opts...)
		if err == nil && !raced && taskID == first.TaskID {
			raced = true
			// Caller B observes the same task as terminal and archives it, in
			// the window between caller A's EnsureSession call above and A's
			// own confirmConversationTask re-read below.
			origEnsure.states[taskID] = string(taskmodels.TaskSessionStateFailed)
			if _, reusableB, errB := deps.svc.reuseConversationTask(ctx, deps.coordinator.ID, taskID); errB != nil || reusableB {
				t.Fatalf("caller B setup: reusable=%v err=%v", reusableB, errB)
			}
		}
		return resp, err
	}))

	_, err = deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if !errors.Is(err, ErrConversationConflict) {
		t.Fatalf("caller A OpenConversation() error = %v, want ErrConversationConflict: the task was archived by a concurrent opener between EnsureSession and confirm", err)
	}
	if len(deps.tasks.archivedIDs) != 1 || deps.tasks.archivedIDs[0] != first.TaskID {
		t.Errorf("archivedIDs = %v, want exactly [%s] (caller B's archive)", deps.tasks.archivedIDs, first.TaskID)
	}
}

func TestOpenConversationDeletedConversationTaskCreatesNew(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	first, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("first OpenConversation() unexpected error: %v", err)
	}
	// The task is deleted out from under the coordinator (operator action or
	// concurrent cleanup), distinct from being archived.
	delete(deps.tasks.tasks, first.TaskID)

	second, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("second OpenConversation() unexpected error: %v", err)
	}
	if second.TaskID == first.TaskID {
		t.Error("second open reused the deleted task id")
	}
	if len(deps.tasks.createdIDs) != 2 {
		t.Errorf("created %d tasks, want 2 (original + replacement)", len(deps.tasks.createdIDs))
	}
}

func TestArchiveClearedConversationTaskCountsNotFoundAndAlreadyArchivedAsDone(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	// Not-found: swallowed, no warning path observable except absence of panic.
	deps.svc.ArchiveClearedConversationTask(ctx, deps.coordinator.ID, "does-not-exist")

	task, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: cleared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	deps.svc.ArchiveClearedConversationTask(ctx, deps.coordinator.ID, task.Task.ID)
	if deps.tasks.tasks[task.Task.ID].ArchivedAt == nil {
		t.Fatal("task was not archived")
	}
	// Already archived: also counts as done, no error surfaces.
	deps.svc.ArchiveClearedConversationTask(ctx, deps.coordinator.ID, task.Task.ID)
}

func TestDeleteConversationTasksForCoordinatorDeletesOnlyMatching(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	mine, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: mine",
		Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: deps.coordinator.ID},
	})
	if err != nil {
		t.Fatalf("create mine: %v", err)
	}
	other, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: other",
		Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: "other-coordinator"},
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	deps.svc.DeleteConversationTasksForCoordinator(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)

	if _, exists := deps.tasks.tasks[mine.Task.ID]; exists {
		t.Error("this coordinator's task was not deleted")
	}
	if _, exists := deps.tasks.tasks[other.Task.ID]; !exists {
		t.Error("another coordinator's task was deleted")
	}
}

func TestCleanupConversationTasksRepairsEachCase(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Add(time.Hour) // after every task created below

	// Case 1: unknown coordinator_id -> deleted.
	orphan, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: orphan",
		Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: "unknown-coordinator"},
	})
	if err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	// Case 2: current task -> left alone.
	current, err := deps.svc.OpenConversation(ctx, deps.coordinator.WorkspaceID, deps.coordinator.ID)
	if err != nil {
		t.Fatalf("open conversation: %v", err)
	}

	// Case 3: unarchived, not current -> archived.
	stale, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: stale",
		Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: deps.coordinator.ID},
	})
	if err != nil {
		t.Fatalf("create stale: %v", err)
	}

	deps.svc.CleanupConversationTasks(ctx, t0)

	if _, exists := deps.tasks.tasks[orphan.Task.ID]; exists {
		t.Error("orphaned task (unknown coordinator) was not deleted")
	}
	if deps.tasks.tasks[current.TaskID].ArchivedAt != nil {
		t.Error("current conversation task was archived, want left alone")
	}
	if deps.tasks.tasks[stale.Task.ID].ArchivedAt == nil {
		t.Error("stale non-current task was not archived")
	}

	// Idempotent: running again changes nothing further.
	beforeDeleted := len(deps.tasks.deletedIDs)
	beforeArchived := len(deps.tasks.archivedIDs)
	deps.svc.CleanupConversationTasks(ctx, t0)
	if len(deps.tasks.deletedIDs) != beforeDeleted || len(deps.tasks.archivedIDs) != beforeArchived {
		t.Error("second CleanupConversationTasks run was not a no-op")
	}
}

func TestCleanupConversationTasksIgnoresTasksAtOrAfterT0(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Add(-time.Hour) // before every task created below

	stale, err := deps.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: deps.coordinator.WorkspaceID, Title: "Coordinator: fresh",
		Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: deps.coordinator.ID},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	deps.svc.CleanupConversationTasks(ctx, t0)

	if _, exists := deps.tasks.tasks[stale.Task.ID]; !exists {
		t.Error("a task created at/after t0 was touched by cleanup")
	}
	if deps.tasks.tasks[stale.Task.ID].ArchivedAt != nil {
		t.Error("a task created at/after t0 was archived by cleanup")
	}
}

func TestConversationTaskCoordinatorID(t *testing.T) {
	if got := conversationTaskCoordinatorID(nil); got != "" {
		t.Errorf("nil task = %q, want empty", got)
	}
	if got := conversationTaskCoordinatorID(&taskmodels.Task{}); got != "" {
		t.Errorf("nil metadata = %q, want empty", got)
	}
	task := &taskmodels.Task{Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: "co-1"}}
	if got := conversationTaskCoordinatorID(task); got != "co-1" {
		t.Errorf("coordinator id = %q, want %q", got, "co-1")
	}
	nonString := &taskmodels.Task{Metadata: map[string]interface{}{taskmodels.MetaKeyCoordinatorID: 42}}
	if got := conversationTaskCoordinatorID(nonString); got != "" {
		t.Errorf("non-string metadata value = %q, want empty", got)
	}
}
