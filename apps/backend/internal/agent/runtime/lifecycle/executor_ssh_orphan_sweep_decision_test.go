package lifecycle

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.14, AC-EXECUTORS-SSH-EXECUTOR-001.15
func TestDecideSSHOrphanProcess(t *testing.T) {
	archivedAt := time.Now()

	session := func(id string, state models.TaskSessionState) *models.TaskSession {
		return &models.TaskSession{ID: id, State: state}
	}
	runningRow := func(pid int, sessionID, status string) *models.ExecutorRunning {
		return &models.ExecutorRunning{PID: pid, SessionID: sessionID, Status: status}
	}

	cases := []struct {
		name          string
		pid           int
		claimedID     string
		claimed       bool
		tainted       bool
		taskCtx       *sshOrphanTaskContext
		wantVerdict   sshOrphanVerdict
		wantSessionID string
	}{
		{
			name:        "tainted task preserves regardless of everything else",
			pid:         100,
			tainted:     true,
			taskCtx:     &sshOrphanTaskContext{Task: &models.Task{ID: "t1", ArchivedAt: &archivedAt}},
			wantVerdict: sshOrphanPreserve,
		},
		{
			name:        "unknown task preserves",
			pid:         100,
			taskCtx:     &sshOrphanTaskContext{Task: nil},
			wantVerdict: sshOrphanPreserve,
		},
		{
			name: "live executors_running row for a non-terminal session blocks stop even for an archived task",
			pid:  100,
			taskCtx: &sshOrphanTaskContext{
				Task:     &models.Task{ID: "t1", ArchivedAt: &archivedAt},
				Sessions: []*models.TaskSession{session("s1", models.TaskSessionStateRunning)},
				Running:  []*models.ExecutorRunning{runningRow(100, "s1", models.ExecutorRunningStatusRunning)},
			},
			wantVerdict: sshOrphanPreserve,
		},
		{
			name: "active workspace execution blocks stop when its conversation session is terminal",
			pid:  100,
			taskCtx: &sshOrphanTaskContext{
				Task:     &models.Task{ID: "t1", ArchivedAt: &archivedAt},
				Sessions: []*models.TaskSession{session("s1", models.TaskSessionStateCompleted)},
				Running:  []*models.ExecutorRunning{runningRow(100, "s1", models.ExecutorRunningStatusReady)},
			},
			wantVerdict: sshOrphanPreserve,
		},
		{
			name: "tracked terminal-session runtime still blocks stop",
			pid:  100,
			taskCtx: &sshOrphanTaskContext{
				Task:     &models.Task{ID: "t1", ArchivedAt: &archivedAt},
				Sessions: []*models.TaskSession{session("s1", models.TaskSessionStateCompleted)},
				Running:  []*models.ExecutorRunning{runningRow(100, "s1", models.ExecutorRunningStatusComplete)},
			},
			wantVerdict: sshOrphanPreserve,
		},
		{
			name: "executors_running row referencing an unknown session blocks stop",
			pid:  100,
			taskCtx: &sshOrphanTaskContext{
				Task:    &models.Task{ID: "t1", ArchivedAt: &archivedAt},
				Running: []*models.ExecutorRunning{runningRow(100, "missing-session", models.ExecutorRunningStatusStarting)},
			},
			wantVerdict: sshOrphanPreserve,
		},
		{
			name:      "claimed process whose session is terminal stops",
			pid:       200,
			claimedID: "s1",
			claimed:   true,
			taskCtx: &sshOrphanTaskContext{
				Task:     &models.Task{ID: "t1"},
				Sessions: []*models.TaskSession{session("s1", models.TaskSessionStateFailed)},
			},
			wantVerdict:   sshOrphanStop,
			wantSessionID: "s1",
		},
		{
			name:      "claimed process whose session is non-terminal preserves",
			pid:       200,
			claimedID: "s1",
			claimed:   true,
			taskCtx: &sshOrphanTaskContext{
				Task:     &models.Task{ID: "t1"},
				Sessions: []*models.TaskSession{session("s1", models.TaskSessionStateRunning)},
			},
			wantVerdict:   sshOrphanPreserve,
			wantSessionID: "s1",
		},
		{
			name:      "claimed process whose session the task does not have preserves (unproven ownership)",
			pid:       200,
			claimedID: "ghost-session",
			claimed:   true,
			taskCtx: &sshOrphanTaskContext{
				Task:     &models.Task{ID: "t1"},
				Sessions: []*models.TaskSession{session("s1", models.TaskSessionStateCompleted)},
			},
			wantVerdict:   sshOrphanPreserve,
			wantSessionID: "ghost-session",
		},
		{
			name: "unclaimed process on an archived task stops",
			pid:  300,
			taskCtx: &sshOrphanTaskContext{
				Task: &models.Task{ID: "t1", ArchivedAt: &archivedAt},
			},
			wantVerdict: sshOrphanStop,
		},
		{
			name: "unclaimed process whose task has only terminal sessions stops",
			pid:  300,
			taskCtx: &sshOrphanTaskContext{
				Task: &models.Task{ID: "t1"},
				Sessions: []*models.TaskSession{
					session("s1", models.TaskSessionStateCompleted),
					session("s2", models.TaskSessionStateCancelled),
				},
			},
			wantVerdict: sshOrphanStop,
		},
		{
			name: "unclaimed process whose task has zero sessions stops (vacuously all-terminal)",
			pid:  300,
			taskCtx: &sshOrphanTaskContext{
				Task: &models.Task{ID: "t1"},
			},
			wantVerdict: sshOrphanStop,
		},
		{
			name: "unclaimed process whose task has any non-terminal session preserves",
			pid:  300,
			taskCtx: &sshOrphanTaskContext{
				Task: &models.Task{ID: "t1"},
				Sessions: []*models.TaskSession{
					session("s1", models.TaskSessionStateCompleted),
					session("s2", models.TaskSessionStateRunning),
				},
			},
			wantVerdict: sshOrphanPreserve,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideSSHOrphanProcess(tc.pid, tc.claimedID, tc.claimed, tc.tainted, tc.taskCtx)
			if got.Verdict != tc.wantVerdict {
				t.Fatalf("Verdict = %v, want %v (reason: %s)", got.Verdict, tc.wantVerdict, got.Reason)
			}
			if got.SessionID != tc.wantSessionID {
				t.Fatalf("SessionID = %q, want %q", got.SessionID, tc.wantSessionID)
			}
			if got.Reason == "" {
				t.Fatal("Reason must never be empty")
			}
		})
	}
}
