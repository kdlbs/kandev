package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	workflowengine "github.com/kandev/kandev/internal/workflow/engine"
)

// TestRunsServiceEngineAdapter_QueueRun_CarriesWaveIdentity pins
// runsServiceEngineAdapter.QueueRun's field-by-field copy from the engine's
// QueueRunRequest to the runs-service's QueueRunRequest: it is the only
// seam P2 (queueChildrenCompletedRun) and P3 (ParentWakeReconciler.Tick),
// both engine-routed producers, use to get a completion-wave's WaveKey/
// WaveString into the row insertRun eventually persists as
// wake_wave_key/wake_wave_string. A future edit dropping either field from
// the copy would silently break AC-002.10 cross-producer wake equivalence
// for both producers without any other test in the repository noticing.
func TestRunsServiceEngineAdapter_QueueRun_CarriesWaveIdentity(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	// resolveCausation's workspace lookup (AC-OFFICE-RUN-CAUSATION-001.20)
	// needs a resolvable agent_profiles row for agent-wave-1, mirroring
	// runs/service's own seedAgentProfile test helper.
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"test-agent", "test-agent", now, now,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO agent_profiles (
			id, agent_id, name, agent_display_name, created_at, updated_at, workspace_id
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"agent-wave-1", "test-agent", "agent-wave-1", "agent-wave-1", now, now, "ws-wave-1",
	); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eb := bus.NewMemoryEventBus(log)
	svc := runsservice.New(officeRepo.RunsRepository(), eb, log, nil)

	adapter := &runsServiceEngineAdapter{svc: svc}
	ctx := context.Background()

	outcome, err := adapter.QueueRun(ctx, workflowengine.QueueRunRequest{
		Reason:     "task_children_completed",
		Payload:    map[string]any{"agent_profile_id": "agent-wave-1", "task_id": "parent-1"},
		WaveKey:    "task_children_completed:parent-1:deadbeef",
		WaveString: "parent-1|child-1,child-2",
	})
	if err != nil {
		t.Fatalf("QueueRun: %v", err)
	}
	if outcome != workflowengine.QueueOutcomeQueued {
		t.Fatalf("outcome = %q, want %q", outcome, workflowengine.QueueOutcomeQueued)
	}

	var row struct {
		WakeWaveKey    string `db:"wake_wave_key"`
		WakeWaveString string `db:"wake_wave_string"`
	}
	if err := officeRepo.RunsRepository().Reader().GetContext(ctx, &row,
		`SELECT wake_wave_key, wake_wave_string FROM runs WHERE agent_profile_id = ?`,
		"agent-wave-1",
	); err != nil {
		t.Fatalf("read persisted run: %v", err)
	}
	if row.WakeWaveKey != "task_children_completed:parent-1:deadbeef" {
		t.Errorf("wake_wave_key = %q, want %q", row.WakeWaveKey, "task_children_completed:parent-1:deadbeef")
	}
	if row.WakeWaveString != "parent-1|child-1,child-2" {
		t.Errorf("wake_wave_string = %q, want %q", row.WakeWaveString, "parent-1|child-1,child-2")
	}
}
