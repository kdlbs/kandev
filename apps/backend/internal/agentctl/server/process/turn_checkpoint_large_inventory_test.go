package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointCapturesLargeInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("full 30001-file checkout exceeds capture deadlines on Windows; large scans covered separately")
	}
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

func TestTurnCheckpointScansLargeInventory(t *testing.T) {
	root := t.TempDir()
	var flags, staged bytes.Buffer
	for i := 0; i < 30_001; i++ {
		fmt.Fprintf(&flags, "H tracked-%05d.txt\x00", i)
		fmt.Fprintf(&staged, "H 100644 0000000000000000000000000000000000000000 0\ttracked-%05d.txt\x00", i)
	}
	flags.WriteString("h final.txt\x00")
	staged.WriteString("S 100644 0000000000000000000000000000000000000000 0\tfinal.txt\x00")
	assume, skip, err := turnCheckpointFlaggedPaths(context.Background(), flags.Bytes(), root)
	if err != nil || string(assume) != "final.txt\x00" || len(skip) != 0 {
		t.Fatalf("flag scan: assume=%q skip=%q err=%v", assume, skip, err)
	}
	preserved, err := preserveMissingTurnCheckpointEntries(context.Background(), staged.Bytes(), root)
	want := "100644 0000000000000000000000000000000000000000 0\tfinal.txt\x00"
	if err != nil || string(preserved) != want {
		t.Fatalf("staged scan: preserved=%q err=%v", preserved, err)
	}
}

type checkpointScanContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *checkpointScanContext) Err() error {
	c.checks++
	if c.checks == 2 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestTurnCheckpointInventoryScansHonorCancellation(t *testing.T) {
	root := t.TempDir()
	for _, scan := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"flags", func(ctx context.Context) error {
			_, _, err := turnCheckpointFlaggedPaths(ctx, []byte("H tracked\x00H tracked\x00"), root)
			return err
		}},
		{"staged entries", func(ctx context.Context) error {
			entry := []byte("H 100644 0000000000000000000000000000000000000000 0\ttracked\x00")
			_, err := preserveMissingTurnCheckpointEntries(ctx, append(bytes.Clone(entry), entry...), root)
			return err
		}},
	} {
		t.Run(scan.name, func(t *testing.T) {
			if err := scan.run(context.Background()); err != nil {
				t.Fatalf("uncancelled scan: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			checked := &checkpointScanContext{Context: ctx, cancel: cancel}
			if err := scan.run(checked); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled scan returned %v, want context.Canceled", err)
			}
		})
	}
}
