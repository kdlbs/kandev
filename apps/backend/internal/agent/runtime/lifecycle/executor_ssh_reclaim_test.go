package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeSSHRunner answers recorded command outputs. Every test in this file
// drives the reclaimer through it, so no test opens an SSH connection and no
// test can remove anything from the machine running the suite.
type fakeSSHRunner struct {
	responses map[string]fakeSSHResponse
	fallback  *fakeSSHResponse
	calls     []string
}

type directFakeSSHRunner struct {
	*fakeSSHRunner
	directResponses map[string]fakeSSHResponse
	directCalls     []string
}

func (f *directFakeSSHRunner) RunDirect(_ context.Context, cmd string) (string, string, error) {
	f.directCalls = append(f.directCalls, cmd)
	if resp, ok := f.directResponses[cmd]; ok {
		return resp.stdout, resp.stderr, resp.err
	}
	return "", "", errors.New("unexpected direct command: " + cmd)
}

type fakeSSHResponse struct {
	stdout string
	stderr string
	err    error
}

func newFakeSSHRunner() *fakeSSHRunner {
	return &fakeSSHRunner{responses: map[string]fakeSSHResponse{}}
}

func (f *fakeSSHRunner) on(cmd, stdout string) *fakeSSHRunner {
	f.responses[cmd] = fakeSSHResponse{stdout: stdout}
	return f
}

func (f *fakeSSHRunner) onErr(cmd string, err error) *fakeSSHRunner {
	f.responses[cmd] = fakeSSHResponse{err: err}
	return f
}

func (f *fakeSSHRunner) Run(_ context.Context, cmd string) (string, string, error) {
	f.calls = append(f.calls, cmd)
	if resp, ok := f.responses[cmd]; ok {
		return resp.stdout, resp.stderr, resp.err
	}
	if f.fallback != nil {
		return f.fallback.stdout, f.fallback.stderr, f.fallback.err
	}
	return "", "", errors.New("unexpected command: " + cmd)
}

func (f *fakeSSHRunner) ranAny(substr string) bool {
	for _, call := range f.calls {
		if strings.Contains(call, substr) {
			return true
		}
	}
	return false
}

func newTestReclaimer(runner sshRemoteRunner) *SSHTaskDirReclaimer {
	return &SSHTaskDirReclaimer{runner: runner}
}

const (
	testWorkdirRoot = "/home/dev/.kandev"
	testTaskDir     = "/home/dev/.kandev/tasks/task-abc123"
)

// cleanCheckout registers the four probe answers that make one checkout safe.
func cleanCheckout(f *fakeSSHRunner, dir string) *fakeSSHRunner {
	f.on(sshReclaimIsCheckoutCmd(dir), "true")
	f.on(sshReclaimStatusCmd(dir), "")
	f.on(sshReclaimUnpushedCmd(dir), "0")
	f.on(sshReclaimStashCmd(dir), "")
	return f
}

// safeSingleRepoRunner is the baseline: a task dir that is itself a clean
// checkout, with no additional repository children.
func safeSingleRepoRunner() *fakeSSHRunner {
	f := newFakeSSHRunner()
	f.on(sshReclaimChildReposCmd(testTaskDir), "")
	cleanCheckout(f, testTaskDir)
	return f
}

func TestSSHTaskDirReclaimValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		path    string
		wantErr bool
	}{
		{name: "canonical task dir", root: testWorkdirRoot, path: testTaskDir},
		{name: "trailing slash tolerated", root: testWorkdirRoot, path: testTaskDir + "/"},
		{name: "root with trailing slash", root: testWorkdirRoot + "/", path: testTaskDir},
		{name: "tasks dir itself", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks", wantErr: true},
		{name: "tasks dir with slash", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks/", wantErr: true},
		{name: "empty segment", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks//", wantErr: true},
		{name: "parent traversal", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks/..", wantErr: true},
		{name: "traversal escaping root", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks/../../etc", wantErr: true},
		{name: "dot segment", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks/.", wantErr: true},
		{name: "nested segment", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks/a/b", wantErr: true},
		{name: "outside root", root: testWorkdirRoot, path: "/home/dev/other/tasks/task-abc123", wantErr: true},
		{name: "workdir root itself", root: testWorkdirRoot, path: testWorkdirRoot, wantErr: true},
		{name: "filesystem root", root: testWorkdirRoot, path: "/", wantErr: true},
		{name: "empty path", root: testWorkdirRoot, path: "", wantErr: true},
		{name: "empty root", root: "", path: testTaskDir, wantErr: true},
		{name: "relative path", root: testWorkdirRoot, path: "tasks/task-abc123", wantErr: true},
		{name: "sibling prefix confusion", root: testWorkdirRoot, path: testWorkdirRoot + "/tasks-other/x", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSSHReclaimPath(tc.root, tc.path)
			if tc.wantErr && err == nil {
				t.Fatalf("validateSSHReclaimPath(%q, %q) = nil, want error", tc.root, tc.path)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateSSHReclaimPath(%q, %q) = %v, want nil", tc.root, tc.path, err)
			}
			if tc.wantErr && !errors.Is(err, ErrSSHReclaimPathRefused) {
				t.Fatalf("error %v does not wrap ErrSSHReclaimPathRefused", err)
			}
		})
	}
}

func TestSSHTaskDirReclaimProbeSafe(t *testing.T) {
	f := safeSingleRepoRunner()
	verdict := newTestReclaimer(f).probe(context.Background(), testTaskDir)
	if !verdict.Safe {
		t.Fatalf("probe verdict = %+v, want safe", verdict)
	}
}

func TestSSHTaskDirReclaimProbeSkipReasons(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*fakeSSHRunner)
		wantReason SSHReclaimSkipReason
	}{
		{
			name:       "dirty worktree",
			mutate:     func(f *fakeSSHRunner) { f.on(sshReclaimStatusCmd(testTaskDir), " M internal/foo.go") },
			wantReason: SSHReclaimSkipDirtyWorktree,
		},
		{
			name:       "untracked file",
			mutate:     func(f *fakeSSHRunner) { f.on(sshReclaimStatusCmd(testTaskDir), "?? scratch.txt") },
			wantReason: SSHReclaimSkipDirtyWorktree,
		},
		{
			name:       "unpushed commits",
			mutate:     func(f *fakeSSHRunner) { f.on(sshReclaimUnpushedCmd(testTaskDir), "3") },
			wantReason: SSHReclaimSkipUnpushedCommits,
		},
		{
			name:       "stash present",
			mutate:     func(f *fakeSSHRunner) { f.on(sshReclaimStashCmd(testTaskDir), "stash@{0}: WIP on main") },
			wantReason: SSHReclaimSkipStashPresent,
		},
		{
			name:       "task dir is not a checkout",
			mutate:     func(f *fakeSSHRunner) { f.on(sshReclaimIsCheckoutCmd(testTaskDir), "false") },
			wantReason: SSHReclaimSkipNotACheckout,
		},
		{
			name:       "checkout probe errors",
			mutate:     func(f *fakeSSHRunner) { f.onErr(sshReclaimIsCheckoutCmd(testTaskDir), errors.New("git: not found")) },
			wantReason: SSHReclaimSkipProbeFailed,
		},
		{
			name:       "status probe errors",
			mutate:     func(f *fakeSSHRunner) { f.onErr(sshReclaimStatusCmd(testTaskDir), errors.New("connection lost")) },
			wantReason: SSHReclaimSkipProbeFailed,
		},
		{
			name:       "unpushed probe errors",
			mutate:     func(f *fakeSSHRunner) { f.onErr(sshReclaimUnpushedCmd(testTaskDir), errors.New("unborn HEAD")) },
			wantReason: SSHReclaimSkipProbeFailed,
		},
		{
			name:       "unpushed probe returns unparseable output",
			mutate:     func(f *fakeSSHRunner) { f.on(sshReclaimUnpushedCmd(testTaskDir), "not-a-number") },
			wantReason: SSHReclaimSkipProbeFailed,
		},
		{
			name:       "unpushed probe returns empty output",
			mutate:     func(f *fakeSSHRunner) { f.on(sshReclaimUnpushedCmd(testTaskDir), "") },
			wantReason: SSHReclaimSkipProbeFailed,
		},
		{
			name:       "stash probe errors",
			mutate:     func(f *fakeSSHRunner) { f.onErr(sshReclaimStashCmd(testTaskDir), errors.New("boom")) },
			wantReason: SSHReclaimSkipProbeFailed,
		},
		{
			name: "child enumeration errors",
			mutate: func(f *fakeSSHRunner) {
				f.onErr(sshReclaimChildReposCmd(testTaskDir), errors.New("find: permission denied"))
			},
			wantReason: SSHReclaimSkipProbeFailed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := safeSingleRepoRunner()
			tc.mutate(f)
			verdict := newTestReclaimer(f).probe(context.Background(), testTaskDir)
			if verdict.Safe {
				t.Fatalf("probe verdict = safe, want skip %q", tc.wantReason)
			}
			if verdict.Reason != tc.wantReason {
				t.Fatalf("probe reason = %q, want %q", verdict.Reason, tc.wantReason)
			}
		})
	}
}

func TestSSHTaskDirReclaimProbePreservesIgnoredFiles(t *testing.T) {
	f := safeSingleRepoRunner()
	f.on(sshReclaimStatusCmd(testTaskDir), "!! .env\n")

	verdict := newTestReclaimer(f).probe(context.Background(), testTaskDir)
	if verdict.Safe {
		t.Fatal("probe verdict = safe, want ignored file to preserve the directory")
	}
	if verdict.Reason != SSHReclaimSkipDirtyWorktree {
		t.Fatalf("probe reason = %q, want %q", verdict.Reason, SSHReclaimSkipDirtyWorktree)
	}
}

func TestSSHTaskDirReclaimProbeIgnoresKandevRuntimeFiles(t *testing.T) {
	f := safeSingleRepoRunner()
	f.on(sshReclaimStatusCmd(testTaskDir), "!! .kandev/\n")

	verdict := newTestReclaimer(f).probe(context.Background(), testTaskDir)
	if !verdict.Safe {
		t.Fatalf("probe verdict = %+v, want safe for Kandev-owned runtime files", verdict)
	}
}

func TestSSHTaskDirReclaimProbeInspectsChildRepositories(t *testing.T) {
	childA := testTaskDir + "/api-main"
	childB := testTaskDir + "/web-main"

	f := newFakeSSHRunner()
	f.on(sshReclaimChildReposCmd(testTaskDir), childA+"/.git\n"+childB+"/.git\n")
	cleanCheckout(f, testTaskDir)
	cleanCheckout(f, childA)
	cleanCheckout(f, childB)

	if verdict := newTestReclaimer(f).probe(context.Background(), testTaskDir); !verdict.Safe {
		t.Fatalf("probe verdict = %+v, want safe", verdict)
	}

	// A second repository holding unpushed commits must veto the whole task
	// directory even though the primary checkout is clean.
	f.on(sshReclaimUnpushedCmd(childB), "2")
	verdict := newTestReclaimer(f).probe(context.Background(), testTaskDir)
	if verdict.Safe {
		t.Fatal("probe verdict = safe, want skip for unpushed commits in a child repository")
	}
	if verdict.Reason != SSHReclaimSkipUnpushedCommits {
		t.Fatalf("probe reason = %q, want %q", verdict.Reason, SSHReclaimSkipUnpushedCommits)
	}
	if !strings.Contains(verdict.Detail, childB) {
		t.Fatalf("verdict detail %q does not name the offending checkout %q", verdict.Detail, childB)
	}
}

func TestSSHTaskDirReclaimProbeRejectsUnexpectedChildEntry(t *testing.T) {
	f := safeSingleRepoRunner()
	f.on(sshReclaimChildReposCmd(testTaskDir), testTaskDir+"/not-a-git-entry\n")

	verdict := newTestReclaimer(f).probe(context.Background(), testTaskDir)
	if verdict.Safe {
		t.Fatal("probe verdict = safe, want malformed child enumeration to preserve the directory")
	}
	if verdict.Reason != SSHReclaimSkipProbeFailed {
		t.Fatalf("probe reason = %q, want %q", verdict.Reason, SSHReclaimSkipProbeFailed)
	}
}

func TestSSHTaskDirReclaimProbeIsReadOnly(t *testing.T) {
	f := safeSingleRepoRunner()
	newTestReclaimer(f).probe(context.Background(), testTaskDir)
	for _, forbidden := range []string{"fetch", "push", "prune", " gc", "reset", "clean", "rm ", "checkout"} {
		if f.ranAny(forbidden) {
			t.Fatalf("probe ran a mutating command containing %q: %v", forbidden, f.calls)
		}
	}
}

func TestSSHTaskDirReclaimRemovesWhenSafe(t *testing.T) {
	f := safeSingleRepoRunner()
	f.on(sshReclaimRemoveCmd(testTaskDir), "")
	f.on(sshReclaimExistsCmd(testTaskDir), sshReclaimGoneMarker)

	outcome, verdict, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, testTaskDir)
	if err != nil {
		t.Fatalf("Reclaim returned error: %v", err)
	}
	if outcome != SSHReclaimOutcomeRemoved {
		t.Fatalf("outcome = %q, want %q (verdict %+v)", outcome, SSHReclaimOutcomeRemoved, verdict)
	}
	if !f.ranAny("rm -rf") {
		t.Fatalf("expected a removal command, got calls: %v", f.calls)
	}
}

func TestSSHTaskDirReclaimSkipDoesNotRemove(t *testing.T) {
	f := safeSingleRepoRunner()
	f.on(sshReclaimStatusCmd(testTaskDir), " M main.go")

	outcome, verdict, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, testTaskDir)
	if err != nil {
		t.Fatalf("Reclaim returned error for a safety skip: %v", err)
	}
	if outcome != SSHReclaimOutcomeSkipped {
		t.Fatalf("outcome = %q, want %q", outcome, SSHReclaimOutcomeSkipped)
	}
	if verdict.Reason != SSHReclaimSkipDirtyWorktree {
		t.Fatalf("verdict reason = %q, want %q", verdict.Reason, SSHReclaimSkipDirtyWorktree)
	}
	if f.ranAny("rm -rf") {
		t.Fatalf("removal ran despite an unsafe verdict: %v", f.calls)
	}
}

func TestSSHTaskDirReclaimRefusedPathNeverRemoves(t *testing.T) {
	for _, path := range []string{
		testWorkdirRoot + "/tasks",
		testWorkdirRoot + "/tasks/",
		testWorkdirRoot,
		"/",
		testWorkdirRoot + "/tasks/../../..",
		"/Users/someone/kandev-workspaces/tasks/task-live",
	} {
		t.Run(path, func(t *testing.T) {
			f := newFakeSSHRunner()
			f.fallback = &fakeSSHResponse{}

			outcome, _, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, path)
			if err == nil {
				t.Fatalf("Reclaim(%q) = nil error, want refusal", path)
			}
			if !errors.Is(err, ErrSSHReclaimPathRefused) {
				t.Fatalf("error %v does not wrap ErrSSHReclaimPathRefused", err)
			}
			if outcome == SSHReclaimOutcomeRemoved {
				t.Fatalf("outcome = %q for a refused path", outcome)
			}
			if len(f.calls) != 0 {
				t.Fatalf("refused path still ran remote commands: %v", f.calls)
			}
		})
	}
}

func TestSSHTaskDirReclaimRemovalFailureIsError(t *testing.T) {
	t.Run("rm exits non-zero", func(t *testing.T) {
		f := safeSingleRepoRunner()
		f.onErr(sshReclaimRemoveCmd(testTaskDir), errors.New("rm: permission denied"))

		outcome, _, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, testTaskDir)
		if err == nil {
			t.Fatal("Reclaim returned nil error after a failed rm")
		}
		if outcome == SSHReclaimOutcomeRemoved {
			t.Fatalf("outcome = %q after a failed rm", outcome)
		}
	})

	t.Run("directory survives removal", func(t *testing.T) {
		f := safeSingleRepoRunner()
		f.on(sshReclaimRemoveCmd(testTaskDir), "")
		f.on(sshReclaimExistsCmd(testTaskDir), sshReclaimExistsMarker)

		outcome, _, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, testTaskDir)
		if err == nil {
			t.Fatal("Reclaim returned nil error while the directory still exists")
		}
		if outcome == SSHReclaimOutcomeRemoved {
			t.Fatalf("outcome = %q while the directory still exists", outcome)
		}
	})

	t.Run("post-removal check fails to run", func(t *testing.T) {
		f := safeSingleRepoRunner()
		f.on(sshReclaimRemoveCmd(testTaskDir), "")
		f.onErr(sshReclaimExistsCmd(testTaskDir), errors.New("connection reset"))

		_, _, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, testTaskDir)
		if err == nil {
			t.Fatal("Reclaim returned nil error when the post-removal check could not run")
		}
	})
}

func TestSSHTaskDirReclaimRemovalTargetsOnlyTheTaskDir(t *testing.T) {
	f := safeSingleRepoRunner()
	f.on(sshReclaimRemoveCmd(testTaskDir), "")
	f.on(sshReclaimExistsCmd(testTaskDir), sshReclaimGoneMarker)

	if _, _, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, testTaskDir); err != nil {
		t.Fatalf("Reclaim returned error: %v", err)
	}
	for _, call := range f.calls {
		if !strings.Contains(call, "rm -rf") {
			continue
		}
		if !strings.Contains(call, shellQuote(testTaskDir)) {
			t.Fatalf("removal command %q does not target the quoted task dir", call)
		}
		if strings.Contains(call, shellQuote(testWorkdirRoot+"/tasks")) {
			t.Fatalf("removal command %q targets the tasks root", call)
		}
	}
}

func TestSSHTaskDirReclaimQuotesHostilePaths(t *testing.T) {
	// A task dir name can only reach here after validation, but the quoting
	// must hold regardless: a single quote in the path must not break out of
	// the shell argument.
	hostile := testWorkdirRoot + "/tasks/task-o'brien; rm -rf tmp"
	quoted := shellQuote(hostile)
	if strings.Contains(quoted[1:len(quoted)-1], "'") && !strings.Contains(quoted, `'\''`) {
		t.Fatalf("shellQuote(%q) = %q leaves an unescaped quote", hostile, quoted)
	}
	if err := validateSSHReclaimPath(testWorkdirRoot, hostile); err != nil {
		t.Fatalf("a single path segment containing quotes should validate, got %v", err)
	}
}

// The SSH executor builds a task directory from the *expanded* workspace root
// (ensureRemoteTaskDir calls expandRemoteHome first) while the profile stores
// the unexpanded form, and stores nothing at all when the field is left blank.
// Without matching expansion here the path guard would refuse every
// default-configured host and the feature would silently never fire.
func TestSSHTaskDirReclaimExpandsRemoteHomeInWorkspaceRoot(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{name: "tilde root", root: "~/.kandev"},
		{name: "blank root falls back to the executor default", root: ""},
		{name: "bare tilde", root: "~"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskDir := testTaskDir
			if test.root == "~" {
				taskDir = "/home/dev/tasks/task-abc123"
			}
			f := newFakeSSHRunner()
			f.on(sshReclaimHomeCmd(), "/home/dev\n")
			f.on(sshReclaimChildReposCmd(taskDir), "")
			cleanCheckout(f, taskDir)
			f.on(sshReclaimRemoveCmd(taskDir), "")
			f.on(sshReclaimExistsCmd(taskDir), sshReclaimGoneMarker)

			outcome, _, err := newTestReclaimer(f).Reclaim(context.Background(), test.root, taskDir)
			if err != nil {
				t.Fatalf("Reclaim: %v", err)
			}
			if outcome != SSHReclaimOutcomeRemoved {
				t.Fatalf("outcome = %q, want removed", outcome)
			}
		})
	}
}

func TestSSHTaskDirReclaimUsesDirectHomeProbe(t *testing.T) {
	f := &directFakeSSHRunner{
		fakeSSHRunner:   newFakeSSHRunner(),
		directResponses: map[string]fakeSSHResponse{sshReclaimHomeCmd(): {stdout: "/home/dev\n"}},
	}
	root, err := newTestReclaimer(f).resolveWorkdirRoot(context.Background(), "~/.kandev")
	if err != nil {
		t.Fatalf("resolveWorkdirRoot: %v", err)
	}
	if root != "/home/dev/.kandev" {
		t.Fatalf("resolved root = %q, want /home/dev/.kandev", root)
	}
	if len(f.directCalls) != 1 || f.directCalls[0] != sshReclaimHomeCmd() {
		t.Fatalf("direct calls = %v, want one unwrapped home probe", f.directCalls)
	}
	if len(f.calls) != 0 {
		t.Fatalf("login-shell calls = %v, want none for the home probe", f.calls)
	}
}

func TestSSHTaskDirReclaimAbsoluteRootAsksTheRemoteNothingExtra(t *testing.T) {
	f := safeSingleRepoRunner()
	f.on(sshReclaimRemoveCmd(testTaskDir), "")
	f.on(sshReclaimExistsCmd(testTaskDir), sshReclaimGoneMarker)

	if _, _, err := newTestReclaimer(f).Reclaim(context.Background(), testWorkdirRoot, testTaskDir); err != nil {
		t.Fatalf("Reclaim: %v", err)
	}
	if f.ranAny("$HOME") {
		t.Fatalf("resolved $HOME for an already-absolute root: %v", f.calls)
	}
}

// A workspace root that cannot be resolved leaves the guard unable to say the
// directory is one Kandev manages, so nothing is removed.
func TestSSHTaskDirReclaimUnresolvableHomeNeverRemoves(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeSSHRunner)
	}{
		{
			name:  "probe fails",
			setup: func(f *fakeSSHRunner) { f.onErr(sshReclaimHomeCmd(), errors.New("connection reset")) },
		},
		{
			name:  "empty home",
			setup: func(f *fakeSSHRunner) { f.on(sshReclaimHomeCmd(), "  ") },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFakeSSHRunner()
			test.setup(f)
			outcome, _, err := newTestReclaimer(f).Reclaim(context.Background(), "~/.kandev", testTaskDir)
			if err == nil {
				t.Fatal("Reclaim returned nil; an unresolvable root must be an error")
			}
			if outcome != SSHReclaimOutcomeFailed {
				t.Fatalf("outcome = %q, want failed", outcome)
			}
			if f.ranAny("rm -rf") {
				t.Fatalf("issued a removal without resolving the root: %v", f.calls)
			}
		})
	}
}

// The expansion must not become a way to widen the guard: a task directory that
// is not directly beneath the expanded root is still refused.
func TestSSHTaskDirReclaimExpandedRootStillBoundsThePath(t *testing.T) {
	f := newFakeSSHRunner()
	f.on(sshReclaimHomeCmd(), "/home/dev")

	_, _, err := newTestReclaimer(f).Reclaim(context.Background(), "~/.kandev", "/home/dev/.kandev/tasks/a/b")
	if !errors.Is(err, ErrSSHReclaimPathRefused) {
		t.Fatalf("err = %v, want ErrSSHReclaimPathRefused", err)
	}
	if f.ranAny("rm -rf") {
		t.Fatalf("issued a removal for a refused path: %v", f.calls)
	}
}
