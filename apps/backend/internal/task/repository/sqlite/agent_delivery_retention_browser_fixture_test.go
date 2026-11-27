package sqlite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

const retentionBrowserFixtureSessionID = "retention-browser-settled-session"
const retentionBrowserFixtureStreamID = "retention-browser-settled-stream"
const retentionBrowserFixtureSubmissionID = "retention-browser-settled-prompt"

// The browser harness explicitly supplies its disposable database and task.
// This driver seeds delivery through repository APIs; only the real collector
// can prune payloads or advance the retirement checkpoint.
func TestRetentionBrowserFixtureDriver(t *testing.T) {
	path := os.Getenv("KANDEV_RETENTION_BROWSER_FIXTURE_DB")
	if path == "" {
		t.Skip("set KANDEV_RETENTION_BROWSER_FIXTURE_DB for the disposable browser fixture driver")
	}
	if !filepath.IsAbs(path) {
		t.Fatal("browser fixture database path must be absolute")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	taskID := os.Getenv("KANDEV_RETENTION_BROWSER_FIXTURE_TASK_ID")
	if taskID == "" {
		t.Fatal("set KANDEV_RETENTION_BROWSER_FIXTURE_TASK_ID to an existing disposable task")
	}
	raw, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	defer func() { _ = database.Close() }()
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	seedRetentionBrowserFixture(t, repo, taskID)
	assertRetentionBrowserFixtureIdle(t, repo)
	t.Logf("seeded ordinary settled stream: session=%s stream=%s submission=%s high_water=2", retentionBrowserFixtureSessionID, retentionBrowserFixtureStreamID, retentionBrowserFixtureSubmissionID)
}

func TestDeliveryRetentionBrowserFixture(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			taskID := "retention-browser-fixture-task"
			if err := repo.CreateTask(t.Context(), &models.Task{ID: taskID, WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Browser retention fixture"}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "retention-browser-primary-session", TaskID: taskID,
				IsPrimary: true, State: models.TaskSessionStateWaitingForInput}); err != nil {
				t.Fatal(err)
			}
			seedRetentionBrowserFixture(t, repo, taskID)
			assertRetentionBrowserFixtureIdle(t, repo)
			var primary string
			if err := repo.db.QueryRow(repo.db.Rebind(`SELECT id FROM task_sessions WHERE task_id = ? AND is_primary <> 0`), taskID).Scan(&primary); err != nil || primary != "retention-browser-primary-session" {
				t.Fatalf("fixture replaced primary session: %q %v", primary, err)
			}
			before := retentionCanonicalContent(t, repo, retentionBrowserFixtureSessionID)
			var retired int
			for attempt := 0; attempt < 4; attempt++ {
				result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
				if err != nil {
					t.Fatal(err)
				}
				retired += result.EventRowsRetired
			}
			if retired != 2 {
				t.Fatalf("actual collector must retire both settled fixture events: %d", retired)
			}
			if after := retentionCanonicalContent(t, repo, retentionBrowserFixtureSessionID); after == "" || after != before {
				t.Fatalf("collector changed fixture canonical history: before=%q after=%q", before, after)
			}
			cursor, err := repo.GetAgentDeliveryCursor(t.Context(), retentionBrowserFixtureStreamID)
			if err != nil || cursor.RetiredSequence != 2 || cursor.ProjectedSequence != 2 {
				t.Fatalf("fixture retirement checkpoint: %+v %v", cursor, err)
			}
			prompt, err := repo.GetAgentDeliverySubmissionMetadata(t.Context(), retentionBrowserFixtureSubmissionID)
			if err != nil || len(prompt.Payload) != 0 || prompt.PayloadPrunedAt == nil || prompt.FullySettledAt == nil {
				t.Fatalf("fixture compact prompt summary: %+v %v", prompt, err)
			}
		})
	}
}

func assertRetentionBrowserFixtureIdle(t *testing.T, repo *Repository) {
	t.Helper()
	var primary, executions int
	var state string
	if err := repo.db.QueryRow(repo.db.Rebind(`SELECT is_primary, state,
 (SELECT COUNT(*) FROM executors_running WHERE session_id = task_sessions.id)
 FROM task_sessions WHERE id = ?`), retentionBrowserFixtureSessionID).Scan(&primary, &state, &executions); err != nil {
		t.Fatal(err)
	}
	if primary != 0 || state != string(models.TaskSessionStateWaitingForInput) || executions != 0 {
		t.Fatalf("fixture must remain an idle nonprimary conversation: primary=%d state=%s executions=%d", primary, state, executions)
	}
}

func seedRetentionBrowserFixture(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	old := time.Now().UTC().Add(-48 * time.Hour)
	previousClock := repo.clockNow
	repo.clockNow = func() time.Time { return old }
	defer func() { repo.clockNow = previousClock }()
	seedRetentionBrowserFixtureOwner(t, repo, taskID, old)
	terminal := seedRetentionBrowserFixtureEvents(t, repo, taskID, old)
	if settled, err := repo.SettleAgentDeliveryTerminal(t.Context(), terminal.StreamID, terminal.Sequence, models.DeliverySubmissionCompleted, old); err != nil || !settled {
		t.Fatalf("settle fixture terminal: %t %v", settled, err)
	}
	putRetentionWorkflowOutcome(t, repo, terminal, "not_required", retentionPromptTurnID(t, terminal))
	key := fmt.Sprintf("agent_delivery.terminal:%s:%d", terminal.StreamID, terminal.Sequence)
	if settled, err := repo.CompleteAgentDeliveryTerminalSettlement(t.Context(), key, old); err != nil || !settled {
		t.Fatalf("complete fixture terminal: %t %v", settled, err)
	}
	if err := repo.DeleteExecutorRunningBySessionID(t.Context(), retentionBrowserFixtureSessionID); err != nil {
		t.Fatal(err)
	}
}

func seedRetentionBrowserFixtureOwner(t *testing.T, repo *Repository, taskID string, old time.Time) {
	t.Helper()
	session := &models.TaskSession{ID: retentionBrowserFixtureSessionID, TaskID: taskID,
		QueueIncarnationID: "retention-browser-settled-inc", State: models.TaskSessionStateWaitingForInput}
	if err := repo.CreateTaskSession(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "retention-browser-fixture-running",
		SessionID: session.ID, TaskID: taskID, ExecutorID: "retention-browser-fixture-executor",
		AgentExecutionID: "retention-browser-fixture-execution", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if committed, err := repo.CommitHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
		NativeSessionID: "retention-browser-fixture-native", CreationReason: "initial", CreatedAt: old, CommittedAt: old}, 0); err != nil || !committed {
		t.Fatalf("commit fixture owner generation: %t %v", committed, err)
	}
	if prepared, err := repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
		ID: retentionBrowserFixtureSubmissionID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "retention-browser-fixture-dispatch",
		PayloadHash: "retention-browser-fixture-prompt-hash", Payload: []byte("settled ordinary fixture prompt"),
		State: models.DeliverySubmissionDispatching}); err != nil || !prepared {
		t.Fatalf("prepare fixture prompt: %t %v", prepared, err)
	}
}

func seedRetentionBrowserFixtureEvents(t *testing.T, repo *Repository, taskID string, old time.Time) models.AgentDeliveryEvent {
	t.Helper()
	var terminal models.AgentDeliveryEvent
	for index, eventType := range []string{streams.EventTypeMessageChunk, streams.EventTypeComplete} {
		frame := streams.AgentEvent{Type: eventType, Text: "Preserved ordinary retention fixture output."}
		if index == 1 {
			frame.TurnID = "retention-browser-settled-turn"
		}
		payload, err := json.Marshal(frame)
		if err != nil {
			t.Fatal(err)
		}
		event := models.AgentDeliveryEvent{TaskID: taskID, ExecutionID: "retention-browser-fixture-execution",
			OwnerGeneration: 1, ExpectedStreamID: retentionBrowserFixtureStreamID,
			SessionID: retentionBrowserFixtureSessionID, IncarnationID: "retention-browser-settled-inc", HarnessGeneration: 1,
			StreamID: retentionBrowserFixtureStreamID, Sequence: int64(index + 1), SubmissionID: retentionBrowserFixtureSubmissionID,
			EventType: eventType, Payload: payload, Terminal: index == 1, ReceivedAt: old}
		if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 2); err != nil || !inserted {
			t.Fatalf("receive fixture event: %t %v", inserted, err)
		}
		if index == 0 {
			if _, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), &event, completedRetentionEventEffect(&event)); err != nil {
				t.Fatal(err)
			}
		} else if _, err := repo.ProjectAgentDeliveryEvent(t.Context(), &event, completedRetentionEventEffect(&event)); err != nil {
			t.Fatal(err)
		}
		terminal = event
	}
	return terminal
}
