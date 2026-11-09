package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointArchiveExportsAcceptedClosure(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	writeFile(t, repoDir, "app.txt", "before\n")
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	}); err != nil {
		t.Fatalf("capture start: %v", err)
	}
	writeFile(t, repoDir, "app.txt", "after\n")
	writeFile(t, repoDir, "added.txt", "added\n")
	end, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd,
	})
	if err != nil {
		t.Fatalf("capture end: %v", err)
	}
	pair := acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, "")
	archive, err := operator.ArchiveTurnCheckpoint(context.Background(), archiveRequestForPair(pair))
	if err != nil {
		t.Fatalf("archive accepted interval: %v", err)
	}
	if !archive.Complete || len(archive.Pack) == 0 || archive.ObjectCount == 0 ||
		archive.StartCommitOID != pair.StartCommitOID || archive.EndCommitOID != pair.EndCommitOID ||
		archive.StartTreeOID != pair.StartTreeOID || archive.EndTreeOID != pair.EndTreeOID {
		t.Fatalf("archive = %+v", archive)
	}
	found := map[string]bool{}
	for _, oid := range archive.Objects {
		found[oid] = true
	}
	for _, required := range []string{
		archive.StartCommitOID, archive.EndCommitOID, archive.StartTreeOID, archive.EndTreeOID,
		strings.TrimSpace(end.TreeOID),
	} {
		if !found[required] {
			t.Errorf("archive omitted required object %s", required)
		}
	}
	if archive.UncompressedBytes <= 0 {
		t.Errorf("archive uncompressed byte count = %d", archive.UncompressedBytes)
	}
	if oid, matched := turnCheckpointTreeRecordOID("100644 blob " + end.TreeOID + "\tapp.txt"); !matched || oid != end.TreeOID {
		t.Fatal("blob inventory record was not recognized")
	}
}

func TestTurnCheckpointArchiveRejectsMismatchedEndpointPair(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture start: %v", err)
	}
	writeFile(t, repoDir, "changed.txt", "changed\n")
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd,
	}); err != nil {
		t.Fatalf("capture end: %v", err)
	}
	pair := acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, "")
	pair.StartTreeOID = strings.Repeat("0", len(start.TreeOID))
	_, err = operator.ArchiveTurnCheckpoint(context.Background(), archiveRequestForPair(pair))
	var checkpointErr *TurnCheckpointError
	if !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonContentUnavailable && checkpointErr.Reason != turnchanges.ReasonUnsafeGitState {
		t.Fatalf("mismatched endpoint archive error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoDir, "does-not-exist")); statErr == nil {
		t.Fatal("unexpected fixture state")
	}
}

func TestTurnCheckpointArchiveTransfersOnlyMissingObjects(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	writeFile(t, repoDir, "app.txt", "before\n")
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	}); err != nil {
		t.Fatalf("capture start: %v", err)
	}
	writeFile(t, repoDir, "app.txt", "after\n")
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd,
	}); err != nil {
		t.Fatalf("capture end: %v", err)
	}
	pair := acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, "")
	first, err := operator.ArchiveTurnCheckpoint(context.Background(), archiveRequestForPair(pair))
	if err != nil {
		t.Fatalf("first archive: %v", err)
	}
	known := archiveRequestForPair(pair)
	known.KnownObjects = first.Objects
	second, err := operator.ArchiveTurnCheckpoint(context.Background(), known)
	if err != nil {
		t.Fatalf("deduplicated archive: %v", err)
	}
	if len(second.Pack) != 0 {
		t.Fatalf("deduplicated archive transferred %d bytes, want none", len(second.Pack))
	}
	if len(second.Objects) != len(first.Objects) {
		t.Fatalf("deduplicated archive object inventory = %d, want %d", len(second.Objects), len(first.Objects))
	}
}

func archiveRequestForPair(pair turnchanges.CompareRequest) turnchanges.ArchiveRequest {
	return turnchanges.ArchiveRequest{
		ChangeSetID: pair.ChangeSetID, CheckoutID: pair.CheckoutID, Repo: pair.Repo,
		HashAlgorithm: pair.HashAlgorithm, StartCommitOID: pair.StartCommitOID, StartTreeOID: pair.StartTreeOID,
		EndCommitOID: pair.EndCommitOID, EndTreeOID: pair.EndTreeOID,
	}
}
