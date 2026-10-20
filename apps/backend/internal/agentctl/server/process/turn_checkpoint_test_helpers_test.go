package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

func acceptedTurnCheckpointPair(t testing.TB, operator *GitOperator, changeSetID, checkoutID, repo string) turnchanges.CompareRequest {
	t.Helper()
	_, hashAlgorithm, err := operator.turnCheckpointRepository(context.Background())
	if err != nil {
		t.Fatalf("resolve checkpoint repository: %v", err)
	}
	start, found, err := operator.existingTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	}, turnCheckpointRef(changeSetID, checkoutID, turnchanges.CheckpointStart), hashAlgorithm)
	if err != nil || !found {
		t.Fatalf("load accepted start checkpoint: found=%t err=%v", found, err)
	}
	end, found, err := operator.existingTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd,
	}, turnCheckpointRef(changeSetID, checkoutID, turnchanges.CheckpointEnd), hashAlgorithm)
	if err != nil || !found {
		t.Fatalf("load accepted end checkpoint: found=%t err=%v", found, err)
	}
	return turnchanges.CompareRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Repo: repo, HashAlgorithm: hashAlgorithm,
		StartCommitOID: start.CommitOID, StartTreeOID: start.TreeOID,
		EndCommitOID: end.CommitOID, EndTreeOID: end.TreeOID,
	}
}

func exportRequestForPair(request turnchanges.CompareRequest) turnchanges.ExportRequest {
	return turnchanges.ExportRequest(request)
}
