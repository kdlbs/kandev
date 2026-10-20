package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointCapturesLargeInventory(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	for i := 0; i < 30_001; i++ {
		path := filepath.Join(repoDir, fmt.Sprintf("tracked-%05d.txt", i))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, repoDir, "add", "--all")
	runGit(t, repoDir, "commit", "-m", "large tracked baseline")
	index := readFile(t, filepath.Join(repoDir, ".git", "index"))
	head := runGit(t, repoDir, "rev-parse", "HEAD")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprintf("changed=%t", changed), func(t *testing.T) {
			request := turnchanges.CheckpointRequest{
				ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
			}
			if _, err := operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
				t.Fatalf("start: %v", err)
			}
			var want int64
			if changed {
				writeFile(t, repoDir, "tracked-00000.txt", "turn change\n")
				want = 1
			}
			request.Boundary = turnchanges.CheckpointEnd
			if _, err := operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
				t.Fatalf("end: %v", err)
			}
			pair := acceptedTurnCheckpointPair(t, operator, request.ChangeSetID, request.CheckoutID, "")
			comparison, err := operator.CompareTurnCheckpoints(context.Background(), pair)
			if err != nil {
				t.Fatal(err)
			}
			if !comparison.Complete || comparison.FileCount != want {
				t.Fatalf("comparison = %+v; want %d files", comparison, want)
			}
			if changed && string(comparison.Files[0].PathBytes) != "tracked-00000.txt" {
				t.Fatalf("wrong file: %+v", comparison.Files[0])
			}
			exported, err := operator.ExportTurnCheckpoint(context.Background(), exportRequestForPair(pair))
			if err != nil || !exported.Complete {
				t.Fatalf("export = %+v, error = %v", exported, err)
			}
			if !bytes.Equal(index, readFile(t, filepath.Join(repoDir, ".git", "index"))) {
				t.Fatal("capture changed the user's index")
			}
			if runGit(t, repoDir, "rev-parse", "HEAD") != head {
				t.Fatal("capture changed HEAD")
			}
		})
	}
}
