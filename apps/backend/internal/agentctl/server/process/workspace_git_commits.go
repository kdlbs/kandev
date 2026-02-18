package process

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// getCommitsSince returns commits from baseCommit (exclusive) to HEAD (inclusive)
func (wt *WorkspaceTracker) getCommitsSince(ctx context.Context, baseCommit string) []*types.GitCommitNotification {
	// Get list of commits with metadata
	// Format: SHA|ParentSHA|AuthorName|AuthorEmail|Subject|AuthorDateISO
	cmd := exec.CommandContext(ctx, "git", "log",
		"--format=%H|%P|%an|%ae|%s|%aI",
		baseCommit+"..HEAD")
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		wt.logger.Debug("failed to get commits since base",
			zap.String("base", baseCommit),
			zap.Error(err))
		return nil
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return nil
	}

	lines := strings.Split(output, "\n")
	commits := make([]*types.GitCommitNotification, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 6 {
			continue
		}

		sha := parts[0]
		parentSHA := parts[1]
		// Handle multiple parents (merge commits) - just take the first
		if idx := strings.Index(parentSHA, " "); idx > 0 {
			parentSHA = parentSHA[:idx]
		}

		committedAt, err := time.Parse(time.RFC3339, parts[5])
		if err != nil {
			committedAt = time.Now().UTC()
		}

		// Get stats for this commit
		filesChanged, insertions, deletions := wt.getCommitStats(ctx, sha)

		commits = append(commits, &types.GitCommitNotification{
			Timestamp:    time.Now(),
			CommitSHA:    sha,
			ParentSHA:    parentSHA,
			AuthorName:   parts[2],
			AuthorEmail:  parts[3],
			Message:      parts[4],
			FilesChanged: filesChanged,
			Insertions:   insertions,
			Deletions:    deletions,
			CommittedAt:  committedAt,
		})
	}

	return commits
}

// getCommitStats returns the number of files changed, insertions, and deletions for a commit
func (wt *WorkspaceTracker) getCommitStats(ctx context.Context, sha string) (filesChanged, insertions, deletions int) {
	cmd := exec.CommandContext(ctx, "git", "show", "--stat", "--format=", sha)
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0
	}

	// Parse the last line which contains summary like "3 files changed, 10 insertions(+), 5 deletions(-)"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return 0, 0, 0
	}

	summary := lines[len(lines)-1]
	// Simple parsing - look for numbers before keywords
	parts := strings.Fields(summary)
	for i, part := range parts {
		if strings.Contains(part, "file") && i > 0 {
			_, _ = fmt.Sscanf(parts[i-1], "%d", &filesChanged)
		}
		if strings.Contains(part, "insertion") && i > 0 {
			_, _ = fmt.Sscanf(parts[i-1], "%d", &insertions)
		}
		if strings.Contains(part, "deletion") && i > 0 {
			_, _ = fmt.Sscanf(parts[i-1], "%d", &deletions)
		}
	}

	return filesChanged, insertions, deletions
}
