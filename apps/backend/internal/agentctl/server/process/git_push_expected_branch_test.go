package process

import (
	"context"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// setupPushRemotesRepo builds a checkout on "feature/work" with two bare
// remotes, "origin" and "backup". origin already carries main, so empty-remote
// first publication does not apply unless a test arranges for it.
func setupPushRemotesRepo(t *testing.T) (repoDir, originDir, backupDir string, operator *GitOperator) {
	t.Helper()
	isolateTestGitEnv(t)
	root := t.TempDir()
	originDir = filepath.Join(root, "origin.git")
	backupDir = filepath.Join(root, "backup.git")
	repoDir = filepath.Join(root, "repo")
	runGit(t, root, "init", "--bare", "--initial-branch=main", originDir)
	runGit(t, root, "init", "--bare", "--initial-branch=main", backupDir)
	runGit(t, root, "init", "-b", "main", repoDir)
	runGit(t, repoDir, "config", "user.name", "Task User")
	runGit(t, repoDir, "config", "user.email", "task@example.com")
	runGit(t, repoDir, "remote", "add", "origin", originDir)
	runGit(t, repoDir, "remote", "add", "backup", backupDir)
	writeFile(t, repoDir, "seed.txt", "seed\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed")
	runGit(t, repoDir, "push", "origin", "main")
	runGit(t, repoDir, "checkout", "-b", "feature/work")
	writeFile(t, repoDir, "change.txt", "change\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "task change")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetBaseBranch("main")
	return repoDir, originDir, backupDir, NewGitOperator(repoDir, newTestLogger(t), tracker)
}

func remoteBranchSHA(t *testing.T, remoteDir, branch string) string {
	t.Helper()
	cmd := osExec.Command("git", "-C", remoteDir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// recordGitCommands installs a PATH shim that logs every git invocation before
// delegating to the real binary, so a test can assert command ordering.
func recordGitCommands(t *testing.T) func() []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("git PATH shim is a POSIX shell script")
	}
	realGit, err := osExec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) = %v", err)
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		data, err := os.ReadFile(logPath)
		if err != nil {
			return nil
		}
		var lines []string
		for _, line := range strings.Split(string(data), "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				lines = append(lines, trimmed)
			}
		}
		return lines
	}
}

func TestGitOperatorPushWithoutOptionsIsUnchanged(t *testing.T) {
	repoDir, originDir, backupDir, operator := setupPushRemotesRepo(t)

	result, err := operator.Push(context.Background(), PushOptions{})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	local := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != local {
		t.Errorf("origin feature/work = %q, want %q", got, local)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q, want untouched", got)
	}
	// A request naming no target keeps the result shape it has today.
	if result.PushedRemote != "" || result.PushedBranch != "" {
		t.Errorf("destination fields = (%q, %q), want both empty", result.PushedRemote, result.PushedBranch)
	}
	// The default path still sets upstream on first publication.
	if upstream := operator.getUpstreamRef(context.Background()); upstream != "origin/feature/work" {
		t.Errorf("upstream = %q, want origin/feature/work", upstream)
	}
}

func TestGitOperatorPushOmitsTargetFieldsWithoutExplicitTarget(t *testing.T) {
	_, _, _, operator := setupPushRemotesRepo(t)

	result, err := operator.Push(context.Background(), PushOptions{ExpectedBranch: "feature/work"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	if result.PushedRemote != "" || result.PushedBranch != "" {
		t.Errorf("destination fields = (%q, %q), want both empty", result.PushedRemote, result.PushedBranch)
	}
}

func TestGitOperatorPushPublishesHeadToExpectedBranch(t *testing.T) {
	repoDir, originDir, backupDir, operator := setupPushRemotesRepo(t)

	result, err := operator.Push(context.Background(), PushOptions{
		Remote:         "backup",
		ExpectedBranch: "feature/work",
	})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	local := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != local {
		t.Errorf("backup feature/work = %q, want %q", got, local)
	}
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != "" {
		t.Errorf("origin gained %q, want untouched", got)
	}
	if result.PushedRemote != "backup" || result.PushedBranch != "feature/work" {
		t.Errorf("destination fields = (%q, %q), want (backup, feature/work)", result.PushedRemote, result.PushedBranch)
	}
}

func TestGitOperatorPushCreatesMissingDestinationBranch(t *testing.T) {
	repoDir, _, backupDir, operator := setupPushRemotesRepo(t)

	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	local := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != local {
		t.Errorf("backup feature/work = %q, want the branch created at %q", got, local)
	}
}

func TestGitOperatorPushToNamedRemoteLeavesUpstreamUnset(t *testing.T) {
	_, _, _, operator := setupPushRemotesRepo(t)

	before := operator.getUpstreamRef(context.Background())
	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	if after := operator.getUpstreamRef(context.Background()); after != before {
		t.Errorf("upstream changed from %q to %q", before, after)
	}
}

func TestGitOperatorPushToNamedRemoteStaysNonForce(t *testing.T) {
	repoDir, _, backupDir, operator := setupPushRemotesRepo(t)
	// Give backup a history the local branch does not contain.
	runGit(t, repoDir, "push", "backup", "HEAD:refs/heads/feature/work")
	runGit(t, repoDir, "checkout", "-b", "diverged")
	writeFile(t, repoDir, "diverged.txt", "diverged\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "diverged")
	runGit(t, repoDir, "push", "--force", "backup", "HEAD:refs/heads/feature/work")
	runGit(t, repoDir, "checkout", "feature/work")
	remoteBefore := remoteBranchSHA(t, backupDir, "feature/work")

	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success {
		t.Fatalf("non-force push to a diverged branch succeeded: %+v", result)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != remoteBefore {
		t.Errorf("backup moved to %q, want %q", got, remoteBefore)
	}
}

func TestGitOperatorPushToNamedRemoteSkipsBaselinePublication(t *testing.T) {
	root := t.TempDir()
	isolateTestGitEnv(t)
	originDir := filepath.Join(root, "origin.git")
	backupDir := filepath.Join(root, "backup.git")
	repoDir := filepath.Join(root, "repo")
	runGit(t, root, "init", "--bare", "--initial-branch=main", originDir)
	runGit(t, root, "init", "--bare", "--initial-branch=main", backupDir)
	runGit(t, root, "init", "-b", "main", repoDir)
	runGit(t, repoDir, "config", "user.name", "Task User")
	runGit(t, repoDir, "config", "user.email", "task@example.com")
	runGit(t, repoDir, "remote", "add", "origin", originDir)
	runGit(t, repoDir, "remote", "add", "backup", backupDir)
	writeFile(t, repoDir, "seed.txt", "seed\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed")
	runGit(t, repoDir, "checkout", "-b", "feature/work")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetBaseBranch("main")
	operator := NewGitOperator(repoDir, newTestLogger(t), tracker)

	// origin is empty; a push to backup must not publish a baseline anywhere.
	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	if got := remoteBranchSHA(t, originDir, "main"); got != "" {
		t.Errorf("origin gained a baseline %q, want empty", got)
	}
	if result.BaselinePublished {
		t.Error("BaselinePublished = true, want false")
	}
}

func TestGitOperatorPushRefusesTargetWithSetUpstream(t *testing.T) {
	_, _, backupDir, operator := setupPushRemotesRepo(t)

	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup", SetUpstream: true})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.ErrorCode != pushRemoteUpstreamUnsupportedErrorCode {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, pushRemoteUpstreamUnsupportedErrorCode)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q, want untouched", got)
	}
}

func TestGitOperatorPushRefusesExpectedBranchMismatch(t *testing.T) {
	_, originDir, _, operator := setupPushRemotesRepo(t)

	result, err := operator.Push(context.Background(), PushOptions{ExpectedBranch: "feature/other"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.ErrorCode != pushBranchMismatchErrorCode {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, pushBranchMismatchErrorCode)
	}
	if result.ExpectedBranch != "feature/other" || result.CurrentBranch != "feature/work" {
		t.Errorf("branches = (%q, %q), want (feature/other, feature/work)", result.ExpectedBranch, result.CurrentBranch)
	}
	if result.BaselinePublished {
		t.Error("BaselinePublished = true, want false")
	}
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != "" {
		t.Errorf("origin gained %q, want no remote contact", got)
	}
}

func TestGitOperatorPushRefusesDetachedHeadAsMismatch(t *testing.T) {
	repoDir, originDir, _, operator := setupPushRemotesRepo(t)
	runGit(t, repoDir, "checkout", "--detach", "HEAD")

	result, err := operator.Push(context.Background(), PushOptions{ExpectedBranch: "feature/work"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.ErrorCode != pushBranchMismatchErrorCode {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, pushBranchMismatchErrorCode)
	}
	// A detached HEAD is an empty current branch, never the literal "HEAD".
	if result.CurrentBranch != "" {
		t.Errorf("CurrentBranch = %q, want empty", result.CurrentBranch)
	}
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != "" {
		t.Errorf("origin gained %q, want no remote contact", got)
	}
}

func TestGitOperatorPushRefusesDetachedHeadWithTargetOnly(t *testing.T) {
	repoDir, _, backupDir, operator := setupPushRemotesRepo(t)
	runGit(t, repoDir, "checkout", "--detach", "HEAD")

	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.ErrorCode != pushBranchDetachedErrorCode {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, pushBranchDetachedErrorCode)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q, want untouched", got)
	}
}

func TestGitOperatorPushExpectedBranchAloneOnlyGates(t *testing.T) {
	repoDir, originDir, _, operator := setupPushRemotesRepo(t)

	result, err := operator.Push(context.Background(), PushOptions{ExpectedBranch: "feature/work"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	// The expected branch is a precondition only: remote, refspec and upstream
	// behavior are the ones the request would have had without it.
	local := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != local {
		t.Errorf("origin feature/work = %q, want %q", got, local)
	}
	if upstream := operator.getUpstreamRef(context.Background()); upstream != "origin/feature/work" {
		t.Errorf("upstream = %q, want origin/feature/work", upstream)
	}
}

func TestGitOperatorPushRefusalOrdering(t *testing.T) {
	_, _, _, operator := setupPushRemotesRepo(t)

	for _, tc := range []struct {
		name string
		opts PushOptions
		want string
	}{
		{
			name: "upstream conflict outranks an invalid branch",
			opts: PushOptions{Remote: "backup", SetUpstream: true, ExpectedBranch: "bad..branch"},
			want: pushRemoteUpstreamUnsupportedErrorCode,
		},
		{
			name: "invalid branch outranks target resolution",
			opts: PushOptions{Remote: "missing", ExpectedBranch: "bad..branch"},
			want: pushBranchInvalidErrorCode,
		},
		{
			name: "target resolution outranks a branch mismatch",
			opts: PushOptions{Remote: "missing", ExpectedBranch: "feature/other"},
			want: pushRemoteNotFoundErrorCode,
		},
		{
			name: "detached outranks nothing but is reported for a target alone",
			opts: PushOptions{Remote: "backup", ExpectedBranch: "HEAD"},
			want: pushBranchInvalidErrorCode,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := operator.Push(context.Background(), tc.opts)
			if err != nil {
				t.Fatalf("Push() error = %v", err)
			}
			if result.ErrorCode != tc.want {
				t.Errorf("ErrorCode = %q, want %q", result.ErrorCode, tc.want)
			}
		})
	}
}

func TestGitOperatorPushRefusalsAreNoOps(t *testing.T) {
	repoDir, originDir, backupDir, operator := setupPushRemotesRepo(t)
	upstreamBefore := operator.getUpstreamRef(context.Background())
	headBefore := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	for _, opts := range []PushOptions{
		{Remote: "missing"},
		{Remote: "https://example.test/nope.git"},
		{Remote: "backup", SetUpstream: true},
		{ExpectedBranch: "bad..branch"},
		{ExpectedBranch: "feature/other"},
	} {
		result, err := operator.Push(context.Background(), opts)
		if err != nil {
			t.Fatalf("Push(%+v) error = %v", opts, err)
		}
		if result.Success {
			t.Fatalf("Push(%+v) succeeded, want refusal", opts)
		}
	}

	if got := remoteBranchSHA(t, originDir, "feature/work"); got != "" {
		t.Errorf("origin gained %q", got)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q", got)
	}
	if got := operator.getUpstreamRef(context.Background()); got != upstreamBefore {
		t.Errorf("upstream changed from %q to %q", upstreamBefore, got)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != headBefore {
		t.Errorf("HEAD moved from %q to %q", headBefore, got)
	}
}

func TestGitOperatorPushReportsOperationInProgress(t *testing.T) {
	_, _, _, operator := setupPushRemotesRepo(t)
	if !operator.tryLock("test") {
		t.Fatal("tryLock() = false, want the lock")
	}
	defer operator.unlock()

	_, err := operator.Push(context.Background(), PushOptions{Remote: "backup"})
	if err != ErrOperationInProgress {
		t.Fatalf("Push() error = %v, want ErrOperationInProgress", err)
	}
}

func TestGitOperatorPushRepeatedIdenticalRequestSucceeds(t *testing.T) {
	repoDir, _, backupDir, operator := setupPushRemotesRepo(t)
	opts := PushOptions{Remote: "backup", ExpectedBranch: "feature/work"}

	first, err := operator.Push(context.Background(), opts)
	if err != nil || !first.Success {
		t.Fatalf("first Push() = %+v, err = %v", first, err)
	}
	after := remoteBranchSHA(t, backupDir, "feature/work")

	second, err := operator.Push(context.Background(), opts)
	if err != nil {
		t.Fatalf("second Push() error = %v", err)
	}
	if !second.Success {
		t.Fatalf("second Push() = %+v, want success", second)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != after {
		t.Errorf("destination moved from %q to %q", after, got)
	}
	local := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	if after != local {
		t.Errorf("destination = %q, want %q", after, local)
	}
}

func TestGitOperatorPushNeverCreatesRemote(t *testing.T) {
	repoDir, _, _, operator := setupPushRemotesRepo(t)
	before := strings.TrimSpace(runGit(t, repoDir, "remote"))

	if _, err := operator.Push(context.Background(), PushOptions{Remote: "https://example.test/new.git"}); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if _, err := operator.Push(context.Background(), PushOptions{Remote: "brand-new"}); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if after := strings.TrimSpace(runGit(t, repoDir, "remote")); after != before {
		t.Errorf("remotes changed from %q to %q", before, after)
	}
}

func TestGitOperatorPushResultAndLogsCarryNoRemoteURL(t *testing.T) {
	repoDir, _, backupDir, _ := setupPushRemotesRepo(t)
	log, observed := newObservedTestLogger(t)
	operator := NewGitOperator(repoDir, log, nil)

	result, err := operator.Push(context.Background(), PushOptions{Remote: backupDir})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() result = %+v, want success", result)
	}
	// The URL form resolves to a name, and only the name is reported back.
	if result.PushedRemote != "backup" {
		t.Errorf("PushedRemote = %q, want backup", result.PushedRemote)
	}
	if strings.Contains(result.Error, backupDir) {
		t.Errorf("error carries the remote URL: %q", result.Error)
	}
	for _, entry := range observed.All() {
		if strings.Contains(entry.Message, backupDir) || strings.Contains(fmt.Sprint(entry.ContextMap()), backupDir) {
			t.Errorf("log entry carries the remote URL: message=%q context=%v", entry.Message, entry.ContextMap())
		}
	}
}

func TestGitOperatorPushRedactsCredentialsOnExplicitTargetPath(t *testing.T) {
	repoDir, _, _, operator := setupPushRemotesRepo(t)
	const secret = "s3cr3t-token"
	runGit(t, repoDir, "remote", "add", "secured", "https://user:"+secret+"@example.invalid/repo.git")

	result, err := operator.Push(context.Background(), PushOptions{Remote: "secured"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success {
		t.Fatal("push to an unreachable host succeeded")
	}
	if strings.Contains(result.Output, secret) || strings.Contains(result.Error, secret) {
		t.Errorf("credential leaked: output=%q error=%q", result.Output, result.Error)
	}
}

func TestGitOperatorPushVerifiesBranchAsLastReadBeforePush(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts PushOptions
	}{
		{name: "explicit target", opts: PushOptions{Remote: "backup", ExpectedBranch: "feature/work"}},
		{name: "default path", opts: PushOptions{ExpectedBranch: "feature/work"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, operator := setupPushRemotesRepo(t)
			readLog := recordGitCommands(t)

			result, err := operator.Push(context.Background(), tc.opts)
			if err != nil {
				t.Fatalf("Push() error = %v", err)
			}
			if !result.Success {
				t.Fatalf("Push() result = %+v, want success", result)
			}

			lines := readLog()
			pushIndex := -1
			for i, line := range lines {
				if strings.HasPrefix(line, "push ") {
					pushIndex = i
				}
			}
			if pushIndex <= 0 {
				t.Fatalf("no push command recorded in %v", lines)
			}
			if prev := lines[pushIndex-1]; prev != "symbolic-ref HEAD" {
				t.Errorf("command before push = %q, want the expected-branch read %q (full log: %v)",
					prev, "symbolic-ref HEAD", lines)
			}
		})
	}
}

func TestGitOperatorPushRefusesTargetUnderContributionRouting(t *testing.T) {
	repoDir, _, backupDir, operator := setupPushRemotesRepo(t)
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	const sourceURL = "https://github.com/contributor/widget.git"
	binding := &taskmodels.RemoteContribution{
		Version:      taskmodels.RemoteContributionVersion,
		Provider:     taskmodels.RemoteContributionProviderGitHub,
		Kind:         taskmodels.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/7",
		Number:       7,
		State:        taskmodels.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/work",
		HeadSHA:      head,
		SourceRepository: taskmodels.RemoteContributionRepository{
			Host: "github.com", Path: "contributor/widget", RemoteURL: sourceURL,
		},
		CollaborationAllowed: true,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("binding.Validate() = %v", err)
	}
	runGit(t, repoDir, "remote", "add", binding.ContributionRemoteName(), sourceURL)
	operator.setRemoteContribution(binding)

	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.ErrorCode != pushRemoteContributionConflictErrorCode {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, pushRemoteContributionConflictErrorCode)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q, want untouched", got)
	}
}

// TestGitOperatorPushReportsMismatchAfterBaselinePublication drives the only
// refusal in this capability that is not side-effect-free: HEAD moves after
// empty-remote first publication published the baseline, so the second
// verification refuses a push whose baseline is already on the remote.
func TestGitOperatorPushReportsMismatchAfterBaselinePublication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git PATH shim is a POSIX shell script")
	}
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)
	realGit, err := osExec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) = %v", err)
	}
	// Move HEAD immediately after the baseline push, which is the window the
	// second verification exists to close.
	shimDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \"$1 $2\" in\n" +
		"  'push --force-with-lease=refs/heads/main:')\n" +
		"    " + realGit + " \"$@\"; rc=$?\n" +
		"    " + realGit + " -C " + repoDir + " symbolic-ref HEAD refs/heads/switched\n" +
		"    exit $rc ;;\n" +
		"esac\n" +
		"exec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := operator.Push(context.Background(), PushOptions{ExpectedBranch: "feature/empty"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success {
		t.Fatalf("Push() succeeded, want a post-baseline mismatch refusal: %+v", result)
	}
	if result.ErrorCode != pushBranchMismatchAfterBaselineErrorCode {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, pushBranchMismatchAfterBaselineErrorCode)
	}
	if !result.BaselinePublished {
		t.Error("BaselinePublished = false, want true")
	}
	if result.ExpectedBranch != "feature/empty" || result.CurrentBranch != "switched" {
		t.Errorf("branches = (%q, %q), want (feature/empty, switched)", result.ExpectedBranch, result.CurrentBranch)
	}
	// The baseline landed; the task branch must not have.
	if got := remoteBranchSHA(t, originDir, "main"); got == "" {
		t.Error("baseline was not published, so this test did not exercise its window")
	}
	if got := remoteBranchSHA(t, originDir, "feature/empty"); got != "" {
		t.Errorf("task branch published as %q, want unpublished", got)
	}
}

// TestGitOperatorPushReportsPlainMismatchAtSecondVerification drives the other
// outcome of the second verification: a mismatch that did not follow a
// baseline publication. An explicit push target to a remote other than
// "origin" is never baseline-eligible, so moving HEAD in the window between
// the first and second verification must still be caught, and must carry
// plain push_branch_mismatch rather than the after-baseline code.
func TestGitOperatorPushReportsPlainMismatchAtSecondVerification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git PATH shim is a POSIX shell script")
	}
	repoDir, _, backupDir, operator := setupPushRemotesRepo(t)
	realGit, err := osExec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) = %v", err)
	}
	// The explicit-target, no-baseline path issues exactly two "symbolic-ref
	// HEAD" reads: the first verification inside resolvePushPlan, and the
	// second verification immediately before the push. Move HEAD right after
	// answering the first one.
	shimDir := t.TempDir()
	counterPath := filepath.Join(shimDir, "count")
	script := "#!/bin/sh\n" +
		"if [ \"$1 $2\" = 'symbolic-ref HEAD' ]; then\n" +
		"  n=$(cat " + counterPath + " 2>/dev/null || echo 0)\n" +
		"  n=$((n+1))\n" +
		"  echo $n > " + counterPath + "\n" +
		"  " + realGit + " \"$@\"; rc=$?\n" +
		"  if [ \"$n\" = \"1\" ]; then\n" +
		"    " + realGit + " -C " + repoDir + " symbolic-ref HEAD refs/heads/switched\n" +
		"  fi\n" +
		"  exit $rc\n" +
		"fi\n" +
		"exec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := operator.Push(context.Background(), PushOptions{Remote: "backup", ExpectedBranch: "feature/work"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success {
		t.Fatalf("Push() succeeded, want a second-verification mismatch refusal: %+v", result)
	}
	if result.ErrorCode != pushBranchMismatchErrorCode {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, pushBranchMismatchErrorCode)
	}
	if result.BaselinePublished {
		t.Error("BaselinePublished = true, want false: backup is never baseline-eligible")
	}
	if result.ExpectedBranch != "feature/work" || result.CurrentBranch != "switched" {
		t.Errorf("branches = (%q, %q), want (feature/work, switched)", result.ExpectedBranch, result.CurrentBranch)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q, want untouched", got)
	}
}

// TestGitOperatorPushReportsPlainMismatchWhenBaselineWasAlreadyPublished
// covers the empty-remote path where the baseline was published by an earlier
// request: this request's prepareEmptyRemotePublication only retires the
// local marker and pushes nothing itself. A HEAD race at the second
// verification must still report plain push_branch_mismatch, not the
// after-baseline variant, because no baseline publication happened in this
// request.
func TestGitOperatorPushReportsPlainMismatchWhenBaselineWasAlreadyPublished(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git PATH shim is a POSIX shell script")
	}
	repoDir, originDir, operator := setupEmptyRemoteTaskRepo(t)
	// Publish the baseline directly, bypassing gitbootstrap, to simulate an
	// earlier request having already published it. The local marker is left
	// in place, so this request's empty-remote path takes the
	// already-published, retire-only branch.
	runGit(t, repoDir, "push", originDir, "main:refs/heads/main")

	realGit, err := osExec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) = %v", err)
	}
	// Move HEAD right after the remote-ref probe that decides the baseline is
	// already published, which is before this request issues any push.
	shimDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1 $2\" = 'ls-remote --refs' ]; then\n" +
		"  " + realGit + " \"$@\"; rc=$?\n" +
		"  " + realGit + " -C " + repoDir + " symbolic-ref HEAD refs/heads/switched\n" +
		"  exit $rc\n" +
		"fi\n" +
		"exec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := operator.Push(context.Background(), PushOptions{ExpectedBranch: "feature/empty"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if result.Success {
		t.Fatalf("Push() succeeded, want a second-verification mismatch refusal: %+v", result)
	}
	if result.ErrorCode != pushBranchMismatchErrorCode {
		t.Fatalf("ErrorCode = %q, want %q (not the after-baseline variant: no publication happened this request)", result.ErrorCode, pushBranchMismatchErrorCode)
	}
	if result.BaselinePublished {
		t.Error("BaselinePublished = true, want false: the baseline was published by an earlier request, not this one")
	}
	if result.ExpectedBranch != "feature/empty" || result.CurrentBranch != "switched" {
		t.Errorf("branches = (%q, %q), want (feature/empty, switched)", result.ExpectedBranch, result.CurrentBranch)
	}
	if got := remoteBranchSHA(t, originDir, "feature/empty"); got != "" {
		t.Errorf("task branch published as %q, want unpublished", got)
	}
}

// TestGitOperatorPushUsesExpectedBranchNotARaceableRereadForNoTargetRefspec
// covers the no-explicit-target path (the default-remote and
// contribution-destination cases in buildPushPlan both build a bare-name
// refspec off a branch value). That value must come from the same identity
// verifyExpectedBranch confirms immediately before the push, not from a
// separate read: a separate read can observe a different branch if HEAD
// moves and moves back between the first verification and this one, letting
// the second verification pass while the push publishes unrelated content
// under an unverified name and leaves the intended branch unpublished.
func TestGitOperatorPushUsesExpectedBranchNotARaceableRereadForNoTargetRefspec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git PATH shim is a POSIX shell script")
	}
	repoDir, originDir, _, operator := setupPushRemotesRepo(t)
	runGit(t, repoDir, "branch", "feature/other", "main")

	realGit, err := osExec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) = %v", err)
	}
	// Move HEAD to feature/other around the rev-parse call that would decide
	// the push's branch/refspec independently of the first verification, then
	// move it back before the push runs, so only a re-read would observe the
	// switch.
	shimDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1 $2 $3\" = 'rev-parse --abbrev-ref HEAD' ]; then\n" +
		"  " + realGit + " -C " + repoDir + " checkout feature/other >/dev/null 2>&1\n" +
		"  " + realGit + " \"$@\"; rc=$?\n" +
		"  " + realGit + " -C " + repoDir + " checkout feature/work >/dev/null 2>&1\n" +
		"  exit $rc\n" +
		"fi\n" +
		"exec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	wantSHA := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "feature/work"))

	result, err := operator.Push(context.Background(), PushOptions{ExpectedBranch: "feature/work"})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Push() = %+v, want success", result)
	}
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != wantSHA {
		t.Errorf("origin feature/work = %q, want %q: the verified branch was not what got pushed", got, wantSHA)
	}
	if got := remoteBranchSHA(t, originDir, "feature/other"); got != "" {
		t.Errorf("origin gained feature/other (%q): the push targeted a branch never verified", got)
	}
}
