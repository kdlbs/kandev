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
