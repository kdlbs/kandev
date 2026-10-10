package lifecycle

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// sshOrphanRecheckStore is a minimal sshOrphanSweepStore fake whose
// ListTaskSessions answer changes after the first call, simulating a resume
// that flips a session out of its terminal state between the sweep's cached
// decision and the fresh, uncached recheck immediately before the stop.
type sshOrphanRecheckStore struct {
	task          *models.Task
	sessionID     string
	sessionsCalls int32
}

func (s *sshOrphanRecheckStore) GetTask(context.Context, string) (*models.Task, error) {
	return s.task, nil
}

func (s *sshOrphanRecheckStore) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	n := atomic.AddInt32(&s.sessionsCalls, 1)
	state := models.TaskSessionStateCompleted
	if n > 1 {
		state = models.TaskSessionStateRunning
	}
	return []*models.TaskSession{{ID: s.sessionID, State: state}}, nil
}

func (s *sshOrphanRecheckStore) ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error) {
	return nil, nil
}

func (s *sshOrphanRecheckStore) ListExecutorProfiles(context.Context, string) ([]*models.ExecutorProfile, error) {
	return nil, nil
}

// TestSweepSSHExecutorOrphansUnderRootReChecksBeforeStop proves Review Round
// 2 (R2-F2 part 1): a process whose cached decision (taken once per sweep)
// says Stop must be re-decided against a fresh, uncached task context
// immediately before the stop command is sent. Here the session backing the
// cached decision is terminal on the first read (COMPLETED, driving a Stop
// verdict) but has resumed to RUNNING by the second, fresh read — modeling a
// resume racing in during the sweep's sequential stop loop. The process must
// end up preserved, and no stop command may reach the remote host: the fake
// server's scripted handler has no rule for a stop command and fails the
// test on any unmatched command, so an incorrectly-sent stop would fail this
// test on its own.
//
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.14
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.15
func TestSweepSSHExecutorOrphansUnderRootReChecksBeforeStop(t *testing.T) {
	handler := newSSHScriptedHandler(t,
		sshScriptRule{
			match: "ps -eo pid=,ppid=,command=",
			result: sshOut(
				"PROC\t4242\t1\t/opt/kandev/agentctl --workdir /root/tasks/task-1\n" +
					"PIDFILE\ttask-1\tsess-1\t4242\n",
			),
		},
	)
	server := newFakeSSHServer(t, handler.handle)
	client := server.dial(t)

	store := &sshOrphanRecheckStore{
		task:      &models.Task{ID: "1"},
		sessionID: "sess-1",
	}

	report := &sshOrphanSweepReport{ExecutorID: "executor-1"}
	err := sweepSSHExecutorOrphansUnderRoot(
		context.Background(), client, store, "executor-1", "/root",
		map[string]*sshOrphanTaskContext{}, report, logger.Default(), nil,
	)
	if err != nil {
		t.Fatalf("sweepSSHExecutorOrphansUnderRoot: %v", err)
	}

	if report.Found != 1 || report.Stopped != 0 || report.Preserved != 1 {
		t.Fatalf("report = %+v, want found=1 stopped=0 preserved=1 — a fresh non-terminal recheck must preserve", report)
	}
	if _, ok := server.lastCommandContaining("TARGET_PID=4242"); ok {
		t.Fatalf("a stop command was sent despite the fresh pre-stop recheck finding a non-terminal session")
	}
	if calls := atomic.LoadInt32(&store.sessionsCalls); calls < 2 {
		t.Fatalf("ListTaskSessions was called %d times, want at least 2 (cached decision + fresh recheck)", calls)
	}
}

type sshOrphanSweepFenceStore struct {
	finalRead chan struct{}
	reads     atomic.Int32
}

func (*sshOrphanSweepFenceStore) GetTask(context.Context, string) (*models.Task, error) {
	return &models.Task{ID: "1"}, nil
}

func (*sshOrphanSweepFenceStore) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return []*models.TaskSession{{ID: "sess-1", State: models.TaskSessionStateCompleted}}, nil
}

func (s *sshOrphanSweepFenceStore) ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error) {
	if s.reads.Add(1) == 2 {
		close(s.finalRead)
	}
	return nil, nil
}

func (*sshOrphanSweepFenceStore) ListExecutorProfiles(context.Context, string) ([]*models.ExecutorProfile, error) {
	return nil, nil
}

// TestSweepFencePreventsResumeBetweenFinalReadAndRemoteStop proves that a
// runtime resume cannot reuse an inventoried PID after the fresh database
// read and before the stop signal. The resume starts as soon as the final
// executor-row query begins, then remains blocked until the remote stop ends.
func TestSweepFencePreventsResumeBetweenFinalReadAndRemoteStop(t *testing.T) {
	finalRead := make(chan struct{})
	stopStarted := make(chan struct{})
	allowStop := make(chan struct{})
	var allowStopOnce sync.Once
	releaseRemoteStop := func() { allowStopOnce.Do(func() { close(allowStop) }) }
	defer releaseRemoteStop()
	sweepDone := make(chan error, 1)
	resumeStarted := make(chan struct{})
	creationDone := make(chan error, 1)
	log := newTestLogger()
	backend := &resumeTrackingExecutor{
		MockExecutor:  MockExecutor{name: executor.NameStandalone},
		client:        newReadyAgentctlClient(t, log),
		resumeStarted: resumeStarted,
	}
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)

	handler := func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "ps -eo pid=,ppid=,command="):
			return sshOut("PROC\t4242\t1\t/opt/kandev/agentctl --workdir /root/tasks/task-1\nPIDFILE\ttask-1\tsess-1\t4242\n")
		case strings.Contains(command, "TARGET_PID=4242"):
			close(stopStarted)
			<-allowStop
			return sshOK
		default:
			t.Errorf("unexpected remote command %q", command)
			return sshFail("unexpected command")
		}
	}
	server := newFakeSSHServer(t, handler)
	client := server.dial(t)
	store := &sshOrphanSweepFenceStore{finalRead: finalRead}
	go func() {
		report := &sshOrphanSweepReport{ExecutorID: "executor-1"}
		sweepDone <- sweepSSHExecutorOrphansUnderRoot(
			context.Background(), client, store, "executor-1", "/root",
			map[string]*sshOrphanTaskContext{}, report, logger.Default(), mgr.AcquireSSHOrphanSweepFence,
		)
	}()

	select {
	case <-finalRead:
	case <-time.After(2 * time.Second):
		t.Fatal("sweep did not reach its final executor-row read")
	}
	go func() {
		_, err := mgr.createExecution(context.Background(), "1", &WorkspaceInfo{
			TaskID: "1", SessionID: "sess-1", AgentID: "auggie", WorkspacePath: "/workspace/task-1",
		})
		creationDone <- err
	}()

	select {
	case <-resumeStarted:
		t.Fatal("runtime resume entered between the final database read and remote stop")
	case <-stopStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("sweep did not send the remote stop command")
	}
	select {
	case <-resumeStarted:
		t.Fatal("runtime resume entered while the remote stop was in progress")
	case <-time.After(50 * time.Millisecond):
	}

	releaseRemoteStop()
	select {
	case err := <-sweepDone:
		if err != nil {
			t.Fatalf("sweepSSHExecutorOrphansUnderRoot returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sweep did not finish after the remote stop was released")
	}
	select {
	case <-resumeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime resume did not continue after the sweep released the task fence")
	}
	if err := <-creationDone; err != nil {
		t.Fatalf("createExecution returned error: %v", err)
	}
}
