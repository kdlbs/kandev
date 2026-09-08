package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestGetLatestGitStatusSnapshotsBySessionIDsRanksEachRepositoryIndependently
// covers the multi-repository selector's ranking boundary. A root archive must
// not be demoted by a newer live row from a named repository, and a named
// agent-completed row must not be demoted by a newer root archive. The root
// rows deliberately use absent and explicit-empty repository names to verify
// both normalize to the same partition.
func TestGetLatestGitStatusSnapshotsBySessionIDsRanksEachRepositoryIndependently(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-snapshot-repository-ranking"
		sessionID = "session-snapshot-repository-ranking"
	)
	seedSessionForGit(t, repo, taskID, sessionID)

	base := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	snapshots := []*models.GitSnapshot{
		{
			ID: "root-live-old", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: TriggeredByLiveMonitor,
			HeadCommit: "root-live-old", CreatedAt: base.Add(-time.Hour),
			Metadata: map[string]interface{}{"repository_name": ""},
		},
		{
			ID: "named-completed", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: "agent_completed",
			HeadCommit: "named-completed", CreatedAt: base,
			Metadata: map[string]interface{}{"repository_name": "named"},
		},
		{
			ID: "root-archive", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeArchive,
			HeadCommit:   "root-archive", CreatedAt: base.Add(time.Hour),
			// A nil metadata map serializes without a repository_name key. It
			// must share the root partition with root-live-old's explicit "".
		},
		{
			ID: "named-live-new", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: TriggeredByLiveMonitor,
			HeadCommit: "named-live-new", CreatedAt: base.Add(2 * time.Hour),
			Metadata: map[string]interface{}{"repository_name": "named"},
		},
	}
	for _, snapshot := range snapshots {
		if err := repo.CreateGitSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("CreateGitSnapshot(%s): %v", snapshot.ID, err)
		}
	}

	got, err := repo.GetLatestGitStatusSnapshotsBySessionIDs(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("GetLatestGitStatusSnapshotsBySessionIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("selected %d snapshots (%v), want one per repository", len(got), got)
	}

	byRepository := make(map[string]string, len(got))
	for _, snapshot := range got {
		byRepository[gitSnapshotRepositoryName(snapshot)] = snapshot.ID
	}
	if got := byRepository[""]; got != "root-archive" {
		t.Errorf("root repository selected %q, want root-archive", got)
	}
	if got := byRepository["named"]; got != "named-completed" {
		t.Errorf("named repository selected %q, want named-completed", got)
	}
}
