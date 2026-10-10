package process

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointSupportsSplitIndex(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repoDir, "update-index", "--split-index")
	index := readFile(t, filepath.Join(repoDir, ".git", "index"))
	shared := filepath.Join(repoDir, ".git", "sharedindex.")
	before, err := filepath.Glob(shared + "*")
	if err != nil {
		t.Fatal(err)
	}
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	checkpoint, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture split index: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show", checkpoint.CommitOID+":README.md")); got != "# Test Repo" {
		t.Fatalf("split-index checkpoint content = %q", got)
	}
	if got := readFile(t, filepath.Join(repoDir, ".git", "index")); !bytes.Equal(got, index) {
		t.Fatal("checkpoint changed split user index")
	}
	after, err := filepath.Glob(shared + "*")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("checkpoint created or removed shared indexes: before=%v after=%v", before, after)
	}
}

func TestTurnCheckpointPreservesLiteralPathBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("literal backslash filenames are not supported by Windows filesystems")
	}
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart}); err != nil {
		t.Fatal(err)
	}
	path := "tab\tline\nback\\slash.txt"
	writeFile(t, repoDir, path, "literal path\n")
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd}); err != nil {
		t.Fatal(err)
	}
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	if err != nil {
		t.Fatal(err)
	}
	if comparison.FileCount != 1 || string(comparison.Files[0].PathBytes) != path {
		t.Fatalf("literal path result = %+v, want exact path bytes", comparison.Files)
	}
}

func checkpointUserRefs(t *testing.T, repoDir string) string {
	t.Helper()
	output := runGit(t, repoDir, "for-each-ref", "--format=%(refname) %(objectname)")
	var refs []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line != "" && !strings.Contains(line, "refs/kandev/turn-changes/") {
			refs = append(refs, line)
		}
	}
	return strings.Join(refs, "\n")
}

func runGitExpectedFailure(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	command := exec.Command("git", fullArgs...)
	command.Env = filterTestGitEnv(os.Environ())
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("git %v unexpectedly succeeded", args)
	}
	return string(output)
}
