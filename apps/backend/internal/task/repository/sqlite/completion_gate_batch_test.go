package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

type completionGateBatchReader interface {
	GetTaskCompletionGateSummaries(context.Context, []string) (models.TaskCompletionGateSummaryBatch, error)
}

type completionGateBatchQueryCounts struct {
	queries      atomic.Int64
	transactions atomic.Int64
}

func (c *completionGateBatchQueryCounts) reset() {
	c.queries.Store(0)
	c.transactions.Store(0)
}

type completionGateBatchCountingConnector struct {
	dsn    string
	counts *completionGateBatchQueryCounts
}

func (c completionGateBatchCountingConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite3.SQLiteDriver{}).Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &completionGateBatchCountingConn{Conn: conn, counts: c.counts}, nil
}

func (c completionGateBatchCountingConnector) Driver() driver.Driver {
	return &sqlite3.SQLiteDriver{}
}

type completionGateBatchCountingConn struct {
	driver.Conn
	counts *completionGateBatchQueryCounts
}

func (c *completionGateBatchCountingConn) Begin() (driver.Tx, error) {
	c.counts.transactions.Add(1)
	return c.Conn.(driver.ConnBeginTx).BeginTx(context.Background(), driver.TxOptions{})
}

func (c *completionGateBatchCountingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.counts.transactions.Add(1)
	conn, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, fmt.Errorf("counting connection lacks ConnBeginTx")
	}
	return conn.BeginTx(ctx, opts)
}

func (c *completionGateBatchCountingConn) QueryContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	c.counts.queries.Add(1)
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, fmt.Errorf("SQLite driver does not support QueryContext")
	}
	return queryer.QueryContext(ctx, query, args)
}

func newCompletionGateBatchCountingRepo(t *testing.T) (*Repository, *sqlx.DB, *completionGateBatchQueryCounts) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "completion-gate-batch.db")
	writerRaw, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open SQLite writer: %v", err)
	}
	counts := &completionGateBatchQueryCounts{}
	readerDSN := fmt.Sprintf("file:%s?_foreign_keys=on&mode=ro&_busy_timeout=5000", path)
	readerRaw := sql.OpenDB(completionGateBatchCountingConnector{dsn: readerDSN, counts: counts})
	readerRaw.SetMaxOpenConns(4)
	readerRaw.SetMaxIdleConns(4)
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	repo, err := NewWithDB(writer, reader, nil)
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatalf("create SQLite task repository: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	return repo, writer, counts
}

func TestCompletionGateBatchBounds(t *testing.T) {
	repo, writer, counts := newCompletionGateBatchCountingRepo(t)
	ctx := context.Background()
	const taskCount = 1000
	now := time.Now().UTC()
	tx, err := writer.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin task seed: %v", err)
	}
	statement, err := tx.Preparex(`INSERT INTO tasks (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("prepare task seed: %v", err)
	}
	taskIDs := make([]string, 0, taskCount)
	for index := 0; index < taskCount; index++ {
		taskID := fmt.Sprintf("gate-batch-task-%04d", index)
		if _, err := statement.ExecContext(ctx, taskID, taskID, now, now); err != nil {
			_ = statement.Close()
			_ = tx.Rollback()
			t.Fatalf("insert task %q: %v", taskID, err)
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := statement.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatalf("close task seed statement: %v", err)
	}
	setStatement, err := tx.Preparex(`INSERT INTO task_completion_sets (task_id, workspace_id, revision, updated_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("prepare gate-set seed: %v", err)
	}
	criterionStatement, err := tx.Preparex(`
		INSERT INTO task_completion_criteria (
			task_id, criterion_id, description, criterion_revision, subject_kind, subject_id,
			verified_revision, evidence_kind, evidence_id, evidence_revision
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = setStatement.Close()
		_ = tx.Rollback()
		t.Fatalf("prepare gate-criterion seed: %v", err)
	}
	for _, taskID := range taskIDs {
		if _, err := setStatement.ExecContext(ctx, taskID, "", 1, now); err != nil {
			_ = setStatement.Close()
			_ = criterionStatement.Close()
			_ = tx.Rollback()
			t.Fatalf("insert gate set for %q: %v", taskID, err)
		}
		for criterionIndex := 0; criterionIndex < 10; criterionIndex++ {
			criterionID := fmt.Sprintf("artifact-%02d", criterionIndex)
			artifactID := taskID + "-" + criterionID
			if _, err := criterionStatement.ExecContext(ctx, taskID, criterionID, "Verified immutable evidence", 1,
				models.TaskCompletionEvidenceArtifact, artifactID, 1,
				models.TaskCompletionEvidenceArtifact, artifactID, "artifact-v1"); err != nil {
				_ = setStatement.Close()
				_ = criterionStatement.Close()
				_ = tx.Rollback()
				t.Fatalf("insert criterion %q for %q: %v", criterionID, taskID, err)
			}
		}
	}
	if err := setStatement.Close(); err != nil {
		_ = criterionStatement.Close()
		_ = tx.Rollback()
		t.Fatalf("close gate-set seed statement: %v", err)
	}
	if err := criterionStatement.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatalf("close gate-criterion seed statement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit task seed: %v", err)
	}
	counts.reset()
	reader, ok := any(repo).(completionGateBatchReader)
	if !ok {
		t.Fatal("SQLite task repository does not implement the completion-gate summary batch reader")
	}
	batch, err := reader.GetTaskCompletionGateSummaries(ctx, taskIDs)
	if err != nil {
		t.Fatalf("get completion-gate summaries: %v", err)
	}
	if len(batch.ByTaskID) != taskCount || len(batch.MissingTaskIDs) != 0 {
		t.Fatalf("batch has %d task observations and %d missing IDs, want %d and zero", len(batch.ByTaskID), len(batch.MissingTaskIDs), taskCount)
	}
	if got := counts.transactions.Load(); got != 10 {
		t.Fatalf("completion-gate read snapshots = %d, want 10 for 1,000 tasks", got)
	}
	if got := counts.queries.Load(); got > 60 {
		t.Fatalf("completion-gate data queries = %d, want at most 60 for 1,000 tasks", got)
	}
	for _, taskID := range taskIDs {
		if summary, ok := batch.ByTaskID[taskID]; !ok || summary == nil || summary.CriteriaCount != 10 || summary.VerifiedCount != 10 || summary.BlockerCount != 0 || summary.Blocked {
			t.Fatalf("summary for %q = %+v, present=%v; want ten verified immutable criteria", taskID, summary, ok)
		}
	}

	counts.reset()
	missing, err := reader.GetTaskCompletionGateSummaries(ctx, []string{"missing-gate-batch-task"})
	if err != nil {
		t.Fatalf("read missing completion-gate task: %v", err)
	}
	if len(missing.ByTaskID) != 0 || !reflect.DeepEqual(missing.MissingTaskIDs, []string{"missing-gate-batch-task"}) {
		t.Fatalf("missing task result = %+v, want explicit missing ID", missing)
	}
}

func TestCompletionGateBatchMatchesStandalone(t *testing.T) {
	repo, writer, _, _ := newCompletionGateReadTestRepo(t)
	assertCompletionGateBatchMatchesStandalone(t, repo, writer)
}

func TestCompletionGateBatchPostgres(t *testing.T) {
	repo := openPostgresRepo(t)
	assertCompletionGateBatchMatchesStandalone(t, repo, repo.db)
}

func assertCompletionGateBatchMatchesStandalone(t *testing.T, repo *Repository, writer *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := writer.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS github_task_prs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, head_sha TEXT NOT NULL)`); err != nil {
		t.Fatalf("create PR evidence fixture: %v", err)
	}

	taskIDs := []string{
		"gate-batch-artifact",
		"gate-batch-task-revision",
		"gate-batch-pr-head",
		"gate-batch-execution",
		"gate-batch-unverified",
		"gate-batch-no-gate",
	}
	for _, taskID := range taskIDs {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
			t.Fatalf("create task %q: %v", taskID, err)
		}
	}
	if _, err := writer.ExecContext(ctx, repo.db.Rebind(`INSERT INTO github_task_prs (id, task_id, head_sha) VALUES (?, ?, ?)`),
		"gate-batch-pr-row", "gate-batch-pr-head", "head-1"); err != nil {
		t.Fatalf("insert PR evidence fixture: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "gate-batch-session", TaskID: "gate-batch-execution"}); err != nil {
		t.Fatalf("create execution session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "gate-batch-turn", TaskSessionID: "gate-batch-session", TaskID: "gate-batch-execution",
	}); err != nil {
		t.Fatalf("create execution turn: %v", err)
	}
	if err := repo.CompleteTurn(ctx, "gate-batch-turn"); err != nil {
		t.Fatalf("complete execution turn: %v", err)
	}

	seedCompletionGateCriterion(t, repo, "gate-batch-artifact", models.TaskCompletionEvidenceArtifact, "artifact-run-1", "artifact-v1", true)
	if task, err := repo.GetTask(ctx, "gate-batch-task-revision"); err != nil {
		t.Fatalf("get task-revision fixture: %v", err)
	} else {
		seedCompletionGateCriterion(t, repo, task.ID, models.TaskCompletionEvidenceTaskRevision, task.ID, task.UpdatedAt.UTC().Format(time.RFC3339Nano), true)
	}
	seedCompletionGateCriterion(t, repo, "gate-batch-pr-head", models.TaskCompletionEvidenceGitHubPRHead, "gate-batch-pr-row", "head-1", true)
	seedCompletionGateCriterion(t, repo, "gate-batch-execution", models.TaskCompletionEvidenceExecution, "gate-batch-turn", "gate-batch-turn", true)
	seedCompletionGateCriterion(t, repo, "gate-batch-unverified", models.TaskCompletionEvidenceArtifact, "artifact-run-2", "artifact-v2", false)

	if _, err := writer.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`),
		time.Now().UTC().Add(time.Minute), "gate-batch-task-revision"); err != nil {
		t.Fatalf("stale task-revision evidence: %v", err)
	}
	if _, err := writer.ExecContext(ctx, `UPDATE github_task_prs SET head_sha = 'head-2' WHERE id = 'gate-batch-pr-row'`); err != nil {
		t.Fatalf("stale PR evidence: %v", err)
	}
	if _, err := writer.ExecContext(ctx, `UPDATE task_session_turns SET completed_at = NULL WHERE id = 'gate-batch-turn'`); err != nil {
		t.Fatalf("stale execution evidence: %v", err)
	}

	reader, ok := any(repo).(completionGateBatchReader)
	if !ok {
		t.Fatal("task repository does not implement the completion-gate summary batch reader")
	}
	requested := append(append([]string(nil), taskIDs...), "missing-gate-batch-task")
	batch, err := reader.GetTaskCompletionGateSummaries(ctx, requested)
	if err != nil {
		t.Fatalf("get completion-gate summaries: %v", err)
	}
	if !reflect.DeepEqual(batch.MissingTaskIDs, []string{"missing-gate-batch-task"}) {
		t.Fatalf("missing task IDs = %v, want [missing-gate-batch-task]", batch.MissingTaskIDs)
	}
	for _, taskID := range taskIDs {
		standalone, err := repo.GetTaskCompletionGate(ctx, taskID)
		if err != nil {
			t.Fatalf("get standalone gate for %q: %v", taskID, err)
		}
		want := completionGateSummaryObservation(statussummary.CompletionGateSummaryFromSnapshot(standalone))
		got, ok := batch.ByTaskID[taskID]
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("batch gate for %q = %+v, present=%v; standalone summary = %+v", taskID, got, ok, want)
		}
	}
	if !batch.ByTaskID["gate-batch-task-revision"].Blocked || !batch.ByTaskID["gate-batch-pr-head"].Blocked || !batch.ByTaskID["gate-batch-execution"].Blocked {
		t.Fatal("stale task, PR, and execution evidence must remain blocked in the batch")
	}
	if batch.ByTaskID["gate-batch-artifact"].Blocked {
		t.Fatal("verified immutable artifact evidence should remain unblocked")
	}
}

func completionGateSummaryObservation(summary *statussummary.CompletionGateSummary) *models.TaskCompletionGateSummaryObservation {
	if summary == nil {
		return nil
	}
	return &models.TaskCompletionGateSummaryObservation{
		Revision: summary.Revision, CriteriaCount: summary.CriteriaCount,
		VerifiedCount: summary.VerifiedCount, BlockerCount: summary.BlockerCount,
		Blocked: summary.Blocked,
	}
}

func seedCompletionGateCriterion(
	t *testing.T,
	repo *Repository,
	taskID, kind, evidenceID, evidenceRevision string,
	verify bool,
) {
	t.Helper()
	ctx := context.Background()
	snapshot, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:    taskID,
		ActorKind: "human",
		ActorID:   "completion-gate-batch-test",
		Criteria: []models.TaskCompletionCriterion{{
			ID: "criterion-1", Description: "Keep the summary evaluation equivalent",
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: kind, ID: evidenceID},
		}},
	})
	if err != nil {
		t.Fatalf("set %s criterion for %q: %v", kind, taskID, err)
	}
	if !verify {
		return
	}
	if _, err := repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
		TaskID:           taskID,
		WorkspaceID:      snapshot.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		CriterionID:      "criterion-1",
		ActorKind:        "human",
		ActorID:          "completion-gate-batch-test",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{Kind: kind, ID: evidenceID, Revision: evidenceRevision},
			Summary: "Verified fixture evidence.",
		},
	}); err != nil {
		t.Fatalf("verify %s criterion for %q: %v", kind, taskID, err)
	}
}
