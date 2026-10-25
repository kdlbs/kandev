package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExactProfileAssignmentSchemaExists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	var tableName string
	if err := db.Get(&tableName, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'task_exact_profile_assignments'
	`); err != nil {
		t.Fatalf("exact profile assignment table is missing: %v", err)
	}
}

func TestUpsertExactProfileAssignmentGuardsGeneration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment-generation.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	now := time.Now().UTC().Round(0)
	assignment := &models.ExactProfileAssignment{
		TaskID: "task-exact-assignment", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: now.UnixNano(), Generation: 1, SourceWorkflowID: "workflow-1", SourceWorkflowStepID: "step-1",
		SourceTaskState: "TODO",
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		assignment.TaskID, assignment.WorkspaceID, "Exact profile assignment", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	changed, err := repo.UpsertExactProfileAssignment(context.Background(), assignment)
	if err != nil {
		t.Fatalf("create assignment: %v", err)
	}
	if !changed {
		t.Fatal("initial assignment was not recorded")
	}

	stale := *assignment
	stale.AgentProfileID = "profile-stale"
	changed, err = repo.UpsertExactProfileAssignment(context.Background(), &stale)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("replay assignment error = %v, want generation rejection", err)
	}
	if changed {
		t.Fatal("same generation with a different payload must not replace assignment")
	}
}

func TestAssignExactProfileAssignmentActivatesAtomically(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment-atomic.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	now := time.Now().UTC().Round(0)
	assignment := &models.ExactProfileAssignment{
		TaskID: "task-atomic", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: now.UnixNano(), Generation: 1,
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		assignment.TaskID, assignment.WorkspaceID, "Exact profile assignment", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	changed, err := repo.AssignExactProfileAssignment(context.Background(), assignment)
	if err != nil || !changed {
		t.Fatalf("assign exact profile = (%v, %v), want (true, nil)", changed, err)
	}
	stored, err := repo.GetExactProfileAssignment(context.Background(), assignment.TaskID)
	if err != nil || stored == nil || !stored.Active {
		t.Fatalf("stored assignment = %#v, %v; want active assignment", stored, err)
	}

	changed, err = repo.AssignExactProfileAssignment(context.Background(), assignment)
	if err != nil || changed {
		t.Fatalf("replay exact profile = (%v, %v), want (false, nil)", changed, err)
	}
}

func TestAssignExactProfileAssignmentRejectsMissingProfile(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE agent_profiles (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, enabled INTEGER NOT NULL, deleted_at TIMESTAMP)`,
		`CREATE TABLE agent_profile_revisions (profile_id TEXT PRIMARY KEY, revision BIGINT NOT NULL)`,
	} {
		if _, err := repo.db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if changed, err := repo.AssignExactProfileAssignmentIfProfileCurrent(ctx, assignment); changed || !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) {
		t.Fatalf("missing profile assignment = (%t, %v), want invalid input", changed, err)
	}
}

func TestAssignExactProfileAssignmentReplacementRejectsForeignWorkspace(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	ctx := context.Background()
	if changed, err := repo.AssignExactProfileAssignment(ctx, assignment); err != nil || !changed {
		t.Fatalf("initial assignment = (%t, %v)", changed, err)
	}

	next := *assignment
	next.Generation = 2
	next.WorkspaceID = "workspace-foreign"
	next.AgentProfileID = "profile-foreign"
	if changed, err := repo.AssignExactProfileAssignment(ctx, &next); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("foreign replacement = (%t, %v), want generation rejection", changed, err)
	}
	stored, err := repo.GetExactProfileAssignment(ctx, assignment.TaskID)
	if err != nil || stored == nil || stored.WorkspaceID != assignment.WorkspaceID || stored.Generation != 1 {
		t.Fatalf("stored assignment = %#v, %v; want original workspace and generation", stored, err)
	}
}

func TestAssignExactProfileAssignmentRequiresCurrentProfileToken(t *testing.T) {
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "exact-profile-token.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE agent_profiles (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, enabled INTEGER NOT NULL, deleted_at TIMESTAMP)`,
		`CREATE TABLE agent_profile_revisions (profile_id TEXT PRIMARY KEY, revision BIGINT NOT NULL)`,
		`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES ('task-token', 'ws1', 'Token', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO agent_profiles (id, workspace_id, enabled) VALUES ('profile-token', 'ws1', 1)`,
		`INSERT INTO agent_profile_revisions (profile_id, revision) VALUES ('profile-token', 2)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	assignment := &models.ExactProfileAssignment{
		TaskID: "task-token", WorkspaceID: "ws1", AgentProfileID: "profile-token", ProfileRevision: 1, Generation: 1,
	}
	if changed, err := repo.BindExactProfileSessionIfAssignmentCurrent(ctx, "missing-session", assignment.TaskID,
		models.TaskSessionStateCreated, assignment.AgentProfileID, 1, 0); changed || !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) {
		t.Fatalf("legacy zero-token bind = %t, %v", changed, err)
	}
	if _, err := repo.AssignExactProfileAssignmentIfProfileCurrent(ctx, assignment); !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) {
		t.Fatalf("stale token error = %v", err)
	}
	assignment.ProfileRevision = 2
	changed, err := repo.AssignExactProfileAssignmentIfProfileCurrent(ctx, assignment)
	if err != nil || !changed {
		t.Fatalf("current token result = %t, %v", changed, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-token", TaskID: assignment.TaskID,
		AgentProfileID: assignment.AgentProfileID, State: models.TaskSessionStateCreated}); err != nil {
		t.Fatal(err)
	}
	if bound, err := repo.BindExactProfileSessionIfAssignmentCurrent(ctx, "session-token", assignment.TaskID,
		models.TaskSessionStateCreated, assignment.AgentProfileID, 1, 2); err != nil || !bound {
		t.Fatalf("current profile binding = %t, %v", bound, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE agent_profile_revisions SET revision = 3 WHERE profile_id = 'profile-token'`); err != nil {
		t.Fatal(err)
	}
	if bound, err := repo.BindExactProfileSessionIfAssignmentCurrent(ctx, "session-token", assignment.TaskID,
		models.TaskSessionStateCreated, assignment.AgentProfileID, 1, 2); err != nil || bound {
		t.Fatalf("stale profile binding = %t, %v", bound, err)
	}
	assignment.Generation = 2
	if _, err := repo.AssignExactProfileAssignmentIfProfileCurrent(ctx, assignment); !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) {
		t.Fatalf("replayed stale token error = %v", err)
	}
}

func TestLegacyExactAssignmentHasNoQualifiedProfileToken(t *testing.T) {
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "legacy-exact-profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`DROP TABLE task_exact_profile_assignments`,
		`CREATE TABLE task_exact_profile_assignments (
			task_id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, agent_profile_id TEXT NOT NULL,
			profile_revision TIMESTAMP NOT NULL, generation BIGINT NOT NULL,
			source_workflow_id TEXT NOT NULL DEFAULT '', source_workflow_step_id TEXT NOT NULL DEFAULT '',
			source_task_state TEXT NOT NULL DEFAULT '', active INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`INSERT INTO task_exact_profile_assignments
			(task_id, workspace_id, agent_profile_id, profile_revision, generation, active, created_at, updated_at)
			VALUES ('legacy-task', 'ws1', 'profile-1', CURRENT_TIMESTAMP, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ensureTaskExactProfileSchema(); err != nil {
		t.Fatal(err)
	}
	legacy, err := repo.GetExactProfileAssignment(context.Background(), "legacy-task")
	if err != nil || legacy == nil || legacy.ProfileRevision != 0 {
		t.Fatalf("legacy assignment = %#v, %v; want unqualified zero token", legacy, err)
	}
}

func TestActivateExactProfileAssignmentRejectsStaleGeneration(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.UpsertExactProfileAssignment(context.Background(), assignment); err != nil {
		t.Fatal(err)
	}
	changed, err := repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 2)
	if err != nil || changed {
		t.Fatalf("stale activation = (%v, %v)", changed, err)
	}
	changed, err = repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 1)
	if err != nil || !changed {
		t.Fatalf("activation = (%v, %v)", changed, err)
	}
	changed, err = repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 1)
	if err != nil || changed {
		t.Fatalf("activation replay = (%v, %v)", changed, err)
	}
}

func TestExactProfileLaunchReceiptReplayDoesNotOverwrite(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	receipt := &models.ExactProfileLaunchReceipt{TaskID: assignment.TaskID, SessionID: "session-1", AgentProfileID: assignment.AgentProfileID, Generation: 1, ProfileRevision: assignment.ProfileRevision, Model: "model-1", Outcome: models.ExactProfileLaunchOutcomeApplied, InferenceStarted: true}
	changed, err := repo.RecordExactProfileLaunchReceipt(context.Background(), receipt)
	if err != nil || !changed {
		t.Fatalf("record = (%v, %v)", changed, err)
	}
	receipt.Outcome = models.ExactProfileLaunchOutcomeFailedClosed
	changed, err = repo.RecordExactProfileLaunchReceipt(context.Background(), receipt)
	if err != nil || changed {
		t.Fatalf("replay = (%v, %v)", changed, err)
	}
	stored, err := repo.GetExactProfileLaunchReceipt(context.Background(), assignment.TaskID, receipt.SessionID)
	if err != nil || stored == nil || stored.Outcome != models.ExactProfileLaunchOutcomeApplied {
		t.Fatalf("stored = %#v, %v", stored, err)
	}
	missing, err := repo.GetExactProfileLaunchReceipt(context.Background(), assignment.TaskID, "missing")
	if err != nil || missing != nil {
		t.Fatalf("missing = %#v, %v", missing, err)
	}
}

func TestUpsertExactProfileAssignmentAdvancesGeneration(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.UpsertExactProfileAssignment(context.Background(), assignment); err != nil {
		t.Fatal(err)
	}
	next := *assignment
	next.Generation = 2
	next.AgentProfileID = "profile-2"
	changed, err := repo.UpsertExactProfileAssignment(context.Background(), &next)
	if err != nil || !changed {
		t.Fatalf("advance = (%v, %v)", changed, err)
	}
	changed, err = repo.UpsertExactProfileAssignment(context.Background(), &next)
	if err != nil || changed {
		t.Fatalf("replay = (%v, %v)", changed, err)
	}
}

func TestUpsertExactProfileAssignmentRejectsForeignWorkspace(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	assignment.WorkspaceID = "workspace-foreign"
	changed, err := repo.UpsertExactProfileAssignment(context.Background(), assignment)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) || changed {
		t.Fatalf("foreign workspace = (%v, %v)", changed, err)
	}
}

func newExactProfileAssignmentRepo(t *testing.T) (*Repository, *models.ExactProfileAssignment) {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "exact.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Round(0)
	a := &models.ExactProfileAssignment{TaskID: "task-test", WorkspaceID: "workspace-1", AgentProfileID: "profile-1", ProfileRevision: now.UnixNano(), Generation: 1}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, a.TaskID, a.WorkspaceID, "Exact", now, now); err != nil {
		t.Fatal(err)
	}
	return repo, a
}

func TestFindExactProfileReusableSessionFiltersGenerationRevisionAndTerminalState(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	ctx := context.Background()
	const revision int64 = 42
	for _, session := range []*models.TaskSession{
		{ID: "exact-terminal", TaskID: assignment.TaskID, State: models.TaskSessionStateCompleted, ExactProfileGeneration: 1, ExactProfileRevision: revision},
		{ID: "exact-wrong-generation", TaskID: assignment.TaskID, State: models.TaskSessionStateWaitingForInput, ExactProfileGeneration: 2, ExactProfileRevision: revision},
		{ID: "exact-wrong-revision", TaskID: assignment.TaskID, State: models.TaskSessionStateWaitingForInput, ExactProfileGeneration: 1, ExactProfileRevision: revision + 1},
		{ID: "exact-reusable", TaskID: assignment.TaskID, State: models.TaskSessionStateWaitingForInput, ExactProfileGeneration: 1, ExactProfileRevision: revision},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create session %s: %v", session.ID, err)
		}
	}

	matching, err := repo.FindExactProfileReusableSession(ctx, assignment.TaskID, 1, revision)
	if err != nil || matching == nil || matching.ID != "exact-reusable" {
		t.Fatalf("matching session = %#v, %v; want exact-reusable", matching, err)
	}
	for _, query := range []struct {
		generation int64
		revision   int64
	}{
		{generation: 0, revision: 0},
		{generation: 0, revision: revision},
		{generation: 1, revision: 0},
		{generation: 2, revision: revision + 1},
		{generation: 3, revision: revision},
		{generation: 1, revision: revision + 2},
	} {
		found, err := repo.FindExactProfileReusableSession(ctx, assignment.TaskID, query.generation, query.revision)
		if err != nil || found != nil {
			t.Fatalf("lookup (%d, %d) = %#v, %v; want nil", query.generation, query.revision, found, err)
		}
	}
}

func TestExactProfileAssignmentRejectsInvalidInput(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	ctx := context.Background()

	invalid := *assignment
	invalid.AgentProfileID = ""
	changed, err := repo.UpsertExactProfileAssignment(ctx, &invalid)
	if !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) || changed {
		t.Fatalf("missing profile = (%v, %v), want invalid input", changed, err)
	}

	invalid = *assignment
	invalid.ProfileRevision = 0
	changed, err = repo.UpsertExactProfileAssignment(ctx, &invalid)
	if !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) || changed {
		t.Fatalf("missing revision = (%v, %v), want invalid input", changed, err)
	}

	invalid = *assignment
	invalid.Generation = 0
	changed, err = repo.UpsertExactProfileAssignment(ctx, &invalid)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) || changed {
		t.Fatalf("zero generation = (%v, %v), want generation rejection", changed, err)
	}

	receipt := &models.ExactProfileLaunchReceipt{
		TaskID: assignment.TaskID, SessionID: "session-invalid", AgentProfileID: assignment.AgentProfileID,
		Generation: 1, ProfileRevision: assignment.ProfileRevision, Outcome: "unknown",
	}
	changed, err = repo.RecordExactProfileLaunchReceipt(ctx, receipt)
	if !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) || changed {
		t.Fatalf("invalid receipt outcome = (%v, %v), want invalid input", changed, err)
	}
}
