package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
)

// Stable error codes for the configured-push-target contract. Callers match on
// these rather than on message text.
const (
	pushRemoteNotFoundErrorCode              = "push_remote_not_found"
	pushRemoteURLUnmatchedErrorCode          = "push_remote_url_unmatched"
	pushRemoteFanoutErrorCode                = "push_remote_fanout"
	pushRemoteConfigUnreadableErrorCode      = "push_remote_config_unreadable"
	pushRemoteContributionConflictErrorCode  = "push_remote_contribution_conflict"
	pushRemoteUpstreamUnsupportedErrorCode   = "push_remote_upstream_unsupported"
	pushBranchInvalidErrorCode               = "push_branch_invalid"
	pushBranchMismatchErrorCode              = "push_branch_mismatch"
	pushBranchMismatchAfterBaselineErrorCode = "push_branch_mismatch_after_baseline"
	pushBranchDetachedErrorCode              = "push_branch_detached"
	pushNoRemoteConfiguredErrorCode          = "push_no_remote_configured"
)

// defaultPushRemote is the remote the push contract targets when a request
// names none and no contribution routing applies.
const defaultPushRemote = "origin"

// PushOptions carries the inputs of a push or push-preflight request.
//
// Remote is an explicit push target, given either as a configured remote name
// or as a remote URL that must resolve to an already-configured remote.
// ExpectedBranch is the branch the caller believes it is publishing. Both are
// optional; absent reproduces the behavior of a request that names neither.
type PushOptions struct {
	Force          bool
	SetUpstream    bool
	Remote         string
	ExpectedBranch string
}

// normalized trims both string inputs. A value that is empty after the trim is
// absent, so a whitespace-only input is an omitted input rather than a refusal.
func (o PushOptions) normalized() PushOptions {
	o.Remote = strings.TrimSpace(o.Remote)
	o.ExpectedBranch = strings.TrimSpace(o.ExpectedBranch)
	return o
}

// pushRefusal is a refusal carrying one of the stable error codes above.
type pushRefusal struct {
	code              string
	message           string
	expectedBranch    string
	currentBranch     string
	baselinePublished bool
}

func (r *pushRefusal) apply(result *GitOperationResult) {
	result.ErrorCode = r.code
	result.Error = r.message
	result.ExpectedBranch = r.expectedBranch
	result.CurrentBranch = r.currentBranch
	result.BaselinePublished = r.baselinePublished
}

// branchMismatchRefusal reports the expected and current branches so a caller
// can see what it asked for and what the checkout was actually on. A detached
// HEAD is reported as an empty current branch, not as the literal "HEAD".
func branchMismatchRefusal(expected, current string, baselinePublished bool) *pushRefusal {
	code := pushBranchMismatchErrorCode
	if baselinePublished {
		code = pushBranchMismatchAfterBaselineErrorCode
	}
	shown := current
	if shown == "" {
		shown = "a detached HEAD"
	}
	return &pushRefusal{
		code:              code,
		message:           fmt.Sprintf("expected to publish %q but the checkout is on %s", expected, shown),
		expectedBranch:    expected,
		currentBranch:     current,
		baselinePublished: baselinePublished,
	}
}

// remoteConfig is a configured remote and its effective push URL set: the
// remote's push URLs when it has at least one, otherwise the single entry
// holding its fetch URL. A remote with neither has an empty set.
type remoteConfig struct {
	name     string
	pushURLs []string
}

// configuredRemotes reads every configured remote and its effective push URL
// set. A remote whose name fails the ref allowlist is skipped: it cannot be
// named by the name form either, so it is unreachable through this contract in
// both directions.
func (g *GitOperator) configuredRemotes(ctx context.Context) ([]remoteConfig, error) {
	output, err := g.runGitCommand(ctx, "remote")
	if err != nil {
		return nil, fmt.Errorf("failed to list remotes: %w", err)
	}
	var remotes []remoteConfig
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || !securityutil.IsValidBranchName(name) {
			continue
		}
		urls, err := g.effectivePushURLs(ctx, name)
		if err != nil {
			return nil, err
		}
		remotes = append(remotes, remoteConfig{name: name, pushURLs: urls})
	}
	return remotes, nil
}

func (g *GitOperator) effectivePushURLs(ctx context.Context, name string) ([]string, error) {
	pushURLs, err := g.remoteConfigValues(ctx, name, "pushurl")
	if err != nil {
		return nil, err
	}
	if len(pushURLs) > 0 {
		return pushURLs, nil
	}
	return g.remoteConfigValues(ctx, name, "url")
}

// remoteConfigValues reads one multi-valued remote config key. Git exits 1 when
// the key is simply unset, which is an empty set rather than a read failure.
func (g *GitOperator) remoteConfigValues(ctx context.Context, name, key string) ([]string, error) {
	output, err := g.runGitCommand(ctx, "config", "--get-all", "remote."+name+"."+key)
	if err != nil {
		if isGitConfigKeyUnset(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read remote.%s.%s: %w", name, key, err)
	}
	var values []string
	for _, line := range strings.Split(output, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values, nil
}

func isGitConfigKeyUnset(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

// resolvePushTarget maps an explicit push target onto a configured remote name.
// Resolution never mutates configuration and never places a caller-supplied URL
// on a git command line: only the resolved name reaches a command.
func (g *GitOperator) resolvePushTarget(ctx context.Context, target string) (string, *pushRefusal) {
	remotes, err := g.configuredRemotes(ctx)
	if err != nil {
		return "", &pushRefusal{
			code:    pushRemoteConfigUnreadableErrorCode,
			message: "failed to read the checkout's remote configuration",
		}
	}
	// The permissive ref allowlist is the name/URL discriminator. A value read
	// as a name still has to match an already-configured remote, so a
	// permissive reading cannot reach a destination on its own.
	if securityutil.IsValidBranchName(target) {
		for _, remote := range remotes {
			if remote.name == target {
				return remote.name, nil
			}
		}
		return "", &pushRefusal{
			code:    pushRemoteNotFoundErrorCode,
			message: fmt.Sprintf("no configured remote named %q", target),
		}
	}
	return resolvePushTargetURL(remotes, target)
}

// resolvePushTargetURL matches a URL against each remote's effective push URL
// set. Only a single-entry set matches: a remote that carries the value
// alongside other push URLs would publish to destinations the caller never
// named, so it is refused rather than matched. A caller that wants that
// fan-out names the remote instead.
func resolvePushTargetURL(remotes []remoteConfig, target string) (string, *pushRefusal) {
	var matches []string
	fanout := false
	for _, remote := range remotes {
		if len(remote.pushURLs) == 1 && remote.pushURLs[0] == target {
			matches = append(matches, remote.name)
			continue
		}
		for _, url := range remote.pushURLs {
			if url == target {
				fanout = true
				break
			}
		}
	}
	if len(matches) > 0 {
		// Several remotes can legitimately carry the same URL; they all address
		// the same destination, so first by byte order is well defined.
		sort.Strings(matches)
		return matches[0], nil
	}
	if fanout {
		return "", &pushRefusal{
			code:    pushRemoteFanoutErrorCode,
			message: "the requested push URL belongs to a remote that publishes to additional URLs; name the remote instead",
		}
	}
	return "", &pushRefusal{
		code:    pushRemoteURLUnmatchedErrorCode,
		message: "no configured remote publishes to the requested URL",
	}
}

// currentBranch reads HEAD as a symbolic ref so that a detached HEAD, reported
// as an empty branch, is distinguishable from a branch literally named "HEAD".
// rev-parse --abbrev-ref returns that literal for a detached checkout and so
// cannot make this distinction.
func (g *GitOperator) currentBranch(ctx context.Context) (string, error) {
	output, err := g.runGitCommand(ctx, "symbolic-ref", "HEAD")
	if err == nil {
		ref := strings.TrimSpace(output)
		return strings.TrimPrefix(ref, "refs/heads/"), nil
	}
	// symbolic-ref fails both for a detached HEAD and for an unreadable
	// checkout. A resolvable HEAD commit distinguishes the two.
	if _, headErr := g.runGitCommand(ctx, "rev-parse", "--verify", "HEAD"); headErr == nil {
		return "", nil
	}
	return "", fmt.Errorf("failed to read HEAD: %w", err)
}

// pushPlan is the destination a push or preflight resolved to, together with
// the refspec that reaches it.
type pushPlan struct {
	remote  string
	branch  string
	refspec string
	// explicit records that the request named a push target. A push reports
	// the destination fields only in that case, so a request naming none keeps
	// the result shape it has today.
	explicit bool
	// routed records that contribution routing selected the destination.
	routed bool
	// baselineEligible records that empty-remote first publication applies:
	// the origin path with no contribution routing.
	baselineEligible bool
}

// contributionRouted reports whether a contribution binding selects the
// destination for this operator.
func (g *GitOperator) contributionRouted() bool {
	return g.contributionDestination != nil || g.remoteContribution != nil
}

// resolvePushPlan evaluates the refusal order from the contribution conflict
// through the first expected-branch verification, then yields the destination.
// Every refusal it returns is raised before any remote is contacted, so a
// refused request leaves the checkout, its refs, and every remote unchanged.
// requireConfiguredRemote is set by preflight, which answers whether a
// destination is writable and so must report a missing default remote. A push
// leaves that to the push command itself.
func (g *GitOperator) resolvePushPlan(
	ctx context.Context, opts PushOptions, requireConfiguredRemote bool,
) (*pushPlan, *pushRefusal) {
	if refusal := g.refuseIncompatibleOptions(opts); refusal != nil {
		return nil, refusal
	}
	remote := ""
	if opts.Remote != "" {
		resolved, refusal := g.resolvePushTarget(ctx, opts.Remote)
		if refusal != nil {
			return nil, refusal
		}
		remote = resolved
	}
	if requireConfiguredRemote && remote == "" && !g.contributionRouted() {
		if refusal := g.requireDefaultRemote(ctx); refusal != nil {
			return nil, refusal
		}
	}
	current, err := g.currentBranch(ctx)
	if err != nil {
		return nil, &pushRefusal{message: err.Error()}
	}
	if remote != "" && opts.ExpectedBranch == "" && current == "" {
		return nil, &pushRefusal{
			code:    pushBranchDetachedErrorCode,
			message: "cannot publish a detached HEAD without an expected branch",
		}
	}
	if opts.ExpectedBranch != "" && opts.ExpectedBranch != current {
		return nil, branchMismatchRefusal(opts.ExpectedBranch, current, false)
	}
	return g.buildPushPlan(ctx, opts, remote, current)
}

// refuseIncompatibleOptions covers the request-shape refusals, which are the
// cheapest and so are evaluated before any configuration or checkout read.
func (g *GitOperator) refuseIncompatibleOptions(opts PushOptions) *pushRefusal {
	if opts.Remote != "" && g.contributionRouted() {
		return &pushRefusal{
			code:    pushRemoteContributionConflictErrorCode,
			message: "an explicit push target cannot be combined with a configured contribution",
		}
	}
	if opts.Remote != "" && opts.SetUpstream {
		return &pushRefusal{
			code:    pushRemoteUpstreamUnsupportedErrorCode,
			message: "an explicit push target cannot set upstream tracking",
		}
	}
	if opts.ExpectedBranch != "" && !securityutil.IsValidExpectedBranchName(opts.ExpectedBranch) {
		return &pushRefusal{
			code:    pushBranchInvalidErrorCode,
			message: fmt.Sprintf("invalid expected branch %q", opts.ExpectedBranch),
		}
	}
	return nil
}

// buildPushPlan selects the destination and refspec. The contribution paths
// keep the refspec and branch read they use today; only the explicit-target
// path is new.
func (g *GitOperator) buildPushPlan(ctx context.Context, opts PushOptions, remote, current string) (*pushPlan, *pushRefusal) {
	if remote != "" {
		branch := opts.ExpectedBranch
		if branch == "" {
			branch = current
		}
		return &pushPlan{
			remote:           remote,
			branch:           branch,
			refspec:          "HEAD:refs/heads/" + branch,
			explicit:         true,
			baselineEligible: remote == defaultPushRemote,
		}, nil
	}
	// The paths below name no explicit target and must behave exactly as they
	// do today, including reading the branch through getCurrentBranch.
	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		return nil, &pushRefusal{message: err.Error()}
	}
	switch {
	case g.contributionDestination != nil:
		return &pushPlan{
			remote:  g.contributionDestination.ContributionRemoteName(),
			branch:  branch,
			refspec: branch,
			routed:  true,
		}, nil
	case g.remoteContribution != nil:
		return &pushPlan{
			remote:  g.remoteContribution.ContributionRemoteName(),
			branch:  g.remoteContribution.HeadBranch,
			refspec: "HEAD:refs/heads/" + g.remoteContribution.HeadBranch,
			routed:  true,
		}, nil
	default:
		return &pushPlan{
			remote:           defaultPushRemote,
			branch:           branch,
			refspec:          branch,
			baselineEligible: true,
		}, nil
	}
}

// verifyExpectedBranch is the second verification. It is the last git read
// before the push command, so nothing Kandev issues can move HEAD between the
// check and the push.
func (g *GitOperator) verifyExpectedBranch(ctx context.Context, expected string, baselinePublished bool) *pushRefusal {
	if expected == "" {
		return nil
	}
	current, err := g.currentBranch(ctx)
	if err != nil {
		return &pushRefusal{message: err.Error(), baselinePublished: baselinePublished}
	}
	if current != expected {
		return branchMismatchRefusal(expected, current, baselinePublished)
	}
	return nil
}

// reportDestination populates the destination fields a successful push reports.
// A push that named no explicit target omits them so an existing consumer sees
// an unchanged result shape.
func (p *pushPlan) reportDestination(result *GitOperationResult) {
	if !p.explicit {
		return
	}
	result.PushedRemote = p.remote
	result.PushedBranch = p.branch
}

// requireDefaultRemote reports a checkout that has nowhere to publish, ahead of
// the branch guards so a caller learns that rather than a branch it cannot act
// on yet.
func (g *GitOperator) requireDefaultRemote(ctx context.Context) *pushRefusal {
	remotes, err := g.configuredRemotes(ctx)
	if err != nil {
		return &pushRefusal{
			code:    pushRemoteConfigUnreadableErrorCode,
			message: "failed to read the checkout's remote configuration",
		}
	}
	for _, remote := range remotes {
		if remote.name == defaultPushRemote {
			return nil
		}
	}
	return &pushRefusal{
		code:    pushNoRemoteConfiguredErrorCode,
		message: "the checkout has no " + defaultPushRemote + " remote to publish to",
	}
}
