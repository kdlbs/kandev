package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceWithinRoots(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	inside := filepath.Join(root, "nested")
	sibling := filepath.Join(base, "repo-evil")
	outside := filepath.Join(base, "other")
	for _, dir := range []string{inside, sibling, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	roots := []string{root}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "root itself", path: root, want: true},
		{name: "directory inside the root", path: inside, want: true},
		{name: "unrelated directory", path: outside, want: false},
		{name: "sibling sharing the root prefix", path: sibling, want: false},
		{name: "traversal out of the root", path: root + string(filepath.Separator) + ".." + string(filepath.Separator) + "other", want: false},
		{name: "relative path", path: "repo", want: false},
		{name: "missing path", path: filepath.Join(root, "missing"), want: false},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, struct {
			name string
			path string
			want bool
		}{name: "different case on Windows", path: strings.ToUpper(root), want: true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SourceWithinRoots(tc.path, roots); got != tc.want {
				t.Fatalf("SourceWithinRoots(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
	if SourceWithinRoots(root, nil) {
		t.Fatal("no roots must admit nothing")
	}
}

func TestSourceWithinRootsResolvesSymlinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	outside := filepath.Join(base, "other")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	escape := filepath.Join(root, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if SourceWithinRoots(escape, []string{root}) {
		t.Fatal("a link inside the root that points outside must be rejected")
	}

	linkedRoot := filepath.Join(base, "linked-repo")
	if err := os.Symlink(root, linkedRoot); err != nil {
		t.Fatal(err)
	}
	if !SourceWithinRoots(root, []string{linkedRoot}) {
		t.Fatal("a root named through a link must admit its target")
	}
	if !SourceWithinRoots(linkedRoot, []string{root}) {
		t.Fatal("a path naming the root through a link must be admitted")
	}
}

func gitForSourceTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func initSourceTestRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitForSourceTest(t, dir, "init")
	gitForSourceTest(t, dir, "config", "user.email", "review@example.test")
	gitForSourceTest(t, dir, "config", "user.name", "Review Test")
	gitForSourceTest(t, dir, "commit", "--allow-empty", "-m", "base")
	return dir
}

func TestSourceAdmittedAcceptsOtherWorktreesOfARegisteredRepository(t *testing.T) {
	base := t.TempDir()
	registered := initSourceTestRepo(t, filepath.Join(base, "registered"))
	unrelated := initSourceTestRepo(t, filepath.Join(base, "unrelated"))
	slot := filepath.Join(base, "slots", "gate-1")
	gitForSourceTest(t, registered, "worktree", "add", "-b", "gate-1", slot)
	foreignSlot := filepath.Join(base, "slots", "foreign")
	gitForSourceTest(t, unrelated, "worktree", "add", "-b", "foreign", foreignSlot)
	disguisedSlot := filepath.Join(registered, "foreign-gitfile")
	gitForSourceTest(t, unrelated, "worktree", "add", "-b", "disguised", disguisedSlot)
	plain := filepath.Join(base, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	repositories := []string{registered}
	ctx := context.Background()

	if !SourceAdmitted(ctx, slot, nil, repositories) {
		t.Fatal("another worktree of a registered repository must be admitted")
	}
	if !SourceAdmitted(ctx, filepath.Join(registered, "."), nil, repositories) {
		t.Fatal("the registered checkout itself must be admitted")
	}
	for name, path := range map[string]string{"unrelated repository": unrelated, "unrelated worktree": foreignSlot, "not a repository": plain} {
		if SourceAdmitted(ctx, path, nil, repositories) {
			t.Fatalf("%s must be rejected", name)
		}
	}
	if SourceAdmitted(ctx, slot, nil, nil) {
		t.Fatal("without a registered repository a worktree must be rejected")
	}
	if SourceAdmitted(ctx, disguisedSlot, []string{registered}, repositories) {
		t.Fatal("an in-root gitfile must not admit an unrelated common directory")
	}
	redirected := initSourceTestRepo(t, filepath.Join(registered, "redirected"))
	gitForSourceTest(t, redirected, "config", "core.worktree", unrelated)
	if SourceAdmitted(ctx, redirected, []string{registered}, repositories) {
		t.Fatal("a repository must not redirect worktree collection outside its admitted checkout")
	}
	alternate := initSourceTestRepo(t, filepath.Join(registered, "with-alternates"))
	if err := os.WriteFile(filepath.Join(alternate, ".git", "objects", "info", "alternates"),
		[]byte(filepath.Join(unrelated, ".git", "objects")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if SourceAdmitted(ctx, alternate, []string{registered}, repositories) {
		t.Fatal("an admitted directory must not borrow an unrelated Git object store")
	}

	// The probe describes the candidate itself; an inherited repository
	// binding must not make a non-repository look like the registered one.
	t.Setenv("GIT_DIR", filepath.Join(registered, ".git"))
	t.Setenv("GIT_WORK_TREE", registered)
	if SourceAdmitted(ctx, plain, nil, repositories) {
		t.Fatal("GIT_DIR must not steer the common-dir probe")
	}
}
