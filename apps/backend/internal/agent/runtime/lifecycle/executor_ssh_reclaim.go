package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
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
	sshReclaimExistsMarker       = "exists"
	sshReclaimGoneMarker         = "gone"
	sshReclaimDetachedHeadMarker = "KANDEV_DETACHED_HEAD"

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

type sshReclaimCheckoutKind uint8

const (
	sshReclaimStandalone sshReclaimCheckoutKind = iota + 1
	sshReclaimLinked
)

type sshReclaimCheckout struct {
	path      string
	kind      sshReclaimCheckoutKind
	commonDir string
}

type sshReclaimGitPaths struct {
	top       string
	gitDir    string
	commonDir string
}

type sshReclaimWorktree struct {
	path       string
	locked     bool
	lockReason string
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
	taskName, err := sshReclaimTaskName(root, taskDir)
	if err != nil {
		return SSHReclaimOutcomeFailed, SSHReclaimVerdict{}, err
	}

	canonicalRoot, err := r.physicalRemotePath(ctx, root)
	if err != nil {
		verdict := unsafeVerdict(SSHReclaimSkipProbeFailed, "resolve workspace root: "+err.Error())
		r.log("preserving remote task directory", taskDir, string(verdict.Reason), verdict.Detail)
		return SSHReclaimOutcomeSkipped, verdict, nil
	}
	canonicalTaskDir, err := r.physicalRemotePath(ctx, trimTrailingSlashes(taskDir))
	if err != nil {
		verdict := unsafeVerdict(SSHReclaimSkipProbeFailed, "resolve task directory: "+err.Error())
		r.log("preserving remote task directory", taskDir, string(verdict.Reason), verdict.Detail)
		return SSHReclaimOutcomeSkipped, verdict, nil
	}
	expectedTaskDir := path.Join(canonicalRoot, sshReclaimTasksSegment, taskName)
	if canonicalTaskDir != expectedTaskDir {
		verdict := unsafeVerdict(SSHReclaimSkipProbeFailed, "resolved task path does not match its recorded task directory")
		r.log("preserving remote task directory", taskDir, string(verdict.Reason), verdict.Detail)
		return SSHReclaimOutcomeSkipped, verdict, nil
	}
	if err := validateSSHReclaimPath(canonicalRoot, canonicalTaskDir); err != nil {
		return SSHReclaimOutcomeFailed, SSHReclaimVerdict{}, err
	}
	checkouts, verdict := r.probeForRemoval(ctx, canonicalTaskDir)
	if !verdict.Safe {
		r.log("preserving remote task directory", taskDir, string(verdict.Reason), verdict.Detail)
		return SSHReclaimOutcomeSkipped, verdict, nil
	}
	if err := r.remove(ctx, canonicalTaskDir, checkouts); err != nil {
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

// probe returns each verified checkout so removal can update linked-worktree
// registrations without rediscovering repository identity.
func (r *SSHTaskDirReclaimer) probeForRemoval(ctx context.Context, taskDir string) ([]sshReclaimCheckout, SSHReclaimVerdict) {
	checkouts, err := r.discoverCheckouts(ctx, taskDir)
	if err != nil {
		return nil, unsafeVerdict(SSHReclaimSkipProbeFailed, err.Error())
	}
	verified := make([]sshReclaimCheckout, 0, len(checkouts))
	for _, dir := range checkouts {
		checkout, verdict := r.probeCheckout(ctx, taskDir, dir)
		if !verdict.Safe {
			return nil, verdict
		}
		verified = append(verified, checkout)
	}
	return verified, SSHReclaimVerdict{Safe: true}
}

func (r *SSHTaskDirReclaimer) probe(ctx context.Context, taskDir string) SSHReclaimVerdict {
	_, verdict := r.probeForRemoval(ctx, taskDir)
	return verdict
}

// discoverCheckouts returns the task directory root plus supported direct-child
// checkouts. A deeper .git entry makes the layout unsupported. An enumeration
// that cannot run is an error, not an empty list.
func (r *SSHTaskDirReclaimer) discoverCheckouts(ctx context.Context, taskDir string) ([]string, error) {
	out, _, err := r.runner.Run(ctx, sshReclaimChildReposCmd(taskDir))
	if err != nil {
		return nil, fmt.Errorf("enumerate child repositories: %w", err)
	}
	dirs := []string{taskDir}
	if out != "" && !strings.HasSuffix(out, "\x00") {
		return nil, errors.New("child repository inventory is unterminated")
	}
	for _, entry := range strings.Split(strings.TrimSuffix(out, "\x00"), "\x00") {
		if entry == "" {
			if out == "" {
				continue
			}
			return nil, errors.New("child repository inventory contains an empty path")
		}
		child, ok := strings.CutSuffix(entry, "/.git")
		if !ok {
			return nil, fmt.Errorf("unexpected child repository entry %q", entry)
		}
		if path.Dir(child) != taskDir {
			return nil, fmt.Errorf("nested repository %q is unsupported", child)
		}
		dirs = append(dirs, child)
	}
	sort.Strings(dirs[1:])
	return dirs, nil
}

func (r *SSHTaskDirReclaimer) probeCheckout(
	ctx context.Context,
	taskDir, dir string,
) (sshReclaimCheckout, SSHReclaimVerdict) {
	physicalDir, err := r.physicalRemotePath(ctx, dir)
	if err != nil {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": "+err.Error())
	}
	if !isTaskDirectoryCheckout(taskDir, physicalDir) {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": checkout is outside the task directory")
	}
	inside, _, err := r.runner.Run(ctx, sshReclaimIsCheckoutCmd(physicalDir))
	if err != nil {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, physicalDir+": "+err.Error())
	}
	if strings.TrimSpace(inside) != boolStringTrue {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipNotACheckout, physicalDir)
	}
	paths, err := r.probeGitPaths(ctx, physicalDir)
	if err != nil {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, physicalDir+": "+err.Error())
	}
	if paths.top != physicalDir {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, physicalDir+": Git reported a different checkout root")
	}
	worktrees, err := r.worktreeInventory(ctx, paths.commonDir)
	if err != nil {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, physicalDir+": "+err.Error())
	}
	entry, registered := findWorktree(worktrees, physicalDir)
	if !registered {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, physicalDir+": checkout has no matching Git registration")
	}
	if entry.locked {
		detail := "linked worktree is locked"
		if entry.lockReason != "" {
			detail += ": " + entry.lockReason
		}
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipShared, detail)
	}
	checkout, verdict := classifyReclaimCheckout(paths, worktrees, physicalDir)
	if !verdict.Safe {
		return sshReclaimCheckout{}, verdict
	}
	if checkout.kind == sshReclaimLinked && pathWithin(taskDir, checkout.commonDir) {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipShared, "linked worktree Git storage is inside the task directory")
	}
	if verdict := r.probeCheckoutContents(ctx, checkout); !verdict.Safe {
		return sshReclaimCheckout{}, verdict
	}
	return checkout, SSHReclaimVerdict{Safe: true}
}

func (r *SSHTaskDirReclaimer) probeWorktreeStatus(ctx context.Context, dir string) (SSHReclaimVerdict, bool) {
	out, _, err := r.runner.Run(ctx, sshReclaimStatusCmd(dir))
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": "+err.Error()), false
	}
	if strings.TrimSpace(out) != "" {
		return unsafeVerdict(SSHReclaimSkipDirtyWorktree, dir), false
	}
	return SSHReclaimVerdict{Safe: true}, true
}

func (r *SSHTaskDirReclaimer) probeCheckoutContents(
	ctx context.Context,
	checkout sshReclaimCheckout,
) SSHReclaimVerdict {
	if verdict := r.probeSubmodules(ctx, checkout.path); !verdict.Safe {
		return verdict
	}
	if verdict, ok := r.probeWorktreeStatus(ctx, checkout.path); !ok {
		return verdict
	}
	if verdict, ok := r.probeUnpushed(ctx, checkout.path, checkout.kind); !ok {
		return verdict
	}
	if verdict := r.probeStashes(ctx, checkout); !verdict.Safe {
		return verdict
	}
	return SSHReclaimVerdict{Safe: true}
}

func (r *SSHTaskDirReclaimer) probeSubmodules(ctx context.Context, dir string) SSHReclaimVerdict {
	out, _, err := r.runner.Run(ctx, sshReclaimSubmoduleStatusCmd(dir))
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": submodule status: "+err.Error())
	}
	if strings.TrimSpace(out) != "" {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": registered submodules are unsupported")
	}
	return SSHReclaimVerdict{Safe: true}
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
func (r *SSHTaskDirReclaimer) probeUnpushed(
	ctx context.Context,
	dir string,
	kind sshReclaimCheckoutKind,
) (SSHReclaimVerdict, bool) {
	cmd := sshReclaimUnpushedCmd(dir)
	if kind == sshReclaimLinked {
		cmd = sshReclaimLinkedUnpushedCmd(dir)
	}
	out, _, err := r.runner.Run(ctx, cmd)
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": "+err.Error()), false
	}
	count, ok := parseReclaimCommitCount(out)
	if !ok {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, dir+": unreadable commit count"), false
	}
	if count > 0 {
		return unsafeVerdict(SSHReclaimSkipUnpushedCommits, dir), false
	}
	return SSHReclaimVerdict{Safe: true}, true
}

func (r *SSHTaskDirReclaimer) probeStashes(
	ctx context.Context,
	checkout sshReclaimCheckout,
) SSHReclaimVerdict {
	out, _, err := r.runner.Run(ctx, sshReclaimStashCmd(checkout.path))
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, checkout.path+": "+err.Error())
	}
	lines := nonEmptyLines(out)
	if len(lines) == 0 {
		return SSHReclaimVerdict{Safe: true}
	}
	if checkout.kind == sshReclaimStandalone {
		return unsafeVerdict(SSHReclaimSkipStashPresent, checkout.path)
	}
	branch, err := r.currentBranch(ctx, checkout.path)
	if err != nil {
		return unsafeVerdict(SSHReclaimSkipProbeFailed, checkout.path+": "+err.Error())
	}
	if branch == "HEAD" {
		return unsafeVerdict(SSHReclaimSkipStashPresent, checkout.path+": detached HEAD has shared stashes")
	}
	for _, line := range lines {
		stashBranch, ok := parseReclaimStashBranch(line)
		if !ok {
			return unsafeVerdict(SSHReclaimSkipStashPresent, checkout.path+": stash attribution is unknown")
		}
		if stashBranch == branch {
			return unsafeVerdict(SSHReclaimSkipStashPresent, checkout.path)
		}
	}
	return SSHReclaimVerdict{Safe: true}
}

func (r *SSHTaskDirReclaimer) currentBranch(ctx context.Context, dir string) (string, error) {
	out, _, err := r.runner.Run(ctx, sshReclaimBranchCmd(dir))
	if err != nil {
		return "", err
	}
	ref, ok := commandOutputLine(out)
	if !ok {
		return "", errors.New("unreadable symbolic branch identity")
	}
	if ref == sshReclaimDetachedHeadMarker {
		return "HEAD", nil
	}
	branch, ok := strings.CutPrefix(ref, "refs/heads/")
	if !ok || branch == "" {
		return "", errors.New("empty branch identity")
	}
	return branch, nil
}

func (r *SSHTaskDirReclaimer) physicalRemotePath(ctx context.Context, remotePath string) (string, error) {
	out, _, err := r.runner.Run(ctx, sshReclaimPhysicalPathCmd(remotePath))
	if err != nil {
		return "", err
	}
	physical, ok := commandOutputLine(out)
	if !ok || physical == "" || !path.IsAbs(physical) {
		return "", errors.New("unreadable physical path")
	}
	return path.Clean(physical), nil
}

func (r *SSHTaskDirReclaimer) probeGitPaths(ctx context.Context, dir string) (sshReclaimGitPaths, error) {
	out, _, err := r.runner.Run(ctx, sshReclaimCheckoutPathsCmd(dir))
	if err != nil {
		return sshReclaimGitPaths{}, err
	}
	if !strings.HasSuffix(out, "\n") {
		return sshReclaimGitPaths{}, errors.New("unreadable Git checkout paths")
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 3 {
		return sshReclaimGitPaths{}, errors.New("unreadable Git checkout paths")
	}
	values := make([]string, 3)
	for i, value := range lines {
		if value == "" || !path.IsAbs(value) {
			return sshReclaimGitPaths{}, errors.New("unreadable Git checkout paths")
		}
		values[i], err = r.physicalRemotePath(ctx, value)
		if err != nil {
			return sshReclaimGitPaths{}, err
		}
	}
	return sshReclaimGitPaths{top: values[0], gitDir: values[1], commonDir: values[2]}, nil
}

func (r *SSHTaskDirReclaimer) worktreeInventory(ctx context.Context, commonDir string) ([]sshReclaimWorktree, error) {
	out, _, err := r.runner.Run(ctx, sshReclaimWorktreeListCmd(commonDir))
	if err != nil {
		return nil, err
	}
	return parseReclaimWorktreeList(out)
}

// remove deletes the directory and then confirms it is gone. A silent failure
// here would report reclaimed disk that was never reclaimed.
func (r *SSHTaskDirReclaimer) remove(
	ctx context.Context,
	taskDir string,
	checkouts []sshReclaimCheckout,
) error {
	for _, child := range checkouts[1:] {
		if child.kind != sshReclaimLinked {
			continue
		}
		if err := r.removeLinkedWorktree(ctx, child); err != nil {
			return err
		}
	}
	root := checkouts[0]
	if root.kind == sshReclaimLinked {
		if err := r.removeLinkedWorktree(ctx, root); err != nil {
			return err
		}
	} else if _, stderr, err := r.runner.Run(ctx, sshReclaimRemoveCmd(taskDir)); err != nil {
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

func (r *SSHTaskDirReclaimer) removeLinkedWorktree(ctx context.Context, checkout sshReclaimCheckout) error {
	if _, stderr, err := r.runner.Run(ctx, sshReclaimWorktreeRemoveCmd(checkout.commonDir, checkout.path)); err != nil {
		return fmt.Errorf("remove linked SSH checkout %s: %w (%s)", checkout.path, err, strings.TrimSpace(stderr))
	}
	return nil
}

func classifyReclaimCheckout(
	paths sshReclaimGitPaths,
	worktrees []sshReclaimWorktree,
	checkoutPath string,
) (sshReclaimCheckout, SSHReclaimVerdict) {
	if _, ok := findWorktree(worktrees, checkoutPath); !ok {
		return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, checkoutPath+": checkout has no matching Git registration")
	}
	if paths.gitDir == paths.commonDir && paths.gitDir == path.Join(paths.top, ".git") {
		if len(worktrees) != 1 {
			return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipShared, checkoutPath+": standalone repository has additional worktrees")
		}
		return sshReclaimCheckout{path: checkoutPath, kind: sshReclaimStandalone, commonDir: paths.commonDir}, SSHReclaimVerdict{Safe: true}
	}
	if paths.gitDir != paths.commonDir && pathWithin(paths.commonDir, paths.gitDir) {
		return sshReclaimCheckout{path: checkoutPath, kind: sshReclaimLinked, commonDir: paths.commonDir}, SSHReclaimVerdict{Safe: true}
	}
	return sshReclaimCheckout{}, unsafeVerdict(SSHReclaimSkipProbeFailed, checkoutPath+": unsupported Git storage layout")
}

func findWorktree(worktrees []sshReclaimWorktree, checkoutPath string) (sshReclaimWorktree, bool) {
	for _, worktree := range worktrees {
		if path.Clean(worktree.path) == path.Clean(checkoutPath) {
			return worktree, true
		}
	}
	return sshReclaimWorktree{}, false
}

func parseReclaimWorktreeList(output string) ([]sshReclaimWorktree, error) {
	fields := strings.Split(output, "\x00")
	worktrees := make([]sshReclaimWorktree, 0, 2)
	var current *sshReclaimWorktree
	appendCurrent := func() error {
		if current == nil {
			return nil
		}
		if !path.IsAbs(current.path) {
			return errors.New("worktree inventory contains an invalid path")
		}
		worktrees = append(worktrees, *current)
		current = nil
		return nil
	}
	for _, field := range fields {
		if field == "" {
			if err := appendCurrent(); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(field, "worktree ") {
			if err := appendCurrent(); err != nil {
				return nil, err
			}
			current = &sshReclaimWorktree{path: strings.TrimPrefix(field, "worktree ")}
			continue
		}
		if current == nil {
			return nil, errors.New("worktree inventory has fields before a path")
		}
		if field == "locked" || strings.HasPrefix(field, "locked ") {
			current.locked = true
			current.lockReason = strings.TrimPrefix(field, "locked ")
		}
	}
	if err := appendCurrent(); err != nil {
		return nil, err
	}
	if len(worktrees) == 0 {
		return nil, errors.New("empty worktree inventory")
	}
	return worktrees, nil
}

func parseReclaimCommitCount(output string) (int, bool) {
	value := strings.TrimSpace(output)
	if value == "" {
		return 0, false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	count, err := strconv.Atoi(value)
	return count, err == nil
}

func parseReclaimStashBranch(description string) (string, bool) {
	for _, prefix := range []string{"WIP on ", "On "} {
		if !strings.HasPrefix(description, prefix) {
			continue
		}
		branch, _, ok := strings.Cut(strings.TrimPrefix(description, prefix), ": ")
		return branch, ok && branch != "" && branch != "(no branch)"
	}
	return "", false
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func pathWithin(root, candidate string) bool {
	root = path.Clean(root)
	candidate = path.Clean(candidate)
	if root == candidate {
		return true
	}
	if root == "/" {
		return strings.HasPrefix(candidate, "/")
	}
	return strings.HasPrefix(candidate, root+"/")
}

func isTaskDirectoryCheckout(taskDir, checkoutDir string) bool {
	return checkoutDir == taskDir || path.Dir(checkoutDir) == taskDir
}

func sshReclaimTaskName(workdirRoot, taskDir string) (string, error) {
	if err := validateSSHReclaimPath(workdirRoot, taskDir); err != nil {
		return "", err
	}
	root := trimTrailingSlashes(strings.TrimSpace(workdirRoot))
	prefix := root + "/" + sshReclaimTasksSegment + "/"
	if root == "/" {
		prefix = "/" + sshReclaimTasksSegment + "/"
	}
	return strings.TrimPrefix(trimTrailingSlashes(taskDir), prefix), nil
}

// validateSSHReclaimPath enforces that the target is exactly one non-empty
// segment beneath <workdir_root>/tasks/. This is the last of three independent
// barriers (profile opt-in, ownership resolution, path shape), and it is the
// one that makes an empty or corrupted task-dir name incapable of resolving to
// the tasks root itself.
func validateSSHReclaimPath(workdirRoot, taskDir string) error {
	root := strings.TrimSpace(workdirRoot)
	taskPath := taskDir
	if root == "" || strings.TrimSpace(taskPath) == "" {
		return fmt.Errorf("%w: empty workspace root or task directory", ErrSSHReclaimPathRefused)
	}
	if !strings.HasPrefix(root, "/") || !strings.HasPrefix(taskPath, "/") {
		return fmt.Errorf("%w: %q is not an absolute path under %q", ErrSSHReclaimPathRefused, taskPath, root)
	}
	root = trimTrailingSlashes(root)
	taskPath = trimTrailingSlashes(taskPath)

	prefix := root + "/" + sshReclaimTasksSegment + "/"
	if root == "/" {
		prefix = "/" + sshReclaimTasksSegment + "/"
	}
	if !strings.HasPrefix(taskPath, prefix) {
		return fmt.Errorf("%w: %q is not directly beneath %q", ErrSSHReclaimPathRefused, taskPath, prefix)
	}
	segment := strings.TrimPrefix(taskPath, prefix)
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
	return "GIT_OPTIONAL_LOCKS=0 git -C " + shellQuote(dir) + " " + args
}

func commandOutputLine(output string) (string, bool) {
	if !strings.HasSuffix(output, "\n") {
		return "", false
	}
	line := strings.TrimSuffix(output, "\n")
	return line, line != "" && !strings.Contains(line, "\n")
}

func sshReclaimHomeCmd() string {
	return "printf %s \"$HOME\""
}

func sshReclaimChildReposCmd(taskDir string) string {
	return "find " + shellQuote(taskDir) + " -mindepth 2 -name .git -prune -print0"
}

func sshReclaimIsCheckoutCmd(dir string) string {
	return gitProbeCmd(dir, "rev-parse --is-inside-work-tree")
}

func sshReclaimCheckoutPathsCmd(dir string) string {
	return gitProbeCmd(dir, "rev-parse --path-format=absolute --show-toplevel --git-dir --git-common-dir")
}

func sshReclaimPhysicalPathCmd(dir string) string {
	return "cd " + shellQuote(dir) + " && pwd -P"
}

func sshReclaimWorktreeListCmd(commonDir string) string {
	return "GIT_OPTIONAL_LOCKS=0 git --git-dir=" + shellQuote(commonDir) + " worktree list --porcelain -z"
}

func sshReclaimWorktreeRemoveCmd(commonDir, checkout string) string {
	return "git --git-dir=" + shellQuote(commonDir) + " worktree remove -- " + shellQuote(checkout)
}

func sshReclaimSubmoduleStatusCmd(dir string) string {
	return gitProbeCmd(dir, "submodule status --recursive")
}

func sshReclaimStatusCmd(dir string) string {
	return gitProbeCmd(dir, "status --porcelain --untracked-files=all --ignored=no")
}

func sshReclaimUnpushedCmd(dir string) string {
	return gitProbeCmd(dir, "rev-list --count HEAD --branches --not --remotes")
}

func sshReclaimLinkedUnpushedCmd(dir string) string {
	return gitProbeCmd(dir, "rev-list --count HEAD --not --remotes")
}

func sshReclaimStashCmd(dir string) string {
	return gitProbeCmd(dir, "stash list --format=%gs")
}

func sshReclaimBranchCmd(dir string) string {
	return gitProbeCmd(dir, `symbolic-ref --quiet HEAD || { status=$?; if [ "$status" -eq 1 ]; then printf '%s\n' 'KANDEV_DETACHED_HEAD'; else exit "$status"; fi; }`)
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
