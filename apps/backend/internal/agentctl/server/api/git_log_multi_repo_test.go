package api

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// sortCommitsByCommittedAtDesc must order commits newest-first so the merged
// per-repo log reads chronologically, regardless of which repo bucket each
// commit came from. Unparseable timestamps preserve their relative order via
// SliceStable so we don't randomly reshuffle bad data.
func TestSortCommitsByCommittedAtDesc(t *testing.T) {
	t.Parallel()

	t.Run("interleaves commits from multiple repos by timestamp", func(t *testing.T) {
		commits := []*process.GitCommitInfo{
			{CommitSHA: "a", CommittedAt: "2026-04-26T10:00:00Z", RepositoryName: "frontend"},
			{CommitSHA: "b", CommittedAt: "2026-04-26T12:00:00Z", RepositoryName: "backend"},
			{CommitSHA: "c", CommittedAt: "2026-04-26T11:00:00Z", RepositoryName: "frontend"},
		}
		sortCommitsByCommittedAtDesc(commits)
		got := []string{commits[0].CommitSHA, commits[1].CommitSHA, commits[2].CommitSHA}
		want := []string{"b", "c", "a"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("position %d: got %q, want %q (full order: %v)", i, got[i], want[i], got)
			}
		}
	})

	t.Run("preserves order on unparseable timestamps", func(t *testing.T) {
		commits := []*process.GitCommitInfo{
			{CommitSHA: "x", CommittedAt: "not-a-date"},
			{CommitSHA: "y", CommittedAt: "also-bad"},
		}
		sortCommitsByCommittedAtDesc(commits)
		if commits[0].CommitSHA != "x" || commits[1].CommitSHA != "y" {
			t.Errorf("expected stable order on bad timestamps; got %s, %s",
				commits[0].CommitSHA, commits[1].CommitSHA)
		}
	})
}
