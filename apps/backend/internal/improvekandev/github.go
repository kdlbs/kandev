package improvekandev

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitHubInfo exposes the authenticated user's login, write-access status,
// and existing-fork lookup. It is used during bootstrap to tell the
// contributor whether forking kdlbs/kandev will work for them when they
// lack push access on the upstream repo. The EMU-restriction signal is
// derived from the login itself (see isEMULogin) and does not require an
// extra API call.
type GitHubInfo interface {
	GetAuthenticatedLogin(ctx context.Context) (string, error)
	HasRepoWriteAccess(ctx context.Context, owner, name string) (bool, error)
	UserHasFork(ctx context.Context, login, name string) (bool, error)
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

// UserHasFork returns true if a repo named name exists under login and is a
// fork. A 404 from GitHub is treated as a missing fork (false, nil) so callers
// can distinguish "no fork yet" from a real error.
func (d *defaultGitHubInfo) UserHasFork(ctx context.Context, login, name string) (bool, error) {
	out, err := runGH(ctx, "api", fmt.Sprintf("repos/%s/%s", login, name), "--jq", ".fork")
	if err != nil {
		if isGHNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}

// isEMULogin reports whether login looks like a GitHub Enterprise Managed
// User shortname. EMU account usernames are always of the form
// "{username}_{enterprise-shortcode}"; regular GitHub usernames cannot
// contain underscores, so the presence of an underscore is a definitive
// signal. EMU accounts typically cannot fork repositories outside their
// owning enterprise, so the contribution flow will fail at the PR step.
func isEMULogin(login string) bool {
	return strings.Contains(login, "_")
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

// isGHNotFound reports whether err originates from gh CLI receiving an HTTP
// 404 from the GitHub API. The default error format embeds the stderr output,
// where gh prints e.g. "HTTP 404: Not Found (https://api.github.com/...)".
func isGHNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "HTTP 404")
}
