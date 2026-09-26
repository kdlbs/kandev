package orchestrator

import (
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func TestArchiveGitStatusMetadataPreservesStatusSummary(t *testing.T) {
	status := &client.GitStatusResult{
		Timestamp:        "2026-08-20T14:00:00Z",
		Modified:         []string{"src/app.ts"},
		Added:            []string{"src/new.ts"},
		Deleted:          []string{"src/old.ts"},
		Untracked:        []string{"tmp.txt"},
		Renamed:          []string{"src/old-name.ts -> src/new-name.ts"},
		RemoteAhead:      2,
		RemoteBehind:     1,
		RemoteHeadCommit: "remote-head",
		BranchAdditions:  8,
		BranchDeletions:  3,
	}

	metadata := archiveGitStatusMetadata(status, map[string]interface{}{
		"src/app.ts":  map[string]interface{}{"status": "modified"},
		"src/new.ts":  map[string]interface{}{"status": "added"},
		"src/old.ts":  map[string]interface{}{"status": "deleted"},
		"src/renamed": map[string]interface{}{"status": "renamed"},
	})
	if metadata["timestamp"] != status.Timestamp {
		t.Errorf("timestamp = %#v, want %q", metadata["timestamp"], status.Timestamp)
	}
	if metadata["changed_files"] != 4 {
		t.Errorf("changed_files = %#v, want 4", metadata["changed_files"])
	}
	if metadata["branch_additions"] != status.BranchAdditions {
		t.Errorf("branch_additions = %#v, want %d", metadata["branch_additions"], status.BranchAdditions)
	}
	if metadata["branch_deletions"] != status.BranchDeletions {
		t.Errorf("branch_deletions = %#v, want %d", metadata["branch_deletions"], status.BranchDeletions)
	}
	if metadata["remote_head_commit"] != status.RemoteHeadCommit {
		t.Errorf("remote_head_commit = %#v, want %q", metadata["remote_head_commit"], status.RemoteHeadCommit)
	}
	for _, key := range []string{"modified", "added", "deleted", "untracked", "renamed"} {
		if metadata[key] == nil {
			t.Errorf("metadata[%q] is nil", key)
		}
	}
}
