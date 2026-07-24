package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeBranchOID = "1111111111111111111111111111111111111111"

func TestGitAddWorktree_FailureRollsBackNewBranchAndPartialWorktree(t *testing.T) {
	branchState := filepath.Join(t.TempDir(), "branch-created")
	registrationState := filepath.Join(t.TempDir(), "worktree-registered")
	scriptDir := writeFakeGitScript(t, failedWorktreeAddGitScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KD_BRANCH_STATE", branchState)
	t.Setenv("KD_REGISTRATION_STATE", registrationState)

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	repoPath := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "partial-worktree")

	_, err = mgr.gitAddWorktree(context.Background(), repoPath, "feature/new", worktreePath, "main")
	if err == nil {
		t.Fatal("gitAddWorktree() error = nil, want checkout failure")
	}
	if _, statErr := os.Stat(branchState); !os.IsNotExist(statErr) {
		t.Fatalf("new branch state remains after rollback: %v", statErr)
	}
	if _, statErr := os.Stat(registrationState); !os.IsNotExist(statErr) {
		t.Fatalf("worktree registration remains after rollback: %v", statErr)
	}
	if _, statErr := os.Stat(worktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("partial worktree remains after rollback: %v", statErr)
	}
}

func TestGitAddWorktree_FailureUsesAtomicBranchOwnership(t *testing.T) {
	branchState := filepath.Join(t.TempDir(), "branch-created")
	registrationState := filepath.Join(t.TempDir(), "worktree-registered")
	gitLog := filepath.Join(t.TempDir(), "git.log")
	scriptDir := writeFakeGitScript(t, failedWorktreeAddGitScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KD_BRANCH_STATE", branchState)
	t.Setenv("KD_REGISTRATION_STATE", registrationState)
	t.Setenv("KD_GIT_LOG", gitLog)

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	_, err = mgr.gitAddWorktree(
		context.Background(), t.TempDir(), "feature/new", filepath.Join(t.TempDir(), "worktree"), "main",
	)
	if err == nil {
		t.Fatal("gitAddWorktree() error = nil, want checkout failure")
	}

	logBytes, readErr := os.ReadFile(gitLog)
	if readErr != nil {
		t.Fatalf("read fake git log: %v", readErr)
	}
	logOutput := string(logBytes)
	zeroOID := strings.Repeat("0", len(fakeBranchOID))
	for _, want := range []string{
		"update-ref refs/heads/feature/new " + fakeBranchOID + " " + zeroOID,
		"update-ref -d refs/heads/feature/new " + fakeBranchOID,
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("git log %q does not contain atomic ref operation %q", logOutput, want)
		}
	}
}

func TestRollbackFailedNewBranchAdd_PreservesCompetingCreatorState(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	branchName := "feature/concurrent-owner"
	ownership, err := mgr.createNewBranchRef(context.Background(), repoPath, branchName, "main")
	if err != nil {
		t.Fatalf("createNewBranchRef() error: %v", err)
	}
	intendedPath := filepath.Join(t.TempDir(), "intended-worktree")
	competitorPath := filepath.Join(t.TempDir(), "competing-worktree")
	runGit(t, repoPath, "worktree", "add", competitorPath, branchName)
	marker := filepath.Join(competitorPath, "competitor.txt")
	if err := os.WriteFile(marker, []byte("competitor-owned"), 0600); err != nil {
		t.Fatalf("write competitor marker: %v", err)
	}

	mgr.rollbackFailedNewBranchAdd(context.Background(), repoPath, branchName, intendedPath, ownership)

	if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "competitor-owned" {
		t.Fatalf("competing creator's worktree was changed: contents=%q err=%v", got, readErr)
	}
	branchOID := strings.TrimSpace(runGit(t, repoPath, "show-ref", "--hash", ownership.branchRef))
	if branchOID != ownership.branchOID {
		t.Fatalf("competing creator's branch OID = %q, want %q", branchOID, ownership.branchOID)
	}
}

func TestRollbackFailedNewBranchAdd_PreservesAdvancedOwnedBranch(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	branchName := "feature/advanced-owner"
	ownership, err := mgr.createNewBranchRef(context.Background(), repoPath, branchName, "main")
	if err != nil {
		t.Fatalf("createNewBranchRef() error: %v", err)
	}
	treeOID := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main^{tree}"))
	advancedOID := strings.TrimSpace(runGit(
		t, repoPath, "commit-tree", treeOID, "-p", ownership.branchOID, "-m", "competitor advance",
	))
	runGit(t, repoPath, "update-ref", ownership.branchRef, advancedOID, ownership.branchOID)

	mgr.rollbackFailedNewBranchAdd(
		context.Background(), repoPath, branchName, filepath.Join(t.TempDir(), "absent-worktree"), ownership,
	)

	gotOID := strings.TrimSpace(runGit(t, repoPath, "show-ref", "--hash", ownership.branchRef))
	if gotOID != advancedOID {
		t.Fatalf("advanced branch OID = %q, want preserved %q", gotOID, advancedOID)
	}
}

func TestGitAddWorktree_FailurePreservesPreexistingBranchAndDirectory(t *testing.T) {
	branchState := filepath.Join(t.TempDir(), "branch-created")
	registrationState := filepath.Join(t.TempDir(), "worktree-registered")
	if err := os.WriteFile(branchState, []byte("preexisting"), 0600); err != nil {
		t.Fatalf("create branch state: %v", err)
	}
	scriptDir := writeFakeGitScript(t, failedWorktreeAddGitScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KD_BRANCH_STATE", branchState)
	t.Setenv("KD_REGISTRATION_STATE", registrationState)

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	repoPath := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "existing-directory")
	if err := os.Mkdir(worktreePath, 0755); err != nil {
		t.Fatalf("create existing worktree directory: %v", err)
	}
	marker := filepath.Join(worktreePath, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatalf("create marker: %v", err)
	}

	_, err = mgr.gitAddWorktree(context.Background(), repoPath, "feature/existing", worktreePath, "main")
	if err == nil {
		t.Fatal("gitAddWorktree() error = nil, want existing branch failure")
	}
	if got, readErr := os.ReadFile(branchState); readErr != nil || string(got) != "preexisting" {
		t.Fatalf("preexisting branch was changed: contents=%q err=%v", got, readErr)
	}
	if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "keep" {
		t.Fatalf("preexisting directory was changed: contents=%q err=%v", got, readErr)
	}
}

func TestGitAddWorktree_PreRegistrationFailureRollsBackOwnedBranch(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	worktreePath := filepath.Join(t.TempDir(), "preexisting-target")
	if err := os.Mkdir(worktreePath, 0755); err != nil {
		t.Fatalf("create preexisting worktree target: %v", err)
	}
	marker := filepath.Join(worktreePath, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatalf("create target marker: %v", err)
	}

	_, err = mgr.gitAddWorktree(context.Background(), repoPath, "feature/pre-registration-failure", worktreePath, "main")
	if err == nil {
		t.Fatal("gitAddWorktree() error = nil, want pre-registration failure")
	}
	exists, existsErr := mgr.branchExists(context.Background(), repoPath, "feature/pre-registration-failure")
	if existsErr != nil {
		t.Fatalf("check owned branch after pre-registration failure: %v", existsErr)
	}
	if exists {
		t.Fatal("owned branch remains after pre-registration failure")
	}
	if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "keep" {
		t.Fatalf("preexisting target was changed: contents=%q err=%v", got, readErr)
	}
}

func TestGitAddWorktree_RegisteredCleanupFailureRestoresOwnedBranch(t *testing.T) {
	branchState := filepath.Join(t.TempDir(), "branch-created")
	registrationState := filepath.Join(t.TempDir(), "worktree-registered")
	scriptDir := writeFakeGitScript(t, failedWorktreeAddGitScript)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KD_BRANCH_STATE", branchState)
	t.Setenv("KD_REGISTRATION_STATE", registrationState)
	t.Setenv("KD_WORKTREE_REMOVE_FAIL", "1")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	worktreePath := filepath.Join(t.TempDir(), "registered-worktree")
	_, err = mgr.gitAddWorktree(context.Background(), t.TempDir(), "feature/new", worktreePath, "main")
	if err == nil {
		t.Fatal("gitAddWorktree() error = nil, want checkout failure")
	}
	if _, statErr := os.Stat(registrationState); statErr != nil {
		t.Fatalf("registered worktree unexpectedly removed: %v", statErr)
	}
	if _, statErr := os.Stat(branchState); statErr != nil {
		t.Fatalf("registered worktree points at a deleted owned branch: %v", statErr)
	}
}

const failedWorktreeAddGitScript = `
while [ "${1:-}" = "-c" ]; do
  shift 2
done

if [ -n "${KD_GIT_LOG:-}" ]; then
  printf "%s\n" "$*" >> "${KD_GIT_LOG}"
fi

case "${1:-} ${2:-}" in
	"rev-parse --verify")
		echo "1111111111111111111111111111111111111111"
		exit 0
		;;
	"update-ref refs/heads/feature/new"|"update-ref refs/heads/feature/existing")
		if [ -f "${KD_BRANCH_STATE:?}" ]; then
			echo "fatal: cannot lock ref '${2:-}': reference already exists" >&2
			exit 128
		fi
		printf "%s\n%s" "${3:?}" "${2:?}" > "${KD_BRANCH_STATE}"
		exit 0
		;;
	"update-ref -d")
		head_oid="$(sed -n '1p' "${KD_BRANCH_STATE:?}")"
		branch_ref="$(sed -n '2p' "${KD_BRANCH_STATE:?}")"
		if [ "${3:-}" = "${branch_ref}" ] && [ "${4:-}" = "${head_oid}" ]; then
			rm -f "${KD_BRANCH_STATE}"
			exit 0
		fi
		exit 1
		;;
  "show-ref --verify")
    if [ -f "${KD_BRANCH_STATE:?}" ]; then exit 0; fi
    exit 1
    ;;
  "worktree list")
    if [ -f "${KD_REGISTRATION_STATE:?}" ]; then
			head_oid="$(sed -n '1p' "${KD_BRANCH_STATE:?}")"
			branch_ref="$(sed -n '2p' "${KD_BRANCH_STATE:?}")"
			printf "worktree %s\\0HEAD %s\\0branch %s\\0\\0" \
				"$(cat "${KD_REGISTRATION_STATE}")" "${head_oid}" "${branch_ref}"
    fi
    exit 0
    ;;
  "worktree add")
		if [ "${3:-}" = "--no-checkout" ]; then
			worktree_path="${4:?}"
		else
			worktree_path="${3:?}"
		fi
		printf "%s" "${worktree_path}" > "${KD_REGISTRATION_STATE}"
		mkdir -p "${worktree_path}"
		printf "partial" > "${worktree_path}/partial.txt"
    echo "error: unable to create file generated/very/long/project.csproj: Filename too long" >&2
    echo "fatal: Could not reset index file to revision 'HEAD'." >&2
    exit 128
    ;;
  "worktree remove")
		if [ -n "${KD_WORKTREE_REMOVE_FAIL:-}" ]; then
			echo "forced worktree remove failure" >&2
			exit 1
		fi
    rm -rf "${4:?}"
    rm -f "${KD_REGISTRATION_STATE:?}"
    exit 0
    ;;
  *)
    echo "unexpected fake git command: $*" >&2
    exit 2
    ;;
esac
`
