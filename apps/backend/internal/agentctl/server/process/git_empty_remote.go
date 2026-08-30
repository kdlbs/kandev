package process

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/gitbootstrap"
)

const (
	emptyRemoteRemoteChangedErrorCode       = "empty_remote_remote_changed"
	emptyRemoteBasePublishFailedErrorCode   = "empty_remote_base_publish_failed"
	emptyRemoteBranchPublishFailedErrorCode = "empty_remote_branch_publish_failed"
)

type emptyRemotePublication struct {
	active    bool
	output    string
	errorCode string
	err       error
}

func (g *GitOperator) prepareEmptyRemotePublication(ctx context.Context, requestedBaseBranch string) emptyRemotePublication {
	baseBranch := g.emptyRemoteBaseBranch(requestedBaseBranch)
	if baseBranch == "" || !securityutil.IsValidBaseBranchRef(baseBranch) {
		return emptyRemotePublication{}
	}
	baseline, present, err := gitbootstrap.Validate(ctx, g.workDir, baseBranch)
	if errors.Is(err, gitbootstrap.ErrBaselineConflict) {
		return emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteRemoteChangedErrorCode,
			err:       errors.New("the local empty-remote baseline no longer matches Git history; refresh the task and retry"),
		}
	}
	if err != nil {
		return emptyRemotePublication{active: true, errorCode: emptyRemoteBasePublishFailedErrorCode, err: err}
	}
	if !present {
		return emptyRemotePublication{}
	}

	refs, err := g.advertisedOriginRefs(ctx)
	if err != nil {
		return emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteBasePublishFailedErrorCode,
			err:       fmt.Errorf("could not verify the remote before publishing the base branch: %w", err),
		}
	}
	baseRef := "refs/heads/" + baseBranch
	if len(refs) == 0 {
		output, pushErr := g.runGitCommand(ctx, "push", "origin", baseBranch)
		if pushErr != nil {
			return emptyRemotePublication{
				active:    true,
				output:    output,
				errorCode: emptyRemoteBasePublishFailedErrorCode,
				err:       fmt.Errorf("failed to publish the empty-remote base branch: %s", g.sanitizePRFailure(output, "", "")),
			}
		}
		return emptyRemotePublication{active: true, output: g.sanitizeGitPushOutput(output)}
	}
	if len(refs) == 1 && refs[baseRef] == baseline.Commit {
		return emptyRemotePublication{active: true}
	}
	return emptyRemotePublication{
		active:    true,
		errorCode: emptyRemoteRemoteChangedErrorCode,
		err:       errors.New("the remote gained Git history before the empty-remote base was published; refresh the task and retry"),
	}
}

func (g *GitOperator) emptyRemoteBaseBranch(requestedBaseBranch string) string {
	branch := strings.TrimSpace(requestedBaseBranch)
	if branch == "" && g.workspaceTracker != nil {
		branch = strings.TrimSpace(g.workspaceTracker.BaseBranch())
	}
	branch = strings.TrimPrefix(branch, "origin/")
	return strings.TrimPrefix(branch, "refs/heads/")
}

func (g *GitOperator) advertisedOriginRefs(ctx context.Context) (map[string]string, error) {
	output, err := g.runGitCommand(ctx, "ls-remote", "--refs", "origin")
	if err != nil {
		return nil, err
	}
	refs := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || !strings.HasPrefix(fields[1], "refs/") {
			if len(fields) == 0 {
				continue
			}
			return nil, errors.New("malformed remote ref advertisement")
		}
		refs[fields[1]] = fields[0]
	}
	return refs, nil
}

func combineGitOutputs(first, second string) string {
	first, second = strings.TrimSpace(first), strings.TrimSpace(second)
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + "\n" + second
}
