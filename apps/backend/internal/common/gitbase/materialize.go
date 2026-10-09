package gitbase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	ErrorInvalidTarget   = "invalid_target"
	ErrorRemoteCollision = "remote_collision"
	ErrorRemoteSetup     = "remote_setup_failed"
	ErrorFetch           = "fetch_failed"
	ErrorRefUnavailable  = "ref_unavailable"
	ErrorOIDMismatch     = "oid_mismatch"
	gitSSHUser           = "git"
)

// GitRunner executes one Git command in the caller's repository and credential
// scope. Callers provide the repository directory through their runner.
type GitRunner func(context.Context, ...string) (string, error)

type commandExitCoder interface {
	ExitCode() int
}

// Materialization identifies the exact remote-tracking commit fetched for a
// qualified PR base.
type Materialization struct {
	RemoteName string
	Ref        string
	OID        string
}

// PullRequestHead identifies the immutable commit observed after fetching a
// GitHub pull-request head into its comparison-only ref.
type PullRequestHead struct {
	Ref string
	OID string
}

// Error retains a bounded failure category while preserving cancellation and
// the underlying command error for the owning subsystem.
type Error struct {
	Code string
	err  error
}

func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return e.Code
	}
	return e.err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func ErrorCode(err error) string {
	var targetErr *Error
	if errors.As(err, &targetErr) {
		return targetErr.Code
	}
	return ""
}

func failure(code string, err error) error {
	return &Error{Code: code, err: err}
}

// Materialize fetches only the validated target branch into its deterministic
// comparison ref. It never updates origin or a user branch. An observed OID is
// checked against the fetched commit before the ref is returned.
func Materialize(ctx context.Context, run GitRunner, base models.PRBase) (Materialization, error) {
	if run == nil {
		return Materialization{}, failure(ErrorInvalidTarget, errors.New("git runner is required"))
	}
	if err := base.Validate(); err != nil {
		return Materialization{}, failure(ErrorInvalidTarget, fmt.Errorf("qualified PR base is invalid: %w", err))
	}
	target := base.Target
	remoteURL, err := comparisonRemoteURLForCheckoutTransport(ctx, run, target)
	if err != nil {
		return Materialization{}, failure(ErrorRemoteSetup, fmt.Errorf("comparison remote transport could not be resolved: %w", err))
	}
	remoteName, err := ensureComparisonRemote(ctx, run, target, remoteURL)
	if err != nil {
		return Materialization{}, err
	}
	ref := target.ComparisonRef()
	refspec := "+refs/heads/" + target.TargetBranch + ":" + ref
	if _, err := run(ctx, "fetch", "--no-tags", remoteName, refspec); err != nil {
		return Materialization{}, failure(ErrorFetch, fmt.Errorf("comparison target fetch failed: %w", err))
	}
	output, err := run(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return Materialization{}, failure(ErrorRefUnavailable, fmt.Errorf("comparison target ref unavailable: %w", err))
	}
	actualOID := strings.TrimSpace(output)
	if actualOID == "" {
		return Materialization{}, failure(ErrorRefUnavailable, errors.New("comparison target ref resolved to an empty object ID"))
	}
	if base.OID != "" && !strings.EqualFold(actualOID, base.OID) {
		return Materialization{}, failure(ErrorOIDMismatch, fmt.Errorf("comparison target OID changed from %s to %s", base.OID, actualOID))
	}
	return Materialization{RemoteName: remoteName, Ref: ref, OID: actualOID}, nil
}

func ensureComparisonRemote(
	ctx context.Context, run GitRunner, target models.ComparisonTarget, remoteURL string,
) (string, error) {
	remoteName := target.ComparisonRemoteName()
	configuredURL, remoteErr := run(ctx, "config", "--get", "remote."+remoteName+".url")
	if err := configureComparisonRemote(ctx, run, target, remoteName, remoteURL, configuredURL, remoteErr); err != nil {
		return "", err
	}
	if _, err := run(ctx, "config", "remote."+remoteName+".pushurl", "DISABLED"); err != nil {
		return "", failure(ErrorRemoteSetup, fmt.Errorf("comparison remote push protection failed: %w", err))
	}
	return remoteName, nil
}

func configureComparisonRemote(
	ctx context.Context, run GitRunner, target models.ComparisonTarget,
	remoteName, remoteURL, configuredURL string, readErr error,
) error {
	if readErr != nil {
		var exitCoder commandExitCoder
		if !errors.As(readErr, &exitCoder) || exitCoder.ExitCode() != 1 {
			return failure(ErrorRemoteSetup, fmt.Errorf("comparison remote configuration could not be read: %w", readErr))
		}
		if _, err := run(ctx, "remote", "add", "--no-tags", remoteName, remoteURL); err != nil {
			return failure(ErrorRemoteSetup, fmt.Errorf("comparison remote setup failed: %w", err))
		}
		return nil
	}
	if strings.TrimSpace(configuredURL) == remoteURL {
		return nil
	}
	if !sameGitHubComparisonRepository(configuredURL, target.TargetRepository.Host, target.TargetRepository.Path) {
		return failure(ErrorRemoteCollision, fmt.Errorf("comparison remote collision for %s", remoteName))
	}
	if _, err := run(ctx, "remote", "set-url", remoteName, remoteURL); err != nil {
		return failure(ErrorRemoteSetup, fmt.Errorf("comparison remote transport update failed: %w", err))
	}
	return nil
}

// FetchPullRequestHead fetches GitHub's pull-request head snapshot from the
// validated base repository. The returned OID remains stable if another fetch
// later updates the comparison ref.
func FetchPullRequestHead(ctx context.Context, run GitRunner, target models.ComparisonTarget) (PullRequestHead, error) {
	if run == nil {
		return PullRequestHead{}, failure(ErrorInvalidTarget, errors.New("git runner is required"))
	}
	if err := target.Validate(); err != nil {
		return PullRequestHead{}, failure(ErrorInvalidTarget, fmt.Errorf("comparison target is invalid: %w", err))
	}
	if target.Provider != models.ComparisonTargetProviderGitHub || target.Kind != models.ComparisonTargetKindPullRequest {
		return PullRequestHead{}, failure(ErrorInvalidTarget, errors.New("target is not a GitHub pull request"))
	}
	remoteName := target.ComparisonRemoteName()
	ref := fmt.Sprintf("refs/remotes/%s/pull/%d/head", remoteName, target.Number)
	refspec := fmt.Sprintf("+refs/pull/%d/head:%s", target.Number, ref)
	if _, err := run(ctx, "fetch", "--no-tags", remoteName, refspec); err != nil {
		return PullRequestHead{}, failure(ErrorFetch, fmt.Errorf("pull request head fetch failed: %w", err))
	}
	output, err := run(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return PullRequestHead{}, failure(ErrorRefUnavailable, fmt.Errorf("pull request head ref unavailable: %w", err))
	}
	oid := strings.TrimSpace(output)
	if oid == "" {
		return PullRequestHead{}, failure(ErrorRefUnavailable, errors.New("pull request head ref resolved to an empty object ID"))
	}
	return PullRequestHead{Ref: ref, OID: oid}, nil
}

func comparisonRemoteURLForCheckoutTransport(
	ctx context.Context, run GitRunner, target models.ComparisonTarget,
) (string, error) {
	originURL, err := run(ctx, "config", "--get", "remote.origin.url")
	if err != nil {
		var exitCoder commandExitCoder
		if errors.As(err, &exitCoder) && exitCoder.ExitCode() == 1 {
			return target.TargetRepository.RemoteURL, nil
		}
		return "", err
	}
	if sshURL := githubSSHComparisonURL(originURL, target.TargetRepository.Host, target.TargetRepository.Path); sshURL != "" {
		return sshURL, nil
	}
	return target.TargetRepository.RemoteURL, nil
}

func githubSSHComparisonURL(originURL, providerHost, repositoryPath string) string {
	if !strings.EqualFold(strings.TrimSpace(providerHost), "github.com") || repositoryPath == "" {
		return ""
	}
	if parsed, ok := parseGitHubSSHURL(originURL); ok {
		parsed.Path = "/" + strings.Trim(repositoryPath, "/") + ".git"
		parsed.RawPath = ""
		return parsed.String()
	}
	host, _, ok := parseGitHubSCPURL(originURL)
	if !ok {
		return ""
	}
	return gitSSHUser + "@" + host + ":" + strings.Trim(repositoryPath, "/") + ".git"
}

func sameGitHubComparisonRepository(raw, providerHost, repositoryPath string) bool {
	if !strings.EqualFold(strings.TrimSpace(providerHost), "github.com") || repositoryPath == "" {
		return false
	}
	host, path, ok := githubComparisonRemoteIdentity(raw)
	if !ok {
		return false
	}
	path = strings.Trim(strings.TrimSuffix(path, ".git"), "/")
	return strings.EqualFold(host, "github.com") && strings.EqualFold(path, strings.Trim(repositoryPath, "/"))
}

func githubComparisonRemoteIdentity(raw string) (string, string, bool) {
	value := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(strings.ToLower(value), "ssh://"):
		parsed, ok := parseGitHubSSHURL(value)
		if !ok {
			return "", "", false
		}
		return parsed.Hostname(), parsed.Path, true
	case strings.HasPrefix(strings.ToLower(value), "https://"):
		return parseGitHubHTTPSURL(value)
	default:
		return parseGitHubSCPURL(value)
	}
}

func parseGitHubSSHURL(value string) (*url.URL, bool) {
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "ssh") ||
		!strings.EqualFold(parsed.Hostname(), "github.com") || parsed.User == nil ||
		parsed.User.Username() != gitSSHUser || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, false
	}
	if _, hasPassword := parsed.User.Password(); hasPassword {
		return nil, false
	}
	return parsed, true
}

func parseGitHubHTTPSURL(value string) (string, string, bool) {
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || !strings.EqualFold(parsed.Host, "github.com") {
		return "", "", false
	}
	return parsed.Host, parsed.Path, true
}

func parseGitHubSCPURL(value string) (string, string, bool) {
	user, hostAndPath, found := strings.Cut(value, "@")
	if !found || user != gitSSHUser {
		return "", "", false
	}
	host, path, found := strings.Cut(hostAndPath, ":")
	if !found || !strings.EqualFold(host, "github.com") || strings.TrimSpace(path) == "" {
		return "", "", false
	}
	return host, path, true
}
