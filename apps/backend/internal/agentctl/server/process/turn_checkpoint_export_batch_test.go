package process

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointExportBatchResumesAtCursor(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	writeFile(t, repoDir, "a.txt", "a\n")
	writeFile(t, repoDir, "b.txt", "b\n")
	writeFile(t, repoDir, "c.txt", "c\n")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
	request := exportRequestForPair(acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	request.ExportBatchVersion = turnchanges.ExportBatchVersion1
	request.MaxEntries = 2

	first, err := operator.ExportTurnCheckpoint(context.Background(), request)
	if err != nil {
		t.Fatalf("first export page: %v", err)
	}
	if len(first.Files) != 2 || first.NextCursor != 2 || first.Complete || first.TotalFiles != 3 {
		t.Fatalf("first page = files:%d cursor:%d complete:%t total:%d", len(first.Files), first.NextCursor, first.Complete, first.TotalFiles)
	}
	if first.InventoryFingerprint == "" {
		t.Fatal("batch export did not advertise an inventory fingerprint")
	}
	request.Cursor = first.NextCursor
	request.InventoryFingerprint = first.InventoryFingerprint
	second, err := operator.ExportTurnCheckpoint(context.Background(), request)
	if err != nil {
		t.Fatalf("second export page: %v", err)
	}
	if len(second.Files) != 1 || second.NextCursor != 3 || !second.Complete {
		t.Fatalf("second page = files:%d cursor:%d complete:%t", len(second.Files), second.NextCursor, second.Complete)
	}
	if !strings.HasPrefix(second.InventoryFingerprint, first.InventoryFingerprint[:8]) {
		t.Fatalf("fingerprint changed between pages: %q vs %q", first.InventoryFingerprint, second.InventoryFingerprint)
	}
}

func TestTurnCheckpointExportBatchRejectsChangedInventory(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	writeFile(t, repoDir, "a.txt", "a\n")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
	request := exportRequestForPair(acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	request.ExportBatchVersion = turnchanges.ExportBatchVersion1
	request.InventoryFingerprint = strings.Repeat("0", 64)
	_, err := operator.ExportTurnCheckpoint(context.Background(), request)
	var checkpointErr *TurnCheckpointError
	if err == nil || !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonHistoryContextChanged {
		t.Fatalf("changed inventory error = %v", err)
	}
}

func TestTurnCheckpointExportPageContinuesPastUnavailableFile(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	writeFile(t, repoDir, "00-large.txt", strings.Repeat("x", turnCheckpointPatchLimit+1024))
	for index := 1; index < 30; index++ {
		writeFile(t, repoDir, fmt.Sprintf("%02d.txt", index), "small\n")
	}
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
	request := exportRequestForPair(acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	request.ExportBatchVersion = turnchanges.ExportBatchVersion1
	request.MaxEntries = 25

	first, err := operator.ExportTurnCheckpoint(context.Background(), request)
	if err != nil {
		t.Fatalf("first export page: %v", err)
	}
	if first.Complete || first.NextCursor != 25 || len(first.Files) != 25 {
		t.Fatalf("first page = complete:%t cursor:%d files:%d", first.Complete, first.NextCursor, len(first.Files))
	}
	if first.Reason != "" {
		t.Fatalf("file-specific failure leaked onto the page: %q", first.Reason)
	}
	if first.Files[0].ContentAvailability != turnchanges.Unavailable ||
		first.Files[0].Reason != turnchanges.ReasonSizeLimit {
		t.Fatalf("large file = %+v", first.Files[0])
	}
	for _, file := range first.Files[1:] {
		if file.ContentAvailability != turnchanges.Ready {
			t.Fatalf("small file after the large one = %+v", file)
		}
	}

	request.Cursor = first.NextCursor
	request.InventoryFingerprint = first.InventoryFingerprint
	second, err := operator.ExportTurnCheckpoint(context.Background(), request)
	if err != nil {
		t.Fatalf("second export page: %v", err)
	}
	if !second.Complete || len(second.Files) != 5 {
		t.Fatalf("second page = complete:%t files:%d", second.Complete, len(second.Files))
	}
	for _, file := range second.Files {
		if file.ContentAvailability != turnchanges.Ready {
			t.Fatalf("remaining file = %+v", file)
		}
	}
}
