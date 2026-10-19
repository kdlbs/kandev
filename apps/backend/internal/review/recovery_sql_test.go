package review

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func newRecoverySQLHarness(t *testing.T) (*runnerHarness, *sqliterepo.Repository, *taskservice.ReviewService) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "review.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlDB.Close() })
	repo, err := sqliterepo.NewWithDB(sqlDB, sqlDB, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(context.Background(), &models.Task{ID: "task-1", Title: "Review"}); err != nil {
		t.Fatal(err)
	}
	service := taskservice.NewReviewService(repo, nil, testLogger(t))
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n old\n+new\n", "", "")},
		[]string{okResponse("a.go", 2)},
	)
	h.runner.store = service
	return h, repo, service
}

func seedRecoveryRun(t *testing.T, repo *sqliterepo.Repository, id string, trigger models.ReviewRunTrigger, status models.ReviewRunStatus, order int) *models.TaskReviewRun {
	t.Helper()
	run := &models.TaskReviewRun{
		ID: id, TaskID: "task-1", SessionID: "sess-original", Trigger: trigger,
		Status: status, AgentID: "original-agent", Model: "original-model",
		WorkflowStepID: "original-step", EntryID: "entry-" + id,
		Summary: "original summary", DurationMs: 125, FindingCount: 1,
		FileCount: 3, RepositoryCount: 2, PromptTokens: 42, ResponseTokens: 17,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, order, 0, time.UTC),
	}
	if err := repo.CreateTaskReviewRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	return run
}

func assertRecoveryCounts(t *testing.T, h *runnerHarness, repo *sqliterepo.Repository, rows, prompts int) {
	t.Helper()
	runs, err := repo.ListTaskReviewRuns(context.Background(), "task-1", 100)
	if err != nil || len(runs) != rows || h.inference.promptCount() != prompts {
		t.Fatalf("runs=%d prompts=%d err=%v; want runs=%d prompts=%d", len(runs), h.inference.promptCount(), err, rows, prompts)
	}
}

func assertRecoverySettled(t *testing.T, repo *sqliterepo.Repository, original *models.TaskReviewRun) {
	t.Helper()
	got, err := repo.GetTaskReviewRun(context.Background(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.ReviewRunFailed || got.ErrorCode != CodeExecutionFailed || got.CompletedAt == nil {
		t.Fatalf("abandoned run was not settled: %+v", got)
	}
	// Only terminal accounting may change. Identity, history and all prior
	// accounting must survive the real service's read/modify/write path.
	got.Status, got.ErrorCode, got.ErrorMessage, got.CompletedAt = original.Status, original.ErrorCode, original.ErrorMessage, original.CompletedAt
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("settlement changed retained history: got %+v want %+v", got, original)
	}
}

func TestRunner_RecoverySettlesNativeBeforeFreshAdmission(t *testing.T) {
	for _, trigger := range []models.ReviewRunTrigger{models.ReviewTriggerManual, models.ReviewTriggerWorkflowStep} {
		for _, status := range []models.ReviewRunStatus{models.ReviewRunPending, models.ReviewRunRunning} {
			t.Run(string(trigger)+"/"+string(status), func(t *testing.T) {
				h, repo, _ := newRecoverySQLHarness(t)
				orphan := seedRecoveryRun(t, repo, "orphan", trigger, status, 1)
				finding := &models.TaskReviewFinding{
					ID: "retained-finding", RunID: orphan.ID, TaskID: orphan.TaskID,
					FilePath: "old.go", StartLine: 1, EndLine: 1, Severity: models.ReviewSeverityMajor,
					Category: "correctness", Title: "Original finding", Body: "Original body",
				}
				if err := repo.CreateTaskReviewFindings(context.Background(), []*models.TaskReviewFinding{finding}); err != nil {
					t.Fatal(err)
				}
				fresh, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-1", Trigger: trigger, EntryID: "fresh-entry"})
				if err != nil || fresh == nil || fresh.ID == orphan.ID {
					t.Fatalf("fresh admission did not replace orphan: run=%+v err=%v", fresh, err)
				}
				assertRecoverySettled(t, repo, orphan)
				retained, err := repo.GetTaskReviewFinding(context.Background(), finding.ID)
				if err != nil || !reflect.DeepEqual(retained, finding) {
					t.Fatalf("original finding changed: %+v err=%v", retained, err)
				}
				assertRecoveryCounts(t, h, repo, 2, 1)
			})
		}
	}
}

func TestRunner_RecoveryExhaustsRetainedNativeRows(t *testing.T) {
	h, repo, _ := newRecoverySQLHarness(t)
	old := seedRecoveryRun(t, repo, "older", models.ReviewTriggerWorkflowStep, models.ReviewRunRunning, 1)
	newer := seedRecoveryRun(t, repo, "newer", models.ReviewTriggerManual, models.ReviewRunPending, 2)
	if _, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-1", Trigger: models.ReviewTriggerManual}); err != nil {
		t.Fatal(err)
	}
	assertRecoverySettled(t, repo, old)
	assertRecoverySettled(t, repo, newer)
	assertRecoveryCounts(t, h, repo, 3, 1)
}

func TestRunner_RecoveryRejoinsExternalOwner(t *testing.T) {
	for _, trigger := range []models.ReviewRunTrigger{models.ReviewTriggerAgent, "legacy-unclassified"} {
		for _, hidden := range []bool{false, true} {
			t.Run(string(trigger)+"/hidden="+map[bool]string{false: "false", true: "true"}[hidden], func(t *testing.T) {
				h, repo, _ := newRecoverySQLHarness(t)
				owner := seedRecoveryRun(t, repo, "owner", trigger, models.ReviewRunRunning, 1)
				rows := 1
				if hidden {
					seedRecoveryRun(t, repo, "native", models.ReviewTriggerManual, models.ReviewRunRunning, 2)
					rows++
				}
				got, err := h.runner.Launch(context.Background(), RunRequest{TaskID: "task-1"})
				if err != nil || !reflect.DeepEqual(got, owner) {
					t.Fatalf("external owner was bypassed: run=%+v err=%v", got, err)
				}
				stored, err := repo.GetTaskReviewRun(context.Background(), owner.ID)
				if err != nil || !reflect.DeepEqual(stored, owner) {
					t.Fatalf("external owner changed: run=%+v err=%v", stored, err)
				}
				assertRecoveryCounts(t, h, repo, rows, 0)
			})
		}
	}
}

func TestRunner_RecoveryRedeliveredEntryKeepsOriginalRun(t *testing.T) {
	h, repo, _ := newRecoverySQLHarness(t)
	orphan := seedRecoveryRun(t, repo, "orphan", models.ReviewTriggerWorkflowStep, models.ReviewRunRunning, 1)
	req := RunRequest{TaskID: "task-1", Trigger: models.ReviewTriggerWorkflowStep, EntryID: orphan.EntryID}
	for range 2 {
		got, err := h.runner.Launch(context.Background(), req)
		if err != nil || got == nil || got.ID != orphan.ID || got.Status != models.ReviewRunFailed {
			t.Fatalf("redelivered entry must rejoin its settled row: %+v err=%v", got, err)
		}
	}
	assertRecoveryCounts(t, h, repo, 1, 0)
	req.EntryID = "new-entry"
	fresh, err := h.runner.Run(context.Background(), req)
	if err != nil || fresh == nil || fresh.ID == orphan.ID {
		t.Fatalf("new entry must admit fresh work: %+v err=%v", fresh, err)
	}
	assertRecoveryCounts(t, h, repo, 2, 1)
}
