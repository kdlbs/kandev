package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
)

// Reclaiming a remote task directory is the most destructive thing Kandev does
// on a machine it does not own, and the failure mode is silent loss of a user's
// work. Every decision in this file is therefore biased the same way: a probe
// that cannot answer preserves the directory. "Clean working tree" is never on
// its own sufficient evidence — a tree can be spotless and still hold commits
// that were never pushed anywhere.
//
// This file deliberately has no callers inside the executor stop path.
// SSHExecutor.StopInstance is per session, swallows errors for targets already
// marked terminal, and cannot see the task graph that decides directory
// ownership. Removal is driven from the durable task-resource cleanup job
// instead; see the system design at
// docs/specs/executors/system-design/remote-task-directory-reclamation.md.

const (
	// sshReclaimTimeout bounds one directory's probe-and-remove attempt. The
	// cleanup job retries, so a slow host costs a retry rather than a stuck
	// worker.
	sshReclaimTimeout = 2 * time.Minute

	// Markers for the post-removal existence check. The remote command always
	// exits zero and reports through stdout, so a transport error stays
	// distinguishable from "the directory is still there".
	sshReclaimExistsMarker = "exists"
	sshReclaimGoneMarker   = "gone"

	// sshReclaimTasksSegment is the fixed directory level between a profile's
	// workspace root and a task directory.
	sshReclaimTasksSegment = "tasks"
)

// ErrSSHReclaimPathRefused is returned when a removal target is not exactly one
// path segment beneath <workdir_root>/tasks/. It means persisted metadata does
// not describe a directory Kandev manages, which is a state error worth
// surfacing loudly rather than a safety skip worth recording quietly.
var ErrSSHReclaimPathRefused = errors.New("ssh reclaim: refusing to remove path outside the managed task root")

// SSHReclaimOutcome is what happened to one task directory.
type SSHReclaimOutcome string

const (
	SSHReclaimOutcomeRemoved SSHReclaimOutcome = "removed"
	SSHReclaimOutcomeSkipped SSHReclaimOutcome = "skipped"
	SSHReclaimOutcomeFailed  SSHReclaimOutcome = "failed"
)

// SSHReclaimSkipReason enumerates why a directory was preserved. A skip is a
// correct result, not an error: it is recorded and the cleanup job still
// succeeds. Only SSHReclaimSkipDisabled and SSHReclaimSkipShared are decided
// above this file, by the cleanup-job phase that owns the profile setting and
// the task graph.
type SSHReclaimSkipReason string

const (
	SSHReclaimSkipDisabled        SSHReclaimSkipReason = "disabled"
	SSHReclaimSkipShared          SSHReclaimSkipReason = "shared"
	SSHReclaimSkipDirtyWorktree   SSHReclaimSkipReason = "dirty_worktree"
	SSHReclaimSkipUnpushedCommits SSHReclaimSkipReason = "unpushed_commits"
	SSHReclaimSkipStashPresent    SSHReclaimSkipReason = "stash_present"
	SSHReclaimSkipNotACheckout    SSHReclaimSkipReason = "not_a_checkout"
	SSHReclaimSkipProbeFailed     SSHReclaimSkipReason = "probe_failed"
)

// SSHReclaimVerdict is the answer to "is this directory disposable".
type SSHReclaimVerdict struct {
	Safe   bool
	Reason SSHReclaimSkipReason
	Detail string
}

func unsafeVerdict(reason SSHReclaimSkipReason, detail string) SSHReclaimVerdict {
	return SSHReclaimVerdict{Reason: reason, Detail: detail}
}

// sshRemoteRunner is the single seam through which this file touches a remote
// host. Tests substitute a fake, so no unit test opens an SSH connection and no
// unit test can remove anything from the machine running the suite.
type sshRemoteRunner interface {
	Run(ctx context.Context, cmd string) (stdout, stderr string, err error)
}

type sshDirectRunner interface {
	RunDirect(ctx context.Context, cmd string) (stdout, stderr string, err error)
}

type sshClientRunner struct {
	client *ssh.Client
	shell  string
}

// Run executes under a login shell for the same reason ProbeRemoteBinary does:
// git is frequently installed via a version manager that only a login shell's
// profile puts on PATH, and a missing git must read as "cannot answer" rather
// than being caused by us.
func (r sshClientRunner) Run(ctx context.Context, cmd string) (string, string, error) {
	return runSSHCommand(ctx, r.client, WrapLoginShell(r.shell, cmd))
}

func (r sshClientRunner) RunDirect(ctx context.Context, cmd string) (string, string, error) {
	return runSSHCommand(ctx, r.client, cmd)
}

// SSHTaskDirReclaimer probes and removes one remote task directory.
type SSHTaskDirReclaimer struct {
	runner sshRemoteRunner
	logger *logger.Logger
}

// NewSSHTaskDirReclaimer builds a reclaimer over a live SSH connection.
func NewSSHTaskDirReclaimer(client *ssh.Client, shell string, log *logger.Logger) *SSHTaskDirReclaimer {
	return &SSHTaskDirReclaimer{runner: sshClientRunner{client: client, shell: shell}, logger: log}
}

// Reclaim validates the target, probes it, and removes it only when every
// checkout beneath it positively demonstrates it holds nothing at risk.
//
// The three return values are deliberately distinct. An unsafe verdict is
// (skipped, verdict, nil) — a correct outcome the caller records without
// retrying. A removal that did not happen is (failed, verdict, err) — the
// caller must propagate it so the cleanup job retries rather than reporting a
// reclamation that never occurred.
func (r *SSHTaskDirReclaimer) Reclaim(
	ctx context.Context,
	workdirRoot string,
	taskDir string,
) (SSHReclaimOutcome, SSHReclaimVerdict, error) {
	ctx, cancel := context.WithTimeout(ctx, sshReclaimTimeout)
	defer cancel()

	root, err := r.resolveWorkdirRoot(ctx, workdirRoot)
	if err != nil {
		return SSHReclaimOutcomeFailed, SSHReclaimVerdict{}, err
	}
	if err := validateSSHReclaimPath(root, taskDir); err != nil {
		return SSHReclaimOutcomeFailed, SSHReclaimVerdict{}, err
	}

	verdict := r.probe(ctx, strings.TrimRight(taskDir, "/"))
	if !verdict.Safe {
		r.log("preserving remote task directory", taskDir, string(verdict.Reason), verdict.Detail)
		return SSHReclaimOutcomeSkipped, verdict, nil
	}
	if err := r.remove(ctx, strings.TrimRight(taskDir, "/")); err != nil {
		return SSHReclaimOutcomeFailed, verdict, err
	}
	r.log("removed remote task directory", taskDir, string(SSHReclaimOutcomeRemoved), "")
	return SSHReclaimOutcomeRemoved, verdict, nil
}

// resolveWorkdirRoot reproduces the launch-time root resolution so the path
// guard compares like with like. ensureRemoteTaskDir builds the task directory
// from the *expanded* root, while the profile stores the unexpanded form (and
// stores nothing at all when the profile leaves it blank). Comparing the
// recorded absolute directory against a literal "~/.kandev" would refuse every
// default-configured host, so the tilde is expanded against the remote $HOME
// exactly as expandRemoteHome does at launch.
//
// A root that needs no expansion issues no remote command, which is what keeps
// a refused path from touching the host at all.
func (r *SSHTaskDirReclaimer) resolveWorkdirRoot(ctx context.Context, workdirRoot string) (string, error) {
	root := strings.TrimSpace(workdirRoot)
	if root == "" {
		root = sshDefaultWorkdir
	}
	if root != "~" && !strings.HasPrefix(root, "~/") {
		return root, nil
	}
	var out string
	var err error
	if direct, ok := r.runner.(sshDirectRunner); ok {
		out, _, err = direct.RunDirect(ctx, sshReclaimHomeCmd())
	} else {
		out, _, err = r.runner.Run(ctx, sshReclaimHomeCmd())
	}
	if err != nil {
		return "", fmt.Errorf("ssh reclaim: resolve remote $HOME: %w", err)
	}
	home := strings.TrimSpace(out)
	if home == "" {
		return "", errors.New("ssh reclaim: remote $HOME is empty")
	}
	if root == "~" {
		return home, nil
	}
	return home + "/" + strings.TrimPrefix(root, "~/"), nil
}

func (r *SSHTaskDirReclaimer) log(msg, taskDir, outcome, detail string) {
	if r.logger == nil {
		return
	}
	r.logger.Info(msg,
		zap.String("remote_task_dir", taskDir),
		zap.String("outcome", outcome),
		zap.String("detail", detail))
}

// probe answers whether the task directory holds anything a user could not
// recover once it is gone. Every checkout must pass; the first failure vetoes
// the whole directory.
func (r *SSHTaskDirReclaimer) probe(ctx context.Context, taskDir string) SSHReclaimVerdict {
	checkouts, err := r.discoverCheckouts(ctx, taskDir)
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, err.Error())
	}
	for _, dir := range checkouts {
		if verdict := r.probeCheckout(ctx, dir); !verdict.Safe {
			return verdict
		}
	}
	return SSHReclaimVerdict{Safe: true}
}

// discoverCheckouts returns the task directory root plus every direct child
// holding a .git entry, matching the remote layout the SSH executor creates.
// An enumeration that cannot run is an error, not an empty list — reporting no
// children for a directory we could not read would let the root's own verdict
// speak for repositories we never looked at.
func (r *SSHTaskDirReclaimer) discoverCheckouts(ctx context.Context, taskDir string) ([]string, error) {
	out, _, err := r.runner.Run(ctx, sshReclaimChildReposCmd(taskDir))
	if err != nil {
		return nil, fmt.Errorf("enumerate child repositories: %w", err)
	}
	dirs := []string{taskDir}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		child, ok := strings.CutSuffix(line, "/.git")
		if !ok {
			return nil, fmt.Errorf("unexpected child repository entry %q", line)
		}
		dirs = append(dirs, child)
	}
	return dirs, nil
}

// probeCheckout runs the four read-only probes against one checkout. They are
// separate round trips on purpose: on a path that decides a deletion, knowing
// exactly which probe objected is worth more than saving a round trip.
func (r *SSHTaskDirReclaimer) probeCheckout(ctx context.Context, dir string) SSHReclaimVerdict {
	// A directory that is not a checkout is inconclusive rather than empty: it
	// means preparation failed or the layout is unrecognized, and it may hold
	// arbitrary user content we have no way to appraise.
	out, _, err := r.runner.Run(ctx, sshReclaimIsCheckoutCmd(dir))
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": "+err.Error())
	}
	if strings.TrimSpace(out) != boolStringTrue {
		return unsafeVerdict(SSHReclaimSkipNotACheckout, dir)
	}
	if verdict, ok := r.probeWorktreeStatus(ctx, dir); !ok {
		return verdict
	}
	if verdict, ok := r.probeUnpushed(ctx, dir); !ok {
		return verdict
	}
	if verdict, ok := r.probeEmptyOutput(ctx, dir, sshReclaimStashCmd(dir), SSHReclaimSkipStashPresent); !ok {
		return verdict
	}
	return SSHReclaimVerdict{Safe: true}
}

func (r *SSHTaskDirReclaimer) probeWorktreeStatus(ctx context.Context, dir string) (SSHReclaimVerdict, bool) {
	out, _, err := r.runner.Run(ctx, sshReclaimStatusCmd(dir))
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": "+err.Error()), false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isKandevRuntimeStatus(line) {
			continue
		}
		return unsafeVerdict(SSHReclaimSkipDirtyWorktree, dir), false
	}
	return SSHReclaimVerdict{Safe: true}, true
}

func isKandevRuntimeStatus(line string) bool {
	const ignoredPrefix = "!! .kandev/"
	return strings.HasPrefix(line, ignoredPrefix)
}

// probeEmptyOutput runs a probe whose safe answer is "no output".
func (r *SSHTaskDirReclaimer) probeEmptyOutput(
	ctx context.Context,
	dir, cmd string,
	reason SSHReclaimSkipReason,
) (SSHReclaimVerdict, bool) {
	out, _, err := r.runner.Run(ctx, cmd)
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": "+err.Error()), false
	}
	if strings.TrimSpace(out) != "" {
		return unsafeVerdict(reason, dir), false
	}
	return SSHReclaimVerdict{Safe: true}, true
}

// probeUnpushed is the load-bearing check, and the reason a clean `git status`
// is not enough on its own. It counts commits reachable from HEAD or any local
// branch that no remote-tracking ref contains — exactly the work that would be
// lost. HEAD is named explicitly because --branches alone misses a detached
// head.
//
// Remote-tracking refs are a local cache and are deliberately not refreshed:
// the question is "was this pushed at some point", and fetching during cleanup
// would need network and credentials for a remote that may no longer exist.
func (r *SSHTaskDirReclaimer) probeUnpushed(ctx context.Context, dir string) (SSHReclaimVerdict, bool) {
	out, _, err := r.runner.Run(ctx, sshReclaimUnpushedCmd(dir))
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": "+err.Error()), false
	}
	count, convErr := strconv.Atoi(strings.TrimSpace(out))
	if convErr != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": unreadable commit count"), false
	}
	if count > 0 {
		return unsafeVerdict(SSHReclaimSkipUnpushedCommits, dir), false
	}
	return SSHReclaimVerdict{Safe: true}, true
}

// remove deletes the directory and then confirms it is gone. A silent failure
// here would report reclaimed disk that was never reclaimed.
func (r *SSHTaskDirReclaimer) remove(ctx context.Context, taskDir string) error {
	if _, stderr, err := r.runner.Run(ctx, sshReclaimRemoveCmd(taskDir)); err != nil {
		return fmt.Errorf("remove remote task dir %s: %w (%s)", taskDir, err, strings.TrimSpace(stderr))
	}
	out, _, err := r.runner.Run(ctx, sshReclaimExistsCmd(taskDir))
	if err != nil {
		return fmt.Errorf("confirm removal of remote task dir %s: %w", taskDir, err)
	}
	if strings.TrimSpace(out) != sshReclaimGoneMarker {
		return fmt.Errorf("remote task dir %s still exists after removal", taskDir)
	}
	return nil
}

// validateSSHReclaimPath enforces that the target is exactly one non-empty
// segment beneath <workdir_root>/tasks/. This is the last of three independent
// barriers (profile opt-in, ownership resolution, path shape), and it is the
// one that makes an empty or corrupted task-dir name incapable of resolving to
// the tasks root itself.
func validateSSHReclaimPath(workdirRoot, taskDir string) error {
	root := strings.TrimSpace(workdirRoot)
	path := strings.TrimSpace(taskDir)
	if root == "" || path == "" {
		return fmt.Errorf("%w: empty workspace root or task directory", ErrSSHReclaimPathRefused)
	}
	if !strings.HasPrefix(root, "/") || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: %q is not an absolute path under %q", ErrSSHReclaimPathRefused, path, root)
	}
	root = trimTrailingSlashes(root)
	path = trimTrailingSlashes(path)

	prefix := root + "/" + sshReclaimTasksSegment + "/"
	if !strings.HasPrefix(path, prefix) {
		return fmt.Errorf("%w: %q is not directly beneath %q", ErrSSHReclaimPathRefused, path, prefix)
	}
	segment := strings.TrimPrefix(path, prefix)
	if segment == "" || segment == "." || segment == ".." || strings.Contains(segment, "/") {
		return fmt.Errorf("%w: %q is not a single task directory name", ErrSSHReclaimPathRefused, segment)
	}
	return nil
}

// trimTrailingSlashes removes trailing separators while preserving "/" itself,
// which must stay recognizably the filesystem root rather than collapsing to
// the empty string.
func trimTrailingSlashes(path string) string {
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

// Remote command builders. Exported to the test as package-level functions so a
// test asserts against the exact string the production path sends.

func gitProbeCmd(dir, args string) string {
	return "git -C " + shellQuote(dir) + " " + args
}

func sshReclaimHomeCmd() string {
	return "printf %s \"$HOME\""
}

func sshReclaimChildReposCmd(taskDir string) string {
	return "find " + shellQuote(taskDir) + " -mindepth 2 -maxdepth 2 -name .git"
}

func sshReclaimIsCheckoutCmd(dir string) string {
	return gitProbeCmd(dir, "rev-parse --is-inside-work-tree")
}

func sshReclaimStatusCmd(dir string) string {
	return gitProbeCmd(dir, "status --porcelain --untracked-files=all --ignored")
}

func sshReclaimUnpushedCmd(dir string) string {
	return gitProbeCmd(dir, "rev-list --count HEAD --branches --not --remotes")
}

func sshReclaimStashCmd(dir string) string {
	return gitProbeCmd(dir, "stash list")
}

func sshReclaimRemoveCmd(taskDir string) string {
	return "rm -rf -- " + shellQuote(taskDir)
}

func sshReclaimExistsCmd(taskDir string) string {
	return "test -e " + shellQuote(taskDir) +
		" && printf %s " + shellQuote(sshReclaimExistsMarker) +
		" || printf %s " + shellQuote(sshReclaimGoneMarker)
}

// SSHReclaimEnabled reports whether the executor profile opted in to reclaiming
// the remote task directory.
//
// Only the exact (whitespace-trimmed) string "true" enables it. Everything
// else — absent, empty, "false", "TRUE", "1" — leaves reclamation off, which is
// the safe direction: a value we do not recognize means the destructive path
// stays closed rather than opening on a guess.
func SSHReclaimEnabled(metadata map[string]interface{}) bool {
	return strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHReclaimTaskDir)) == boolStringTrue
}
