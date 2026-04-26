package process

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// getCommitsSince returns commits from baseCommit (exclusive) to HEAD (inclusive).
// Uses --shortstat to fetch stats in a single git command instead of N+1 git-show calls.
func (wt *WorkspaceTracker) getCommitsSince(ctx context.Context, baseCommit string) []*types.GitCommitNotification {
	// Record separator (\x1e) placed BEFORE fields so --shortstat output stays
	// within the same record:  \x1e<fields>\n <stat summary>\n\x1e<fields>\n...
	cmd := exec.CommandContext(ctx, "git", "log",
		"--format=\x1e%H\x1f%P\x1f%an\x1f%ae\x1f%s\x1f%aI",
		"--shortstat",
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

	records := strings.Split(output, "\x1e")
	commits := make([]*types.GitCommitNotification, 0, len(records))

	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		// Split into field line and optional stat line
		lines := strings.SplitN(record, "\n", 2)
		fieldLine := strings.TrimSpace(lines[0])

		parts := strings.SplitN(fieldLine, "\x1f", 6)
		if len(parts) < 6 {
			continue
		}

		sha := parts[0]
		parentSHA := parts[1]
		if idx := strings.Index(parentSHA, " "); idx > 0 {
			parentSHA = parentSHA[:idx]
		}

		committedAt, err := time.Parse(time.RFC3339, parts[5])
		if err != nil {
			committedAt = time.Now().UTC()
		}

		var filesChanged, insertions, deletions int
		if len(lines) > 1 {
			statLine := strings.TrimSpace(lines[1])
			if statLine != "" {
				filesChanged, insertions, deletions = parseStatSummary(statLine)
			}
		}

		commits = append(commits, &types.GitCommitNotification{
			Timestamp:      time.Now(),
			RepositoryName: wt.repositoryName,
			CommitSHA:      sha,
			ParentSHA:      parentSHA,
			AuthorName:     parts[2],
			AuthorEmail:    parts[3],
			Message:        parts[4],
			FilesChanged:   filesChanged,
			Insertions:     insertions,
			Deletions:      deletions,
			CommittedAt:    committedAt,
		})
	}

	return commits
}
