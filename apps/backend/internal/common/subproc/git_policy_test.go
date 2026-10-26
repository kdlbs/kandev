//go:build unix

package subproc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type policyRepo struct {
	t      *testing.T
	dir    string
	marker string
}

// runGit runs a managed Git command in dir and returns its combined output.
func runGit(ctx context.Context, t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := NewGitCommand(ctx, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=n", "GIT_AUTHOR_EMAIL=n@example.com",
		"GIT_COMMITTER_NAME=n", "GIT_COMMITTER_EMAIL=n@example.com")
	out, err := RunGitCombinedOutputClass(ctx, GitLifecycle, cmd)
	return string(out), err
}

func plainGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	// The fixture plants programs in the repository configuration; the
	// harness's own commands must not start them.
	args = append([]string{"-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null",
		"-c", "filter.evil.clean=", "-c", "filter.evil.smudge="}, args...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=n", "GIT_AUTHOR_EMAIL=n@example.com",
		"GIT_COMMITTER_NAME=n", "GIT_COMMITTER_EMAIL=n@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// newPolicyRepo creates a repository with a marker-writing script for every
// kind of repository-supplied program.
func newPolicyRepo(t *testing.T) *policyRepo {
	t.Helper()
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker")
	repo := &policyRepo{t: t, dir: dir, marker: marker}
	plainGit(t, dir, "init", "-q", "-b", "main")
	script := filepath.Join(t.TempDir(), "mark.sh")
	writeExecutable(t, script, "#!/bin/sh\necho \"$0 $*\" >> '"+marker+"'\ncat\n")
	hooks := filepath.Join(dir, ".git", "hooks")
	for _, name := range []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit",
		"pre-merge-commit", "post-merge", "pre-rebase", "post-rewrite", "post-checkout", "pre-push"} {
		writeExecutable(t, filepath.Join(hooks, name), "#!/bin/sh\necho hook-"+name+" >> '"+marker+"'\n")
	}
	plainGit(t, dir, "config", "core.fsmonitor", script)
	plainGit(t, dir, "config", "diff.evil.textconv", script)
	plainGit(t, dir, "config", "filter.evil.clean", script)
	plainGit(t, dir, "config", "filter.evil.smudge", script)
	plainGit(t, dir, "config", "merge.evil.driver", "echo merge-driver >> '"+marker+"'; exit 1")
	writeExecutable(t, filepath.Join(t.TempDir(), "x"), "")
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.evil filter=evil diff=evil merge=evil\n*.dat filter=evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Seed the first commit with the real binary so the planted programs have
	// not run yet.
	plainGit(t, dir, "add", ".gitattributes", "base.txt")
	plainGit(t, dir, "commit", "-q", "-m", "base")
	return repo
}

func (r *policyRepo) marked() bool {
	_, err := os.Stat(r.marker)
	return err == nil
}

func (r *policyRepo) markerText() string {
	data, _ := os.ReadFile(r.marker)
	return string(data)
}

func mustGit(ctx context.Context, t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(ctx, t, dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func TestManagedGitRunsNoRepositorySuppliedCodeByDefault(t *testing.T) {
	repo := newPolicyRepo(t)
	ctx := context.Background()
	dir := repo.dir
	bare := t.TempDir()
	plainGit(t, bare, "init", "-q", "--bare", "-b", "main")
	plainGit(t, dir, "remote", "add", "origin", bare)

	mustGit(ctx, t, dir, "status", "--porcelain")
	if err := os.WriteFile(filepath.Join(dir, "a.evil"), []byte("evil content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(ctx, t, dir, "add", "a.evil")
	mustGit(ctx, t, dir, "commit", "-m", "second")
	if err := os.WriteFile(filepath.Join(dir, "a.evil"), []byte("evil content two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(ctx, t, dir, "diff")
	mustGit(ctx, t, dir, "diff", "--stat")
	mustGit(ctx, t, dir, "log", "-p", "-n", "2")
	mustGit(ctx, t, dir, "checkout", "-q", "--", "a.evil")
	mustGit(ctx, t, dir, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(ctx, t, dir, "add", "f.txt")
	mustGit(ctx, t, dir, "commit", "-m", "feature")
	mustGit(ctx, t, dir, "checkout", "-q", "main")
	mustGit(ctx, t, dir, "merge", "--no-ff", "-m", "merge", "feature")
	mustGit(ctx, t, dir, "push", "-u", "origin", "main")
	mustGit(ctx, t, dir, "pull", "--rebase", "origin", "main")
	mustGit(ctx, t, dir, "pull", "--no-rebase", "origin", "main")
	mustGit(ctx, t, dir, "revert", "--no-edit", "HEAD~1")
	mustGit(ctx, t, dir, "reset", "-q", "--hard", "HEAD~1")
	mustGit(ctx, t, dir, "worktree", "add", "-q", filepath.Join(t.TempDir(), "wt"), "-b", "wtbranch")
	mustGit(ctx, t, dir, "rebase", "feature")
	mustGit(ctx, t, dir, "stash", "list")

	if repo.marked() {
		t.Fatalf("a repository-supplied program ran:\n%s", repo.markerText())
	}
}

func TestManagedGitRunsHooksWhenRepositoryOptsIn(t *testing.T) {
	repo := newPolicyRepo(t)
	ctx := WithGitHooks(context.Background(), true)
	if err := os.WriteFile(filepath.Join(repo.dir, "f.txt"), []byte("f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(ctx, t, repo.dir, "add", "f.txt")
	mustGit(ctx, t, repo.dir, "commit", "-m", "hooked")
	text := repo.markerText()
	for _, hook := range []string{"hook-pre-commit", "hook-commit-msg", "hook-post-commit"} {
		if !strings.Contains(text, hook) {
			t.Fatalf("%s did not run with hooks enabled:\n%s", hook, text)
		}
	}
	if strings.Contains(text, "mark.sh") {
		t.Fatalf("hooks opt-in must not re-enable other programs:\n%s", text)
	}
}

func TestManagedGitFailingHookFailsOperationWhenEnabled(t *testing.T) {
	repo := newPolicyRepo(t)
	writeExecutable(t, filepath.Join(repo.dir, ".git", "hooks", "pre-commit"), "#!/bin/sh\necho blocked-by-hook >&2\nexit 1\n")
	if err := os.WriteFile(filepath.Join(repo.dir, "f.txt"), []byte("f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := WithGitHooks(context.Background(), true)
	mustGit(ctx, t, repo.dir, "add", "f.txt")
	out, err := runGit(ctx, t, repo.dir, "commit", "-m", "x")
	if err == nil || !strings.Contains(out, "blocked-by-hook") {
		t.Fatalf("expected the hook's output in a failed commit, got err=%v out=%q", err, out)
	}
	if _, err := runGit(context.Background(), t, repo.dir, "commit", "-m", "x"); err != nil {
		t.Fatalf("commit with hooks off must skip the failing hook: %v", err)
	}
}

func TestManagedGitExternalDiffEnvironmentIsRemoved(t *testing.T) {
	repo := newPolicyRepo(t)
	script := filepath.Join(t.TempDir(), "ext.sh")
	writeExecutable(t, script, "#!/bin/sh\necho ext >> '"+repo.marker+"'\n")
	if err := os.WriteFile(filepath.Join(repo.dir, "base.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := NewGitCommand(context.Background(), "diff")
	cmd.Dir = repo.dir
	cmd.Env = append(os.Environ(), "GIT_EXTERNAL_DIFF="+script, "GIT_CONFIG_GLOBAL=/dev/null")
	if _, err := RunGitCombinedOutputClass(context.Background(), GitLifecycle, cmd); err != nil {
		t.Fatal(err)
	}
	if repo.marked() {
		t.Fatal("GIT_EXTERNAL_DIFF program ran")
	}
}

func TestManagedGitRequiredNonAllowlistedFilter(t *testing.T) {
	repo := newPolicyRepo(t)
	plainGit(t, repo.dir, "config", "filter.evil.required", "true")
	if err := os.WriteFile(filepath.Join(repo.dir, "a.evil"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	out, err := runGit(ctx, t, repo.dir, "add", "a.evil")
	if err == nil || !strings.Contains(out+err.Error(), "evil") || !strings.Contains(err.Error(), "required but not permitted") {
		t.Fatalf("content-writing command must be refused naming the filter: err=%v out=%q", err, out)
	}
	mustGit(ctx, t, repo.dir, "status", "--porcelain")
	mustGit(ctx, t, repo.dir, "diff")
	if repo.marked() {
		t.Fatalf("filter ran:\n%s", repo.markerText())
	}
}

func TestManagedGitRefusesUndeterminableDriverNames(t *testing.T) {
	repo := newPolicyRepo(t)
	plainGit(t, repo.dir, "config", "filter.a=b.clean", "touch "+repo.marker)
	_, err := runGit(context.Background(), t, repo.dir, "status")
	if err == nil || !strings.Contains(err.Error(), "a=b") {
		t.Fatalf("expected refusal naming the driver, got %v", err)
	}
	if repo.marked() {
		t.Fatal("program started")
	}
}

func TestManagedGitCallerPolicyOptionsAreDropped(t *testing.T) {
	repo := newPolicyRepo(t)
	cmd := NewGitCommand(context.Background(), "-c", "core.hooksPath=.git/hooks", "-ccore.fsmonitor=x",
		"--config-env=Core.HooksPath=HP", "status")
	cmd.Dir = repo.dir
	cmd.Env = append(os.Environ(), "HP=.git/hooks", "GIT_CONFIG_GLOBAL=/dev/null")
	if _, err := RunGitCombinedOutputClass(context.Background(), GitLifecycle, cmd); err != nil {
		t.Fatal(err)
	}
	if repo.marked() {
		t.Fatalf("policy was weakened:\n%s", repo.markerText())
	}
}

func TestManagedGitRefusesTextConversionOptions(t *testing.T) {
	repo := newPolicyRepo(t)
	for _, args := range [][]string{
		{"blame", "base.txt"}, {"diff", "--textconv"}, {"log", "--ext-diff"}, {"cat-file", "--filters", "HEAD:base.txt"},
		{"--exec-path=/x", "status"}, {"-p", "status"},
	} {
		if _, err := runGit(context.Background(), t, repo.dir, args...); err == nil || !strings.Contains(err.Error(), "refuses") {
			t.Errorf("git %v: expected refusal, got %v", args, err)
		}
	}
}

func TestManagedGitRefusesRecursionOverrideWhenHooksEnabled(t *testing.T) {
	repo := newPolicyRepo(t)
	ctx := WithGitHooks(context.Background(), true)
	if _, err := runGit(ctx, t, repo.dir, "fetch", "--recurse-submodules=yes"); err == nil {
		t.Fatal("expected refusal of --recurse-submodules=yes")
	}
	bare := t.TempDir()
	plainGit(t, bare, "init", "-q", "--bare", "-b", "main")
	plainGit(t, repo.dir, "remote", "add", "origin", bare)
	if out, err := runGit(ctx, t, repo.dir, "fetch", "--no-recurse-submodules", "origin"); err != nil {
		t.Log(out)
		t.Fatalf("--no-recurse-submodules must pass: %v", err)
	}
}

func TestManagedGitMergeDriverIsNeutralized(t *testing.T) {
	repo := newPolicyRepo(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(repo.dir, "m.evil"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plainGit(t, repo.dir, "add", "m.evil")
	plainGit(t, repo.dir, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "m")
	plainGit(t, repo.dir, "checkout", "-q", "-b", "side")
	if err := os.WriteFile(filepath.Join(repo.dir, "m.evil"), []byte("a\nB\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plainGit(t, repo.dir, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-am", "side")
	plainGit(t, repo.dir, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(repo.dir, "m.evil"), []byte("a\nb\nC\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plainGit(t, repo.dir, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-am", "main")
	// Git treats an empty driver command as a failed driver: the merge stops
	// with a conflict and the planted command is never started.
	out, err := runGit(ctx, t, repo.dir, "merge", "side")
	if err == nil || !strings.Contains(out, "CONFLICT") {
		t.Fatalf("expected a conflict from the neutralized driver, err=%v out=%q", err, out)
	}
	if repo.marked() {
		t.Fatalf("merge driver ran:\n%s", repo.markerText())
	}
}

func fakeLFSOnPath(t *testing.T, marker string) {
	t.Helper()
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "git-lfs"), `#!/bin/sh
case "$1" in
  clean) printf 'LFS:'; cat ;;
  smudge) cat ;;
  pre-push) echo "lfs-pre-push $*" >> '`+marker+`'; cat >/dev/null ;;
esac
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestManagedGitAllowlistedLFSFilterStillApplies(t *testing.T) {
	repo := newPolicyRepo(t)
	fakeLFSOnPath(t, repo.marker)
	plainGit(t, repo.dir, "config", "filter.lfs.clean", "git-lfs clean -- %f")
	plainGit(t, repo.dir, "config", "filter.lfs.smudge", "git-lfs smudge -- %f")
	plainGit(t, repo.dir, "config", "filter.lfs.process", "git-lfs filter-process")
	plainGit(t, repo.dir, "config", "filter.lfs.required", "true")
	if err := os.WriteFile(filepath.Join(repo.dir, ".gitattributes"), []byte("*.bin filter=lfs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// filter-process is unsupported by the fake; use clean/smudge only.
	plainGit(t, repo.dir, "config", "--unset", "filter.lfs.process")
	if err := os.WriteFile(filepath.Join(repo.dir, "x.bin"), []byte("payload\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mustGit(ctx, t, repo.dir, "add", ".gitattributes", "x.bin")
	mustGit(ctx, t, repo.dir, "commit", "-m", "lfs")
	blob := mustGit(ctx, t, repo.dir, "cat-file", "-p", "HEAD:x.bin")
	if blob != "LFS:payload\n" {
		t.Fatalf("allowlisted clean filter did not apply, blob = %q", blob)
	}
}

func TestManagedGitPushRunsLFSPrePushOnlyWhenHooksOff(t *testing.T) {
	repo := newPolicyRepo(t)
	fakeLFSOnPath(t, repo.marker)
	plainGit(t, repo.dir, "config", "filter.lfs.clean", "git-lfs clean -- %f")
	plainGit(t, repo.dir, "config", "filter.lfs.smudge", "git-lfs smudge -- %f")
	bare := t.TempDir()
	plainGit(t, bare, "init", "-q", "--bare", "-b", "main")
	plainGit(t, repo.dir, "remote", "add", "origin", bare)
	ctx := context.Background()

	mustGit(ctx, t, repo.dir, "push", "--dry-run", "origin", "main")
	mustGit(ctx, t, repo.dir, "push", "-n", "origin", "main")
	if repo.marked() {
		t.Fatalf("a dry run must run no hook:\n%s", repo.markerText())
	}
	if _, err := runGit(ctx, t, repo.dir, "push", "--no-verify", "origin", "main"); err == nil || !strings.Contains(err.Error(), "--no-verify") {
		t.Fatalf("--no-verify must be refused for an LFS push, got %v", err)
	}
	mustGit(ctx, t, repo.dir, "push", "origin", "main")
	text := repo.markerText()
	if !strings.Contains(text, "lfs-pre-push pre-push origin") || strings.Contains(text, "hook-pre-push") {
		t.Fatalf("expected only the LFS pre-push step, got:\n%s", text)
	}
}

func TestLFSHooksDirIsRepairedWithFreshDirectory(t *testing.T) {
	t.Cleanup(RemoveManagedGitHooksDir)
	first, err := lfsHooksDir("/usr/bin/git-lfs")
	if err != nil {
		t.Fatal(err)
	}
	second, err := lfsHooksDir("/usr/bin/git-lfs")
	if err != nil || second != first {
		t.Fatalf("valid directory must be reused: %q %q %v", first, second, err)
	}
	if err := os.Chmod(filepath.Join(first, "pre-push"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := lfsHooksDir("/usr/bin/git-lfs")
	if err != nil || third == first {
		t.Fatalf("an invalid directory must be replaced by a fresh one: %q %q %v", first, third, err)
	}
	content, _ := os.ReadFile(filepath.Join(third, "pre-push"))
	if !strings.Contains(string(content), "'/usr/bin/git-lfs' pre-push \"$@\"") {
		t.Fatalf("unexpected hook content %q", content)
	}
}

func TestManagedGitArgsIsDeterministicAndNonGitCommandsPassThrough(t *testing.T) {
	repo := newPolicyRepo(t)
	t.Chdir(repo.dir)
	one, err := ManagedGitArgs(context.Background(), os.Environ(), []string{"status"})
	if err != nil {
		t.Fatal(err)
	}
	two, _ := ManagedGitArgs(context.Background(), os.Environ(), []string{"status"})
	if strings.Join(one, "\x00") != strings.Join(two, "\x00") {
		t.Fatalf("vector not deterministic: %v %v", one, two)
	}
	want := "-c core.fsmonitor=false -c core.hooksPath=/dev/null -c protocol.ext.allow=never " +
		"-c filter.evil.clean= -c filter.evil.process= -c filter.evil.smudge= -c filter.evil.required=false " +
		"-c merge.evil.driver= status"
	if got := strings.Join(one, " "); got != want {
		t.Fatalf("vector = %q\nwant     %q", got, want)
	}
	cmd := exec.Command("/bin/echo", "x")
	cmd.Args = []string{"git", "status"}
	if err := applyManagedGitPolicy(context.Background(), cmd); err != nil || strings.Join(cmd.Args, " ") != "git status" {
		t.Fatalf("a non-Git executable must pass through: %v %v", cmd.Args, err)
	}
}

func TestManagedGitFilterFreeCommandsDoNotEnumerate(t *testing.T) {
	// A non-repository directory would make enumeration fail; filter-free
	// commands never run it.
	dir := t.TempDir()
	vector, err := buildManagedGitVector(context.Background(), dir, os.Environ(), []string{"rev-parse", "--git-dir"})
	if err != nil || strings.Contains(strings.Join(vector, " "), "filter.") {
		t.Fatalf("unexpected vector %v err %v", vector, err)
	}
}

func TestTokenizeFilterCommand(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		ok   bool
	}{
		{"git-lfs clean -- %f", []string{"git-lfs", "clean", "--", "%f"}, true},
		{"'/opt/my tool/git-crypt' smudge", []string{"/opt/my tool/git-crypt", "smudge"}, true},
		{`"/opt/git-crypt" clean`, []string{"/opt/git-crypt", "clean"}, true},
		{"git-lfs clean; touch x", nil, false},
		{`"$HOME/git-lfs" clean`, nil, false},
		{"git-lfs clean $(id)", nil, false},
		{"git-lfs 'clean", nil, false},
		{"", nil, false},
		{"git-lfs clean | cat", nil, false},
		{"git-lfs smudge *", nil, false},
	}
	for _, c := range cases {
		got, ok := tokenizeFilterCommand(c.in)
		if ok != c.ok || strings.Join(got, "\x00") != strings.Join(c.want, "\x00") {
			t.Errorf("tokenize(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestClassifyGitSubcommand(t *testing.T) {
	cases := map[string]gitCommandClass{
		"rev-parse": classFilterFree, "status": classFilterReading, "commit": classContentWriting,
		"made-up": classContentWriting, "push": classFilterReading, "ls-files": classFilterReading,
	}
	for sub, want := range cases {
		if got := classifyGitSubcommand(sub, nil); got != want {
			t.Errorf("%s: class %v want %v", sub, got, want)
		}
	}
	worktree := map[string]gitCommandClass{"list": classFilterFree, "prune": classFilterFree, "remove": classFilterReading, "add": classContentWriting, "move": classContentWriting, "lock": classContentWriting}
	for second, want := range worktree {
		if got := classifyGitSubcommand("worktree", []string{second}); got != want {
			t.Errorf("worktree %s: class %v want %v", second, got, want)
		}
	}
	if classifyGitSubcommand("worktree", nil) != classContentWriting {
		t.Error("bare worktree must be content-writing")
	}
	if classifyGitSubcommand("cat-file", []string{"--filters", "x"}) != classContentWriting {
		t.Error("cat-file --filters must be content-writing")
	}
	if classifyGitSubcommand("branch", []string{"--list"}) != classFilterFree {
		t.Error("branch --list must be filter-free")
	}
	for _, rest := range [][]string{nil, {"--show-current"}, {"-m", "x"}, {"yy"}, {"--list", "--no-list", "zz", "HEAD"}, {"--list", "-d", "x"}} {
		if classifyGitSubcommand("branch", rest) != classContentWriting {
			t.Errorf("branch %v must be content-writing", rest)
		}
	}
	for _, sub := range []string{"update-ref", "symbolic-ref", "fetch", "remote", "write-tree"} {
		if classifyGitSubcommand(sub, nil) != classFilterReading {
			t.Errorf("%s must be filter-reading", sub)
		}
	}
}

func TestIsDryRunPush(t *testing.T) {
	for args, want := range map[string]bool{"--dry-run": true, "-n": true, "-nv": true, "-fn": true, "--force": false, "-u": false, "-o": false} {
		if got := isDryRunPush([]string{args}); got != want {
			t.Errorf("%s: %v want %v", args, got, want)
		}
	}
}

func TestManagedGitEnumeratesOutsideARepository(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"credential", "reject"}} {
		if _, err := buildManagedGitVector(context.Background(), dir, os.Environ(), args); err != nil {
			t.Errorf("%v outside a repository: %v", args, err)
		}
	}
}
