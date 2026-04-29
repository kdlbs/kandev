package improvekandev

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitHubInfo exposes the authenticated user's login and write-access status
// for a given repository. It is used during bootstrap to tell the contributor
// whether they will need a fork to push their changes (which they will, in
// every case except a kdlbs/kandev maintainer).
type GitHubInfo interface {
	GetAuthenticatedLogin(ctx context.Context) (string, error)
	HasRepoWriteAccess(ctx context.Context, owner, name string) (bool, error)
}

// defaultGitHubInfo shells out to the gh CLI. The improve-kandev dialog
// gates entry on gh auth being present, so by the time bootstrap runs gh
// is expected to be installed and authenticated.
type defaultGitHubInfo struct{}

func newDefaultGitHubInfo() GitHubInfo { return &defaultGitHubInfo{} }

func (d *defaultGitHubInfo) GetAuthenticatedLogin(ctx context.Context) (string, error) {
	out, err := runGH(ctx, "api", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (d *defaultGitHubInfo) HasRepoWriteAccess(ctx context.Context, owner, name string) (bool, error) {
	out, err := runGH(ctx, "api", fmt.Sprintf("repos/%s/%s", owner, name), "--jq", ".permissions.push")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}

const ghTimeout = 10 * time.Second

func runGH(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("gh %s: %w: %s", args[0], err, stderr.String())
	}
	return stdout.String(), nil
}
