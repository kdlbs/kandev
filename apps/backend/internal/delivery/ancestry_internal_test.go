package delivery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runInternalGit is a package-internal twin of ancestry_test.go's runGit
// (package delivery_test), needed here because this file exercises
// refExists directly, which is unexported.
func runInternalGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newInternalAncestryRepo(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	runInternalGit(t, work, "init", "-b", "main")
	runInternalGit(t, work, "config", "user.email", "test@example.com")
	runInternalGit(t, work, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runInternalGit(t, work, "add", "README.md")
	runInternalGit(t, work, "commit", "-m", "initial")
	return work
}

// TestRefExists_GenuineAbsenceIsNotAnError covers the negative case
// refExists must still handle exactly as before the fix: a ref that
// simply does not resolve (git rev-parse --verify --quiet exits 1) is a
// routine "does not exist" result, not a failure to check.
func TestRefExists_GenuineAbsenceIsNotAnError(t *testing.T) {
	work := newInternalAncestryRepo(t)

	exists, err := refExists(context.Background(), work, "does-not-exist")
	if err != nil {
		t.Fatalf("err = %v, want nil (a genuinely absent ref is not an error)", err)
	}
	if exists {
		t.Fatal("exists = true, want false")
	}
}

// TestRefExists_GenuinePresenceIsNotAnError is the positive-case
// companion, confirming the fix does not regress the ordinary success
// path.
func TestRefExists_GenuinePresenceIsNotAnError(t *testing.T) {
	work := newInternalAncestryRepo(t)

	exists, err := refExists(context.Background(), work, "main")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !exists {
		t.Fatal("exists = false, want true (main was just committed to)")
	}
}

// TestRefExists_FailureToCheckIsAnErrorNotAbsence is Review round 1,
// finding #4: refExists used to collapse "the ref genuinely does not
// exist" (git rev-parse --verify --quiet exits 1, a routine negative) and
// "the check itself could not be performed" (any other failure — here, a
// checkout path that is not a git repository at all, which exits 128)
// into the same bare `false`. resolveDefaultBranchRef then falls back
// from the remote-tracking ref to the local branch on either condition,
// so a transient failure checking the remote ref could make the check run
// against a stale local branch instead of erroring, per spec "Failure
// modes": "the git call fails or times out" must be an error, never a
// negative result. refExists must now distinguish the two: a failure to
// check is returned as a non-nil error, not folded into `false`.
func TestRefExists_FailureToCheckIsAnErrorNotAbsence(t *testing.T) {
	notARepo := t.TempDir() // deliberately never `git init`-ed

	exists, err := refExists(context.Background(), notARepo, "main")
	if err == nil {
		t.Fatal("err = nil, want a non-nil error (the checkout path is not a git repository at all)")
	}
	if exists {
		t.Fatal("exists = true, want false alongside the error")
	}
}

// TestLooksLikeCommitSHA covers Review round 1, finding #7: the commit
// value submitted to the ancestry check comes from
// task_session_git_snapshots.head_commit, which is not format-validated
// anywhere upstream, and apps/backend/internal/common/subproc/git_command.go
// documents that callers must validate a value before it reaches a git
// subprocess as raw argv — a value beginning with "-" could otherwise be
// parsed as a git option rather than a revision. This is the guard that
// makes Check reject such a value before it ever reaches git.
func TestLooksLikeCommitSHA(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"full sha-1", "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", true},
		{"short sha", "a1b2c3d", true},
		{"uppercase hex", "A1B2C3D4", true},
		{"leading dash", "-1", false},
		{"long option", "--all", false},
		{"double dash alone", "--", false},
		{"empty", "", false},
		{"non-hex letters", "not-a-sha", false},
		{"too short", "a1b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeCommitSHA(tc.in); got != tc.want {
				t.Fatalf("looksLikeCommitSHA(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
