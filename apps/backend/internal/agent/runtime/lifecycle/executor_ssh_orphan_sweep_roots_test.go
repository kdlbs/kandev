package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.13
func TestSSHOrphanWorkdirRoots(t *testing.T) {
	tests := []struct {
		name           string
		executorConfig map[string]string
		profiles       []*models.ExecutorProfile
		wantRoots      []string
	}{
		{
			name:           "no config anywhere falls back to default",
			executorConfig: map[string]string{},
			wantRoots:      []string{sshDefaultWorkdir},
		},
		{
			name:           "executor config only",
			executorConfig: map[string]string{"ssh_workdir_root": "/exec/root"},
			wantRoots:      []string{"/exec/root"},
		},
		{
			name:           "profile only, no executor-level root",
			executorConfig: map[string]string{},
			profiles: []*models.ExecutorProfile{
				{ID: "bc154c2b", Config: map[string]string{"ssh_workdir_root": "/Users/neo/kandev-workspaces"}},
			},
			wantRoots: []string{"/Users/neo/kandev-workspaces"},
		},
		{
			name:           "executor config and a distinct profile root are both covered",
			executorConfig: map[string]string{"ssh_workdir_root": "/exec/root"},
			profiles: []*models.ExecutorProfile{
				{ID: "profile-1", Config: map[string]string{"ssh_workdir_root": "/profile/root"}},
			},
			wantRoots: []string{"/exec/root", "/profile/root"},
		},
		{
			name:           "duplicate roots across executor and profile are deduplicated",
			executorConfig: map[string]string{"ssh_workdir_root": "/shared/root"},
			profiles: []*models.ExecutorProfile{
				{ID: "profile-1", Config: map[string]string{"ssh_workdir_root": "/shared/root"}},
				{ID: "profile-2", Config: map[string]string{"ssh_workdir_root": " /shared/root "}},
			},
			wantRoots: []string{"/shared/root"},
		},
		{
			name:           "a nil profile entry is skipped",
			executorConfig: map[string]string{},
			profiles:       []*models.ExecutorProfile{nil, {ID: "p", Config: map[string]string{"ssh_workdir_root": "/p/root"}}},
			wantRoots:      []string{"/p/root"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sshOrphanWorkdirRoots(tt.executorConfig, tt.profiles)
			if len(got) != len(tt.wantRoots) {
				t.Fatalf("roots = %v, want %v", got, tt.wantRoots)
			}
			for i, root := range tt.wantRoots {
				if got[i] != root {
					t.Fatalf("roots = %v, want %v", got, tt.wantRoots)
				}
			}
		})
	}
}

// orphanSweepRootTestStore is a minimal sshOrphanSweepStore fake for exercising
// sweepSSHExecutorOrphans end to end against a fake SSH server. Every process
// discovered is attributed to task, which the tests set up as archived so
// decideSSHOrphanProcess reaches a stop verdict without needing session rows.
type orphanSweepRootTestStore struct {
	profiles []*models.ExecutorProfile
	task     *models.Task
}

func (s *orphanSweepRootTestStore) GetTask(context.Context, string) (*models.Task, error) {
	return s.task, nil
}

func (s *orphanSweepRootTestStore) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, nil
}

func (s *orphanSweepRootTestStore) ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error) {
	return nil, nil
}

func (s *orphanSweepRootTestStore) ListExecutorProfiles(context.Context, string) ([]*models.ExecutorProfile, error) {
	return s.profiles, nil
}

// TestSweepSSHExecutorOrphansUsesProfileWorkdirRoot is the regression Review
// Round 1 (R1-F1) asked for: on neo, ssh_workdir_root lives only on executor
// profile bc154c2b, not on the executor's own config, so a sweep that reads
// executor.Config alone finds nothing to stop. This proves the sweep
// inventories under a root that is configured only on a profile.
//
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.13
func TestSweepSSHExecutorOrphansUsesProfileWorkdirRoot(t *testing.T) {
	archivedAt := time.Now().Add(-time.Hour)
	handler := newSSHScriptedHandler(t,
		sshScriptRule{
			match:  `ROOT='/Users/neo/kandev-workspaces/tasks'`,
			result: sshOut("PROC\t4242\t1\t/opt/kandev/agentctl --workdir /Users/neo/kandev-workspaces/tasks/task-1\n"),
		},
		sshScriptRule{match: "TARGET_PID=4242", result: sshOK},
	)
	server := newFakeSSHServer(t, handler.handle)
	client := server.dial(t)

	store := &orphanSweepRootTestStore{
		profiles: []*models.ExecutorProfile{
			{ID: "bc154c2b", Config: map[string]string{"ssh_workdir_root": "/Users/neo/kandev-workspaces"}},
		},
		task: &models.Task{ID: "1", ArchivedAt: &archivedAt},
	}

	report, err := sweepSSHExecutorOrphans(context.Background(), client, store, "executor-1", map[string]string{}, logger.Default())
	if err != nil {
		t.Fatalf("sweepSSHExecutorOrphans: %v", err)
	}
	if report.Found != 1 || report.Stopped != 1 {
		t.Fatalf("report = %+v, want found=1 stopped=1 — the profile-only workdir root was not swept", report)
	}
	if _, ok := server.lastCommandContaining("TARGET_PID=4242"); !ok {
		t.Fatalf("no stop command was sent for the orphan discovered under the profile's workdir root")
	}
}
