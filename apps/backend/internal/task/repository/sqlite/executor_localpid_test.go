package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestExecutorRunningLocalPIDRoundTrips proves the local_pid column is an
// independent, fully-plumbed column: it is written by UpsertExecutorRunning and
// read back by GetExecutorRunningBySessionID / ListExecutorsRunning without
// aliasing the SSH-only pid column. This is the schema half of the #1597
// executor-row-desync fix (#1597 truthful executor rows): a local/standalone row
// must be able to carry a real host-local liveness handle distinct from the
// remote-host SSH pid.
func TestExecutorRunningLocalPIDRoundTrips(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	lastSeen := time.Now().UTC().Truncate(time.Second)

	seedExecutorRunningCleanupTask(t, repo, "task-1")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-local",
		TaskID: "task-1",
		State:  models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession(session-local): %v", err)
	}

	// A local/standalone row: local_pid is the host-local agentctl PID, pid is
	// 0 (SSH-only, no remote host). last_seen_at reflects a real observation.
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:           "session-local",
		SessionID:    "session-local",
		TaskID:       "task-1",
		ExecutorID:   "exec-local",
		Runtime:      agentruntime.RuntimeStandalone,
		Status:       models.ExecutorRunningStatusReady,
		Resumable:    true,
		AgentctlURL:  "http://127.0.0.1:8765",
		AgentctlPort: 8765,
		PID:          0,
		LocalPID:     424242,
		LastSeenAt:   &lastSeen,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(session-local): %v", err)
	}

	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-local")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	if got.LocalPID != 424242 {
		t.Fatalf("local_pid not persisted/read: got %d want 424242", got.LocalPID)
	}
	if got.PID != 0 {
		t.Fatalf("pid must stay independent of local_pid: got %d want 0", got.PID)
	}
	if got.Status != models.ExecutorRunningStatusReady {
		t.Fatalf("status not persisted: got %q want %q", got.Status, models.ExecutorRunningStatusReady)
	}
	if got.LastSeenAt == nil || !got.LastSeenAt.Equal(lastSeen) {
		t.Fatalf("last_seen_at not persisted: got %v want %v", got.LastSeenAt, lastSeen)
	}

	// The upsert path (ON CONFLICT) must update local_pid too, not just insert it.
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:         "session-local",
		SessionID:  "session-local",
		TaskID:     "task-1",
		ExecutorID: "exec-local",
		Runtime:    agentruntime.RuntimeStandalone,
		Status:     models.ExecutorRunningStatusReady,
		LocalPID:   515151,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(update local_pid): %v", err)
	}
	got, err = repo.GetExecutorRunningBySessionID(ctx, "session-local")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID(after update): %v", err)
	}
	if got.LocalPID != 515151 {
		t.Fatalf("local_pid not updated on conflict: got %d want 515151", got.LocalPID)
	}

	// list path must scan the column too.
	rows, err := repo.ListExecutorsRunning(ctx)
	if err != nil {
		t.Fatalf("ListExecutorsRunning: %v", err)
	}
	var found bool
	for _, row := range rows {
		if row.SessionID == "session-local" {
			found = true
			if row.LocalPID != 515151 {
				t.Fatalf("list did not scan local_pid: got %d want 515151", row.LocalPID)
			}
		}
	}
	if !found {
		t.Fatalf("session-local row missing from ListExecutorsRunning")
	}
}

// TestExecutorRunningLocalPIDSeparateFromSSHPID guards the #1597 pid-semantics rule
// that a local liveness handle must never overload the SSH pid column: an SSH row
// carries pid (remote host) with local_pid==0, and a local row carries local_pid
// with pid==0, and neither leaks into the other.
func TestExecutorRunningLocalPIDSeparateFromSSHPID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedExecutorRunningCleanupTask(t, repo, "task-1")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-ssh", TaskID: "task-1", State: models.TaskSessionStateRunning}); err != nil {
		t.Fatalf("CreateTaskSession(session-ssh): %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:         "session-ssh",
		SessionID:  "session-ssh",
		TaskID:     "task-1",
		ExecutorID: "exec-ssh",
		Runtime:    agentruntime.RuntimeSSH,
		Status:     models.ExecutorRunningStatusRunning,
		PID:        9999, // remote-host agentctl pid
		LocalPID:   0,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(session-ssh): %v", err)
	}

	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-ssh")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID(session-ssh): %v", err)
	}
	if got.PID != 9999 {
		t.Fatalf("ssh pid clobbered: got %d want 9999", got.PID)
	}
	if got.LocalPID != 0 {
		t.Fatalf("ssh row must not carry a local_pid: got %d want 0", got.LocalPID)
	}
}

// TestRepairExecutorRunningDead proves the repair-in-place primitive of the
// resume-safety invariant (#1597 resume-safety invariant): a row whose process is
// gone is marked stopped with its local liveness handle cleared, WITHOUT losing
// the resume_token / worktree that keep the session resumable.
func TestRepairExecutorRunningDead(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedExecutorRunningCleanupTask(t, repo, "task-1")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-r", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput}); err != nil {
		t.Fatalf("CreateTaskSession(session-r): %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:             "session-r",
		SessionID:      "session-r",
		TaskID:         "task-1",
		ExecutorID:     "exec-local",
		Runtime:        agentruntime.RuntimeStandalone,
		Status:         models.ExecutorRunningStatusRunning,
		Resumable:      true,
		ResumeToken:    "acp-resume-xyz",
		WorktreePath:   "/tmp/wt/session-r",
		WorktreeBranch: "feature/x",
		LocalPID:       777777,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(session-r): %v", err)
	}

	if err := repo.RepairExecutorRunningDead(ctx, "session-r"); err != nil {
		t.Fatalf("RepairExecutorRunningDead: %v", err)
	}

	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-r")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID(session-r): %v", err)
	}
	if got.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("repair status: got %q want stopped", got.Status)
	}
	if got.LocalPID != 0 {
		t.Fatalf("repair must clear the local liveness handle: got local_pid=%d", got.LocalPID)
	}
	if got.ResumeToken != "acp-resume-xyz" {
		t.Fatalf("repair must preserve resume_token: got %q", got.ResumeToken)
	}
	if got.WorktreePath != "/tmp/wt/session-r" || got.WorktreeBranch != "feature/x" {
		t.Fatalf("repair must preserve worktree columns: got path=%q branch=%q", got.WorktreePath, got.WorktreeBranch)
	}
	if got.LastSeenAt == nil {
		t.Fatal("repair must stamp a fresh last_seen_at observation")
	}

	// Missing row → ErrExecutorRunningNotFound (idempotent-friendly for callers).
	if err := repo.RepairExecutorRunningDead(ctx, "no-such-session"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("repair on missing row: got %v want ErrExecutorRunningNotFound", err)
	}
}

// TestExecutorRunningLocalPIDMigrationOnLegacyDB is the same-database replay test
// mandated by ADR 0027 for a schema change: it proves the `local_pid` ADD COLUMN
// migration upgrades a DB that PREDATES the column, adding it with the default
// while leaving existing rows intact and readable. This is the real production
// upgrade path — the operator's existing executors_running rows (all pid=0) gain
// local_pid=0 on first boot of the new binary, not a fresh install.
func TestExecutorRunningLocalPIDMigrationOnLegacyDB(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Seed a row on the current (migrated) schema.
	seedExecutorRunningCleanupTask(t, repo, "task-legacy")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-legacy", TaskID: "task-legacy", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "session-legacy",
		SessionID:   "session-legacy",
		TaskID:      "task-legacy",
		ExecutorID:  "exec-legacy",
		Runtime:     agentruntime.RuntimeStandalone,
		Status:      models.ExecutorRunningStatusStarting,
		Resumable:   true,
		ResumeToken: "legacy-resume-token",
		LocalPID:    321,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	// Rewind to a pre-local_pid schema: drop the column so the row now looks like
	// one written by an older binary that never had it.
	if _, err := repo.db.Exec(`ALTER TABLE executors_running DROP COLUMN local_pid`); err != nil {
		t.Fatalf("simulate legacy schema (drop local_pid): %v", err)
	}

	// Re-run migrations, exactly as a boot of the new binary would. The
	// idempotent ADD COLUMN must re-add local_pid without erroring.
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations on legacy DB: %v", err)
	}

	// The pre-existing row survives, is readable, and its resume_token is intact —
	// the migration must not drop rows. local_pid comes back as the column default
	// (0) for a row that predated the column.
	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-legacy")
	if err != nil {
		t.Fatalf("legacy row must survive the migration: %v", err)
	}
	if got.LocalPID != 0 {
		t.Errorf("legacy row local_pid = %d, want 0 (column default after ADD COLUMN)", got.LocalPID)
	}
	if got.ResumeToken != "legacy-resume-token" {
		t.Errorf("resume_token lost across migration: got %q", got.ResumeToken)
	}
	if got.Status != models.ExecutorRunningStatusStarting {
		t.Errorf("status lost across migration: got %q", got.Status)
	}

	// And the re-added column is fully usable: a repair writes/reads local_pid=0.
	if err := repo.RepairExecutorRunningDead(ctx, "session-legacy"); err != nil {
		t.Fatalf("repair after migration: %v", err)
	}
	repaired, err := repo.GetExecutorRunningBySessionID(ctx, "session-legacy")
	if err != nil {
		t.Fatalf("read after repair: %v", err)
	}
	if repaired.Status != models.ExecutorRunningStatusStopped || repaired.LocalPID != 0 {
		t.Errorf("re-added local_pid column must be writable; got status=%q local_pid=%d", repaired.Status, repaired.LocalPID)
	}
}
