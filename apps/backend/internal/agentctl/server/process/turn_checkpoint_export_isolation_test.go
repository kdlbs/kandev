package process

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/stretchr/testify/require"
)

func TestTurnCheckpointCopyPatchExcludesChangedSource(t *testing.T) {
	copiedPaths := []string{"copy.txt", "copy with spaces.txt"}
	if runtime.GOOS != "windows" {
		copiedPaths = append(copiedPaths, "copy with \t quote\".txt")
	}
	for _, copiedPath := range copiedPaths {
		t.Run(copiedPath, func(t *testing.T) {
			dir, cleanup := setupTestRepo(t)
			t.Cleanup(cleanup)
			original := strings.Repeat("original source line\n", 100)
			writeFile(t, dir, "source.txt", original)
			runGit(t, dir, "add", ".")
			runGit(t, dir, "commit", "-m", "base")
			operator := NewGitOperator(dir, newTestLogger(t), nil)
			setID, checkoutID := uuid.NewString(), uuid.NewString()
			request := turnchanges.CheckpointRequest{ChangeSetID: setID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart}
			_, err := operator.CaptureTurnCheckpoint(context.Background(), request)
			require.NoError(t, err)
			writeFile(t, dir, copiedPath, original)
			writeFile(t, dir, "source.txt", "source changed\n")
			request.Boundary = turnchanges.CheckpointEnd
			_, err = operator.CaptureTurnCheckpoint(context.Background(), request)
			require.NoError(t, err)
			exported, err := operator.ExportTurnCheckpoint(context.Background(), exportRequestForPair(acceptedTurnCheckpointPair(t, operator, setID, checkoutID, "")))
			require.NoError(t, err)
			require.True(t, exported.Complete)
			require.Len(t, exported.Files, 2)
			for _, file := range exported.Files {
				if string(file.File.PathBytes) != copiedPath {
					continue
				}
				require.Equal(t, "copied", file.File.Kind)
				require.EqualValues(t, 0, *file.File.Added)
				for _, patch := range [][]byte{file.CanonicalPatch, file.FilteredPatch} {
					require.Equal(t, 1, strings.Count(string(patch), "diff --git "), "per-file patch includes source modification")
					require.NotContains(t, string(patch), "+source changed")
				}
				require.Equal(t, original, string(file.OldRendering))
				require.Equal(t, original, string(file.NewRendering))
				return
			}
			t.Fatal("copied entry not found")
		})
	}
}
