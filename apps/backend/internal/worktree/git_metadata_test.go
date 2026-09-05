package worktree

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveGitMetadata_LinkedWorktreeContainsOnlyOwnedMetadata(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)

	projection, err := ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatalf("ResolveGitMetadata: %v", err)
	}

	if projection.CheckoutPath != checkout {
		t.Fatalf("CheckoutPath = %q, want %q", projection.CheckoutPath, checkout)
	}
	if projection.GitDir == filepath.Join(repo, ".git") {
		t.Fatal("GitDir must be the linked worktree metadata directory, not the source .git")
	}
	if projection.CommonDir != filepath.Join(repo, ".git") {
		t.Fatalf("CommonDir = %q, want %q", projection.CommonDir, filepath.Join(repo, ".git"))
	}
	if projection.CurrentRef != "refs/heads/task-branch" {
		t.Fatalf("CurrentRef = %q", projection.CurrentRef)
	}
	if !containsGitMetadataPath(projection.AgentWritablePaths, projection.GitDir) {
		t.Fatalf("AgentWritablePaths must contain owned GitDir: %#v", projection.AgentWritablePaths)
	}
	if containsGitMetadataPath(projection.AgentWritablePaths, projection.CommonDir) {
		t.Fatalf("AgentWritablePaths must not contain common .git root: %#v", projection.AgentWritablePaths)
	}
	if err := projection.Revalidate(); err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
}

func TestResolveGitMetadataRejectsForgedLinkedWorktreePointer(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)

	gitDir := runGitMetadata(t, checkout, "rev-parse", "--git-dir")
	if err := os.WriteFile(filepath.Join(gitDir, "gitdir"), []byte(filepath.Join(t.TempDir(), ".git")+"\n"), 0o600); err != nil {
		t.Fatalf("forge reciprocal gitdir: %v", err)
	}
	if _, err := ResolveGitMetadata(checkout); err == nil {
		t.Fatal("ResolveGitMetadata accepted forged reciprocal pointer")
	}
}

func TestResolveGitMetadataRejectsSymlinkedGitEntry(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)
	gitEntry := filepath.Join(checkout, ".git")
	if err := os.Remove(gitEntry); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "forged-git"), gitEntry); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := ResolveGitMetadata(checkout); !errors.Is(err, ErrGitMetadataProjectionInvalid) {
		t.Fatalf("ResolveGitMetadata error = %v, want symlink rejection", err)
	}
}

func TestResolveGitMetadataRejectsTraversalCurrentBranchRef(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)
	gitDir := runGitMetadata(t, checkout, "rev-parse", "--path-format=absolute", "--git-dir")
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/../../config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveGitMetadata(checkout); !errors.Is(err, ErrGitMetadataProjectionInvalid) {
		t.Fatalf("ResolveGitMetadata error = %v, want ref traversal rejection", err)
	}
}

func TestValidBranchRefRejectsNullByte(t *testing.T) {
	if ValidBranchRef("refs/heads/main\x00unsafe") {
		t.Fatal("ValidBranchRef accepted a null byte in the branch ref")
	}
}

func TestValidBranchRefRejectsCarriageReturnAndNewline(t *testing.T) {
	for _, ref := range []string{"refs/heads/main\r", "refs/heads/ma\nin"} {
		if ValidBranchRef(ref) {
			t.Fatalf("ValidBranchRef accepted control character ref %q", ref)
		}
	}
}

func TestValidBranchRefRejectsNonCanonicalBranchName(t *testing.T) {
	// securityutil.IsValidBranchName requires the branch name to start with an
	// alphanumeric character; a leading dash could otherwise be misread as a
	// git command-line flag by any future caller that shells out with the ref.
	if ValidBranchRef("refs/heads/-flag-like") {
		t.Fatal("ValidBranchRef accepted a branch name starting with a dash")
	}
}

func TestResolveGitMetadataForRepositoryRejectsDifferentValidCommonDirectory(t *testing.T) {
	repositoryA := initGitMetadataRepository(t)
	repositoryB := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repositoryB, "worktree", "add", "-b", "task-branch", checkout)

	if _, err := ResolveGitMetadataForRepository(checkout, repositoryA); !errors.Is(err, ErrGitMetadataProjectionInvalid) {
		t.Fatalf("ResolveGitMetadataForRepository error = %v, want trusted repository rejection", err)
	}
}

func TestResolveGitMetadataForRepositoryRejectsCheckoutEqualToRepository(t *testing.T) {
	// A task checkout must always be a distinct linked worktree. If a caller
	// (by bug or forged durable state) passes the trusted repository's own
	// path as checkoutPath, the self-comparison used to detect "is this a
	// linked worktree" trivially matches; ResolveGitMetadataForRepository must
	// still reject it rather than granting the source checkout a projection.
	repository := initGitMetadataRepository(t)

	if _, err := ResolveGitMetadataForRepository(repository, repository); !errors.Is(err, ErrGitMetadataProjectionInvalid) {
		t.Fatalf("ResolveGitMetadataForRepository error = %v, want rejection of checkoutPath == repositoryPath", err)
	}
}

func TestGitMetadataProjectionRevalidateRejectsCommonDirectorySwap(t *testing.T) {
	repositoryA := initGitMetadataRepository(t)
	repositoryB := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repositoryA, "worktree", "add", "-b", "task-branch", checkout)
	projection, err := ResolveGitMetadataForRepository(checkout, repositoryA)
	if err != nil {
		t.Fatal(err)
	}

	gitPointer := filepath.Join(checkout, ".git")
	replacement := filepath.Join(t.TempDir(), "replacement")
	runGitMetadata(t, repositoryB, "worktree", "add", "-b", "replacement", replacement)
	replacementGitDir := runGitMetadata(t, replacement, "rev-parse", "--path-format=absolute", "--git-dir")
	if err := os.WriteFile(filepath.Join(replacementGitDir, "gitdir"), []byte(gitPointer+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitPointer, []byte("gitdir: "+replacementGitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(gitPointer); err != nil {
		t.Fatal(err)
	}
	if err := projection.Revalidate(); !errors.Is(err, ErrGitMetadataProjectionInvalid) {
		t.Fatalf("Revalidate error = %v, want common-directory swap rejection", err)
	}
}

func TestGitMetadataProjectionPreservesNativeIndexLockAndCommit(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)
	projection, err := ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatal(err)
	}
	siblingRefPath := filepath.Join(projection.CommonDir, "refs", "heads", "main")
	siblingReflogPath := filepath.Join(projection.CommonDir, "logs", "refs", "heads", "main")
	siblingRefBefore, err := os.ReadFile(siblingRefPath)
	if err != nil {
		t.Fatal(err)
	}
	siblingReflogBefore, err := os.ReadFile(siblingReflogPath)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(projection.GitDir, "index.lock")
	if err := os.WriteFile(lockPath, []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("git", "-C", checkout, "add", "tracked.txt").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "index.lock") {
		t.Fatalf("git add error=%v output=%s, want native index.lock conflict", err, output)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "tracked.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitMetadata(t, checkout, "add", "tracked.txt")
	refLockPath := projection.CurrentRefPath + ".lock"
	if err := os.WriteFile(refLockPath, []byte("held"), 0o600); err != nil {
		t.Fatalf("hold current branch ref lock: %v", err)
	}
	output, err = exec.Command("git", "-C", checkout, "commit", "-m", "blocked by ref lock").CombinedOutput()
	if err == nil || !strings.Contains(string(output), ".lock") {
		t.Fatalf("git commit error=%v output=%s, want native ref lock conflict", err, output)
	}
	if err := os.Remove(refLockPath); err != nil {
		t.Fatal(err)
	}
	runGitMetadata(t, checkout, "commit", "-m", "task change")
	runGitMetadata(t, checkout, "fsck", "--strict")
	for path, before := range map[string][]byte{
		siblingRefPath:    siblingRefBefore,
		siblingReflogPath: siblingReflogBefore,
	} {
		after, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("task commit mutated sibling metadata %q", path)
		}
	}
}

// TestGitMetadataProjectionRepairsReadOnlyExternalIndexLock reproduces the
// task-sandbox symptom: the checkout itself is writable, but Git cannot create
// index.lock in the linked worktree metadata. Reapplying the generated owned
// metadata grant restores ordinary add/commit without changing source or
// sibling metadata.
func TestGitMetadataProjectionRepairsReadOnlyExternalIndexLock(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)
	projection, err := ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "tracked.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	gitDirInfo, err := os.Stat(projection.GitDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(projection.GitDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(projection.GitDir, gitDirInfo.Mode().Perm()) })
	probePath := filepath.Join(projection.GitDir, "permission-probe")
	if err := os.WriteFile(probePath, []byte("probe"), 0o600); err == nil {
		_ = os.Remove(probePath)
		t.Skip("test environment bypasses Git metadata directory permissions")
	}
	output, err := exec.Command("git", "-C", checkout, "add", "tracked.txt").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "index.lock") {
		t.Fatalf("git add error=%v output=%s, want read-only index.lock failure", err, output)
	}

	if err := os.Chmod(projection.GitDir, gitDirInfo.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	runGitMetadata(t, checkout, "add", "tracked.txt")
	runGitMetadata(t, checkout, "commit", "-m", "task change")
	runGitMetadata(t, checkout, "fsck", "--strict")
}

func TestGitMetadataProjectionSeparatesExactRefWritesFromMountSupport(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)

	projection, err := ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatal(err)
	}
	for _, exactPath := range []string{
		projection.CurrentRefPath,
		projection.CurrentRefPath + ".lock",
		projection.ReflogPath,
		projection.ReflogPath + ".lock",
	} {
		if !containsGitMetadataPath(projection.AgentWritablePaths, exactPath) {
			t.Fatalf("AgentWritablePaths = %#v, missing exact Git update path %q", projection.AgentWritablePaths, exactPath)
		}
	}
	for _, parent := range []string{
		filepath.Dir(projection.CurrentRefPath),
		filepath.Dir(projection.ReflogPath),
	} {
		if containsGitMetadataPath(projection.AgentWritablePaths, parent) {
			t.Fatalf("AgentWritablePaths = %#v, grants sibling mutation through parent %q", projection.AgentWritablePaths, parent)
		}
		if !containsGitMetadataPath(projection.MountSupportPaths, parent) {
			t.Fatalf("MountSupportPaths = %#v, missing native lock parent %q", projection.MountSupportPaths, parent)
		}
	}
}

func TestGitMetadataProjectionRejectsSymlinkedObjectsDirectory(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)
	commonDir := runGitMetadata(t, repo, "rev-parse", "--path-format=absolute", "--git-dir")

	realObjects := filepath.Join(commonDir, "objects")
	forgedTarget := t.TempDir()
	if err := os.Rename(realObjects, forgedTarget+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(forgedTarget+"-moved", realObjects); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := ResolveGitMetadata(checkout); !errors.Is(err, ErrGitMetadataProjectionInvalid) {
		t.Fatalf("ResolveGitMetadata error = %v, want symlinked objects directory rejection", err)
	}
}

func TestGitMetadataProjectionUsesExactPathsForPotentialReflogCreation(t *testing.T) {
	repo := initGitMetadataRepository(t)
	runGitMetadata(t, repo, "config", "core.logAllRefUpdates", "false")
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)
	commonDir := runGitMetadata(t, repo, "rev-parse", "--path-format=absolute", "--git-dir")
	reflogPath := filepath.Join(commonDir, "logs", "refs", "heads", "task-branch")
	if _, err := os.Stat(reflogPath); err == nil {
		t.Fatalf("test setup: reflog unexpectedly exists at %q", reflogPath)
	}

	projection, err := ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if !containsGitMetadataPath(projection.AgentWritablePaths, projection.ReflogPath) {
		t.Fatalf("AgentWritablePaths must authorize only the potential reflog file: %#v", projection.AgentWritablePaths)
	}
	if !containsGitMetadataPath(projection.AgentWritablePaths, projection.ReflogPath+".lock") {
		t.Fatalf("AgentWritablePaths must authorize only the potential reflog lock: %#v", projection.AgentWritablePaths)
	}
	if containsGitMetadataPath(projection.AgentWritablePaths, filepath.Dir(projection.ReflogPath)) {
		t.Fatalf("AgentWritablePaths must not authorize sibling reflogs through their parent: %#v", projection.AgentWritablePaths)
	}
}

func TestGitMetadataProjectionSupportsMultipleTaskRepositories(t *testing.T) {
	repoA := initGitMetadataRepository(t)
	repoB := initGitMetadataRepository(t)
	checkoutA := filepath.Join(t.TempDir(), "primary")
	checkoutB := filepath.Join(t.TempDir(), "attached")
	runGitMetadata(t, repoA, "worktree", "add", "-b", "task-a", checkoutA)
	runGitMetadata(t, repoB, "worktree", "add", "-b", "task-b", checkoutB)

	projectionA, err := ResolveGitMetadata(checkoutA)
	if err != nil {
		t.Fatal(err)
	}
	projectionB, err := ResolveGitMetadata(checkoutB)
	if err != nil {
		t.Fatal(err)
	}
	if projectionA.GitDir == projectionB.GitDir || projectionA.CommonDir == projectionB.CommonDir {
		t.Fatalf("multi-repository projections overlap: %#v %#v", projectionA, projectionB)
	}
	for _, checkout := range []string{checkoutA, checkoutB} {
		if err := os.WriteFile(filepath.Join(checkout, "tracked.txt"), []byte("changed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGitMetadata(t, checkout, "add", "tracked.txt")
		runGitMetadata(t, checkout, "commit", "-m", "task change")
		runGitMetadata(t, checkout, "fsck", "--strict")
	}
}

func initGitMetadataRepository(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	runGitMetadata(t, "", "init", "-b", "main", repo)
	runGitMetadata(t, repo, "config", "user.email", "test@example.com")
	runGitMetadata(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("initial\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGitMetadata(t, repo, "add", "tracked.txt")
	runGitMetadata(t, repo, "commit", "-m", "initial")
	return repo
}

func runGitMetadata(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmdArgs := args
	if directory != "" {
		cmdArgs = append([]string{"-C", directory}, args...)
	}
	output, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", cmdArgs, err, output)
	}
	return strings.TrimSpace(string(output))
}

func containsGitMetadataPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
