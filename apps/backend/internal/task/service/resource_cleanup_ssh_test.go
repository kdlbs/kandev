package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// Reclamation is the only Kandev operation that deletes data on a machine
// Kandev does not own, so these tests are written the way the feature is: they
// assert what is NOT removed at least as hard as what is. Every one of them
// drives a fake reclaimer. None opens an SSH connection, and none names a path
// that exists on the machine running the suite.

const (
	testReclaimHost    = "build.example"
	testReclaimRoot    = "/home/dev/.kandev"
	testReclaimTaskDir = "/home/dev/.kandev/tasks/task-abc123"
)

type fakeSSHTaskDirReclaimer struct {
	mu       sync.Mutex
	requests []SSHTaskDirReclaimRequest
	respond  func(SSHTaskDirReclaimRequest) (SSHTaskDirReclaimResult, error)
}

func (f *fakeSSHTaskDirReclaimer) ReclaimTaskDir(
	_ context.Context,
	req SSHTaskDirReclaimRequest,
) (SSHTaskDirReclaimResult, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		return respond(req)
	}
	return SSHTaskDirReclaimResult{Removed: true}, nil
}

func (f *fakeSSHTaskDirReclaimer) calls() []SSHTaskDirReclaimRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SSHTaskDirReclaimRequest(nil), f.requests...)
}

func sshRunningMetadata(taskDir string, enabled bool) map[string]interface{} {
	md := map[string]interface{}{
		sshMetaHost:            testReclaimHost,
		sshMetaPort:            "2222",
		sshMetaUser:            "dev",
		sshMetaHostFingerprint: "SHA256:pinned",
		sshMetaWorkdirRoot:     testReclaimRoot,
		sshMetaRemoteTaskDir:   taskDir,
		sshMetaShell:           "zsh",
	}
	if enabled {
		md[sshMetaReclaimTaskDir] = sshReclaimEnabledValue
	}
	return md
}

func enabledReclaimTarget() sshReclaimTarget {
	return sshReclaimTarget{
		Host: testReclaimHost, Port: 2222, User: "dev",
		HostFingerprint: "SHA256:pinned", Shell: "zsh",
		WorkdirRoot: testReclaimRoot, TaskDir: testReclaimTaskDir, Enabled: true,
	}
}

// newReclaimJob stores a cleanup job carrying the given snapshot and returns it.
func newReclaimJob(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
	trigger models.TaskResourceCleanupTrigger,
	snapshot taskResourceCleanupSnapshot,
) *models.TaskResourceCleanupJob {
	t.Helper()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return newReclaimJobFromRawSnapshot(t, repo, taskID, trigger, string(encoded))
}

func newReclaimJobFromRawSnapshot(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
	trigger models.TaskResourceCleanupTrigger,
	snapshot string,
) *models.TaskResourceCleanupJob {
	t.Helper()
	job := &models.TaskResourceCleanupJob{
		ID:               "reclaim-job-" + taskID + "-" + string(trigger),
		OperationID:      string(trigger) + ":" + taskID,
		TaskID:           taskID,
		Trigger:          trigger,
		State:            models.TaskResourceCleanupStatePending,
		ResourceSnapshot: snapshot,
	}
	if err := repo.CreateTaskResourceCleanupJob(context.Background(), job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	return job
}

// newReclaimTask creates a task, its owned environment record, and wires a fake
// reclaimer onto the service. The environment is returned rather than stored so
// a test can hand the job an environment owned by somebody else.
func newReclaimTask(t *testing.T, taskSvc *Service, title string) (string, *models.TaskEnvironment) {
	t.Helper()
	result, err := taskSvc.CreateTask(context.Background(), &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: title, ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	env := &models.TaskEnvironment{
		ID:     "env-" + result.Task.ID,
		TaskID: result.Task.ID,
	}
	return result.Task.ID, env
}

func jobState(t *testing.T, repo *sqliterepo.Repository, jobID string) *models.TaskResourceCleanupJob {
	t.Helper()
	got, err := repo.GetTaskResourceCleanupJob(context.Background(), jobID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	return got
}

// TestSSHReclaimRemovesDirectoryOnTerminalOutcomes is the positive case: an
// opted-in profile whose task reaches a terminal archive or delete has its
// remote directory removed exactly once, even across sibling sessions.
func TestSSHReclaimRemovesDirectoryOnTerminalOutcomes(t *testing.T) {
	triggers := []models.TaskResourceCleanupTrigger{
		models.TaskResourceCleanupTriggerArchive,
		models.TaskResourceCleanupTriggerDelete,
		models.TaskResourceCleanupTriggerCascadeArchive,
		models.TaskResourceCleanupTriggerCascadeDelete,
	}
	for _, trigger := range triggers {
		t.Run(string(trigger), func(t *testing.T) {
			taskSvc, repo := setupOfficeTest(t)
			taskSvc.StopTaskResourceCleanupWorker()
			reclaimer := &fakeSSHTaskDirReclaimer{}
			taskSvc.SetSSHTaskDirReclaimer(reclaimer)

			taskID, env := newReclaimTask(t, taskSvc, "Reclaimed task")
			if trigger == models.TaskResourceCleanupTriggerArchive || trigger == models.TaskResourceCleanupTriggerCascadeArchive {
				if err := repo.ArchiveTask(context.Background(), taskID); err != nil {
					t.Fatalf("ArchiveTask: %v", err)
				}
			}
			job := newReclaimJob(t, repo, taskID, trigger, taskResourceCleanupSnapshot{
				TaskEnvironment: env,
				SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
			})

			if err := taskSvc.processTaskResourceCleanupJob(context.Background(), job.ID); err != nil {
				t.Fatalf("processTaskResourceCleanupJob: %v", err)
			}
			calls := reclaimer.calls()
			if len(calls) != 1 {
				t.Fatalf("reclaim attempts = %d, want 1 (%+v)", len(calls), calls)
			}
			if calls[0].TaskDir != testReclaimTaskDir || calls[0].WorkdirRoot != testReclaimRoot {
				t.Fatalf("reclaim request = %+v, want the recorded task dir under its root", calls[0])
			}
			if calls[0].HostFingerprint != "SHA256:pinned" {
				t.Fatalf("pinned fingerprint = %q, want it carried to the connection", calls[0].HostFingerprint)
			}
			if got := jobState(t, repo, job.ID); got.State != models.TaskResourceCleanupStateSucceeded {
				t.Fatalf("job state = %q, want succeeded", got.State)
			}
		})
	}
}

// TestSSHReclaimSnapshotDeDuplicatesSiblingSessions proves a multi-session task
// yields one target per remote directory and a second host yields a second.
func TestSSHReclaimSnapshotDeDuplicatesSiblingSessions(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	taskID, _ := newReclaimTask(t, taskSvc, "Multi session task")

	const otherHostDir = "/home/dev/.kandev/tasks/task-elsewhere"
	rows := []struct {
		sessionID string
		metadata  map[string]interface{}
	}{
		{"session-a", sshRunningMetadata(testReclaimTaskDir, true)},
		{"session-b", sshRunningMetadata(testReclaimTaskDir, true)},
		{"session-c", sshRunningMetadata(otherHostDir, true)},
		{"session-d", map[string]interface{}{}},
	}
	for _, row := range rows {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: row.sessionID, TaskID: taskID, State: models.TaskSessionStateCompleted,
		}); err != nil {
			t.Fatalf("CreateTaskSession: %v", err)
		}
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: row.sessionID, SessionID: row.sessionID, TaskID: taskID, ExecutorID: "executor-ssh",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
			Metadata: row.metadata,
		}); err != nil {
			t.Fatalf("UpsertExecutorRunning: %v", err)
		}
	}

	targets, err := taskSvc.gatherSSHReclaimTargets(ctx, taskID)
	if err != nil {
		t.Fatalf("gatherSSHReclaimTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2 (%+v)", len(targets), targets)
	}
	seen := map[string]bool{}
	for _, target := range targets {
		seen[target.TaskDir] = true
		if !target.Enabled {
			t.Fatalf("target %s not enabled; the opt-in did not survive the projection", target.TaskDir)
		}
		if target.Port != 2222 || target.User != "dev" || target.Shell != "zsh" {
			t.Fatalf("connection tuple lost in projection: %+v", target)
		}
	}
	if !seen[testReclaimTaskDir] || !seen[otherHostDir] {
		t.Fatalf("captured dirs = %v, want both recorded directories", seen)
	}
}

func TestSSHReclaimSnapshotPreservesOptOutAcrossDuplicateSessions(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	taskID, _ := newReclaimTask(t, taskSvc, "Mixed reclamation settings")

	for index, enabled := range []bool{true, false} {
		sessionID := fmt.Sprintf("mixed-session-%d", index)
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCompleted,
		}); err != nil {
			t.Fatalf("CreateTaskSession: %v", err)
		}
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: sessionID, SessionID: sessionID, TaskID: taskID, ExecutorID: "executor-ssh",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
			Metadata: sshRunningMetadata(testReclaimTaskDir, enabled),
		}); err != nil {
			t.Fatalf("UpsertExecutorRunning: %v", err)
		}
	}

	targets, err := taskSvc.gatherSSHReclaimTargets(ctx, taskID)
	if err != nil {
		t.Fatalf("gatherSSHReclaimTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1 (%+v)", len(targets), targets)
	}
	if targets[0].Enabled {
		t.Fatal("duplicate target remained enabled after one session opted out")
	}
}

func TestSSHReclaimSnapshotMatchesHostNamesWithoutCase(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	taskID, _ := newReclaimTask(t, taskSvc, "Host spelling aliases")

	for index, host := range []string{"BUILD.EXAMPLE", "build.example"} {
		sessionID := fmt.Sprintf("host-session-%d", index)
		metadata := sshRunningMetadata(testReclaimTaskDir, true)
		metadata[sshMetaHost] = host
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCompleted,
		}); err != nil {
			t.Fatalf("CreateTaskSession: %v", err)
		}
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: sessionID, SessionID: sessionID, TaskID: taskID, ExecutorID: "executor-ssh",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
			Metadata: metadata,
		}); err != nil {
			t.Fatalf("UpsertExecutorRunning: %v", err)
		}
	}

	targets, err := taskSvc.gatherSSHReclaimTargets(ctx, taskID)
	if err != nil {
		t.Fatalf("gatherSSHReclaimTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want one host identity (%+v)", len(targets), targets)
	}
}

// TestSSHReclaimSkipsWithoutOptIn covers the default: a profile that never
// enabled reclamation keeps the pre-existing keep-forever behavior, and the
// job still succeeds because preserving the directory is a correct outcome.
func TestSSHReclaimSkipsWithoutOptIn(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)

	taskID, env := newReclaimTask(t, taskSvc, "Opt-out task")
	target := enabledReclaimTarget()
	target.Enabled = false
	job := newReclaimJob(t, repo, taskID, models.TaskResourceCleanupTriggerDelete,
		taskResourceCleanupSnapshot{TaskEnvironment: env, SSHTaskDirs: []sshReclaimTarget{target}})

	if err := taskSvc.processTaskResourceCleanupJob(context.Background(), job.ID); err != nil {
		t.Fatalf("processTaskResourceCleanupJob: %v", err)
	}
	if calls := reclaimer.calls(); len(calls) != 0 {
		t.Fatalf("reclaim attempts = %d, want 0 without the profile opt-in", len(calls))
	}
	if got := jobState(t, repo, job.ID); got.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("job state = %q, want succeeded", got.State)
	}
}

// TestSSHReclaimPreservesInheritedParentWorkspace is the regression that
// matters most: an inherit_parent subtask binds its session to the parent's
// environment, so it owns no environment of its own. Deleting the subtask must
// leave the parent's live workspace untouched.
func TestSSHReclaimPreservesInheritedParentWorkspace(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)

	parentID, parentEnv := newReclaimTask(t, taskSvc, "Parent task")
	subtaskID, _ := newReclaimTask(t, taskSvc, "Inherited subtask")
	_ = parentID

	// The subtask's snapshot carries the PARENT's environment, exactly as
	// propagateInheritedEnvironment binds it at launch.
	job := newReclaimJob(t, repo, subtaskID, models.TaskResourceCleanupTriggerDelete,
		taskResourceCleanupSnapshot{
			TaskEnvironment: parentEnv,
			SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
		})

	if err := taskSvc.processTaskResourceCleanupJob(context.Background(), job.ID); err != nil {
		t.Fatalf("processTaskResourceCleanupJob: %v", err)
	}
	if calls := reclaimer.calls(); len(calls) != 0 {
		t.Fatalf("reclaim attempts = %d, want 0 for a borrowed parent workspace", len(calls))
	}
	if got := jobState(t, repo, job.ID); got.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("job state = %q, want succeeded", got.State)
	}
}

// TestSSHReclaimPreservesEnvironmentWithLiveBorrower covers the mirror case: the
// cleaned task does own the environment row, but another task still has an
// active session bound to it.
func TestSSHReclaimPreservesEnvironmentWithLiveBorrower(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)
	ctx := context.Background()

	ownerID, env := newReclaimTask(t, taskSvc, "Owner task")
	borrowerID, _ := newReclaimTask(t, taskSvc, "Borrower task")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-borrower", TaskID: borrowerID, TaskEnvironmentID: env.ID,
		State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	job := newReclaimJob(t, repo, ownerID, models.TaskResourceCleanupTriggerDelete,
		taskResourceCleanupSnapshot{
			TaskEnvironment: env,
			SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
		})

	if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatalf("processTaskResourceCleanupJob: %v", err)
	}
	if calls := reclaimer.calls(); len(calls) != 0 {
		t.Fatalf("reclaim attempts = %d, want 0 while another task holds the environment", len(calls))
	}
}

// TestSSHReclaimPreservesDirectoryClaimedByAnotherTask covers the cross-task
// check the environment row cannot see: a live executor row belonging to some
// other task points at the same remote directory.
func TestSSHReclaimPreservesDirectoryClaimedByAnotherTask(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)
	ctx := context.Background()

	taskID, env := newReclaimTask(t, taskSvc, "Cleaned task")
	otherID, _ := newReclaimTask(t, taskSvc, "Other task")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-other", TaskID: otherID, State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-other", SessionID: "session-other", TaskID: otherID, ExecutorID: "executor-ssh",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
		Metadata: sshRunningMetadata(testReclaimTaskDir, true),
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	job := newReclaimJob(t, repo, taskID, models.TaskResourceCleanupTriggerDelete,
		taskResourceCleanupSnapshot{
			TaskEnvironment: env,
			SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
		})

	if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatalf("processTaskResourceCleanupJob: %v", err)
	}
	if calls := reclaimer.calls(); len(calls) != 0 {
		t.Fatalf("reclaim attempts = %d, want 0 while another task claims the directory", len(calls))
	}
}

// TestSSHReclaimSafetySkipCompletesJob proves a probe verdict that preserves the
// directory is a success, not a permanently retrying job. A user who leaves
// unpushed work behind should not see a failed cleanup forever.
func TestSSHReclaimSafetySkipCompletesJob(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{
		respond: func(SSHTaskDirReclaimRequest) (SSHTaskDirReclaimResult, error) {
			return SSHTaskDirReclaimResult{SkipReason: "unpushed_commits", Detail: "2 commits"}, nil
		},
	}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)

	taskID, env := newReclaimTask(t, taskSvc, "Unpushed work task")
	job := newReclaimJob(t, repo, taskID, models.TaskResourceCleanupTriggerDelete,
		taskResourceCleanupSnapshot{
			TaskEnvironment: env,
			SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
		})

	if err := taskSvc.processTaskResourceCleanupJob(context.Background(), job.ID); err != nil {
		t.Fatalf("processTaskResourceCleanupJob: %v", err)
	}
	if len(reclaimer.calls()) != 1 {
		t.Fatalf("reclaim attempts = %d, want 1", len(reclaimer.calls()))
	}
	got := jobState(t, repo, job.ID)
	if got.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("job state = %q, want succeeded for a safety skip", got.State)
	}
	if got.LastError != "" {
		t.Fatalf("last_error = %q, want empty for a safety skip", got.LastError)
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(got.ResourceSnapshot), &snapshot); err != nil {
		t.Fatalf("decode persisted snapshot: %v", err)
	}
	if len(snapshot.SSHTaskDirs) != 1 {
		t.Fatalf("persisted targets = %d, want 1", len(snapshot.SSHTaskDirs))
	}
	if target := snapshot.SSHTaskDirs[0]; target.Outcome != sshReclaimOutcomeSkipped ||
		target.SkipReason != "unpushed_commits" || target.Detail != "2 commits" {
		t.Fatalf("persisted reclaim result = %+v, want skipped unpushed_commits detail", target)
	}
}

// TestSSHReclaimFailureIsARealError is the anti-D6 test: a removal that did not
// happen must never leave the job reporting success.
func TestSSHReclaimFailureIsARealError(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{
		respond: func(SSHTaskDirReclaimRequest) (SSHTaskDirReclaimResult, error) {
			return SSHTaskDirReclaimResult{}, errors.New("remote directory still present after rm")
		},
	}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)

	taskID, env := newReclaimTask(t, taskSvc, "Failing reclaim task")
	job := newReclaimJob(t, repo, taskID, models.TaskResourceCleanupTriggerDelete,
		taskResourceCleanupSnapshot{
			TaskEnvironment: env,
			SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
		})

	err := taskSvc.processTaskResourceCleanupJob(context.Background(), job.ID)
	if err == nil {
		t.Fatal("processTaskResourceCleanupJob returned nil; a failed removal must propagate")
	}
	got := jobState(t, repo, job.ID)
	if got.State == models.TaskResourceCleanupStateSucceeded {
		t.Fatal("job state = succeeded after a failed removal")
	}
	if got.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("job state = %q, want retry_wait", got.State)
	}
	if !strings.Contains(got.LastError, "still present after rm") {
		t.Fatalf("last_error = %q, want the remote failure recorded", got.LastError)
	}
	if got.NextAttemptAt == nil {
		t.Fatal("next_attempt_at is nil; the failure did not enter the retry schedule")
	}
}

// TestSSHReclaimRetryReRunsTheProbe proves no verdict is cached across
// attempts: each attempt re-decodes the snapshot and asks the remote again.
func TestSSHReclaimRetryReRunsTheProbe(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	attempts := 0
	reclaimer := &fakeSSHTaskDirReclaimer{
		respond: func(SSHTaskDirReclaimRequest) (SSHTaskDirReclaimResult, error) {
			attempts++
			if attempts == 1 {
				return SSHTaskDirReclaimResult{}, errors.New("host unreachable")
			}
			return SSHTaskDirReclaimResult{Removed: true}, nil
		},
	}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)

	taskID, env := newReclaimTask(t, taskSvc, "Retrying task")
	snapshot := taskResourceCleanupSnapshot{
		TaskEnvironment: env,
		SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
	}
	job := newReclaimJob(t, repo, taskID, models.TaskResourceCleanupTriggerDelete, snapshot)

	if err := taskSvc.executeTaskResourceCleanupJob(context.Background(), job, &snapshot); err == nil {
		t.Fatal("first attempt returned nil, want the transport failure")
	}
	if err := taskSvc.executeTaskResourceCleanupJob(context.Background(), job, &snapshot); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("reclaim attempts = %d, want 2; a cached verdict would skip the retry", attempts)
	}
}

// TestSSHReclaimIgnoresSnapshotWrittenBeforeTheFeature proves the snapshot
// field is additive: a job row from an older backend decodes and reclaims
// nothing rather than failing to decode.
func TestSSHReclaimIgnoresSnapshotWrittenBeforeTheFeature(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)

	taskID, _ := newReclaimTask(t, taskSvc, "Legacy snapshot task")
	job := newReclaimJobFromRawSnapshot(t, repo, taskID, models.TaskResourceCleanupTriggerDelete,
		`{"delete_environment_row":true}`)

	if err := taskSvc.processTaskResourceCleanupJob(context.Background(), job.ID); err != nil {
		t.Fatalf("processTaskResourceCleanupJob: %v", err)
	}
	if calls := reclaimer.calls(); len(calls) != 0 {
		t.Fatalf("reclaim attempts = %d, want 0 for a pre-feature snapshot", len(calls))
	}
	if got := jobState(t, repo, job.ID); got.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("job state = %q, want succeeded", got.State)
	}
}

// TestSSHReclaimNeverStartsOnACancelledContext covers backend shutdown, which
// cancels the cleanup worker mid-job. Reclamation is irreversible, so it must
// not begin while the process is going away; the durable job survives and the
// next attempt runs it with a live context.
func TestSSHReclaimNeverStartsOnACancelledContext(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)

	taskID, env := newReclaimTask(t, taskSvc, "Shutdown task")
	snapshot := taskResourceCleanupSnapshot{
		TaskEnvironment: env,
		SSHTaskDirs:     []sshReclaimTarget{enabledReclaimTarget()},
	}
	job := newReclaimJob(t, repo, taskID, models.TaskResourceCleanupTriggerDelete, snapshot)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = taskSvc.executeTaskResourceCleanupJob(ctx, job, &snapshot)

	if calls := reclaimer.calls(); len(calls) != 0 {
		t.Fatalf("reclaim attempts = %d, want 0 during shutdown", len(calls))
	}
}

// TestSSHReclaimIsUnreachableWithoutACleanupJob pins the structural reason an
// ordinary stop preserves the workspace: reclamation is a phase of the durable
// cleanup job, and a cleanup job only ever exists for a terminal trigger. An
// ordinary stop creates no job, so there is no path from it to a removal.
func TestSSHReclaimIsUnreachableWithoutACleanupJob(t *testing.T) {
	terminal := map[models.TaskResourceCleanupTrigger]bool{
		models.TaskResourceCleanupTriggerArchive:         true,
		models.TaskResourceCleanupTriggerCascadeArchive:  true,
		models.TaskResourceCleanupTriggerDelete:          true,
		models.TaskResourceCleanupTriggerCascadeDelete:   true,
		models.TaskResourceCleanupTriggerWorkspaceDelete: true,
		models.TaskResourceCleanupTriggerQuickChatExpire: true,
	}
	for trigger := range terminal {
		if _, ok := terminal[trigger]; !ok {
			t.Fatalf("trigger %q is not a terminal outcome", trigger)
		}
	}

	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	reclaimer := &fakeSSHTaskDirReclaimer{}
	taskSvc.SetSSHTaskDirReclaimer(reclaimer)
	ctx := context.Background()

	taskID, _ := newReclaimTask(t, taskSvc, "Merely stopped task")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-stopped", TaskID: taskID, State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-stopped", SessionID: "session-stopped", TaskID: taskID, ExecutorID: "executor-ssh",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
		Metadata: sshRunningMetadata(testReclaimTaskDir, true),
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	// Draining every due job the way the worker does finds nothing to run: the
	// task was stopped, not archived or deleted.
	if err := taskSvc.processDueTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("processDueTaskResourceCleanupJobs: %v", err)
	}
	if calls := reclaimer.calls(); len(calls) != 0 {
		t.Fatalf("reclaim attempts = %d, want 0 after an ordinary stop", len(calls))
	}
}
