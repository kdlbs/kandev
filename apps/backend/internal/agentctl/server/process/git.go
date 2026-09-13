// Package process provides git operation execution for agentctl.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ErrOperationInProgress is returned when a git operation is already in progress.
var ErrOperationInProgress = errors.New("git operation already in progress")

// ErrInvalidBranchName is returned when a branch name contains invalid characters.
var ErrInvalidBranchName = errors.New("invalid branch name")

const preflightReasonHistoryUpdateRequired = "history_update_required"

var contributionPreflightEnvironment = map[string]string{
	"GIT_TERMINAL_PROMPT": "0",
	"LANG":                "C",
	"LC_ALL":              "C",
}

// GitOperationResult represents the result of a git operation.
type GitOperationResult struct {
	Success         bool     `json:"success"`
	Operation       string   `json:"operation"`
	Output          string   `json:"output"`
	Error           string   `json:"error,omitempty"`
	ErrorCode       string   `json:"error_code,omitempty"`
	PreflightReason string   `json:"preflight_reason,omitempty"`
	ConflictFiles   []string `json:"conflict_files,omitempty"`
	RecoveryBranch  string   `json:"recovery_branch,omitempty"`
	// PushedRemote and PushedBranch name the destination a push or preflight
	// validated. A push reports them only when the request carried an explicit
	// push target, so a request that named none keeps its existing shape.
	PushedRemote string `json:"pushed_remote,omitempty"`
	PushedBranch string `json:"pushed_branch,omitempty"`
	// ExpectedBranch and CurrentBranch accompany a branch-mismatch refusal.
	// CurrentBranch is empty for a detached HEAD.
	ExpectedBranch string `json:"expected_branch,omitempty"`
	CurrentBranch  string `json:"current_branch,omitempty"`
	// BaselinePublished marks a mismatch refused after empty-remote first
	// publication already published the baseline in this request.
	BaselinePublished bool `json:"baseline_published,omitempty"`
}

// GitOperator executes git operations in a workspace directory.
type GitOperator struct {
	workDir          string
	logger           *logger.Logger
	workspaceTracker *WorkspaceTracker
	environment      func() []string
	// repoName is the multi-repo subpath this operator runs in (e.g. "kandev").
	// Empty for the workspace-root operator. Stamped on emitted commit
	// notifications so the frontend can group commits per repo.
	repoName                   string
	remoteContribution         *models.RemoteContribution
	remoteContributionErr      error
	contributionDestination    *models.ContributionDestination
	contributionDestinationErr error
	// prCreateRetryAttempts is the number of times to attempt PR creation after a
	// successful branch push. GitHub's eventual consistency can briefly return
	// "no commits between base and head" for a ref it has not yet indexed, so we
	// retry that known transient failure instead of surfacing it to the user.
	// Set to 1 to disable the backoff (push, then one create attempt).
	prCreateRetryAttempts int
	// prCreateRetryBaseDelay is the initial backoff between create attempts. Each
	// attempt waits baseDelay*attempt (linear backoff) before retrying.
	prCreateRetryBaseDelay time.Duration

	mu         sync.Mutex // Prevents concurrent git operations
	inProgress bool
	currentOp  string
}

// NewGitOperator creates a new GitOperator for the given workspace directory.
func NewGitOperator(workDir string, log *logger.Logger, workspaceTracker *WorkspaceTracker) *GitOperator {
	return &GitOperator{
		workDir:                workDir,
		logger:                 log.WithFields(zap.String("component", "git-operator")),
		workspaceTracker:       workspaceTracker,
		environment:            os.Environ,
		prCreateRetryAttempts:  3,
		prCreateRetryBaseDelay: 2 * time.Second,
	}
}

// NewGitOperatorForRepo creates a GitOperator scoped to a multi-repo subpath
// so its emitted events (e.g. commit notifications) carry the repo name.
func NewGitOperatorForRepo(workDir, repoName string, log *logger.Logger, workspaceTracker *WorkspaceTracker) *GitOperator {
	op := NewGitOperator(workDir, log, workspaceTracker)
	op.repoName = repoName
	return op
}

func (g *GitOperator) setEnvironmentProvider(provider func() []string) {
	if provider != nil {
		g.environment = provider
	}
}

func (g *GitOperator) setRemoteContribution(binding *models.RemoteContribution) {
	if binding == nil {
		return
	}
	if err := binding.Validate(); err != nil {
		g.remoteContributionErr = fmt.Errorf("invalid remote contribution binding: %w", err)
		return
	}
	copy := *binding
	g.remoteContribution = &copy
}

func (g *GitOperator) setContributionDestination(destination *models.ContributionDestination) {
	if destination == nil {
		return
	}
	if err := destination.Validate(); err != nil {
		g.contributionDestinationErr = fmt.Errorf("invalid contribution destination: %w", err)
		return
	}
	copy := *destination
	g.contributionDestination = &copy
}

func (g *GitOperator) validateContributionRemote(ctx context.Context) error {
	if g.remoteContribution == nil {
		return nil
	}
	remoteName := g.remoteContribution.ContributionRemoteName()
	urlsOutput, err := g.runGitCommand(ctx, "config", "--get-all", "remote."+remoteName+".pushurl")
	if err != nil {
		// A remote without an explicit pushurl pushes to its configured URL.
		urlsOutput, err = g.runGitCommand(ctx, "config", "--get-all", "remote."+remoteName+".url")
	}
	if err != nil {
		return errors.New("contribution remote is unavailable")
	}
	urls := strings.Split(strings.TrimSpace(urlsOutput), "\n")
	if len(urls) == 0 || urls[0] == "" {
		return errors.New("contribution remote has no configured URL")
	}
	for _, configured := range urls {
		if strings.TrimSpace(configured) != g.remoteContribution.SourceRepository.RemoteURL {
			return errors.New("contribution remote push URL does not match the validated source")
		}
	}
	return nil
}

func (g *GitOperator) validateContributionDestinationRemote(ctx context.Context) error {
	if g.contributionDestination == nil {
		return nil
	}
	remoteName := g.contributionDestination.ContributionRemoteName()
	urlsOutput, err := g.runGitCommand(ctx, "config", "--get-all", "remote."+remoteName+".pushurl")
	if err != nil {
		return errors.New("contribution destination remote is unavailable")
	}
	urls := strings.Split(strings.TrimSpace(urlsOutput), "\n")
	if len(urls) == 0 || urls[0] == "" {
		return errors.New("contribution destination remote has no configured push URL")
	}
	for _, configured := range urls {
		if strings.TrimSpace(configured) != g.contributionDestination.TargetRepository.RemoteURL {
			return errors.New("contribution destination push URL does not match the validated target")
		}
	}
	return nil
}

func (g *GitOperator) environmentValues() []string {
	if g.environment == nil {
		return os.Environ()
	}
	return g.environment()
}

func (g *GitOperator) environmentValue(key string) string {
	prefix := key + "="
	env := g.environmentValues()
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

// runGitCommand executes a git command in the workDir with defense-in-depth validation.
// Validates both flags and branch/ref arguments to prevent command injection.
func (g *GitOperator) runGitCommand(ctx context.Context, args ...string) (string, error) {
	return g.runGitCommandWithEnvironment(ctx, nil, args...)
}

func (g *GitOperator) runGitCommandWithEnvironment(
	ctx context.Context,
	environmentOverrides map[string]string,
	args ...string,
) (string, error) {
	if err := validateGitCommandArgs(args); err != nil {
		return "", err
	}

	// All args validated: flags in securityutil.IsKnownSafeGitFlag whitelist, branch names via securityutil.IsValidBranchName
	// regex, commit SHAs via securityutil.LooksLikeCommitSHA pattern, args after "--" separator skipped.
	// This defense-in-depth validation prevents injection of arbitrary commands.
	cmd := subproc.NewGitCommand(ctx, args...)
	cmd.Dir = g.workDir
	cmd.Env = withEnvironmentOverrides(filterGitEnv(g.environmentValues()), environmentOverrides)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	g.logger.Debug("executing git command", zap.Strings("args", args))

	err := subproc.RunGitClass(ctx, subproc.GitInteractive, cmd)
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}
	stderrOutput := stderr.String()
	if len(args) > 0 && args[0] == "push" {
		output = g.sanitizeGitPushOutput(output)
		stderrOutput = g.sanitizeGitPushOutput(stderrOutput)
	}

	if err != nil {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderrOutput))
	}

	return output, nil
}

func validateGitCommandArgs(args []string) error {
	// Validate that user-controlled arguments don't introduce command injection risks.
	// exec.CommandContext does not use a shell, but git still interprets unsafe flags.
	skipNextArg := false
	afterDoubleDash := false
	for i, arg := range args {
		if i == 0 || skipNextArg || afterDoubleDash {
			skipNextArg = false
			continue
		}
		nextArgIsValue, separator, err := validateGitCommandArgument(arg)
		if err != nil {
			return err
		}
		skipNextArg = nextArgIsValue
		afterDoubleDash = separator
	}
	return nil
}

func validateGitCommandArgument(arg string) (skipNextArg, separator bool, err error) {
	if strings.HasPrefix(arg, contributionLeaseFlagPrefix) {
		return false, false, validateContributionLeaseFlag(arg)
	}
	if strings.HasPrefix(arg, "-") {
		if !securityutil.IsKnownSafeGitFlag(arg) {
			return false, false, fmt.Errorf("potentially unsafe flag: %s", arg)
		}
		return arg == "-m" || arg == "--format", arg == "--", nil
	}
	if securityutil.IsKnownSafeGitLiteral(arg) || securityutil.LooksLikeCommitSHA(arg) {
		return false, false, nil
	}
	if strings.HasPrefix(arg, "HEAD:refs/heads/") {
		if !securityutil.IsValidBranchName(strings.TrimPrefix(arg, "HEAD:refs/heads/")) {
			return false, false, ErrInvalidBranchName
		}
		return false, false, nil
	}
	if source, destination, ok := strings.Cut(arg, ":refs/heads/"); ok {
		if securityutil.LooksLikeCommitSHA(source) && securityutil.IsValidBranchName(destination) {
			return false, false, nil
		}
	}
	if strings.Contains(arg, "/") {
		return false, false, securityutil.ValidateBranchReference(arg)
	}
	return false, false, nil
}

func withEnvironmentOverrides(environment []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return environment
	}
	result := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, overridden := overrides[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

// filterGitEnv removes GIT_DIR and GIT_WORK_TREE from the environment.
// This ensures that external tools like gh CLI correctly detect the repository
// from the working directory, which is essential for worktrees where these
// env vars could point to the wrong location.
func filterGitEnv(env []string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_DIR=") || strings.HasPrefix(e, "GIT_WORK_TREE=") {
			continue
		}
		result = append(result, e)
	}
	return result
}

// triggerRefresh refreshes git status in the workspace tracker immediately.
// Called after git operations like commit, push, pull, etc. to refresh the UI
// without waiting for the next poll cycle.
func (g *GitOperator) triggerRefresh() {
	if g.workspaceTracker != nil {
		g.workspaceTracker.RefreshGitStatus(context.Background())
	}
}

// getCurrentBranch returns the current branch name
func (g *GitOperator) getCurrentBranch(ctx context.Context) (string, error) {
	output, err := g.runGitCommand(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// getUpstreamRef returns the current branch's upstream tracking ref, or "" if none is set.
func (g *GitOperator) getUpstreamRef(ctx context.Context) string {
	output, err := g.runGitCommand(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

// getDefaultRemoteBranch returns "main" or "master", whichever exists on origin.
func (g *GitOperator) getDefaultRemoteBranch(ctx context.Context) string {
	if _, err := g.runGitCommand(ctx, "rev-parse", "--verify", "origin/main"); err == nil {
		return "main"
	}
	if _, err := g.runGitCommand(ctx, "rev-parse", "--verify", "origin/master"); err == nil {
		return "master"
	}
	return ""
}

// hasUncommittedChanges checks if there are uncommitted changes
func (g *GitOperator) hasUncommittedChanges(ctx context.Context) (bool, error) {
	output, err := g.runGitCommand(ctx, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check uncommitted changes: %w", err)
	}
	return strings.TrimSpace(output) != "", nil
}

// parseConflictFiles parses conflict file names from git output
func (g *GitOperator) parseConflictFiles(output string) []string {
	var conflicts []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for "CONFLICT" markers in git output
		if strings.HasPrefix(line, "CONFLICT") {
			// Extract file name from patterns like:
			// "CONFLICT (content): Merge conflict in <file>"
			// "CONFLICT (add/add): Merge conflict in <file>"
			if idx := strings.Index(line, "Merge conflict in "); idx != -1 {
				file := strings.TrimSpace(line[idx+len("Merge conflict in "):])
				if file != "" {
					conflicts = append(conflicts, file)
				}
			}
		}
	}

	return conflicts
}

// Pull performs a git pull operation.
func (g *GitOperator) Pull(ctx context.Context, rebase bool) (*GitOperationResult, error) {
	if !g.tryLock("pull") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "pull",
	}

	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	remote := "origin"
	pullBranch := branch
	if g.remoteContributionErr != nil {
		result.Error = g.remoteContributionErr.Error()
		return result, nil
	}
	if g.contributionDestinationErr != nil {
		result.Error = g.contributionDestinationErr.Error()
		return result, nil
	}
	if g.remoteContribution != nil {
		if err := g.validateContributionRemote(ctx); err != nil {
			result.Error = err.Error()
			return result, nil
		}
		remote = g.remoteContribution.ContributionRemoteName()
		pullBranch = g.remoteContribution.HeadBranch
	} else if upstream := g.getUpstreamRef(ctx); upstream == "" {
		// Use the default branch when the local branch has no upstream.
		if defaultBranch := g.getDefaultRemoteBranch(ctx); defaultBranch != "" {
			pullBranch = defaultBranch
		}
	}

	var args []string
	if rebase {
		args = []string{"pull", "--rebase", remote, pullBranch}
	} else {
		args = []string{"pull", remote, pullBranch}
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		result.ConflictFiles = g.parseConflictFiles(output)

		// For rebase conflicts, auto-abort to restore clean state
		if rebase && len(result.ConflictFiles) > 0 {
			g.logger.Info("rebase conflict detected, aborting rebase")
			if _, abortErr := g.runGitCommand(ctx, "rebase", "--abort"); abortErr != nil {
				g.logger.Warn("failed to abort rebase", zap.Error(abortErr))
			}
		}
		return result, nil
	}
	result.Success = true
	g.logger.Info("pull completed", zap.String("branch", pullBranch), zap.String("remote", remote), zap.Bool("rebase", rebase))
	return result, nil
}

// Push performs a git push operation.
func (g *GitOperator) Push(ctx context.Context, opts PushOptions) (*GitOperationResult, error) {
	opts = opts.normalized()
	if !g.tryLock("push") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "push",
	}
	if refusal := g.validateContributionState(ctx, opts.Force); refusal != nil {
		refusal.apply(result)
		return result, nil
	}

	// Every refusal below is raised before any remote is contacted, so a
	// refused request is a no-op.
	plan, refusal := g.resolvePushPlan(ctx, opts, false)
	if refusal != nil {
		refusal.apply(result)
		return result, nil
	}

	basePublication := emptyRemotePublication{}
	if plan.baselineEligible {
		basePublication = g.prepareEmptyRemotePublication(ctx, "")
		if basePublication.err != nil {
			result.Error = basePublication.err.Error()
			result.ErrorCode = basePublication.errorCode
			result.Output = basePublication.output
			return result, nil
		}
	}

	// Every remaining read happens before the second verification, so that the
	// branch read is the last git command before the push.
	shouldSetUpstream := g.resolveSetUpstream(ctx, opts, plan)
	if refusal := g.verifyExpectedBranch(ctx, opts.ExpectedBranch, basePublication.published); refusal != nil {
		refusal.apply(result)
		result.Output = basePublication.output
		return result, nil
	}

	args := []string{"push"}
	if shouldSetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.Force {
		// Use --force-with-lease for safer force push
		args = append(args, "--force-with-lease")
	}
	args = append(args, plan.remote, plan.refspec)

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		if basePublication.active {
			result.ErrorCode = emptyRemoteBranchPublishFailedErrorCode
			result.Output = combineGitOutputs(basePublication.output, output)
		}
		return result, nil
	}
	result.Output = combineGitOutputs(basePublication.output, output)

	result.Success = true
	plan.reportDestination(result)
	g.logger.Info("push completed",
		zap.String("branch", plan.branch),
		zap.String("remote", plan.remote),
		zap.Bool("force", opts.Force),
		zap.Bool("set_upstream", shouldSetUpstream),
		zap.Bool("expected_branch_supplied", opts.ExpectedBranch != ""),
		zap.Bool("explicit_target", plan.explicit))
	return result, nil
}

// validateContributionState runs the existing contribution binding checks,
// which precede every refusal this capability adds.
func (g *GitOperator) validateContributionState(ctx context.Context, force bool) *pushRefusal {
	if g.remoteContributionErr != nil {
		return pushRefusalFromError(g.remoteContributionErr)
	}
	if g.contributionDestinationErr != nil {
		return pushRefusalFromError(g.contributionDestinationErr)
	}
	if g.contributionDestination != nil {
		if err := g.validateContributionDestinationRemote(ctx); err != nil {
			return pushRefusalFromError(err)
		}
	}
	if g.contributionRouted() && force {
		return &pushRefusal{message: "force push is not allowed for a remote contribution"}
	}
	if err := g.validateContributionRemote(ctx); err != nil {
		return pushRefusalFromError(err)
	}
	return nil
}

func pushRefusalFromError(err error) *pushRefusal {
	return &pushRefusal{
		code:    classifyPushPreflightError(err),
		message: err.Error(),
	}
}

func (g *GitOperator) validateContributionSource(
	ctx context.Context,
	environmentOverrides map[string]string,
) *pushRefusal {
	if g.remoteContribution == nil {
		return nil
	}
	remote := g.remoteContribution.ContributionRemoteName()
	destinationRef := "refs/heads/" + g.remoteContribution.HeadBranch
	output, err := g.runGitCommandWithEnvironment(
		ctx,
		environmentOverrides,
		"ls-remote",
		"--refs",
		remote,
		destinationRef,
	)
	if err != nil {
		return pushRefusalFromError(err)
	}
	if strings.TrimSpace(output) == "" {
		return &pushRefusal{
			code:    models.AgentErrorCauseCodeSourceBranchMissing,
			message: "contribution source branch is missing",
		}
	}
	return nil
}

// resolveSetUpstream reads the upstream tracking ref only on the path that can
// use it. The explicit-target path never sets upstream and never reads it,
// because that flag combination is refused before the push.
func (g *GitOperator) resolveSetUpstream(ctx context.Context, opts PushOptions, plan *pushPlan) bool {
	if plan.explicit {
		return false
	}
	if plan.routed {
		return opts.SetUpstream
	}
	return opts.SetUpstream || g.getUpstreamRef(ctx) == ""
}

// PushPreflight verifies that the configured contribution remote and final
// head-branch refspec are writable without mutating the remote or local refs.
func (g *GitOperator) PushPreflight(ctx context.Context, opts PushOptions) (*GitOperationResult, error) {
	opts = opts.normalized()
	if !g.tryLock("push-preflight") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()
	result := &GitOperationResult{Operation: "push_preflight"}
	if refusal := g.validateContributionState(ctx, false); refusal != nil {
		refusal.apply(result)
		return result, nil
	}
	// Preflight verifies the expected branch once. It publishes no baseline, so
	// there is no window for a second read to close.
	plan, refusal := g.resolvePushPlan(ctx, opts, true)
	if refusal != nil {
		refusal.apply(result)
		return result, nil
	}
	if refusal := g.validateContributionSource(ctx, contributionPreflightEnvironment); refusal != nil {
		refusal.apply(result)
		return result, nil
	}

	// --no-verify: a dry-run push still invokes the local pre-push hook, which
	// can mutate the worktree or perform arbitrary side effects. Preflight must
	// not mutate anything, so the hook must not run.
	output, err := g.runGitCommandWithEnvironment(
		ctx,
		contributionPreflightEnvironment,
		"push",
		"--dry-run",
		"--no-verify",
		"--porcelain",
		plan.remote,
		plan.refspec,
	)
	result.Output = output
	if err != nil {
		if destinationRef, ok := strings.CutPrefix(plan.refspec, "HEAD:"); ok &&
			classifyPushPreflightHistoryUpdate(output, destinationRef) {
			result.PreflightReason = preflightReasonHistoryUpdateRequired
		}
		setPushPreflightError(result, err)
		return result, nil
	}
	result.Success = true
	// A preflight under contribution routing keeps the result shape it has
	// today; every other preflight reports what it validated.
	if !plan.routed {
		result.PushedRemote = plan.remote
		result.PushedBranch = plan.branch
	}
	g.logger.Info("push preflight completed",
		zap.String("branch", plan.branch),
		zap.String("remote", plan.remote),
		zap.Bool("contribution_routed", plan.routed),
		zap.Bool("explicit_target", plan.explicit))
	return result, nil
}

func setPushPreflightError(result *GitOperationResult, err error) {
	if result == nil || err == nil {
		return
	}
	result.Error = err.Error()
	result.ErrorCode = classifyPushPreflightError(err)
}

func classifyPushPreflightError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return models.AgentErrorCauseCodeTimeout
	}
	normalized := strings.ToLower(err.Error())
	switch {
	case strings.Contains(normalized, "authentication required"),
		strings.Contains(normalized, "authentication failed"),
		strings.Contains(normalized, "could not read username"):
		return models.AgentErrorCauseCodeAuthenticationRequired
	case strings.Contains(normalized, "permission denied"),
		strings.Contains(normalized, "access denied"),
		strings.Contains(normalized, "protected branch"):
		return models.AgentErrorCauseCodePermissionDenied
	case strings.Contains(normalized, "remote repository is invalid"),
		strings.Contains(normalized, "remote has no configured"),
		strings.Contains(normalized, "push url does not match"),
		strings.Contains(normalized, "invalid contribution"):
		return models.AgentErrorCauseCodeDestinationInvalid
	case strings.Contains(normalized, "could not resolve host"),
		strings.Contains(normalized, "unable to access"),
		strings.Contains(normalized, "connection refused"),
		strings.Contains(normalized, "network is unreachable"),
		strings.Contains(normalized, "remote is unavailable"):
		return models.AgentErrorCauseCodeTransportUnavailable
	default:
		return models.AgentErrorCauseCodeUnknown
	}
}

func classifyPushPreflightHistoryUpdate(output, destinationRef string) bool {
	failedStatusLines := 0
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "!" {
			continue
		}
		failedStatusLines++
		if !pushStatusTargets(fields[1:], destinationRef) || !hasRejectedStatus(fields) {
			return false
		}
		if !hasHistoryUpdateReason(fields) {
			return false
		}
	}
	return failedStatusLines == 1
}

func pushStatusTargets(fields []string, destinationRef string) bool {
	for _, field := range fields {
		candidate := strings.Trim(field, "\"'")
		if _, destination, ok := strings.Cut(candidate, ":"); ok {
			candidate = destination
		}
		if candidate == destinationRef {
			return true
		}
	}
	return false
}

func hasRejectedStatus(fields []string) bool {
	for _, field := range fields {
		if field == "[rejected]" {
			return true
		}
	}
	return false
}

func hasHistoryUpdateReason(fields []string) bool {
	for i, field := range fields {
		if field == "(non-fast-forward)" {
			return true
		}
		if field == "(fetch" && i+1 < len(fields) && fields[i+1] == "first)" {
			return true
		}
	}
	return false
}

// Rebase performs a git rebase onto the specified base branch.
func (g *GitOperator) Rebase(ctx context.Context, baseBranch string) (*GitOperationResult, error) {
	// Validate branch name to prevent command injection
	if !securityutil.IsValidBranchName(baseBranch) {
		return nil, ErrInvalidBranchName
	}

	if !g.tryLock("rebase") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "rebase",
	}

	prepareOutput, target, err := g.prepareBaseBranchTarget(ctx, baseBranch)
	if err != nil {
		result.Error = err.Error()
		result.Output = prepareOutput
		return result, nil
	}

	// Perform the rebase
	rebaseOutput, err := g.runGitCommand(ctx, "rebase", target)
	result.Output = prepareOutput + rebaseOutput

	if err != nil {
		result.Error = err.Error()
		result.ConflictFiles = g.parseConflictFiles(rebaseOutput)

		// Auto-abort rebase on conflicts to restore clean state
		if len(result.ConflictFiles) > 0 {
			g.logger.Info("rebase conflict detected, aborting rebase")
			if _, abortErr := g.runGitCommand(ctx, "rebase", "--abort"); abortErr != nil {
				g.logger.Warn("failed to abort rebase", zap.Error(abortErr))
			}
		}
		return result, nil
	}

	result.Success = true
	g.logger.Info("rebase completed", zap.String("base_branch", baseBranch))
	return result, nil
}

// Merge performs a git merge of the specified base branch.
func (g *GitOperator) Merge(ctx context.Context, baseBranch string) (*GitOperationResult, error) {
	// Validate branch name to prevent command injection
	if !securityutil.IsValidBranchName(baseBranch) {
		return nil, ErrInvalidBranchName
	}

	if !g.tryLock("merge") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "merge",
	}

	prepareOutput, target, err := g.prepareBaseBranchTarget(ctx, baseBranch)
	if err != nil {
		result.Error = err.Error()
		result.Output = prepareOutput
		return result, nil
	}

	// Perform the merge
	mergeOutput, err := g.runGitCommand(ctx, "merge", target)
	result.Output = prepareOutput + mergeOutput

	if err != nil {
		result.Error = err.Error()
		result.ConflictFiles = g.parseConflictFiles(mergeOutput)
		// For merge conflicts, leave in place so user can resolve
		// Do NOT auto-abort like we do for rebase
		return result, nil
	}

	result.Success = true
	g.logger.Info("merge completed", zap.String("base_branch", baseBranch))
	return result, nil
}

// Commit creates a git commit with the specified message.
// If stageAll is true, it stages all changes before committing.
// If amend is true, it amends the previous commit instead of creating a new one.
func (g *GitOperator) Commit(ctx context.Context, message string, stageAll bool, amend bool) (*GitOperationResult, error) {
	if !g.tryLock("commit") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "commit",
	}

	// For amend, we don't require staged changes if we're just changing the message
	if !amend {
		// Check if there are changes to commit
		hasChanges, err := g.hasUncommittedChanges(ctx)
		if err != nil {
			result.Error = err.Error()
			return result, nil
		}

		if !hasChanges {
			result.Error = "no changes to commit"
			return result, nil
		}
	}

	// Stage all changes if requested
	if stageAll {
		stageOutput, err := g.runGitCommand(ctx, "add", "-A")
		if err != nil {
			result.Error = fmt.Sprintf("failed to stage changes: %s", err.Error())
			result.Output = stageOutput
			return result, nil
		}
		result.Output = stageOutput
	}

	// Create the commit (with --amend if requested)
	args := []string{"commit", "-m", message}
	if amend {
		args = append(args, "--amend")
	}
	commitOutput, err := g.runGitCommand(ctx, args...)
	result.Output += commitOutput

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("commit completed", zap.String("message", message), zap.Bool("amend", amend))

	// Publish commit notification if we have a workspace tracker
	if g.workspaceTracker != nil {
		// Get commit details
		commitSHA, _ := g.runGitCommand(ctx, "rev-parse", "HEAD")
		parentSHA, _ := g.runGitCommand(ctx, "rev-parse", "HEAD~1")

		// Get commit info (author name|author email)
		authorInfo, _ := g.runGitCommand(ctx, "show", "-s", "--format=%an|%ae", "HEAD")
		authorParts := strings.Split(strings.TrimSpace(authorInfo), "|")
		authorName := ""
		authorEmail := ""
		if len(authorParts) >= 2 {
			authorName = authorParts[0]
			authorEmail = authorParts[1]
		}

		// Get commit stats
		filesChanged, insertions, deletions := g.getCommitStats(ctx, strings.TrimSpace(commitSHA))

		commit := &streams.GitCommitNotification{
			RepositoryName: g.repoName,
			CommitSHA:      strings.TrimSpace(commitSHA),
			ParentSHA:      strings.TrimSpace(parentSHA),
			Message:        message,
			AuthorName:     authorName,
			AuthorEmail:    authorEmail,
			FilesChanged:   filesChanged,
			Insertions:     insertions,
			Deletions:      deletions,
			CommittedAt:    time.Now().UTC(),
		}

		g.workspaceTracker.NotifyGitCommit(commit)
		// Refresh git status so the UI's "unstaged" list clears immediately.
		// NotifyGitCommit only updates cachedHeadSHA — without an explicit
		// refresh, currentStatus keeps the pre-commit "modified" entries until
		// the next poll tick (which never fires when polling is paused).
		g.triggerRefresh()
	}

	return result, nil
}

// Stage stages files for commit using git add.
// If paths is empty, stages all changes (git add -A).
func (g *GitOperator) Stage(ctx context.Context, paths []string) (*GitOperationResult, error) {
	if !g.tryLock("stage") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "stage",
	}

	var args []string
	if len(paths) == 0 {
		// Stage all changes
		args = []string{"add", "-A"}
	} else {
		// Stage specific files
		args = append([]string{"add", "--"}, paths...)
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("stage completed", zap.Int("files", len(paths)))

	// Refresh git status so the UI reflects the staged state
	if g.workspaceTracker != nil {
		g.workspaceTracker.RefreshGitStatus(ctx)
	}

	return result, nil
}

// Unstage unstages files from the index using git reset.
// If paths is empty, unstages all changes (git reset HEAD).
func (g *GitOperator) Unstage(ctx context.Context, paths []string) (*GitOperationResult, error) {
	if !g.tryLock("unstage") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "unstage",
	}

	var args []string
	if len(paths) == 0 {
		// Unstage all changes
		args = []string{"reset", "HEAD"}
	} else {
		// Unstage specific files
		args = append([]string{"reset", "HEAD", "--"}, paths...)
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("unstage completed", zap.Int("files", len(paths)))

	// Refresh git status so the UI reflects the unstaged state
	if g.workspaceTracker != nil {
		g.workspaceTracker.RefreshGitStatus(ctx)
	}

	return result, nil
}

// Discard discards changes to files, reverting them to HEAD state.
// This removes both staged and unstaged changes.
// If paths is empty, returns an error (discarding all files requires explicit confirmation).
func (g *GitOperator) Discard(ctx context.Context, paths []string) (*GitOperationResult, error) {
	if !g.tryLock("discard") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "discard",
	}

	// Require explicit paths for safety
	if len(paths) == 0 {
		result.Error = "no files specified to discard"
		return result, nil
	}

	// Separate files into categories based on their git status
	// We need to handle untracked/new files differently from tracked files
	untrackedFiles := []string{}
	trackedFiles := []string{}

	// Get status for each file to determine how to discard it
	for _, path := range paths {
		statusArgs := []string{"status", "--porcelain", "--", path}
		statusOutput, err := g.runGitCommand(ctx, statusArgs...)
		if err != nil {
			// If we can't get status, assume it's tracked and try to restore it
			trackedFiles = append(trackedFiles, path)
			continue
		}

		statusLine := strings.TrimSpace(statusOutput)
		if len(statusLine) >= 2 {
			indexStatus := statusLine[0]
			workTreeStatus := statusLine[1]

			// Untracked files (??), or added files (A ) that don't exist in HEAD
			if (indexStatus == '?' && workTreeStatus == '?') || indexStatus == 'A' {
				untrackedFiles = append(untrackedFiles, path)
			} else {
				trackedFiles = append(trackedFiles, path)
			}
		} else if statusLine == "" {
			// Empty status means file is not modified - nothing to discard
			continue
		}
	}

	outputs, errors := g.discardUntrackedFiles(ctx, untrackedFiles)
	trackedOutputs, trackedErrors := g.discardTrackedFiles(ctx, trackedFiles)
	outputs = append(outputs, trackedOutputs...)
	errors = append(errors, trackedErrors...)

	// Combine outputs and errors
	result.Output = strings.Join(outputs, "\n")
	if len(errors) > 0 {
		result.Error = strings.Join(errors, "; ")
		result.Success = false
	} else {
		result.Success = true
	}

	g.triggerRefresh()
	g.logger.Info("discard completed",
		zap.Int("total_files", len(paths)),
		zap.Int("untracked_files", len(untrackedFiles)),
		zap.Int("tracked_files", len(trackedFiles)),
		zap.Bool("success", result.Success))
	return result, nil
}

// RevertCommit undoes the latest commit using git reset --soft HEAD~1.
// The previously committed changes remain staged so the caller can review
// and re-commit or discard as needed.
func (g *GitOperator) RevertCommit(ctx context.Context, commitSHA string) (*GitOperationResult, error) {
	if !g.tryLock("revert_commit") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "revert_commit",
	}

	if errMsg := validateCommitSHA(commitSHA); errMsg != "" {
		result.Error = errMsg
		return result, nil
	}

	// Only the HEAD commit can be reverted via reset --soft.
	headSHA, err := g.runGitCommand(ctx, "rev-parse", "HEAD")
	if err != nil {
		result.Error = "failed to get HEAD: " + err.Error()
		return result, nil
	}
	if strings.TrimSpace(headSHA) != commitSHA {
		result.Error = "can only revert the latest commit"
		return result, nil
	}

	// git reset --soft HEAD~1 moves HEAD backward while keeping the committed
	// files staged. The git poller detects the backward HEAD movement and
	// automatically emits a GitResetNotification, which triggers DB cleanup and
	// the frontend commits_reset event.
	output, err := g.runGitCommand(ctx, "reset", "--soft", "HEAD~1")
	if err != nil {
		result.Error = err.Error()
		if output != "" {
			result.Output = output
		}
		return result, nil
	}

	result.Success = true
	result.Output = output
	g.logger.Info("revert commit completed",
		zap.String("commit_sha", commitSHA),
		zap.Bool("success", result.Success))
	return result, nil
}

// RenameBranch renames the current branch to a new name.
// Uses git branch -m <new_name>.
func (g *GitOperator) RenameBranch(ctx context.Context, newName string) (*GitOperationResult, error) {
	if !securityutil.IsValidBranchName(newName) {
		return nil, ErrInvalidBranchName
	}

	if !g.tryLock("rename_branch") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "rename_branch",
	}

	// Get current branch name for logging
	currentBranch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get current branch: %s", err.Error())
		return result, nil
	}

	// Rename the branch
	output, err := g.runGitCommand(ctx, "branch", "-m", newName)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("branch renamed",
		zap.String("from", currentBranch),
		zap.String("to", newName))

	// Refresh git status so the UI reflects the new branch name
	if g.workspaceTracker != nil {
		g.workspaceTracker.RefreshGitStatus(ctx)
	}

	return result, nil
}

// Reset resets HEAD to the specified commit.
// mode can be "soft" (keep changes staged), "mixed" (keep changes unstaged), or "hard" (discard all changes).
func (g *GitOperator) Reset(ctx context.Context, commitSHA string, mode string) (*GitOperationResult, error) {
	if !g.tryLock("reset") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "reset",
	}

	// Validate mode
	validModes := map[string]bool{"soft": true, "mixed": true, "hard": true}
	if !validModes[mode] {
		result.Error = fmt.Sprintf("invalid reset mode: %s (must be soft, mixed, or hard)", mode)
		return result, nil
	}

	// Validate commit SHA format
	if errMsg := validateCommitSHA(commitSHA); errMsg != "" {
		result.Error = errMsg
		return result, nil
	}

	// Validate commit SHA exists (peel to commit object)
	if _, err := g.runGitCommand(ctx, "rev-parse", "--verify", commitSHA+"^{commit}"); err != nil {
		result.Error = fmt.Sprintf("invalid commit: %s", commitSHA)
		return result, nil
	}

	// Capture current HEAD for reset notification
	previousHead, err := g.runGitCommand(ctx, "rev-parse", "HEAD")
	if err != nil {
		result.Error = "failed to resolve HEAD: " + err.Error()
		return result, nil
	}
	previousHead = strings.TrimSpace(previousHead)

	// Perform the reset
	output, err := g.runGitCommand(ctx, "reset", "--"+mode, commitSHA)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("reset completed",
		zap.String("mode", mode),
		zap.String("commit", commitSHA))

	// Send reset notification and refresh git status
	if g.workspaceTracker != nil {
		// Notify about the reset
		newHead, headErr := g.runGitCommand(ctx, "rev-parse", "HEAD")
		if headErr != nil {
			g.logger.Warn("failed to resolve HEAD after reset", zap.Error(headErr))
		} else {
			reset := &streams.GitResetNotification{
				Timestamp:      time.Now().UTC(),
				RepositoryName: g.repoName,
				PreviousHead:   previousHead,
				CurrentHead:    strings.TrimSpace(newHead),
			}
			g.workspaceTracker.NotifyGitReset(reset)
		}

		// Refresh git status
		g.workspaceTracker.RefreshGitStatus(ctx)
	}

	return result, nil
}

func (g *GitOperator) discardUntrackedFiles(ctx context.Context, paths []string) (outputs, errors []string) {
	for _, path := range paths {
		resetArgs := []string{"rm", "--cached", "--force", "--", path}
		resetOutput, resetErr := g.runGitCommand(ctx, resetArgs...)
		if resetErr != nil && !strings.Contains(resetErr.Error(), "did not match any files") {
			errors = append(errors, fmt.Sprintf("failed to unstage %s: %s", path, resetErr.Error()))
		}
		if resetOutput != "" {
			outputs = append(outputs, resetOutput)
		}
		fullPath := filepath.Join(g.workDir, path)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("failed to remove %s: %s", path, err.Error()))
		}
	}
	return outputs, errors
}

func (g *GitOperator) discardTrackedFiles(ctx context.Context, paths []string) (outputs, errors []string) {
	if len(paths) == 0 {
		return nil, nil
	}
	args := append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, paths...)
	output, err := g.runGitCommand(ctx, args...)
	if output != "" {
		outputs = append(outputs, output)
	}
	if err != nil {
		errors = append(errors, err.Error())
	}
	return outputs, errors
}

// Abort aborts an in-progress merge or rebase operation.
func (g *GitOperator) Abort(ctx context.Context, operation string) (*GitOperationResult, error) {
	if !g.tryLock("abort") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "abort",
	}

	var args []string
	switch operation {
	case "merge":
		args = []string{"merge", "--abort"}
	case "rebase":
		args = []string{"rebase", "--abort"}
	default:
		result.Error = fmt.Sprintf("unsupported operation to abort: %s (must be 'merge' or 'rebase')", operation)
		return result, nil
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("abort completed", zap.String("operation", operation))
	return result, nil
}

// tryLock attempts to acquire the operation lock without blocking.
// Returns true if the lock was acquired, false if an operation is in progress.
func (g *GitOperator) tryLock(opName string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inProgress {
		return false
	}
	g.inProgress = true
	g.currentOp = opName
	return true
}

// unlock releases the operation lock and triggers a git status refresh.
func (g *GitOperator) unlock() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inProgress = false
	g.currentOp = ""

	// Refresh git status in the workspace tracker immediately.
	// This is called after every git operation completes.
	g.triggerRefresh()
}

// PRCreateResult represents the result of a PR creation operation.
type PRCreateResult struct {
	Success      bool   `json:"success"`
	BranchPushed bool   `json:"branch_pushed,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Output       string `json:"output,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
}

// CreatePR creates a pull request using the repository host's CLI.
// It first pushes the current branch to the remote, then creates the PR.
func (g *GitOperator) CreatePR(ctx context.Context, title, body, baseBranch string, draft bool) (*PRCreateResult, error) {
	if !g.tryLock("create-pr") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &PRCreateResult{}
	if g.remoteContributionErr != nil {
		result.Error = g.remoteContributionErr.Error()
		return result, nil
	}
	if g.remoteContribution != nil {
		if err := g.validateContributionRemote(ctx); err != nil {
			result.Error = err.Error()
			return result, nil
		}
		branch, err := g.getCurrentBranch(ctx)
		if err != nil {
			result.Error = fmt.Sprintf("failed to get current branch: %s", err.Error())
			return result, nil
		}
		output, err := g.runGitCommand(ctx, "push", g.remoteContribution.ContributionRemoteName(), "HEAD:refs/heads/"+g.remoteContribution.HeadBranch)
		if err != nil {
			result.Error = fmt.Sprintf("failed to push contribution branch: %s", g.sanitizePRFailure(output, title, body))
			result.Output = g.sanitizeGitPushOutput(output)
			return result, nil
		}
		result.Success = true
		result.BranchPushed = true
		result.PRURL = g.remoteContribution.CanonicalURL
		result.Provider = g.remoteContribution.Provider
		result.Output = g.sanitizeGitPushOutput(output)
		g.logger.Info("updated existing remote contribution", zap.String("branch", branch), zap.String("provider", result.Provider))
		return result, nil
	}
	if g.contributionDestinationErr != nil {
		result.Error = g.contributionDestinationErr.Error()
		return result, nil
	}
	if g.contributionDestination != nil {
		return g.createManagedContributionPR(ctx, title, body, baseBranch, draft)
	}

	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get current branch: %s", err.Error())
		return result, nil
	}
	g.logger.Debug("current branch", zap.String("branch", branch))

	remoteURL, err := g.getOriginRemoteURL(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	g.logger.Debug("origin remote", zap.String("remote", redactRemoteURL(remoteURL)))

	provider := g.detectPRProvider(remoteURL)
	var gitLabInfo *gitLabRepoInfo
	switch provider {
	case prProviderAzureRepos:
		result.Provider = string(prProviderAzureRepos)
		if _, parseErr := parseAzureRepoInfo(remoteURL); parseErr != nil {
			result.Error = parseErr.Error()
			return result, nil
		}
	case prProviderGitHub:
		result.Provider = string(prProviderGitHub)
	case prProviderGitLab:
		result.Provider = string(prProviderGitLab)
		gitLabInfo, err = parseGitLabRepoInfo(remoteURL, g.environmentValue(gitLabHostEnv))
		if err != nil {
			result.Error = err.Error()
			return result, nil
		}
	default:
		result.Error = fmt.Sprintf(
			"unsupported git remote for PR creation: %s (GitHub, GitLab, and Azure Repos are supported)",
			redactRemoteURL(remoteURL),
		)
		return result, nil
	}

	basePublication := g.prepareEmptyRemotePublication(ctx, baseBranch)
	if basePublication.err != nil {
		result.Error = basePublication.err.Error()
		result.ErrorCode = basePublication.errorCode
		result.Output = basePublication.output
		return result, nil
	}

	pushOutput, err := g.runGitCommand(ctx, "push", "--set-upstream", "origin", "HEAD")
	if err != nil {
		sanitizedOutput := g.sanitizePRFailure(pushOutput, title, body)
		result.Error = fmt.Sprintf("failed to push branch: %s", sanitizedOutput)
		result.Output = combineGitOutputs(basePublication.output, sanitizedOutput)
		if basePublication.active {
			result.ErrorCode = emptyRemoteBranchPublishFailedErrorCode
		}
		return result, nil
	}
	result.Output = combineGitOutputs(basePublication.output, g.sanitizeGitPushOutput(pushOutput))
	result.BranchPushed = true
	g.logger.Debug("pushed branch to remote", zap.String("output", g.sanitizeGitPushOutput(pushOutput)))

	switch provider {
	case prProviderAzureRepos:
		return g.createPRWithRetryAfterPush(ctx, result, title, body,
			func() (*PRCreateResult, error) {
				return g.createAzureReposPR(ctx, result, remoteURL, branch, title, body, baseBranch, draft)
			})
	case prProviderGitHub:
		return g.createPRWithRetryAfterPush(ctx, result, title, body,
			func() (*PRCreateResult, error) {
				return g.createGitHubPR(ctx, result, branch, title, body, baseBranch, draft)
			})
	case prProviderGitLab:
		return g.createPRWithRetryAfterPush(ctx, result, title, body,
			func() (*PRCreateResult, error) {
				return g.createGitLabPR(ctx, result, gitLabInfo, branch, title, body, baseBranch, draft)
			})
	default:
		result.Error = "unsupported git remote for PR creation"
		return result, nil
	}
}

func (g *GitOperator) createManagedContributionPR(
	ctx context.Context,
	title, body, baseBranch string,
	draft bool,
) (*PRCreateResult, error) {
	result := &PRCreateResult{}
	originURL, err := g.getOriginRemoteURL(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	if g.detectPRProvider(originURL) != prProviderGitHub {
		result.Error = fmt.Sprintf("managed contribution destination requires a GitHub origin: %s", redactRemoteURL(originURL))
		return result, nil
	}
	if err := g.validateContributionDestinationRemote(ctx); err != nil {
		result.Error = err.Error()
		return result, nil
	}
	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get current branch: %s", err.Error())
		return result, nil
	}
	output, err := g.runGitCommand(ctx, "push", g.contributionDestination.ContributionRemoteName(), "HEAD:refs/heads/"+branch)
	if err != nil {
		result.Error = fmt.Sprintf("failed to push contribution destination: %s", g.sanitizePRFailure(output, title, body))
		result.Output = g.sanitizeGitPushOutput(output)
		return result, nil
	}
	result.Provider = string(prProviderGitHub)
	result.BranchPushed = true
	return g.createPRWithRetryAfterPush(ctx, result, title, body,
		func() (*PRCreateResult, error) {
			return g.createGitHubPR(ctx, result, branch, title, body, baseBranch, draft)
		})
}

// createPRWithRetryAfterPush runs a provider create call with bounded backoff.
// The branch has already been pushed by the caller, so only the known
// eventual-consistency response is retried. Other failures can be ambiguous
// after a non-idempotent create request, so they are finalized immediately.
// The first success wins; once attempts are exhausted the final partial failure
// is finalized into the existing user-facing prompt.
func (g *GitOperator) createPRWithRetryAfterPush(
	ctx context.Context,
	result *PRCreateResult,
	title, body string,
	attempt func() (*PRCreateResult, error),
) (*PRCreateResult, error) {
	attempts := g.prCreateRetryAttempts
	if attempts < 1 {
		attempts = 1
	}
	if result == nil {
		result = &PRCreateResult{}
	}
	var last = result
	var lastErr error
	for attemptIdx := range attempts {
		i := attemptIdx + 1
		if attemptIdx > 0 {
			delay := prCreateRetryDelay(g.prCreateRetryBaseDelay, attemptIdx)
			if !sleepCtx(ctx, delay) {
				return finalizePRCreationAfterPush(last, lastErr)
			}
		}
		resetPRCreateAttemptResult(result)
		attemptResult, attemptErr := attempt()
		if attemptResult == nil {
			attemptResult = result
		}
		if attemptResult.Provider == "" {
			attemptResult.Provider = result.Provider
		}
		if result.BranchPushed {
			attemptResult.BranchPushed = true
		}
		last, lastErr = attemptResult, attemptErr
		if lastErr == nil && last != nil && last.Success {
			return last, nil
		}
		if !isRetryablePRCreationFailure(lastErr, last) || attemptIdx == attempts-1 {
			return finalizePRCreationAfterPush(last, lastErr)
		}
		g.logger.Warn("PR creation attempt failed after push; retrying",
			zap.Int("attempt", i),
			zap.Int("total_attempts", attempts),
			zap.String("error", g.sanitizePRFailure(errOrEmpty(lastErr, last), title, body)),
		)
	}
	return finalizePRCreationAfterPush(last, lastErr)
}

func prCreateRetryDelay(base time.Duration, retryIndex int) time.Duration {
	if retryIndex < 1 {
		return 0
	}
	return base * time.Duration(retryIndex)
}

func resetPRCreateAttemptResult(result *PRCreateResult) {
	if result == nil {
		return
	}
	result.Success = false
	result.PRURL = ""
	result.Output = ""
	result.Error = ""
}

func isRetryablePRCreationFailure(err error, result *PRCreateResult) bool {
	if result == nil {
		return false
	}
	message := strings.ToLower(errOrEmpty(err, result))
	if !strings.Contains(message, "no commits between") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(result.Provider)) {
	case string(prProviderGitHub), string(prProviderGitLab), string(prProviderAzureRepos):
		return true
	default:
		return false
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// errOrEmpty renders a create failure as a single string for logging: the
// explicit error if present, otherwise the result's Error field.
func errOrEmpty(err error, result *PRCreateResult) string {
	if err != nil {
		return err.Error()
	}
	if result != nil {
		switch {
		case result.Error != "" && result.Output != "":
			return result.Error + "\n" + result.Output
		case result.Error != "":
			return result.Error
		default:
			return result.Output
		}
	}
	return ""
}

func finalizePRCreationAfterPush(result *PRCreateResult, createErr error) (*PRCreateResult, error) {
	if result == nil {
		result = &PRCreateResult{}
	}
	if createErr == nil && result.Success {
		return result, nil
	}
	requestName := "pull request"
	if result.Provider == string(prProviderGitLab) {
		requestName = "merge request"
	}
	result.Success = false
	result.BranchPushed = true
	result.PRURL = ""
	result.Output = ""
	result.Error = "branch was pushed; retry " + requestName + " creation"
	return result, nil
}

// parseStatSummary parses a git --shortstat / --stat summary line like
// " 3 files changed, 10 insertions(+), 5 deletions(-)" and returns the counts.
func parseStatSummary(summary string) (filesChanged, insertions, deletions int) {
	if idx := strings.Index(summary, " file"); idx > 0 {
		part := strings.TrimSpace(summary[:idx])
		parts := strings.Fields(part)
		if len(parts) > 0 {
			_, _ = fmt.Sscanf(parts[len(parts)-1], "%d", &filesChanged)
		}
	}
	if idx := strings.Index(summary, " insertion"); idx > 0 {
		start := strings.LastIndex(summary[:idx], " ") + 1
		if start > 0 && start < idx {
			_, _ = fmt.Sscanf(summary[start:idx], "%d", &insertions)
		}
	}
	if idx := strings.Index(summary, " deletion"); idx > 0 {
		start := strings.LastIndex(summary[:idx], " ") + 1
		if start > 0 && start < idx {
			_, _ = fmt.Sscanf(summary[start:idx], "%d", &deletions)
		}
	}
	return filesChanged, insertions, deletions
}

// getCommitStats returns the number of files changed, insertions, and deletions for a commit
func (g *GitOperator) getCommitStats(ctx context.Context, commitSHA string) (filesChanged, insertions, deletions int) {
	// git show --stat --format="" HEAD gives us the stat summary
	output, err := g.runGitCommand(ctx, "show", "--stat", "--format=", commitSHA)
	if err != nil {
		return 0, 0, 0
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return 0, 0, 0
	}

	return parseStatSummary(lines[len(lines)-1])
}
