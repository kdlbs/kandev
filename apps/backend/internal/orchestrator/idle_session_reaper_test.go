package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func seedIdleReaperRow(
	t *testing.T,
	repo interface {
		UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
	},
	taskID, sessionID string,
	state models.TaskSessionState,
	status string,
) {
	t.Helper()
	if sqliteRepo, ok := repo.(interface {
		CreateTask(context.Context, *models.Task) error
		CreateTaskSession(context.Context, *models.TaskSession) error
	}); ok {
		now := time.Now().UTC()
		if err := sqliteRepo.CreateTask(context.Background(), &models.Task{
			ID: taskID, Title: taskID, State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("create task: %v", err)
		}
		if err := sqliteRepo.CreateTaskSession(context.Background(), &models.TaskSession{
			ID: sessionID, TaskID: taskID, State: state, StartedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		AgentExecutionID: "exec-" + sessionID,
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           status,
	}); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}
}

func TestIdleReaper_TickFiltersRowsAndStopsPreservedRows(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedIdleReaperRow(t, repo, "task-recent", "s-recent", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)
	seedIdleReaperRow(t, repo, "task-zero", "s-zero", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)
	seedIdleReaperRow(t, repo, "task-future", "s-future", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)
	seedIdleReaperRow(t, repo, "task-stopped", "s-stopped", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusStopped)
	seedIdleReaperRow(t, repo, "task-old", "s-old", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)

	now := time.Now().UTC()
	for sessionID, updatedAt := range map[string]time.Time{
		"s-recent":  now,
		"s-zero":    time.Time{},
		"s-future":  now.Add(time.Hour),
		"s-stopped": now.Add(-time.Hour),
		"s-old":     now.Add(-time.Hour),
	} {
		if _, err := repo.DB().ExecContext(ctx, `UPDATE executors_running SET updated_at = ? WHERE session_id = ?`, updatedAt, sessionID); err != nil {
			t.Fatalf("set updated_at for %s: %v", sessionID, err)
		}
	}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isAgentRunning: false})
	svc.turnService = &inactiveTurnService{}
	svc.idleReaper = newIdleSessionReaper()
	svc.idleReaper.minIdle = 100 * time.Millisecond

	svc.reclaimIdleSessionsOnce(ctx)
	for sessionID, wantStatus := range map[string]string{
		"s-recent":  models.ExecutorRunningStatusRunning,
		"s-zero":    models.ExecutorRunningStatusRunning,
		"s-future":  models.ExecutorRunningStatusRunning,
		"s-stopped": models.ExecutorRunningStatusStopped,
		"s-old":     models.ExecutorRunningStatusStopped,
	} {
		row, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		if err != nil {
			t.Fatalf("load %s: %v", sessionID, err)
		}
		if row.Status != wantStatus {
			t.Fatalf("%s status = %q, want %q", sessionID, row.Status, wantStatus)
		}
	}
	// A preserved stopped row must not be reconsidered on the next tick.
	svc.reclaimIdleSessionsOnce(ctx)
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-old")
	if err != nil || row.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("old row changed after second tick: row=%+v err=%v", row, err)
	}
}

func TestIdleReaper_TickSkipsLiveRuntime(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedIdleReaperRow(t, repo, "task-live", "s-live", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)
	if _, err := repo.DB().ExecContext(ctx, `UPDATE executors_running SET updated_at = ?, local_pid = ? WHERE session_id = ?`, time.Now().UTC().Add(-time.Hour), 9999, "s-live"); err != nil {
		t.Fatal(err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isAgentRunning: true})
	svc.turnService = &inactiveTurnService{}
	svc.idleReaper = newIdleSessionReaper()
	svc.idleReaper.minIdle = 0

	svc.reclaimIdleSessionsOnce(ctx)
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-live")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != models.ExecutorRunningStatusRunning || row.LocalPID != 9999 {
		t.Fatalf("live row changed: status=%q local_pid=%d", row.Status, row.LocalPID)
	}
}

func TestIdleReaper_TickSkipsActiveTurn(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedIdleReaperRow(t, repo, "task-turn", "s-turn", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)
	if _, err := repo.DB().ExecContext(ctx, `UPDATE executors_running SET updated_at = ? WHERE session_id = ?`, time.Now().UTC().Add(-time.Hour), "s-turn"); err != nil {
		t.Fatal(err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isAgentRunning: false})
	svc.turnService = &alwaysActiveTurnService{}
	svc.idleReaper = newIdleSessionReaper()
	svc.idleReaper.minIdle = 0

	svc.reclaimIdleSessionsOnce(ctx)
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-turn")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("active-turn row status = %q, want running", row.Status)
	}
}

func TestIdleReaper_TickSkipsWrongState(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedIdleReaperRow(t, repo, "task-running", "s-running", models.TaskSessionStateRunning, models.ExecutorRunningStatusRunning)
	if _, err := repo.DB().ExecContext(ctx, `UPDATE executors_running SET updated_at = ? WHERE session_id = ?`, time.Now().UTC().Add(-time.Hour), "s-running"); err != nil {
		t.Fatal(err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isAgentRunning: false})
	svc.turnService = &inactiveTurnService{}
	svc.idleReaper = newIdleSessionReaper()
	svc.idleReaper.minIdle = 0

	svc.reclaimIdleSessionsOnce(ctx)
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-running")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("running session status = %q, want running", row.Status)
	}
}

func TestIdleReaper_DoesNotRepairAReplacedExecution(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedIdleReaperRow(t, repo, "task-rotate", "s-rotate", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)
	if _, err := repo.DB().ExecContext(ctx, `UPDATE executors_running SET updated_at = ? WHERE session_id = ?`, time.Now().UTC().Add(-time.Hour), "s-rotate"); err != nil {
		t.Fatal(err)
	}

	base := &mockAgentManager{isAgentRunning: false}
	agent := &rotatingReaperAgentManager{AgentManagerClient: base, repo: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	svc.turnService = &inactiveTurnService{}
	svc.idleReaper = newIdleSessionReaper()
	svc.idleReaper.minIdle = 0

	svc.reclaimIdleSessionsOnce(ctx)
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-rotate")
	if err != nil {
		t.Fatal(err)
	}
	if row.AgentExecutionID != "execution-new" || row.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("successor row was repaired: execution=%q status=%q", row.AgentExecutionID, row.Status)
	}
}

type rotatingReaperAgentManager struct {
	executor.AgentManagerClient
	repo *taskrepo.Repository
}

func (m *rotatingReaperAgentManager) IsAgentRunningForSession(context.Context, string) bool {
	return false
}

func (m *rotatingReaperAgentManager) CleanupStaleExecutionBySessionIDIfCurrent(
	ctx context.Context,
	sessionID, _ string,
	_ time.Time,
) error {
	_, err := m.repo.DB().ExecContext(ctx, `
		UPDATE executors_running
		SET agent_execution_id = ?, status = ?, updated_at = ?
		WHERE session_id = ?
	`, "execution-new", models.ExecutorRunningStatusRunning, time.Now().UTC(), sessionID)
	return err
}

func TestIdleReaper_LoopInheritsParentCancellation(t *testing.T) {
	r := newIdleSessionReaper()
	r.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var ticks atomic.Int32
	if !r.start(ctx, func(context.Context) { ticks.Add(1) }) {
		t.Fatal("start returned false")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for ticks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ticks.Load() == 0 {
		t.Fatal("reaper did not tick")
	}
	cancel()
	count := ticks.Load()
	time.Sleep(30 * time.Millisecond)
	if ticks.Load() != count {
		t.Fatalf("ticks after parent cancellation = %d, want %d", ticks.Load(), count)
	}
	r.stop()
}

func TestIdleReaper_StopStartRestartsLoop(t *testing.T) {
	r := newIdleSessionReaper()
	r.interval = 5 * time.Millisecond
	ctx := context.Background()
	var ticks atomic.Int32
	if !r.start(ctx, func(context.Context) { ticks.Add(1) }) {
		t.Fatal("first start returned false")
	}
	r.stop()
	if r.started {
		t.Fatal("reaper remains started after stop")
	}
	firstCount := ticks.Load()
	if !r.start(ctx, func(context.Context) { ticks.Add(1) }) {
		t.Fatal("second start returned false")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for ticks.Load() == firstCount && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ticks.Load() == firstCount {
		t.Fatal("reaper did not tick after restart")
	}
	r.stop()
}

func TestIdleReaper_StopPreventsFurtherTicks(t *testing.T) {
	r := newIdleSessionReaper()
	r.interval = 5 * time.Millisecond
	var ticks atomic.Int32
	if !r.start(context.Background(), func(context.Context) { ticks.Add(1) }) {
		t.Fatal("start returned false")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for ticks.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ticks.Load() < 2 {
		t.Fatalf("ticks before stop = %d, want at least 2", ticks.Load())
	}
	r.stop()
	afterStop := ticks.Load()
	time.Sleep(30 * time.Millisecond)
	if ticks.Load() != afterStop {
		t.Fatalf("ticks after stop = %d, want %d", ticks.Load(), afterStop)
	}
}

type alwaysActiveTurnService struct{}

func (*alwaysActiveTurnService) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return &models.Turn{ID: "turn-always"}, nil
}
func (*alwaysActiveTurnService) StartTurn(context.Context, string) (*models.Turn, error) {
	panic("alwaysActiveTurnService: StartTurn should not be called")
}
func (*alwaysActiveTurnService) ReserveTurn(context.Context, string, *models.PromptDispatchRecovery) (*models.Turn, error) {
	panic("alwaysActiveTurnService: ReserveTurn should not be called")
}
func (*alwaysActiveTurnService) MarkReservedTurnDispatchAttempted(context.Context, *models.Turn) error {
	panic("alwaysActiveTurnService: MarkReservedTurnDispatchAttempted should not be called")
}
func (*alwaysActiveTurnService) PublishReservedTurn(context.Context, *models.Turn) error {
	panic("alwaysActiveTurnService: PublishReservedTurn should not be called")
}
func (*alwaysActiveTurnService) RollbackReservedTurn(context.Context, string, string) (bool, error) {
	panic("alwaysActiveTurnService: RollbackReservedTurn should not be called")
}
func (*alwaysActiveTurnService) ReconcileUnpublishedPromptTurns(context.Context) (int, error) {
	return 0, nil
}
func (*alwaysActiveTurnService) CompleteTurn(context.Context, string) error { return nil }
func (*alwaysActiveTurnService) GetTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}
func (*alwaysActiveTurnService) UpdateTurn(context.Context, *models.Turn) error { return nil }
func (*alwaysActiveTurnService) PatchTurnMetadata(context.Context, string, string, map[string]interface{}) error {
	return nil
}
func (*alwaysActiveTurnService) AbandonOpenTurns(context.Context, string) error { return nil }
