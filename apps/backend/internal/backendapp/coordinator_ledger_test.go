package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/mcp/handlers"
)

const ledgerWiringExternalTables = `
	CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, workflow_step_id TEXT, state TEXT, archived_at DATETIME, updated_at DATETIME);
	CREATE TABLE workflow_steps (id TEXT PRIMARY KEY, complete_task_on_enter INTEGER NOT NULL DEFAULT 0);
	CREATE TABLE task_sessions (id TEXT PRIMARY KEY, state TEXT);
	CREATE TABLE task_usage_events (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, turn_id TEXT, agent_type TEXT, model TEXT, created_at DATETIME);
`

func ledgerRowCount(t *testing.T, p routeParams) int {
	t.Helper()
	var n int
	if err := p.dbPool.Reader().Get(&n, `SELECT COUNT(*) FROM coordinator_turns`); err != nil {
		t.Fatalf("count turns: %v", err)
	}
	return n
}

type recordingReaderSetter struct {
	reader handlers.CoordinatorTurnReader
	svc    *coordinator.Service
}

func (r *recordingReaderSetter) SetCoordinatorTurnReader(reader handlers.CoordinatorTurnReader) {
	r.reader = reader
}

func runLedgerWiring(t *testing.T, phase31 bool) (routeParams, bus.EventBus, string) {
	p, eventBus, convTask, _ := runLedgerWiringWithSetter(t, phase31)
	return p, eventBus, convTask
}

func runLedgerWiringWithSetter(t *testing.T, phase31 bool) (routeParams, bus.EventBus, string, *recordingReaderSetter) {
	t.Helper()
	harness := newBootStateTestHarness(t)
	pool := newCoordinatorTestPool(t)
	pool.Writer().SetMaxOpenConns(1)
	if _, err := pool.Writer().Exec(ledgerWiringExternalTables); err != nil {
		t.Fatalf("external tables: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc, err := initCoordinatorWiring(ctx, pool, newCoordinatorTestTracker(t), harness.taskSvc, harness.workflowSvc, nil, true, true, true, phase31, newTestLogger())
	if err != nil || svc == nil {
		t.Fatalf("initCoordinatorWiring: svc=%v err=%v", svc, err)
	}
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	c := &coordinator.Coordinator{WorkspaceID: workspaces[0].ID, Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatal(err)
	}
	const convTask = "conv-task"
	if _, err := pool.Writer().Exec(`UPDATE coordinators SET conversation_task_id = ? WHERE id = ?`, convTask, c.ID); err != nil {
		t.Fatal(err)
	}
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	var cleanups []func() error
	p := routeParams{
		ctx: ctx, dbPool: pool, eventBus: eventBus, taskSvc: harness.taskSvc, log: newTestLogger(),
		addCleanup: func(fn func() error) { cleanups = append(cleanups, fn) },
	}
	setter := &recordingReaderSetter{svc: svc}
	wireCoordinatorLedger(p, svc, setter)
	t.Cleanup(func() {
		for _, fn := range cleanups {
			_ = fn()
		}
	})
	return p, eventBus, convTask, setter
}

func publishTurnStarted(t *testing.T, eventBus bus.EventBus, taskID string) {
	t.Helper()
	err := eventBus.Publish(context.Background(), events.TurnStarted, &bus.Event{Data: map[string]any{
		"id": "st-1", "session_id": "sess-1", "task_id": taskID, "started_at": time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func waitForLedgerRows(t *testing.T, p routeParams, want int) {
	t.Helper()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(5 * time.Second)
	for ledgerRowCount(t, p) != want {
		select {
		case <-tick.C:
		case <-timeout:
			t.Fatalf("ledger rows = %d, want %d", ledgerRowCount(t, p), want)
		}
	}
}

func TestWireCoordinatorLedger_RecordsRegardlessOfThePhase31Flag(t *testing.T) {
	for _, phase31 := range []bool{false, true} {
		p, eventBus, convTask := runLedgerWiring(t, phase31)
		publishTurnStarted(t, eventBus, convTask)
		waitForLedgerRows(t, p, 1)
	}
}

func TestWireCoordinatorLedger_IgnoresTurnsOfOtherTasks(t *testing.T) {
	p, eventBus, _ := runLedgerWiring(t, false)
	publishTurnStarted(t, eventBus, "some-other-task")
	if err := eventBus.Publish(context.Background(), events.TurnStarted, &bus.Event{Data: map[string]any{
		"id": "st-2", "session_id": "sess-2", "task_id": "conv-task", "started_at": time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
	waitForLedgerRows(t, p, 1)
}

func TestWireCoordinatorLedger_DoesNothingWithPhase2Off(t *testing.T) {
	harness := newBootStateTestHarness(t)
	pool := newCoordinatorTestPool(t)
	svc, err := initCoordinatorWiring(context.Background(), pool, newCoordinatorTestTracker(t), harness.taskSvc, harness.workflowSvc, nil, true, false, false, false, newTestLogger())
	if err != nil || svc == nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	registered := false
	wireCoordinatorLedger(routeParams{
		ctx: context.Background(), dbPool: pool, eventBus: bus.NewMemoryEventBus(newTestLogger()), taskSvc: harness.taskSvc,
		log: newTestLogger(), addCleanup: func(func() error) { registered = true },
	}, svc, &recordingReaderSetter{})
	if registered {
		t.Fatal("ledger started with coordinator phase 2 off")
	}
}

func TestWireCoordinatorLedger_InstallsTheTurnReaderOnlyWithPhase31(t *testing.T) {
	for _, phase31 := range []bool{false, true} {
		_, _, _, setter := runLedgerWiringWithSetter(t, phase31)
		if installed := setter.reader != nil; installed != phase31 {
			t.Fatalf("phase31=%v: reader installed = %v", phase31, installed)
		}
	}
}

func TestWireCoordinatorLedger_SettlesTheRowOnTurnCompletion(t *testing.T) {
	p, eventBus, convTask := runLedgerWiring(t, false)
	publishTurnStarted(t, eventBus, convTask)
	waitForLedgerRows(t, p, 1)
	started := time.Now().UTC().Add(-time.Second)
	if err := eventBus.Publish(context.Background(), events.TurnCompleted, &bus.Event{Data: map[string]any{
		"id": "st-1", "session_id": "sess-1", "task_id": convTask, "started_at": started, "completed_at": time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(5 * time.Second)
	for {
		var finished int
		if err := p.dbPool.Reader().Get(&finished, `SELECT COUNT(*) FROM coordinator_turns WHERE finished_at IS NOT NULL AND outcome IS NOT NULL`); err != nil {
			t.Fatal(err)
		}
		if finished == 1 {
			return
		}
		select {
		case <-tick.C:
		case <-timeout:
			t.Fatal("completion event did not settle the ledger row")
		}
	}
}

func TestWireCoordinatorLedger_StampsTheHashOfTheStandingInstructions(t *testing.T) {
	p, eventBus, convTask, setter := runLedgerWiringWithSetter(t, false)
	var c struct {
		ID          string `db:"id"`
		WorkspaceID string `db:"workspace_id"`
	}
	if err := p.dbPool.Reader().Get(&c, `SELECT id, workspace_id FROM coordinators LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.dbPool.Writer().Exec(`UPDATE coordinators SET context = ? WHERE id = ?`, "watch the release queue", c.ID); err != nil {
		t.Fatal(err)
	}
	ws, err := p.taskSvc.GetWorkspace(context.Background(), c.WorkspaceID)
	if err != nil || ws == nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	content, err := coordinatorStandingInstructionsReader(setter.svc, newTestLogger())(context.Background(), c.ID, ws.Name, c.WorkspaceID)
	if err != nil || content == "" {
		t.Fatalf("instructions: %q %v", content, err)
	}
	sum := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(sum[:])

	publishTurnStarted(t, eventBus, convTask)
	waitForLedgerRows(t, p, 1)
	var got string
	if err := p.dbPool.Reader().Get(&got, `SELECT prompt_hash FROM coordinator_turns`); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("prompt_hash = %q, want %q", got, want)
	}
}
