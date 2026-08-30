package process

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/gitbootstrap"
	"go.uber.org/zap"
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
	baseBranch, baseline, result := g.validateEmptyRemoteBaseline(ctx, requestedBaseBranch)
	if result.err != nil || baseBranch == "" {
		return result
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
		return g.publishEmptyRemoteBase(ctx, baseBranch, baseline)
	}
	if refs[baseRef] == baseline.Commit {
		if retireErr := gitbootstrap.Retire(ctx, g.workDir, baseline); retireErr != nil {
			return emptyRemotePublication{
				active:    true,
				errorCode: emptyRemoteBasePublishFailedErrorCode,
				err:       fmt.Errorf("empty-remote base is already published but its local marker could not be retired: %s", g.sanitizePRFailure(retireErr.Error())),
			}
		}
		return emptyRemotePublication{active: true}
	}
	return emptyRemotePublication{
		active:    true,
		errorCode: emptyRemoteRemoteChangedErrorCode,
		err:       errors.New("the remote gained Git history before the empty-remote base was published; refresh the task and retry"),
	}
}

func (g *GitOperator) validateEmptyRemoteBaseline(
	ctx context.Context, requestedBaseBranch string,
) (string, gitbootstrap.Baseline, emptyRemotePublication) {
	requestedBaseBranch = normalizeEmptyRemoteBaseBranch(requestedBaseBranch)
	baseBranch := g.emptyRemoteBaseBranch("")
	if baseBranch == "" {
		baseBranch = requestedBaseBranch
	}
	if baseBranch == "" || !securityutil.IsValidBaseBranchRef(baseBranch) {
		return "", gitbootstrap.Baseline{}, emptyRemotePublication{}
	}
	baseline, present, err := gitbootstrap.Validate(ctx, g.workDir, baseBranch)
	if errors.Is(err, gitbootstrap.ErrBaselineConflict) {
		return "", gitbootstrap.Baseline{}, emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteRemoteChangedErrorCode,
			err:       errors.New("the local empty-remote baseline no longer matches Git history; refresh the task and retry"),
		}
	}
	if err != nil {
		return "", gitbootstrap.Baseline{}, emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteBasePublishFailedErrorCode,
			err:       err,
		}
	}
	if !present {
		return "", gitbootstrap.Baseline{}, emptyRemotePublication{}
	}
	if requestedBaseBranch != "" && requestedBaseBranch != baseBranch {
		return "", gitbootstrap.Baseline{}, emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteRemoteChangedErrorCode,
			err:       errors.New("the requested change-request base does not match the empty-remote task base; refresh the task and retry"),
		}
	}
	return baseBranch, baseline, emptyRemotePublication{active: true}
}

func (g *GitOperator) publishEmptyRemoteBase(
	ctx context.Context, baseBranch string, baseline gitbootstrap.Baseline,
) emptyRemotePublication {
	baseRef := "refs/heads/" + baseBranch
	lease := "--force-with-lease=" + baseRef + ":"
	refspec := baseline.Commit + ":" + baseRef
	output, pushErr := g.runGitCommand(ctx, "push", lease, "origin", refspec)
	if pushErr != nil {
		if currentRefs, probeErr := g.advertisedOriginRefs(ctx); probeErr == nil && len(currentRefs) > 0 {
			return emptyRemotePublication{
				active:    true,
				output:    g.sanitizeGitPushOutput(output),
				errorCode: emptyRemoteRemoteChangedErrorCode,
				err:       errors.New("the remote changed while the empty-remote base was being published; refresh the task and retry"),
			}
		}
		return emptyRemotePublication{
			active:    true,
			output:    g.sanitizeGitPushOutput(output),
			errorCode: emptyRemoteBasePublishFailedErrorCode,
			err:       fmt.Errorf("failed to publish the empty-remote base branch: %s", g.sanitizePRFailure(output, "", "")),
		}
	}

	result := g.validatePublishedEmptyRemoteBase(ctx, baseBranch, baseline)
	if result.err != nil {
		result.output = g.sanitizeGitPushOutput(output)
		return result
	}
	if retireErr := gitbootstrap.Retire(ctx, g.workDir, baseline); retireErr != nil {
		return emptyRemotePublication{
			active:    true,
			output:    g.sanitizeGitPushOutput(output),
			errorCode: emptyRemoteBasePublishFailedErrorCode,
			err:       fmt.Errorf("empty-remote base was published but its local marker could not be retired: %s", g.sanitizePRFailure(retireErr.Error())),
		}
	}
	return emptyRemotePublication{active: true, output: g.sanitizeGitPushOutput(output)}
}

func (g *GitOperator) validatePublishedEmptyRemoteBase(
	ctx context.Context, baseBranch string, baseline gitbootstrap.Baseline,
) emptyRemotePublication {
	localBaseline, localPresent, localErr := gitbootstrap.Validate(ctx, g.workDir, baseBranch)
	if localErr != nil {
		if errors.Is(localErr, gitbootstrap.ErrBaselineConflict) {
			return emptyRemotePublication{
				active:    true,
				errorCode: emptyRemoteRemoteChangedErrorCode,
				err:       errors.New("the local empty-remote baseline changed while the base was being published; refresh the task and retry"),
			}
		}
		return emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteBasePublishFailedErrorCode,
			err:       fmt.Errorf("could not revalidate the empty-remote baseline: %s", g.sanitizePRFailure(localErr.Error())),
		}
	}
	if !localPresent || localBaseline.Commit != baseline.Commit {
		return emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteRemoteChangedErrorCode,
			err:       errors.New("the local empty-remote baseline changed while the base was being published; refresh the task and retry"),
		}
	}

	publishedRefs, probeErr := g.advertisedOriginRefs(ctx)
	if probeErr != nil {
		return emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteBasePublishFailedErrorCode,
			err:       fmt.Errorf("the empty-remote base was published but could not be verified: %s", g.sanitizePRFailure(probeErr.Error())),
		}
	}
	baseRef := "refs/heads/" + baseBranch
	if len(publishedRefs) != 1 || publishedRefs[baseRef] != baseline.Commit {
		return emptyRemotePublication{
			active:    true,
			errorCode: emptyRemoteRemoteChangedErrorCode,
			err:       errors.New("the remote changed while the empty-remote base was being published; refresh the task and retry"),
		}
	}
	return emptyRemotePublication{}
}

func (g *GitOperator) emptyRemoteBaseBranch(requestedBaseBranch string) string {
	branch := strings.TrimSpace(requestedBaseBranch)
	if branch == "" && g.workspaceTracker != nil {
		branch = strings.TrimSpace(g.workspaceTracker.BaseBranch())
	}
	return normalizeEmptyRemoteBaseBranch(branch)
}

func normalizeEmptyRemoteBaseBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	branch = strings.TrimPrefix(branch, "origin/")
	return strings.TrimPrefix(branch, "refs/heads/")
}

func (g *GitOperator) advertisedOriginRefs(ctx context.Context) (map[string]string, error) {
	output, err := g.runGitCommand(ctx, "ls-remote", "--refs", "origin")
	if err != nil {
		g.logger.Debug("could not inspect remote refs before empty-remote publication",
			zap.String("error", g.sanitizePRFailure(err.Error())))
		return nil, errors.New("remote ref advertisement unavailable")
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
