package process

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
)

func (g *GitOperator) resolvePullUpstream(ctx context.Context, branch string) (string, string, error) {
	if !securityutil.IsValidBranchName(branch) {
		return "", "", ErrInvalidBranchName
	}
	ref := "refs/heads/" + branch
	output, err := g.runGitCommand(ctx, "for-each-ref",
		"--format=%(refname)%00%(upstream:remotename)%00%(upstream:remoteref)", ref)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve Pull upstream: %w", err)
	}
	fields := strings.Split(strings.TrimSuffix(output, "\n"), "\x00")
	if len(fields) != 3 || fields[0] != ref {
		return "", "", errors.New("could not resolve Pull upstream")
	}
	remote, sourceRef := fields[1], fields[2]
	if (remote != "." && !securityutil.IsValidBranchName(remote)) ||
		!strings.HasPrefix(sourceRef, "refs/") || !securityutil.IsValidBranchName(sourceRef) {
		return "", "", errors.New("Pull upstream has an unsupported remote or source ref")
	}
	if remote != "." {
		if _, err := g.runGitCommand(ctx, "remote", "get-url", remote); err != nil {
			return "", "", fmt.Errorf("Pull upstream remote is unavailable: %w", err)
		}
	}
	return remote, sourceRef, nil
}
