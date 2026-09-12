package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

const (
	contributionHistoryObservationTimeout = 2 * time.Second
	contributionHistoryReflogLimit        = 200
	contributionHistoryCommitLimit        = 2000
	contributionHistoryOutputLimit        = 1 << 20
	contributionHistoryKindLocalRebase    = "local_rebase"
	contributionHistoryKindUnexplained    = "unexplained"
	contributionHistoryReasonMatched      = "matched_reflog"
	contributionHistoryReasonNoMatch      = "no_matching_reflog"
	contributionHistoryReasonMissing      = "missing_objects"
	contributionHistoryReasonAmbiguous    = "ambiguous"
	contributionHistoryReasonStale        = "stale"
	contributionHistoryReasonLimit        = "limit"
	contributionHistoryReasonUnavailable  = "unavailable"
)

var (
	errContributionHistoryMissingObjects = errors.New("contribution history objects are unavailable")
	errContributionHistoryAmbiguous      = errors.New("contribution history is ambiguous")
	errContributionHistoryLimit          = errors.New("contribution history observation limit reached")
	errContributionHistoryStale          = errors.New("contribution history identity is stale")
)

// ContributionHistoryExplanationResult is a bounded, read-only explanation of
// a task branch and its published contribution head.
type ContributionHistoryExplanationResult struct {
	Repo                 string `json:"repo,omitempty"`
	Branch               string `json:"branch"`
	ExpectedLocalHead    string `json:"expected_local_head"`
	ExpectedRemoteHead   string `json:"expected_remote_head"`
	Kind                 string `json:"kind"`
	Reason               string `json:"reason"`
	OntoHead             string `json:"onto_head,omitempty"`
	TaskCommitCount      *int   `json:"task_commit_count,omitempty"`
	PublishedCommitCount *int   `json:"published_commit_count,omitempty"`
	NewBaseCommitCount   *int   `json:"new_base_commit_count,omitempty"`
}

type contributionHistoryReflogEntry struct {
	sha     string
	subject string
}

type contributionHistoryCountResult struct {
	count   *int
	limited bool
	failed  bool
}

// contributionHistoryOutputWriter captures command output without allowing a
// reflog or commit walk to grow the agentctl process without bound.
type contributionHistoryOutputWriter struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newContributionHistoryOutputWriter(limit int) *contributionHistoryOutputWriter {
	return &contributionHistoryOutputWriter{limit: limit}
}

func (w *contributionHistoryOutputWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		keep := len(data)
		if keep > remaining {
			keep = remaining
		}
		_, _ = w.buffer.Write(data[:keep])
	}
	if len(data) > remaining {
		w.truncated = true
	}
	return len(data), nil
}

func (w *contributionHistoryOutputWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func (w *contributionHistoryOutputWriter) Truncated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.truncated
}

// ExplainContributionHistory observes local Git state for a selected
// contribution identity. It never fetches, rewrites, or publishes refs.
func (g *GitOperator) ExplainContributionHistory(
	ctx context.Context,
	branch, expectedLocalHead, expectedRemoteHead string,
) (*ContributionHistoryExplanationResult, error) {
	result := newContributionHistoryResult(g.repoName, branch, expectedLocalHead, expectedRemoteHead)
	if !validContributionHistoryRequest(branch, expectedLocalHead, expectedRemoteHead) {
		return result, nil
	}

	observeCtx, cancel := context.WithTimeout(ctx, contributionHistoryObservationTimeout)
	defer cancel()

	if err := g.requireNoContributionHistoryRebase(observeCtx); err != nil {
		result.Reason = contributionHistoryFailureReason(err, contributionHistoryReasonAmbiguous)
		return result, nil
	}
	if err := g.requireContributionHistoryIdentity(observeCtx, branch, expectedLocalHead); err != nil {
		result.Reason = contributionHistoryFailureReason(err, contributionHistoryReasonUnavailable)
		return result, nil
	}
	if err := g.requireContributionHistoryObjects(observeCtx, expectedLocalHead, expectedRemoteHead); err != nil {
		result.Reason = contributionHistoryFailureReason(err, contributionHistoryReasonMissing)
		return result, nil
	}

	ontoHead, err := g.findContributionHistoryRebase(observeCtx, branch, expectedLocalHead, expectedRemoteHead)
	if err != nil {
		result.Reason = contributionHistoryFailureReason(err, contributionHistoryReasonNoMatch)
		return result, nil
	}
	if err := g.requireContributionHistoryObject(observeCtx, ontoHead); err != nil {
		result.Reason = contributionHistoryFailureReason(err, contributionHistoryReasonMissing)
		return result, nil
	}
	ancestor, err := g.contributionHistoryIsAncestor(observeCtx, ontoHead, expectedLocalHead)
	if err != nil {
		result.Reason = contributionHistoryFailureReason(err, contributionHistoryReasonUnavailable)
		return result, nil
	}
	if !ancestor {
		result.Reason = contributionHistoryReasonAmbiguous
		return result, nil
	}

	result.Kind = contributionHistoryKindLocalRebase
	result.Reason = contributionHistoryReasonMatched
	result.OntoHead = ontoHead
	if limited, failed := g.populateContributionHistoryCounts(observeCtx, result); failed {
		result.Kind = contributionHistoryKindUnexplained
		result.Reason = contributionHistoryReasonUnavailable
		result.OntoHead = ""
	} else if limited {
		result.Reason = contributionHistoryReasonLimit
	}
	if err := g.requireContributionHistoryIdentity(observeCtx, branch, expectedLocalHead); err != nil {
		result = newContributionHistoryResult(g.repoName, branch, expectedLocalHead, expectedRemoteHead)
		result.Reason = contributionHistoryFailureReason(err, contributionHistoryReasonStale)
		return result, nil
	}
	return result, nil
}

func newContributionHistoryResult(repo, branch, localHead, remoteHead string) *ContributionHistoryExplanationResult {
	return &ContributionHistoryExplanationResult{
		Repo:               repo,
		Branch:             branch,
		ExpectedLocalHead:  localHead,
		ExpectedRemoteHead: remoteHead,
		Kind:               contributionHistoryKindUnexplained,
		Reason:             contributionHistoryReasonUnavailable,
	}
}

func validContributionHistoryRequest(branch, localHead, remoteHead string) bool {
	return securityutil.IsValidBranchName(branch) && !securityutil.IsGitSymbolicRef(branch) &&
		isFullCommitSHA(localHead) && isFullCommitSHA(remoteHead)
}

func contributionHistoryFailureReason(err error, fallback string) string {
	switch {
	case errors.Is(err, errContributionHistoryLimit):
		return contributionHistoryReasonLimit
	case errors.Is(err, errContributionHistoryMissingObjects):
		return contributionHistoryReasonMissing
	case errors.Is(err, errContributionHistoryAmbiguous):
		return contributionHistoryReasonAmbiguous
	case errors.Is(err, errContributionHistoryStale):
		return contributionHistoryReasonStale
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return contributionHistoryReasonUnavailable
	default:
		return fallback
	}
}

func (g *GitOperator) requireContributionHistoryIdentity(ctx context.Context, branch, expectedHead string) error {
	currentBranch, err := g.contributionHistoryValue(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	currentHead, err := g.contributionHistoryValue(ctx, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if currentBranch != branch || !strings.EqualFold(currentHead, expectedHead) {
		return fmt.Errorf("%w: selected branch or head changed", errContributionHistoryStale)
	}
	return nil
}

func (g *GitOperator) requireContributionHistoryObjects(ctx context.Context, heads ...string) error {
	for _, head := range heads {
		if err := g.requireContributionHistoryObject(ctx, head); err != nil {
			return err
		}
	}
	return nil
}

func (g *GitOperator) requireContributionHistoryObject(ctx context.Context, head string) error {
	resolved, err := g.contributionHistoryValue(ctx, "rev-parse", "--verify", head+"^{commit}")
	if err != nil {
		return fmt.Errorf("%w: %v", errContributionHistoryMissingObjects, err)
	}
	if !strings.EqualFold(resolved, head) {
		return fmt.Errorf("%w: resolved head differs", errContributionHistoryMissingObjects)
	}
	return nil
}

func (g *GitOperator) requireNoContributionHistoryRebase(ctx context.Context) error {
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		path, err := g.contributionHistoryValue(ctx, "rev-parse", "--git-path", state)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(g.workDir, path)
		}
		if _, err := os.Stat(path); err == nil {
			return errContributionHistoryAmbiguous
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (g *GitOperator) findContributionHistoryRebase(
	ctx context.Context, branch, localHead, remoteHead string,
) (string, error) {
	branchOutput, err := g.contributionHistoryValue(ctx, "reflog", "show",
		"--format=%H%x1f%gs", fmt.Sprintf("-n%d", contributionHistoryReflogLimit), branch)
	if err != nil {
		return "", err
	}
	branchEntries, err := parseContributionHistoryReflog(branchOutput)
	if err != nil {
		return "", err
	}
	ontoHead, err := matchContributionHistoryBranchReflog(branchEntries, branch, localHead, remoteHead)
	if err != nil {
		return "", err
	}

	headOutput, err := g.contributionHistoryValue(ctx, "reflog", "show",
		"--format=%H%x1f%gs", fmt.Sprintf("-n%d", contributionHistoryReflogLimit), "HEAD")
	if err != nil {
		return "", err
	}
	headEntries, err := parseContributionHistoryReflog(headOutput)
	if err != nil {
		return "", err
	}
	if err := matchContributionHistoryHeadReflog(headEntries, branch, localHead, ontoHead); err != nil {
		return "", err
	}
	return ontoHead, nil
}

func parseContributionHistoryReflog(output string) ([]contributionHistoryReflogEntry, error) {
	var entries []contributionHistoryReflogEntry
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sha, subject, ok := strings.Cut(line, "\x1f")
		if !ok || !isFullCommitSHA(sha) {
			return nil, errContributionHistoryAmbiguous
		}
		entries = append(entries, contributionHistoryReflogEntry{sha: sha, subject: strings.TrimSpace(subject)})
		if len(entries) > contributionHistoryReflogLimit {
			return nil, errContributionHistoryLimit
		}
	}
	return entries, nil
}

func matchContributionHistoryBranchReflog(
	entries []contributionHistoryReflogEntry, branch, localHead, remoteHead string,
) (string, error) {
	if len(entries) < 2 || !strings.EqualFold(entries[0].sha, localHead) ||
		!strings.EqualFold(entries[1].sha, remoteHead) {
		return "", errors.New("no matching branch reflog")
	}
	prefix := "rebase (finish): refs/heads/" + branch + " onto "
	if !strings.HasPrefix(entries[0].subject, prefix) {
		return "", errors.New("no matching branch reflog")
	}
	ontoHead := strings.TrimSpace(strings.TrimPrefix(entries[0].subject, prefix))
	if !isFullCommitSHA(ontoHead) {
		return "", errContributionHistoryAmbiguous
	}
	return ontoHead, nil
}

func matchContributionHistoryHeadReflog(
	entries []contributionHistoryReflogEntry, branch, localHead, ontoHead string,
) error {
	if len(entries) < 3 || !strings.EqualFold(entries[0].sha, localHead) {
		return errors.New("no matching HEAD reflog")
	}
	if entries[0].subject != "rebase (finish): returning to refs/heads/"+branch {
		return errors.New("no matching HEAD reflog")
	}

	startPrefix := "rebase (start):"
	replayCount := 0
	for _, entry := range entries[1:] {
		switch {
		case strings.HasPrefix(entry.subject, "rebase (pick):"),
			strings.HasPrefix(entry.subject, "rebase (continue):"),
			strings.HasPrefix(entry.subject, "rebase (reword):"),
			strings.HasPrefix(entry.subject, "rebase (edit):"):
			replayCount++
		case strings.HasPrefix(entry.subject, startPrefix):
			if replayCount == 0 || !strings.EqualFold(entry.sha, ontoHead) {
				return errContributionHistoryAmbiguous
			}
			return nil
		default:
			return errContributionHistoryAmbiguous
		}
	}
	return errContributionHistoryAmbiguous
}

func (g *GitOperator) contributionHistoryIsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	_, err := g.contributionHistoryCommand(ctx, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func (g *GitOperator) populateContributionHistoryCounts(
	ctx context.Context, result *ContributionHistoryExplanationResult,
) (limited, failed bool) {
	base, err := g.contributionHistoryMergeBase(ctx, result.ExpectedRemoteHead, result.OntoHead)
	if err != nil {
		return errors.Is(err, errContributionHistoryLimit), errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	}

	task := g.contributionHistoryCount(ctx, result.OntoHead, result.ExpectedLocalHead)
	published := g.contributionHistoryCount(ctx, base, result.ExpectedRemoteHead)
	newBase := g.contributionHistoryCount(ctx, base, result.OntoHead)
	result.TaskCommitCount = task.count
	result.PublishedCommitCount = published.count
	result.NewBaseCommitCount = newBase.count
	return task.limited || published.limited || newBase.limited,
		task.failed || published.failed || newBase.failed
}

func (g *GitOperator) contributionHistoryMergeBase(ctx context.Context, left, right string) (string, error) {
	output, err := g.contributionHistoryValue(ctx, "merge-base", "--all", left, right)
	if err != nil {
		return "", err
	}
	lines := strings.Fields(output)
	if len(lines) != 1 || !isFullCommitSHA(lines[0]) {
		return "", errContributionHistoryAmbiguous
	}
	return lines[0], nil
}

func (g *GitOperator) contributionHistoryCount(ctx context.Context, from, to string) contributionHistoryCountResult {
	rangeRef := from + ".." + to
	merges, err := g.contributionHistoryValue(ctx, "rev-list", "--merges", "-n1", rangeRef)
	if err != nil {
		return contributionHistoryCountResult{failed: errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)}
	}
	if strings.TrimSpace(merges) != "" {
		return contributionHistoryCountResult{}
	}
	output, err := g.contributionHistoryValue(ctx, "rev-list", "--count",
		fmt.Sprintf("-n%d", contributionHistoryCommitLimit+1), rangeRef)
	if err != nil {
		return contributionHistoryCountResult{failed: errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)}
	}
	count, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return contributionHistoryCountResult{failed: true}
	}
	if count > contributionHistoryCommitLimit {
		return contributionHistoryCountResult{limited: true}
	}
	return contributionHistoryCountResult{count: &count}
}

func (g *GitOperator) contributionHistoryValue(ctx context.Context, args ...string) (string, error) {
	output, err := g.contributionHistoryCommand(ctx, args...)
	return strings.TrimSpace(output), err
}

func (g *GitOperator) contributionHistoryCommand(ctx context.Context, args ...string) (string, error) {
	if err := validateGitCommandArgs(args); err != nil {
		return "", err
	}
	cmd := subproc.NewGitCommand(ctx, args...)
	cmd.Dir = g.workDir
	cmd.Env = filterGitEnv(g.environmentValues())
	output := newContributionHistoryOutputWriter(contributionHistoryOutputLimit)
	cmd.Stdout = output
	cmd.Stderr = output
	g.logger.Debug("executing bounded contribution history command", zap.Strings("args", args))
	err := subproc.RunGitClass(ctx, subproc.GitInteractive, cmd)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return output.String(), ctxErr
	}
	if output.Truncated() {
		return output.String(), errContributionHistoryLimit
	}
	if err != nil {
		return output.String(), fmt.Errorf("git command failed: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}
