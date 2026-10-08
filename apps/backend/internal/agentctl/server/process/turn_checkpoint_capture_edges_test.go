package process

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointSupportsUnbornRepository(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--initial-branch=main")
	writeFile(t, repoDir, "first.txt", "first content\n")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	checkpoint, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture unborn repository: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show", checkpoint.CommitOID+":first.txt")); got != "first content" {
		t.Fatalf("unborn checkpoint content = %q", got)
	}
	if output := runGit(t, repoDir, "symbolic-ref", "--short", "HEAD"); strings.TrimSpace(output) != "main" {
		t.Fatalf("capture changed unborn branch: %q", output)
	}
}

func TestTurnCheckpointSupportsSHA256ObjectFormat(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--object-format=sha256", "--initial-branch=main")
	runGit(t, repoDir, "config", "user.email", "test@test.com")
	runGit(t, repoDir, "config", "user.name", "Test User")
	writeFile(t, repoDir, "README.md", "sha256 repository\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "sha256 fixture")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	checkpoint, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture sha256 repository: %v", err)
	}
	if checkpoint.HashAlgorithm != "sha256" || len(checkpoint.CommitOID) != 64 || len(checkpoint.TreeOID) != 64 {
		t.Fatalf("sha256 checkpoint = %+v", checkpoint)
	}
}

func TestTurnCheckpointRejectsUntrustedIdentity(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	_, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: "../../main", CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	var checkpointErr *TurnCheckpointError
	if !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonUnsafeGitState {
		t.Fatalf("invalid identity error = %v, want unsafe-state error", err)
	}
}
