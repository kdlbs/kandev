package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCaptureWorkspaceRecoverySelectionSnapshotReadsSelectedInventoryOnce(t *testing.T) {
	session := &TaskSession{ID: "session", TaskID: "task", TaskEnvironmentID: "environment"}
	environment := &TaskEnvironment{
		ID: "environment", TaskID: "task", OwnershipGeneration: 1,
		Repos: []*TaskEnvironmentRepo{
			{ID: "slot-active", RepositoryID: "repo-active", Status: "active"},
			{ID: "slot-legacy", RepositoryID: "repo-legacy", Status: ""},
			{ID: "slot-failed", RepositoryID: "repo-failed", Status: "failed"},
			{ID: "slot-deleted", RepositoryID: "repo-deleted", Status: "active", DeletedAt: ptrTimeForRecoveryTest()},
		},
	}
	reads := make(map[string]int)
	snapshot, err := CaptureWorkspaceRecoverySelectionSnapshot(session, environment, func(id string) (*Repository, error) {
		reads[id]++
		return &Repository{ID: id, LocalPath: "/repos/" + id}, nil
	})
	require.NoError(t, err)
	require.Equal(t, map[string]int{"repo-active": 1, "repo-legacy": 1}, reads)
	require.True(t, snapshot.Complete())
	require.Len(t, snapshot.Slots, 2)
	require.Equal(t, []string{"slot-active", "slot-legacy"}, []string{
		snapshot.Slots[0].EnvironmentRepoID, snapshot.Slots[1].EnvironmentRepoID,
	})
	require.Equal(t, "active", snapshot.Slots[1].Status)
}

func TestWorkspaceRecoverySelectionSessionEnvironmentCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		snapshot WorkspaceRecoverySelectionSnapshot
		want     bool
	}{
		{
			name: "explicit selected environment",
			snapshot: WorkspaceRecoverySelectionSnapshot{
				TaskID: "task", SessionTaskEnvironmentID: "environment", TaskEnvironmentID: "environment",
			},
			want: true,
		},
		{
			name: "persisted legacy session owned by selected task",
			snapshot: WorkspaceRecoverySelectionSnapshot{
				TaskID: "task", SessionPersisted: true, TaskEnvironmentID: "environment",
				EnvironmentOwnerTaskID: "task",
			},
			want: true,
		},
		{
			name: "prepared session without binding",
			snapshot: WorkspaceRecoverySelectionSnapshot{
				TaskID: "task", TaskEnvironmentID: "environment", EnvironmentOwnerTaskID: "task",
			},
		},
		{
			name: "legacy session cannot inherit another task environment",
			snapshot: WorkspaceRecoverySelectionSnapshot{
				TaskID: "task", SessionPersisted: true, TaskEnvironmentID: "environment",
				EnvironmentOwnerTaskID: "parent-task",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.snapshot.SessionEnvironmentMatchesSelected())
		})
	}
}

func ptrTimeForRecoveryTest() *time.Time {
	deletedAt := time.Unix(1, 0)
	return &deletedAt
}
