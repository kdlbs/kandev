package api

import (
	"errors"
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

// TestMergeGitLogResults verifies that mergeGitLogResults aggregates per-repo
// outcomes into the right shape: commits stamped with RepositoryName, partial
// failures surfaced via PerRepoErrors, and Success/Error reflecting whether
// the whole fan-out failed.
func TestMergeGitLogResults(t *testing.T) {
	t.Parallel()

	t.Run("partial failure: one repo succeeds, one fails", func(t *testing.T) {
		outcomes := []perRepoLogOutcome{
			{
				subpath: "frontend",
				result: &process.GitLogResult{
					Success: true,
					Commits: []*process.GitCommitInfo{
						{CommitSHA: "a", CommittedAt: "2026-04-26T10:00:00Z"},
					},
				},
			},
			{
				subpath: "backend",
				err:     errors.New("repo not found"),
			},
		}
		got := mergeGitLogResults(outcomes, 100)
		if !got.Success {
			t.Errorf("expected Success=true when at least one repo succeeded; got false (Error=%q)", got.Error)
		}
		if got.Error != "" {
			t.Errorf("expected empty Error on partial success; got %q", got.Error)
		}
		if len(got.PerRepoErrors) != 1 {
			t.Fatalf("expected 1 per-repo error; got %d", len(got.PerRepoErrors))
		}
		if got.PerRepoErrors[0].RepositoryName != "backend" {
			t.Errorf("per-repo error subject: got %q, want %q",
				got.PerRepoErrors[0].RepositoryName, "backend")
		}
		if got.PerRepoErrors[0].Error != "repo not found" {
			t.Errorf("per-repo error text: got %q, want %q",
				got.PerRepoErrors[0].Error, "repo not found")
		}
		if len(got.Commits) != 1 || got.Commits[0].CommitSHA != "a" {
			t.Errorf("expected 1 commit from frontend; got %v", got.Commits)
		}
		if got.Commits[0].RepositoryName != "frontend" {
			t.Errorf("commit RepositoryName not stamped: got %q", got.Commits[0].RepositoryName)
		}
	})

	t.Run("partial failure: result.Success=false from per-repo", func(t *testing.T) {
		outcomes := []perRepoLogOutcome{
			{
				subpath: "frontend",
				result: &process.GitLogResult{
					Success: true,
					Commits: []*process.GitCommitInfo{{CommitSHA: "a", CommittedAt: "2026-04-26T10:00:00Z"}},
				},
			},
			{
				subpath: "backend",
				result:  &process.GitLogResult{Success: false, Error: "git log: bad ref"},
			},
		}
		got := mergeGitLogResults(outcomes, 100)
		if !got.Success {
			t.Errorf("expected Success=true with partial failure")
		}
		if len(got.PerRepoErrors) != 1 || got.PerRepoErrors[0].Error != "git log: bad ref" {
			t.Errorf("expected PerRepoErrors carrying the inner error; got %+v", got.PerRepoErrors)
		}
	})

	t.Run("all repos fail: Success=false with summary error", func(t *testing.T) {
		outcomes := []perRepoLogOutcome{
			{subpath: "frontend", err: errors.New("transport boom")},
			{subpath: "backend", result: &process.GitLogResult{Success: false, Error: "bad ref"}},
		}
		got := mergeGitLogResults(outcomes, 100)
		if got.Success {
			t.Errorf("expected Success=false when every repo failed")
		}
		if got.Error == "" {
			t.Errorf("expected non-empty Error summary when every repo failed")
		}
		if len(got.PerRepoErrors) != 2 {
			t.Fatalf("expected 2 per-repo errors; got %d", len(got.PerRepoErrors))
		}
		// Order matches input order.
		if got.PerRepoErrors[0].RepositoryName != "frontend" ||
			got.PerRepoErrors[1].RepositoryName != "backend" {
			t.Errorf("per-repo error order: got %+v", got.PerRepoErrors)
		}
		if len(got.Commits) != 0 {
			t.Errorf("expected no commits; got %d", len(got.Commits))
		}
	})

	t.Run("all repos succeed: Success=true with empty PerRepoErrors", func(t *testing.T) {
		outcomes := []perRepoLogOutcome{
			{
				subpath: "frontend",
				result: &process.GitLogResult{
					Success: true,
					Commits: []*process.GitCommitInfo{{CommitSHA: "a", CommittedAt: "2026-04-26T10:00:00Z"}},
				},
			},
			{
				subpath: "backend",
				result: &process.GitLogResult{
					Success: true,
					Commits: []*process.GitCommitInfo{{CommitSHA: "b", CommittedAt: "2026-04-26T11:00:00Z"}},
				},
			},
		}
		got := mergeGitLogResults(outcomes, 100)
		if !got.Success {
			t.Errorf("expected Success=true; Error=%q", got.Error)
		}
		if got.Error != "" {
			t.Errorf("expected empty Error on full success; got %q", got.Error)
		}
		if len(got.PerRepoErrors) != 0 {
			t.Errorf("expected empty PerRepoErrors; got %+v", got.PerRepoErrors)
		}
		if len(got.Commits) != 2 {
			t.Fatalf("expected 2 commits merged; got %d", len(got.Commits))
		}
		// Newest first ordering.
		if got.Commits[0].CommitSHA != "b" || got.Commits[1].CommitSHA != "a" {
			t.Errorf("expected newest-first order [b, a]; got [%s, %s]",
				got.Commits[0].CommitSHA, got.Commits[1].CommitSHA)
		}
		// RepositoryName stamped.
		if got.Commits[0].RepositoryName != "backend" || got.Commits[1].RepositoryName != "frontend" {
			t.Errorf("RepositoryName not stamped correctly: %+v", got.Commits)
		}
	})

	t.Run("empty outcomes: Success=true with no commits or errors", func(t *testing.T) {
		// Defensive: callers shouldn't invoke us with zero subpaths, but if
		// they do we should not classify "no work" as "everything failed".
		got := mergeGitLogResults(nil, 100)
		if !got.Success {
			t.Errorf("expected Success=true on empty input; got false (Error=%q)", got.Error)
		}
		if len(got.Commits) != 0 || len(got.PerRepoErrors) != 0 {
			t.Errorf("expected empty result on empty input; got %+v", got)
		}
	})
}
