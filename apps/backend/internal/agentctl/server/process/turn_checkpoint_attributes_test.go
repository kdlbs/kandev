package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/stretchr/testify/require"
)

func TestTurnCheckpointUsesCapturedAttributesAfterWorkspaceChanges(t *testing.T) {
	for _, attribute := range []string{"*.txt diff\n", "*.txt -diff\n"} {
		t.Run(attribute, func(t *testing.T) {
			dir, cleanup := setupTestRepo(t)
			t.Cleanup(cleanup)
			writeFile(t, dir, ".gitattributes", attribute)
			writeFile(t, dir, "file.txt", "old\n")
			runGit(t, dir, "add", ".")
			runGit(t, dir, "commit", "-m", "attribute baseline")
			operator := NewGitOperator(dir, newTestLogger(t), nil)
			setID, checkoutID := uuid.NewString(), uuid.NewString()
			request := turnchanges.CheckpointRequest{ChangeSetID: setID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart}
			_, err := operator.CaptureTurnCheckpoint(context.Background(), request)
			require.NoError(t, err)
			writeFile(t, dir, "file.txt", "new\n")
			request.Boundary = turnchanges.CheckpointEnd
			_, err = operator.CaptureTurnCheckpoint(context.Background(), request)
			require.NoError(t, err)
			pair := acceptedTurnCheckpointPair(t, operator, setID, checkoutID, "")
			before, err := operator.CompareTurnCheckpoints(context.Background(), pair)
			require.NoError(t, err)
			exported, err := operator.ExportTurnCheckpoint(context.Background(), exportRequestForPair(pair))
			require.NoError(t, err)
			changedAttribute := "*.txt -diff\n"
			if attribute == changedAttribute {
				changedAttribute = "*.txt diff\n"
			}
			writeFile(t, dir, ".gitattributes", changedAttribute)
			// Mutable local and user attribute overrides cannot reinterpret old endpoints.
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "info", "attributes"), []byte(changedAttribute), 0o600))
			userAttributes := filepath.Join(t.TempDir(), "attributes")
			require.NoError(t, os.WriteFile(userAttributes, []byte(changedAttribute), 0o600))
			runGit(t, dir, "config", "core.attributesFile", userAttributes)
			after, err := operator.CompareTurnCheckpoints(context.Background(), pair)
			require.NoError(t, err)
			replayed, err := operator.ExportTurnCheckpoint(context.Background(), exportRequestForPair(pair))
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, exported, replayed)
			require.Len(t, after.Files, 1)
			require.Equal(t, attribute == "*.txt -diff\n", after.Files[0].Binary)
		})
	}
}
