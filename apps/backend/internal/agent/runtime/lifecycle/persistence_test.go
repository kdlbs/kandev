package lifecycle

import (
	"context"
	"errors"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type captureExecutorRunningWriter struct {
	running *models.ExecutorRunning
}

func (w *captureExecutorRunningWriter) UpsertExecutorRunning(_ context.Context, running *models.ExecutorRunning) error {
	w.running = running
	return nil
}

func (w *captureExecutorRunningWriter) DeleteExecutorRunningBySessionID(_ context.Context, _ string) error {
	return nil
}

func (w *captureExecutorRunningWriter) RepairExecutorRunningDead(_ context.Context, _ string) error {
	return nil
}

func TestBuildRunningFromExecutionPersistsLiveAgentctlEndpoint(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)

	running := buildRunningFromExecution(&AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		standalonePort: 45678,
	}, nil)

	if running.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("Status = %q, want running", running.Status)
	}
	if running.AgentctlURL != "http://127.0.0.1:45678" {
		t.Fatalf("AgentctlURL = %q, want live client URL", running.AgentctlURL)
	}
	if running.AgentctlPort != 45678 {
		t.Fatalf("AgentctlPort = %d, want 45678", running.AgentctlPort)
	}
	if running.LastSeenAt == nil {
		t.Fatal("LastSeenAt = nil, want live endpoint observation timestamp")
	}
}

func TestBuildRunningFromExecutionBindsResumeTokenToExecutionProfile(t *testing.T) {
	prior := &models.ExecutorRunning{
		ExecutionProfileID: "codex-profile",
		ResumeToken:        "codex-session",
		LastMessageUUID:    "last-codex-message",
	}
	running := buildRunningFromExecution(&AgentExecution{
		ID:             "exec-2",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "claude-profile",
	}, prior)

	if running.ExecutionProfileID != "claude-profile" {
		t.Fatalf("ExecutionProfileID = %q, want claude-profile", running.ExecutionProfileID)
	}
	if running.ResumeToken != "" || running.LastMessageUUID != "" {
		t.Fatalf("cross-profile resume state leaked: %+v", running)
	}
}

func TestBuildRunningFromExecutionPersistsSSHRuntimePID(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 43001, log)

	running := buildRunningFromExecution(&AgentExecution{
		ID:          "exec-ssh",
		TaskID:      "task-1",
		SessionID:   "session-1",
		RuntimeName: agentruntime.RuntimeSSH,
		Status:      v1.AgentStatusRunning,
		agentctl:    client,
		Metadata: map[string]interface{}{
			MetadataKeySSHLocalForwardPort:   "43001",
			MetadataKeySSHRemoteAgentctlPID:  "9321",
			MetadataKeySSHRemoteAgentctlPort: "43000",
		},
	}, nil)

	if running.AgentctlURL != "http://127.0.0.1:43001" {
		t.Fatalf("AgentctlURL = %q, want local forward URL", running.AgentctlURL)
	}
	if running.AgentctlPort != 43001 {
		t.Fatalf("AgentctlPort = %d, want local forward port", running.AgentctlPort)
	}
	if running.PID != 9321 {
		t.Fatalf("PID = %d, want remote agentctl pid", running.PID)
	}
	if running.LastSeenAt == nil {
		t.Fatal("LastSeenAt = nil, want live endpoint observation timestamp")
	}
}

func TestMarkReadyPersistsReadyExecutorRunningStatus(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(writer)

	if err := mgr.executionStore.Add(&AgentExecution{
		ID:             "exec-ready",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		standalonePort: 45678,
	}); err != nil {
		t.Fatalf("Add execution: %v", err)
	}

	if err := mgr.MarkReady("exec-ready"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	if writer.running == nil {
		t.Fatal("expected MarkReady to persist executors_running row")
	}
	if writer.running.Status != models.ExecutorRunningStatusReady {
		t.Fatalf("persisted status = %q, want ready", writer.running.Status)
	}
	if writer.running.AgentctlPort != 45678 {
		t.Fatalf("persisted port = %d, want 45678", writer.running.AgentctlPort)
	}
}

// TestLocalStandaloneRowCarriesLocalPID is the characterization test for the
// #1597 symptom "every local row reports pid=0 with no local liveness handle".
// A standalone execution persisted through the manager must carry the host-local
// agentctl PID in executors_running.local_pid, an observed last_seen_at, and a
// live endpoint — so a dead row is distinguishable from a live one.
func TestLocalStandaloneRowCarriesLocalPID(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(writer)
	mgr.SetStandaloneHostPID(918273)

	if err := mgr.executionStore.Add(&AgentExecution{
		ID:             "exec-local",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		standalonePort: 45678,
	}); err != nil {
		t.Fatalf("Add execution: %v", err)
	}

	if err := mgr.MarkBootReady("exec-local"); err != nil {
		t.Fatalf("MarkBootReady: %v", err)
	}

	if writer.running == nil {
		t.Fatal("expected boot-ready to persist executors_running row")
	}
	if writer.running.Status != models.ExecutorRunningStatusReady {
		t.Fatalf("persisted status = %q, want ready (not stuck starting/prepared)", writer.running.Status)
	}
	if writer.running.LocalPID != 918273 {
		t.Fatalf("persisted local_pid = %d, want the host agentctl pid 918273", writer.running.LocalPID)
	}
	if writer.running.AgentctlPort != 45678 {
		t.Fatalf("persisted agentctl_port = %d, want 45678", writer.running.AgentctlPort)
	}
	if writer.running.LastSeenAt == nil {
		t.Fatal("persisted last_seen_at = nil, want an actual liveness observation")
	}
}

// TestSSHRowNeverCarriesLocalPID guards the #1597 pid-semantics constraint: an SSH row's process
// lives on a remote host, so the local liveness handle must stay 0 for it — the
// remote pid keeps living in the PID column, never local_pid.
func TestSSHRowNeverCarriesLocalPID(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 43001, log)
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(writer)
	mgr.SetStandaloneHostPID(918273) // a local pid exists, but must not leak onto an SSH row

	if err := mgr.executionStore.Add(&AgentExecution{
		ID:          "exec-ssh",
		TaskID:      "task-1",
		SessionID:   "session-1",
		RuntimeName: agentruntime.RuntimeSSH,
		Status:      v1.AgentStatusRunning,
		agentctl:    client,
		Metadata: map[string]interface{}{
			MetadataKeySSHLocalForwardPort:  "43001",
			MetadataKeySSHRemoteAgentctlPID: "9321",
		},
	}); err != nil {
		t.Fatalf("Add execution: %v", err)
	}

	if err := mgr.MarkReady("exec-ssh"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	if writer.running == nil {
		t.Fatal("expected MarkReady to persist executors_running row")
	}
	if writer.running.LocalPID != 0 {
		t.Fatalf("ssh row local_pid = %d, want 0 (remote process, not local)", writer.running.LocalPID)
	}
	if writer.running.PID != 9321 {
		t.Fatalf("ssh row pid = %d, want remote agentctl pid 9321", writer.running.PID)
	}
}

// TestMarkCompletedPersistsTerminalRow is the characterization test for the
// core population gap: MarkCompleted (the process-exit/crash boundary) used to
// skip persistence, leaving the row claiming a `running` process after it exited.
// The terminal status must land in executors_running.
func TestMarkCompletedPersistsTerminalRow(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(writer)
	mgr.SetStandaloneHostPID(918273)

	if err := mgr.executionStore.Add(&AgentExecution{
		ID:             "exec-done",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		standalonePort: 45678,
	}); err != nil {
		t.Fatalf("Add execution: %v", err)
	}

	if err := mgr.MarkCompleted("exec-done", 0, ""); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	if writer.running == nil {
		t.Fatal("expected MarkCompleted to persist executors_running row (it historically did not)")
	}
	if writer.running.Status != models.ExecutorRunningStatusComplete {
		t.Fatalf("persisted status = %q, want completed (row must not keep claiming a running process)", writer.running.Status)
	}
	if writer.running.LastSeenAt == nil {
		t.Fatal("persisted last_seen_at = nil, want a fresh observation at the exit boundary")
	}
	// A terminal row must not carry a live local liveness handle: the resolved
	// standalone PID is the shared agentctl control server, which outlives this
	// session, and a completed row claiming a live process would read Alive from
	// RowProcessLiveness — the exact lie #1597 truthful executor rows removes.
	// Matches RepairExecutorRunningDead, which clears local_pid on repair.
	if writer.running.LocalPID != 0 {
		t.Fatalf("terminal row local_pid = %d, want 0 (must not claim the live shared agentctl process)", writer.running.LocalPID)
	}
}

// readErrExecutorRunningWriter implements executorRunningReader and returns a
// transient (non-NotFound) error from the prior-row read, recording whether
// UpsertExecutorRunning was subsequently called.
type readErrExecutorRunningWriter struct {
	readErr    error
	upserted   bool
	lastUpsert *models.ExecutorRunning
}

func (w *readErrExecutorRunningWriter) GetExecutorRunningBySessionID(_ context.Context, _ string) (*models.ExecutorRunning, error) {
	return nil, w.readErr
}

func (w *readErrExecutorRunningWriter) UpsertExecutorRunning(_ context.Context, running *models.ExecutorRunning) error {
	w.upserted = true
	w.lastUpsert = running
	return nil
}

func (w *readErrExecutorRunningWriter) DeleteExecutorRunningBySessionID(_ context.Context, _ string) error {
	return nil
}

func (w *readErrExecutorRunningWriter) RepairExecutorRunningDead(_ context.Context, _ string) error {
	return nil
}

// TestPersistSkipsUpsertWhenPriorReadFails guards #1597 resume-safety invariant:
// a transient prior-row read failure must NOT proceed to a blind upsert, because
// that would blank a live resume_token (the upsert overwrites it with the empty
// excluded value). MarkCompleted newly persists at the exit boundary, where a
// completed session may still hold a resume_token — so the fail-safe matters most
// there. A "not found" read, by contrast, is a real first-insert and must proceed.
func TestPersistSkipsUpsertWhenPriorReadFails(t *testing.T) {
	mgr := newTestManager(t)

	transient := &readErrExecutorRunningWriter{readErr: errors.New("database is locked")}
	mgr.SetExecutorRunningWriter(transient)
	mgr.persistExecutorRunning(context.Background(), &AgentExecution{
		ID:          "exec-1",
		TaskID:      "task-1",
		SessionID:   "session-1",
		RuntimeName: agentruntime.RuntimeStandalone,
		Status:      v1.AgentStatusCompleted,
	})
	if transient.upserted {
		t.Fatal("prior-read failure must NOT clobber the row with a blind upsert")
	}

	// A positive "not found" is a first insert and must still write.
	notFound := &readErrExecutorRunningWriter{readErr: models.ErrExecutorRunningNotFound}
	mgr.SetExecutorRunningWriter(notFound)
	mgr.persistExecutorRunning(context.Background(), &AgentExecution{
		ID:          "exec-2",
		TaskID:      "task-1",
		SessionID:   "session-2",
		RuntimeName: agentruntime.RuntimeStandalone,
		Status:      v1.AgentStatusReady,
	})
	if !notFound.upserted {
		t.Fatal("a not-found prior read is a first insert and must proceed with the upsert")
	}
}

func TestUpdateStatusPersistsRunningExecutorRunningStatus(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(writer)

	if err := mgr.executionStore.Add(&AgentExecution{
		ID:             "exec-running",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusReady,
		agentctl:       client,
		standalonePort: 45678,
	}); err != nil {
		t.Fatalf("Add execution: %v", err)
	}

	if err := mgr.UpdateStatus("exec-running", v1.AgentStatusRunning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if writer.running == nil {
		t.Fatal("expected UpdateStatus to persist executors_running row")
	}
	if writer.running.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("persisted status = %q, want running", writer.running.Status)
	}
}
