package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// participantSeatSchema is the subset of tables AddTaskParticipant reads and
// writes: the task it resolves a step from, the seat slate it mutates, and
// the decisions table findClaimableAutoSeat joins against.
const participantSeatSchema = `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		workflow_step_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS workflow_step_participants (
		id TEXT PRIMARY KEY,
		step_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		decision_required INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
		provenance TEXT NOT NULL DEFAULT 'manual'
	);
	CREATE TABLE IF NOT EXISTS workflow_step_decisions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL DEFAULT '',
		step_id TEXT NOT NULL DEFAULT '',
		participant_id TEXT NOT NULL DEFAULT '',
		decision TEXT NOT NULL DEFAULT '',
		decided_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
		superseded_at TIMESTAMP NULL
	);
`

// newDivergentHandleRepo builds an office repository whose writer and reader
// point at two different databases, where the same task row names a
// different current step in each.
//
// This exists to make AC-OFFICE-SEAT-PROVENANCE-004.10 falsifiable.
// AddTaskParticipant must resolve the task's step on its transaction handle,
// inside its own exclusion — never through the read-only pool, which would
// escape that exclusion. In every other test both handles point at the same
// database, so swapping stepIDForTaskTx for stepIDForTask is a one-word edit
// that changes nothing any of them observe. Here it changes the answer.
//
// Returns the repo and the writer handle. Assertions must use the writer: the
// repo's own ReaderDB() accessor now points at the wrong database by
// construction.
func newDivergentHandleRepo(t *testing.T, taskID, writerStepID, readerStepID string) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()

	writer, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	writer.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = writer.Close() })

	reader, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	reader.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = reader.Close() })

	// Schema initialisation runs against the writer, so the reader needs its
	// own tasks table and row spelled out explicitly.
	if _, _, err := settingsstore.Provide(writer, writer, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if _, err := writer.Exec(participantSeatSchema); err != nil {
		t.Fatalf("writer schema: %v", err)
	}
	if _, err := reader.Exec(participantSeatSchema); err != nil {
		t.Fatalf("reader schema: %v", err)
	}

	if _, err := writer.Exec(
		`INSERT INTO tasks (id, workspace_id, workflow_step_id, title) VALUES (?, 'ws-1', ?, 'Task')`,
		taskID, writerStepID); err != nil {
		t.Fatalf("seed writer task: %v", err)
	}
	if _, err := reader.Exec(
		`INSERT INTO tasks (id, workspace_id, workflow_step_id, title) VALUES (?, 'ws-1', ?, 'Task')`,
		taskID, readerStepID); err != nil {
		t.Fatalf("seed reader task: %v", err)
	}
	return repo, writer
}

// TestAddTaskParticipant_ResolvesStepOnTheWritingHandle covers
// AC-OFFICE-SEAT-ASSURANCE-002.3, pinning
// AC-OFFICE-SEAT-PROVENANCE-004.10.
//
// The two handles disagree about which step the task stands at. The seat must
// land at the step the writing handle names, because that is the read taken
// inside the exclusion that serialises this writer against automatic casting.
// An implementation reading through the read-only pool lands at the reader's
// step instead, and this case is the only one in the suite that can tell.
//
// A divergent reader row is deliberate rather than an absent one: divergence
// proves the seat landed at the writer's step, where absence would only prove
// the reader was not consulted.
func TestAddTaskParticipant_ResolvesStepOnTheWritingHandle(t *testing.T) {
	ctx := context.Background()
	repo, writer := newDivergentHandleRepo(t, "split-task", "step-writer", "step-reader")

	if _, err := writer.Exec(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES ('agent-human', '', 'agent-human', 'agent-human', 'ws-1', '', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}

	result, err := repo.AddTaskParticipant(ctx, "split-task", "agent-human", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q", result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if result.StepID != "step-writer" {
		t.Errorf("reported step = %q, want step-writer (the writing handle's view)", result.StepID)
	}

	// Read through the writer: the repo's reader points at the other database.
	var stepID string
	if err := writer.Get(&stepID, `
		SELECT step_id FROM workflow_step_participants
		WHERE task_id = 'split-task' AND role = 'reviewer'
	`); err != nil {
		t.Fatalf("read seat step: %v", err)
	}
	if stepID != "step-writer" {
		t.Errorf("seat landed at step %q, want step-writer — the step was resolved outside the write's exclusion", stepID)
	}
}
