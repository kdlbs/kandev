package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// openSecondSQLiteWriter opens an independent, non-shared-cache connection
// onto the repository's own database file, used to simulate a concurrent
// writer whose commit provokes a real driver contention error in the
// repository's own reconciliation transaction.
func openSecondSQLiteWriter(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=on", dbPath))
	if err != nil {
		t.Fatalf("open second sqlite writer: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// commitViaSecondConn reads stepID's current events through conn, applies
// mutate, and writes the result back unconditionally, simulating a
// concurrent writer's committed change landing between the repository's own
// SELECT and UPDATE.
func commitViaSecondConn(t *testing.T, conn *sql.DB, stepID string, mutate func(*wfmodels.StepEvents)) {
	t.Helper()
	var raw sql.NullString
	if err := conn.QueryRow(`SELECT events FROM workflow_steps WHERE id = ?`, stepID).Scan(&raw); err != nil {
		t.Fatalf("second-conn read events for %s: %v", stepID, err)
	}
	var events wfmodels.StepEvents
	if raw.String != "" {
		if err := json.Unmarshal([]byte(raw.String), &events); err != nil {
			t.Fatalf("second-conn unmarshal events for %s: %v", stepID, err)
		}
	}
	mutate(&events)
	updated, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("second-conn marshal events for %s: %v", stepID, err)
	}
	if _, err := conn.Exec(`UPDATE workflow_steps SET events = ?, updated_at = ? WHERE id = ?`, string(updated), time.Now().UTC(), stepID); err != nil {
		t.Fatalf("second-conn write events for %s: %v", stepID, err)
	}
}

// addAgentErrorMarkerAction replaces on_agent_error with a single queue_run
// marker action the participant seat reconciler does not manage, so tests
// exercising that reconciler can assert the concurrent writer's exact last
// committed value survived.
func addAgentErrorMarkerAction(events *wfmodels.StepEvents, marker string) {
	events.OnAgentError = []wfmodels.GenericAction{{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "marker-" + marker, "reason": "concurrent_writer_marker"},
	}}
}

// addOnEnterMarkerAction replaces on_enter with a single clear_decisions
// marker action the agent-error reconciler does not manage, so tests
// exercising that reconciler can assert the concurrent writer's exact last
// committed value survived.
func addOnEnterMarkerAction(events *wfmodels.StepEvents, marker string) {
	events.OnEnter = []wfmodels.OnEnterAction{{
		Type:   wfmodels.OnEnterClearDecisions,
		Config: map[string]interface{}{"marker": marker},
	}}
}

func newLoggerWithObserver(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	return log, logs
}

func TestIsRetryableStateRaceError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sqlite database is locked", errors.New("database is locked"), true},
		{"sqlite database table is locked", errors.New("database table is locked"), true},
		{"wrapped sqlite database is locked", fmt.Errorf("write step events: %w", errors.New("database is locked")), true},
		{"wrapped sqlite database table is locked", fmt.Errorf("write step events: %w", errors.New("database table is locked")), true},
		{"pg serialization_failure 40001", &pgconn.PgError{Code: "40001"}, true},
		{"pg deadlock_detected 40P01", &pgconn.PgError{Code: "40P01"}, true},
		{"pg lock_not_available 55P03", &pgconn.PgError{Code: "55P03"}, true},
		{"wrapped pg serialization_failure", fmt.Errorf("write step events: %w", &pgconn.PgError{Code: "40001"}), true},
		{"pg unrelated code", &pgconn.PgError{Code: "23505"}, false},
		{"context canceled", context.Canceled, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableStateRaceError(tt.err); got != tt.want {
				t.Errorf("isRetryableStateRaceError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestTryHealWorkflowStepEvents_PerSiteTransientRetries drives a transient
// contention error injected at each of the five database sites through the
// participant seat reconciler's retry loop, asserting exactly one retry and
// a successful outcome.
func TestTryHealWorkflowStepEvents_PerSiteTransientRetries(t *testing.T) {
	sites := []stepEventsDBSite{
		stepEventsDBSiteBegin,
		stepEventsDBSiteRead,
		stepEventsDBSiteWrite,
		stepEventsDBSiteRowsAffected,
		stepEventsDBSiteCommit,
	}
	for _, site := range sites {
		t.Run(string(site), func(t *testing.T) {
			repo := newRepoForBuiltinWorkflowTests(t)
			stepID := "site-transient-" + string(site)
			seedStaleOfficeReviewStep(t, repo, "wf-"+stepID, stepID)

			var beginCalls int
			repo.workflowStepEventsDBFailpoint = func(_ string, gotSite stepEventsDBSite) error {
				if gotSite == stepEventsDBSiteBegin {
					beginCalls++
				}
				if gotSite == site && beginCalls == 1 {
					return errors.New("database is locked")
				}
				return nil
			}

			if err := repo.healParticipantSeatRowWithRetry(stepID, "wf-"+stepID, "office-default", "Review", "reviewer"); err != nil {
				t.Fatalf("healParticipantSeatRowWithRetry: %v", err)
			}
			if beginCalls != 2 {
				t.Errorf("begin site calls = %d, want 2", beginCalls)
			}
			step := loadReviewStepEvents(t, repo, stepID)
			found := 0
			for _, action := range step.Events.OnEnter {
				if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
					found++
				}
			}
			if found != 1 {
				t.Errorf("seat action count = %d, want 1", found)
			}
		})
	}
}

// TestTryHealWorkflowStepEvents_PerSiteNonTransientErrors drives a
// non-transient, non-classified error injected at each of the five database
// sites, asserting the reconciler returns it immediately after one attempt
// without modifying the row.
func TestTryHealWorkflowStepEvents_PerSiteNonTransientErrors(t *testing.T) {
	sites := []stepEventsDBSite{
		stepEventsDBSiteBegin,
		stepEventsDBSiteRead,
		stepEventsDBSiteWrite,
		stepEventsDBSiteRowsAffected,
		stepEventsDBSiteCommit,
	}
	for _, site := range sites {
		t.Run(string(site), func(t *testing.T) {
			repo := newRepoForBuiltinWorkflowTests(t)
			stepID := "site-nontransient-" + string(site)
			seedStaleOfficeReviewStep(t, repo, "wf-"+stepID, stepID)

			injected := errors.New("injected non-transient error at " + string(site))
			var beginCalls int
			repo.workflowStepEventsDBFailpoint = func(_ string, gotSite stepEventsDBSite) error {
				if gotSite == stepEventsDBSiteBegin {
					beginCalls++
				}
				if gotSite == site && beginCalls == 1 {
					return injected
				}
				return nil
			}

			err := repo.healParticipantSeatRowWithRetry(stepID, "wf-"+stepID, "office-default", "Review", "reviewer")
			if !errors.Is(err, injected) {
				t.Fatalf("healParticipantSeatRowWithRetry error = %v, want wrapping %v", err, injected)
			}
			if beginCalls != 1 {
				t.Errorf("begin site calls = %d, want 1", beginCalls)
			}
			step := loadReviewStepEvents(t, repo, stepID)
			for _, action := range step.Events.OnEnter {
				if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
					t.Error("row was modified despite a non-transient error")
				}
			}
		})
	}
}

// TestHealParticipantSeatRowWithRetry_CancellationBetweenAttemptsIsTerminal
// and its agent-error counterpart below cover
// AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.7: a startup context canceled
// between attempts is never retried or swallowed.
func TestHealParticipantSeatRowWithRetry_CancellationBetweenAttemptsIsTerminal(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	log, logs := newLoggerWithObserver(t)
	repo.log = log

	stepID := "cancel-seat-review"
	seedStaleOfficeReviewStep(t, repo, "wf-cancel-seat", stepID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo.migrate = db.NewRequiredMigrateLoggerContext(repo.db, nil, ctx)

	var beginCalls, writeCalls int
	repo.workflowStepEventsDBFailpoint = func(_ string, site stepEventsDBSite) error {
		switch site {
		case stepEventsDBSiteBegin:
			beginCalls++
		case stepEventsDBSiteWrite:
			writeCalls++
			if writeCalls == 1 {
				cancel()
				return errors.New("database is locked")
			}
		}
		return nil
	}

	err := repo.healParticipantSeatRowWithRetry(stepID, "wf-cancel-seat", "office-default", "Review", "reviewer")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("healParticipantSeatRowWithRetry error = %v, want wrapping context.Canceled", err)
	}
	if beginCalls != 2 {
		t.Errorf("begin site calls = %d, want 2", beginCalls)
	}
	if writeCalls != 1 {
		t.Errorf("write site calls = %d, want 1 (no second write after cancellation)", writeCalls)
	}
	step := loadReviewStepEvents(t, repo, stepID)
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			t.Error("row was modified despite cancellation")
		}
	}
	if logs.Len() != 0 {
		t.Errorf("expected no exhaustion warning on a terminal cancellation error, got %d", logs.Len())
	}
}

func TestHealAgentErrorRowWithRetry_CancellationBetweenAttemptsIsTerminal(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	log, logs := newLoggerWithObserver(t)
	repo.log = log

	stepID := "cancel-agent-err-review"
	seedStaleOfficeStepWithoutAgentError(t, repo, "wf-cancel-agent-err", stepID, "Review", "review")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo.migrate = db.NewRequiredMigrateLoggerContext(repo.db, nil, ctx)

	var beginCalls, writeCalls int
	repo.workflowStepEventsDBFailpoint = func(_ string, site stepEventsDBSite) error {
		switch site {
		case stepEventsDBSiteBegin:
			beginCalls++
		case stepEventsDBSiteWrite:
			writeCalls++
			if writeCalls == 1 {
				cancel()
				return errors.New("database is locked")
			}
		}
		return nil
	}

	action := wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error"},
	}
	err := repo.healAgentErrorRowWithRetry(stepID, "wf-cancel-agent-err", "office-default", "Review", action)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("healAgentErrorRowWithRetry error = %v, want wrapping context.Canceled", err)
	}
	if beginCalls != 2 {
		t.Errorf("begin site calls = %d, want 2", beginCalls)
	}
	if writeCalls != 1 {
		t.Errorf("write site calls = %d, want 1 (no second write after cancellation)", writeCalls)
	}
	step := loadStepEvents(t, repo, stepID)
	if len(step.Events.OnAgentError) != 0 {
		t.Error("row was modified despite cancellation")
	}
	if logs.Len() != 0 {
		t.Errorf("expected no exhaustion warning on a terminal cancellation error, got %d", logs.Len())
	}
}

// TestHealParticipantSeatRowWithRetry_RealDriverTransientContentionAtWrite
// and its agent-error counterpart drive a real SQLite WAL BUSY_SNAPSHOT
// error (not a synthetic one) by committing through a second, independent
// connection between the repository's own SELECT and UPDATE.
func TestHealParticipantSeatRowWithRetry_RealDriverTransientContentionAtWrite(t *testing.T) {
	repo, dbPath := newRepoForBuiltinWorkflowTestsWithPath(t)
	log, logs := newLoggerWithObserver(t)
	repo.log = log

	stepID := "real-transient-seat-review"
	seedStaleOfficeReviewStep(t, repo, "wf-real-transient-seat", stepID)
	conn := openSecondSQLiteWriter(t, dbPath)

	attempts := 0
	repo.workflowStepEventsAfterReadHook = func(gotStepID string) {
		attempts++
		if attempts == 1 {
			commitViaSecondConn(t, conn, gotStepID, func(events *wfmodels.StepEvents) {
				addAgentErrorMarkerAction(events, "1")
			})
		}
	}

	if err := repo.healParticipantSeatRowWithRetry(stepID, "wf-real-transient-seat", "office-default", "Review", "reviewer"); err != nil {
		t.Fatalf("healParticipantSeatRowWithRetry: %v", err)
	}

	step := loadReviewStepEvents(t, repo, stepID)
	seatCount := 0
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			seatCount++
		}
	}
	if seatCount != 1 {
		t.Errorf("seat action count = %d, want 1", seatCount)
	}
	if len(step.Events.OnAgentError) != 1 {
		t.Errorf("expected the concurrent writer's marker action to survive, got %+v", step.Events.OnAgentError)
	}
	if logs.Len() != 0 {
		t.Errorf("expected no warning for contention that resolved within budget, got %d", logs.Len())
	}
}

func TestHealAgentErrorRowWithRetry_RealDriverTransientContentionAtWrite(t *testing.T) {
	repo, dbPath := newRepoForBuiltinWorkflowTestsWithPath(t)
	log, logs := newLoggerWithObserver(t)
	repo.log = log

	stepID := "real-transient-agent-err-review"
	seedStaleOfficeStepWithoutAgentError(t, repo, "wf-real-transient-agent-err", stepID, "Review", "review")
	conn := openSecondSQLiteWriter(t, dbPath)

	attempts := 0
	repo.workflowStepEventsAfterReadHook = func(gotStepID string) {
		attempts++
		if attempts == 1 {
			commitViaSecondConn(t, conn, gotStepID, func(events *wfmodels.StepEvents) {
				addOnEnterMarkerAction(events, "1")
			})
		}
	}

	action := wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error"},
	}
	if err := repo.healAgentErrorRowWithRetry(stepID, "wf-real-transient-agent-err", "office-default", "Review", action); err != nil {
		t.Fatalf("healAgentErrorRowWithRetry: %v", err)
	}

	step := loadStepEvents(t, repo, stepID)
	if len(step.Events.OnAgentError) != 1 {
		t.Errorf("on_agent_error len = %d, want 1", len(step.Events.OnAgentError))
	}
	found := false
	for _, onEnter := range step.Events.OnEnter {
		if onEnter.Type == wfmodels.OnEnterClearDecisions {
			found = true
		}
	}
	if !found {
		t.Error("expected the concurrent writer's marker action to survive")
	}
	if logs.Len() != 0 {
		t.Errorf("expected no warning for contention that resolved within budget, got %d", logs.Len())
	}
}

// TestHealParticipantSeatRowWithRetry_RealDriverPersistentContention and its
// agent-error counterpart cover exhaustion: the concurrent writer commits a
// distinct value on every attempt, so the row is left at the writer's last
// committed value and the exhaustion warning carries retry_cause and error.
func TestHealParticipantSeatRowWithRetry_RealDriverPersistentContention(t *testing.T) {
	repo, dbPath := newRepoForBuiltinWorkflowTestsWithPath(t)
	log, logs := newLoggerWithObserver(t)
	repo.log = log

	stepID := "real-persistent-seat-review"
	seedStaleOfficeReviewStep(t, repo, "wf-real-persistent-seat", stepID)
	conn := openSecondSQLiteWriter(t, dbPath)

	attempts := 0
	repo.workflowStepEventsAfterReadHook = func(gotStepID string) {
		attempts++
		commitViaSecondConn(t, conn, gotStepID, func(events *wfmodels.StepEvents) {
			addAgentErrorMarkerAction(events, strconv.Itoa(attempts))
		})
	}

	if err := repo.healParticipantSeatRowWithRetry(stepID, "wf-real-persistent-seat", "office-default", "Review", "reviewer"); err != nil {
		t.Fatalf("healParticipantSeatRowWithRetry: %v", err)
	}
	if attempts != maxParticipantSeatReconcileAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, maxParticipantSeatReconcileAttempts)
	}

	step := loadReviewStepEvents(t, repo, stepID)
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			t.Error("row was modified despite persistent contention exhausting the budget")
		}
	}
	if len(step.Events.OnAgentError) != 1 {
		t.Fatalf("expected the row to equal the writer's last committed value, got %+v", step.Events.OnAgentError)
	}
	wantTarget := "marker-" + strconv.Itoa(maxParticipantSeatReconcileAttempts)
	if got, _ := step.Events.OnAgentError[0].Config["target"].(string); got != wantTarget {
		t.Errorf("final marker target = %q, want %q", got, wantTarget)
	}

	assertLockContentionExhaustionWarning(t, logs)
}

func TestHealAgentErrorRowWithRetry_RealDriverPersistentContention(t *testing.T) {
	repo, dbPath := newRepoForBuiltinWorkflowTestsWithPath(t)
	log, logs := newLoggerWithObserver(t)
	repo.log = log

	stepID := "real-persistent-agent-err-review"
	seedStaleOfficeStepWithoutAgentError(t, repo, "wf-real-persistent-agent-err", stepID, "Review", "review")
	conn := openSecondSQLiteWriter(t, dbPath)

	attempts := 0
	repo.workflowStepEventsAfterReadHook = func(gotStepID string) {
		attempts++
		commitViaSecondConn(t, conn, gotStepID, func(events *wfmodels.StepEvents) {
			addOnEnterMarkerAction(events, strconv.Itoa(attempts))
		})
	}

	action := wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error"},
	}
	if err := repo.healAgentErrorRowWithRetry(stepID, "wf-real-persistent-agent-err", "office-default", "Review", action); err != nil {
		t.Fatalf("healAgentErrorRowWithRetry: %v", err)
	}
	if attempts != maxAgentErrorReconcileAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, maxAgentErrorReconcileAttempts)
	}

	step := loadStepEvents(t, repo, stepID)
	if len(step.Events.OnAgentError) != 0 {
		t.Error("row was modified despite persistent contention exhausting the budget")
	}
	if len(step.Events.OnEnter) != 1 {
		t.Fatalf("expected the row to equal the writer's last committed value, got %+v", step.Events.OnEnter)
	}
	wantMarker := strconv.Itoa(maxAgentErrorReconcileAttempts)
	if got, _ := step.Events.OnEnter[0].Config["marker"].(string); got != wantMarker {
		t.Errorf("final marker = %q, want %q", got, wantMarker)
	}

	assertLockContentionExhaustionWarning(t, logs)
}

func assertLockContentionExhaustionWarning(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning after retry exhaustion, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["retry_cause"] != string(stepEventsRetryCauseLockContention) {
		t.Errorf("retry_cause = %v, want %q", fields["retry_cause"], stepEventsRetryCauseLockContention)
	}
	errField, ok := fields["error"].(string)
	if !ok || errField == "" {
		t.Errorf("expected a non-empty error field for lock_contention exhaustion, got %v", fields["error"])
	}
}

// TestHealStepParticipantSeatRole_RealDriverContinuationPastSkippedRow
// covers AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.10: a target exhausted by
// persistent contention does not stop a later, unaffected target from being
// reconciled.
func TestHealStepParticipantSeatRole_RealDriverContinuationPastSkippedRow(t *testing.T) {
	repo, dbPath := newRepoForBuiltinWorkflowTestsWithPath(t)
	log, logs := newLoggerWithObserver(t)
	repo.log = log

	stepA := "continuation-seat-a"
	stepB := "continuation-seat-b"
	seedStaleOfficeReviewStep(t, repo, "wf-continuation-a", stepA)
	seedStaleOfficeReviewStep(t, repo, "wf-continuation-b", stepB)
	conn := openSecondSQLiteWriter(t, dbPath)

	attemptsA := 0
	repo.workflowStepEventsAfterReadHook = func(gotStepID string) {
		if gotStepID != stepA {
			return
		}
		attemptsA++
		commitViaSecondConn(t, conn, gotStepID, func(events *wfmodels.StepEvents) {
			addAgentErrorMarkerAction(events, strconv.Itoa(attemptsA))
		})
	}

	if err := repo.healStepParticipantSeatRole("office-default", "Review", "reviewer"); err != nil {
		t.Fatalf("healStepParticipantSeatRole: %v", err)
	}

	stepAEvents := loadReviewStepEvents(t, repo, stepA)
	for _, action := range stepAEvents.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			t.Error("row A was modified despite persistent contention exhausting the budget")
		}
	}
	stepBEvents := loadReviewStepEvents(t, repo, stepB)
	found := false
	for _, action := range stepBEvents.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			found = true
		}
	}
	if !found {
		t.Error("row B (unaffected by contention) was not reconciled")
	}
	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning (for row A only), got %d", logs.Len())
	}
}

// TestHealParticipantSeatRowWithRetry_NonTransientErrorIsTerminal covers the
// non-transient real-driver regression: malformed stored events JSON returns
// an error after one attempt without an exhaustion warning.
func TestHealParticipantSeatRowWithRetry_NonTransientErrorIsTerminal(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('malformed-office-seats', 'ws-1', 'Office', 'office-default', 1, 1, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('malformed-office-seats-review', 'malformed-office-seats', 'Review', 2, 'review', ?, 0, ?, ?)
	`), "{not valid json", legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert malformed step: %v", err)
	}

	log, logs := newLoggerWithObserver(t)
	repo.log = log

	err = repo.healParticipantSeatRowWithRetry("malformed-office-seats-review", "malformed-office-seats", "office-default", "Review", "reviewer")
	if err == nil {
		t.Fatal("expected a terminal error for malformed stored events JSON")
	}
	if logs.Len() != 0 {
		t.Errorf("expected no exhaustion warning on a terminal error, got %d", logs.Len())
	}
}
