package lifecycle

import (
	"context"
	"sync/atomic"
	"testing"

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
			match: `ROOT='/root/tasks'`,
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
		map[string]*sshOrphanTaskContext{}, report, logger.Default(),
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
