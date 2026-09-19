package database

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/system/jobs"
)

func TestFactoryReset_WrongConfirm_ReturnsError(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)

	id, err := svc.FactoryReset(context.Background(), "WRONG")
	if err == nil {
		t.Fatal("expected error for wrong confirm, got nil")
	}
	if !errors.Is(err, ErrResetNotConfirmed) {
		t.Errorf("err = %v, want ErrResetNotConfirmed", err)
	}
	if id != "" {
		t.Errorf("expected empty id when not confirmed, got %q", id)
	}
	if len(tracker.List()) != 0 {
		t.Errorf("no jobs should be started when confirm fails; got %d", len(tracker.List()))
	}
}

func TestFactoryReset_Confirmed_RunsFullSequence(t *testing.T) {
	svc, tracker, _, dataDir := newTestService(t)

	var shutdownCalls atomic.Int32
	svc.OrchestratorShutdown = func() { shutdownCalls.Add(1) }

	id, err := svc.FactoryReset(context.Background(), "RESET")
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty job id")
	}
	job := waitForState(t, tracker, id, jobs.StateSucceeded)
	if job.State != jobs.StateSucceeded {
		t.Fatalf("state = %s, want succeeded; message=%s", job.State, job.Message)
	}

	// Snapshot path should be in the result map and the file must exist.
	rawPath, ok := job.Result["snapshot_path"]
	if !ok {
		t.Fatalf("result missing snapshot_path: %+v", job.Result)
	}
	snapshotPath, _ := rawPath.(string)
	if snapshotPath == "" {
		t.Fatalf("snapshot_path empty")
	}
	if !strings.HasPrefix(filepath.Base(snapshotPath), "kandev-pre-reset-") {
		t.Errorf("snapshot filename %q should start with kandev-pre-reset-", filepath.Base(snapshotPath))
	}
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Errorf("snapshot file missing: %v", err)
	}
	// Path must live inside <dataDir>/backups
	if filepath.Dir(snapshotPath) != filepath.Join(dataDir, "backups") {
		t.Errorf("snapshot dir = %s, want %s", filepath.Dir(snapshotPath), filepath.Join(dataDir, "backups"))
	}

	// tables_dropped must reflect both seeded user tables.
	dropped, _ := job.Result["tables_dropped"].(int)
	if dropped < 2 {
		t.Errorf("tables_dropped = %d, want >= 2 (users, sessions_t)", dropped)
	}

	// Verify user tables are gone, kandev_meta is kept.
	rows, err := svc.pool.Reader().Query(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var remaining []string
	for rows.Next() {
		var n string
		if scanErr := rows.Scan(&n); scanErr != nil {
			t.Fatalf("scan: %v", scanErr)
		}
		remaining = append(remaining, n)
	}
	if len(remaining) != 1 || remaining[0] != "kandev_meta" {
		t.Errorf("remaining tables = %v, want [kandev_meta]", remaining)
	}

	// Wiped subdirs must be gone.
	for _, p := range []string{svc.dirs.Worktrees, svc.dirs.Repos, svc.dirs.Sessions, svc.dirs.Tasks, svc.dirs.QuickChat} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("subdir %s still exists (err=%v)", p, err)
		}
	}

	// restart_required must be set so the frontend dialog prompts the user
	// to quit and relaunch Kandev (no auto re-exec).
	if got, _ := job.Result["restart_required"].(bool); !got {
		t.Errorf("restart_required missing or false in job result: %+v", job.Result)
	}
	if shutdownCalls.Load() != 1 {
		t.Errorf("OrchestratorShutdown called %d times, want 1", shutdownCalls.Load())
	}
}

// TestFactoryReset_DeletesDeliveryLedgerActivationKeys covers
// docs/specs/task-delivery-ledger/spec.md, "Reset parity": an activated
// database resets with both telemetry.*.activated_at keys absent from
// kandev_meta afterward, while the table itself (and unrelated keys) are
// kept.
func TestFactoryReset_DeletesDeliveryLedgerActivationKeys(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)

	if _, err := svc.pool.Writer().Exec(`
		INSERT OR REPLACE INTO kandev_meta (key, value) VALUES
			('telemetry.delivery_ledger.activated_at', '2026-08-01T00:00:00Z'),
			('telemetry.run_outcome.activated_at', '2026-08-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed activation keys: %v", err)
	}

	id, err := svc.FactoryReset(context.Background(), "RESET")
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}
	job := waitForState(t, tracker, id, jobs.StateSucceeded)
	if job.State != jobs.StateSucceeded {
		t.Fatalf("state = %s, want succeeded; message=%s", job.State, job.Message)
	}

	var count int
	if err := svc.pool.Reader().QueryRow(`
		SELECT COUNT(*) FROM kandev_meta
		WHERE key IN ('telemetry.delivery_ledger.activated_at', 'telemetry.run_outcome.activated_at')
	`).Scan(&count); err != nil {
		t.Fatalf("query kandev_meta: %v", err)
	}
	if count != 0 {
		t.Errorf("activation keys remaining = %d, want 0", count)
	}

	// kandev_meta itself, and its unrelated keys, must survive the reset.
	var version string
	if err := svc.pool.Reader().QueryRow(
		`SELECT value FROM kandev_meta WHERE key = 'kandev_version'`,
	).Scan(&version); err != nil {
		t.Fatalf("kandev_version missing after reset: %v", err)
	}
	if version != "v0.99.0" {
		t.Errorf("kandev_version = %q, want unchanged v0.99.0", version)
	}
}

func TestFactoryReset_WaitsForDatabaseWorkersBeforeMaintenance(t *testing.T) {
	svc, _, _, _ := newTestService(t)

	quiesceStarted := make(chan struct{})
	releaseQuiesce := make(chan struct{})
	svc.DatabaseQuiesce = func() error {
		close(quiesceStarted)
		<-releaseQuiesce
		return nil
	}

	resultCh := make(chan error, 1)
	go func() {
		_, err := svc.runFactoryReset(context.Background())
		resultCh <- err
	}()

	select {
	case <-quiesceStarted:
	case <-time.After(time.Second):
		t.Fatal("factory reset did not quiesce database workers")
	}

	var usersTable int
	if err := svc.pool.Reader().QueryRow(`
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'
	`).Scan(&usersTable); err != nil {
		t.Fatalf("check users table while quiescing: %v", err)
	}
	if usersTable != 1 {
		t.Fatalf("users table count while quiescing = %d, want 1", usersTable)
	}

	close(releaseQuiesce)
	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("factory reset: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("factory reset did not finish after database workers stopped")
	}

}

// TestFactoryReset_ActivationKeyDeleteFailureLeavesUserTablesIntact is
// Review round 2, finding #2: runFactoryReset's own comment documents a
// deliberate fail-safe ordering — delete the delivery-ledger/run-outcome
// activation keys BEFORE dropping any user table, specifically so that if
// key deletion fails, the reset aborts before dropUserTables ever runs.
// Nothing exercised that path: this test forces persistence.DeleteKeys to
// fail (by dropping kandev_meta, the table it deletes rows from, before
// the reset runs) and asserts the job surfaces as failed while the seeded
// user tables (users, sessions_t) are still present.
func TestFactoryReset_ActivationKeyDeleteFailureLeavesUserTablesIntact(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)

	if _, err := svc.pool.Writer().Exec(`DROP TABLE kandev_meta`); err != nil {
		t.Fatalf("drop kandev_meta: %v", err)
	}

	id, err := svc.FactoryReset(context.Background(), "RESET")
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}
	job := waitForState(t, tracker, id, jobs.StateFailed)
	if job.State != jobs.StateFailed {
		t.Fatalf("state = %s, want failed; message=%s", job.State, job.Message)
	}

	rows, err := svc.pool.Reader().Query(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()
	remaining := map[string]bool{}
	for rows.Next() {
		var n string
		if scanErr := rows.Scan(&n); scanErr != nil {
			t.Fatalf("scan: %v", scanErr)
		}
		remaining[n] = true
	}
	for _, want := range []string{"users", "sessions_t"} {
		if !remaining[want] {
			t.Errorf("table %q missing after a failed activation-key delete; want it kept (drop must not have run)", want)
		}
	}
}

func TestHandleReset_WrongConfirm_Returns400(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/reset", HandleReset(svc))

	w := serveHTTP(r, httpPost(t, "/reset", `{"confirm":"NOPE"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleReset_Confirmed_Returns202WithJobID(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)
	// FactoryReset no longer re-execs; just install a no-op shutdown so the
	// orchestrator hook fires without side effects.
	svc.OrchestratorShutdown = func() {}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/reset", HandleReset(svc))

	w := serveHTTP(r, httpPost(t, "/reset", `{"confirm":"RESET"}`))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), `"job_id"`) {
		t.Errorf("body missing job_id: %s", w.Body.String())
	}
	// Make sure the spawned job finishes before the test exits so the temp dir cleanup
	// doesn't race with VACUUM INTO.
	for _, j := range tracker.List() {
		waitForState(t, tracker, j.ID, jobs.StateSucceeded)
	}
}
