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

func TestRepairExecutorRunningDeadIfCurrentRejectsRotatedExecution(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-cas")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-cas", TaskID: "task-cas", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-cas", SessionID: "session-cas", TaskID: "task-cas",
		AgentExecutionID: "execution-old", Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	observed, err := repo.GetExecutorRunningBySessionID(ctx, "session-cas")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}

	if _, err := repo.DB().ExecContext(ctx, `
		UPDATE executors_running
		SET agent_execution_id = ?, status = ?, updated_at = ?
		WHERE session_id = ?
	`, "execution-new", models.ExecutorRunningStatusRunning, time.Now().UTC(), "session-cas"); err != nil {
		t.Fatalf("rotate execution row: %v", err)
	}
	if err := repo.RepairExecutorRunningDeadIfCurrent(ctx, "session-cas", observed.AgentExecutionID, observed.UpdatedAt); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("CAS repair error = %v, want ErrExecutionRotated", err)
	}
	current, err := repo.GetExecutorRunningBySessionID(ctx, "session-cas")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID after rejected repair: %v", err)
	}
	if current.AgentExecutionID != "execution-new" || current.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("successor row changed after rejected repair: execution=%q status=%q", current.AgentExecutionID, current.Status)
	}
	if err := repo.DeleteExecutorRunningIfCurrent(ctx, "session-cas", observed.AgentExecutionID, observed.UpdatedAt); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("CAS delete error = %v, want ErrExecutionRotated", err)
	}
	if err := repo.DeleteExecutorRunningIfCurrent(ctx, "session-cas", current.AgentExecutionID, current.UpdatedAt); err != nil {
		t.Fatalf("delete current execution: %v", err)
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-cas"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("row after current delete error = %v, want ErrExecutorRunningNotFound", err)
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
	if err := repo.runMigrations(context.Background()); err != nil {
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

func TestExecutorRunningIdleSuspensionCASPreservesConversation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-idle-suspension")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-idle-suspension", TaskID: "task-idle-suspension", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-idle-suspension", SessionID: "session-idle-suspension", TaskID: "task-idle-suspension",
		ExecutorID: "profile", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusReady,
		Resumable: true, ResumeToken: "provider-conversation", LocalPID: 4242, AgentExecutionID: "execution-idle-suspension",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	observed, err := repo.GetExecutorRunningBySessionID(ctx, "session-idle-suspension")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	if err := repo.CompareAndSetExecutorRunningIdleSuspension(
		ctx, observed.SessionID, observed.AgentExecutionID, observed.UpdatedAt,
		models.ExecutorIdleSuspensionNone, models.ExecutorIdleSuspensionInProgress,
	); err != nil {
		t.Fatalf("claim idle suspension: %v", err)
	}
	if err := repo.CompareAndSetExecutorRunningIdleSuspension(
		ctx, observed.SessionID, observed.AgentExecutionID, observed.UpdatedAt,
		models.ExecutorIdleSuspensionNone, models.ExecutorIdleSuspensionInProgress,
	); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale idle suspension claim = %v, want ErrExecutionRotated", err)
	}
	if err := repo.CompareAndSetExecutorRunningIdleSuspension(
		ctx, observed.SessionID, observed.AgentExecutionID, time.Time{},
		models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionSuspended,
	); err != nil {
		t.Fatalf("complete idle suspension: %v", err)
	}
	got, err := repo.GetExecutorRunningBySessionID(ctx, observed.SessionID)
	if err != nil {
		t.Fatalf("read suspended row: %v", err)
	}
	if got.IdleSuspensionState != models.ExecutorIdleSuspensionSuspended || got.Status != models.ExecutorRunningStatusStopped || got.LocalPID != 0 {
		t.Fatalf("suspended row state = %q/%q local_pid=%d", got.IdleSuspensionState, got.Status, got.LocalPID)
	}
	if got.ResumeToken != "provider-conversation" || !got.Resumable {
		t.Fatalf("suspension lost restore data: token=%q resumable=%v", got.ResumeToken, got.Resumable)
	}
}

func TestExecutorRunningIdleSuspensionClaimRequiresCurrentWorkspacePolicy(t *testing.T) {
	for _, tc := range []struct {
		name                string
		policyEnabled       bool
		stalePolicyRevision bool
		staleExecutionRow   bool
		wantClaim           bool
	}{
		{name: "current enabled policy", policyEnabled: true, wantClaim: true},
		{name: "disabled policy", wantClaim: false},
		{name: "changed policy revision", policyEnabled: true, stalePolicyRevision: true, wantClaim: false},
		{name: "changed execution row", policyEnabled: true, staleExecutionRow: true, wantClaim: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID, sessionID := "task-idle-claim", "session-idle-claim"
			seedExecutorRunningCleanupTask(t, repo, taskID)
			workspace, err := repo.GetWorkspace(ctx, "ws-"+taskID)
			if err != nil {
				t.Fatal(err)
			}
			workspace.ACPIdleSuspensionEnabled = tc.policyEnabled
			if err := repo.UpdateWorkspace(ctx, workspace); err != nil {
				t.Fatal(err)
			}
			workspace, err = repo.GetWorkspace(ctx, workspace.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				Status: models.ExecutorRunningStatusReady, AgentExecutionID: "execution-idle-claim",
			}); err != nil {
				t.Fatal(err)
			}
			running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
			if err != nil {
				t.Fatal(err)
			}
			policyRevision := workspace.UpdatedAt
			rowRevision := running.UpdatedAt
			if tc.stalePolicyRevision {
				policyRevision = policyRevision.Add(-time.Second)
			}
			if tc.staleExecutionRow {
				rowRevision = rowRevision.Add(-time.Second)
			}

			claimed, err := repo.ClaimExecutorRunningIdleSuspension(
				ctx, sessionID, running.AgentExecutionID, rowRevision, workspace.ID, policyRevision,
			)
			if err != nil {
				t.Fatal(err)
			}
			if claimed != tc.wantClaim {
				t.Fatalf("claim = %v, want %v", claimed, tc.wantClaim)
			}
			got, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
			if err != nil {
				t.Fatal(err)
			}
			wantState := models.ExecutorIdleSuspensionNone
			if tc.wantClaim {
				wantState = models.ExecutorIdleSuspensionInProgress
			}
			if got.IdleSuspensionState != wantState {
				t.Fatalf("idle suspension state = %q, want %q", got.IdleSuspensionState, wantState)
			}
		})
	}
}

func TestExecutorRunningIdleSuspensionMigrationOnLegacyDB(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-idle-legacy")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-idle-legacy", TaskID: "task-idle-legacy", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-idle-legacy", SessionID: "session-idle-legacy", TaskID: "task-idle-legacy",
		ExecutorID: "profile", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStopped,
		Resumable: true, ResumeToken: "legacy-conversation", AgentExecutionID: "execution-idle-legacy",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	if _, err := repo.db.Exec(`ALTER TABLE executors_running DROP COLUMN idle_suspension_state`); err != nil {
		t.Fatalf("simulate legacy schema: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-idle-legacy")
	if err != nil {
		t.Fatalf("read legacy row: %v", err)
	}
	if got.IdleSuspensionState != models.ExecutorIdleSuspensionNone || got.ResumeToken != "legacy-conversation" {
		t.Fatalf("legacy row migration state=%q token=%q", got.IdleSuspensionState, got.ResumeToken)
	}
}
