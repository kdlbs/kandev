package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestHandleGitRenameGeneratedBranchRefusesUnexpectedCheckout(t *testing.T) {
	fixture := newGitAPIFixture(t)
	head := fixture.head(t)
	rec := postGitAPI(t, fixture.server, "/api/v1/git/rename-branch", map[string]interface{}{
		"new_name":                  "feature/renamed",
		"expected_generated_branch": "feature/other",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("rename status = %d, body=%s", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if result.Success || !strings.Contains(result.Error, "feature/other") {
		t.Fatalf("rename result = %+v, want refusal naming the expected branch", result)
	}
	if branch := fixture.branch(t); branch != "feature/work" {
		t.Fatalf("branch = %q, want unchanged feature/work", branch)
	}
	if got := fixture.head(t); got != head {
		t.Fatalf("HEAD = %s, want unchanged %s", got, head)
	}
}

// TestHandleGitRenameGeneratedBranchPreservesTurnComparison exercises the real
// rename route between two captured endpoints. The title handoff carries the
// verified old generated branch; a proven rename must keep the interval
// comparison direct even though the symbolic ref name changed.
func TestHandleGitRenameGeneratedBranchPreservesTurnComparison(t *testing.T) {
	fixture := newGitAPIFixture(t)
	request := turnchanges.CheckpointRequest{
		ChangeSetID: "8d244e9c-0686-4264-b455-270724658999",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
		Boundary:    turnchanges.CheckpointStart,
	}
	startRec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", request)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start checkpoint status = %d, body=%s", startRec.Code, startRec.Body.String())
	}
	var startResult turnchanges.CheckpointResult
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResult); err != nil {
		t.Fatalf("decode start checkpoint: %v", err)
	}

	renameRec := postGitAPI(t, fixture.server, "/api/v1/git/rename-branch", map[string]interface{}{
		"new_name":                  "feature/renamed",
		"expected_generated_branch": "feature/work",
	})
	if renameRec.Code != http.StatusOK {
		t.Fatalf("rename status = %d, body=%s", renameRec.Code, renameRec.Body.String())
	}
	renameResult := decodeGitOperationResult(t, renameRec)
	if !renameResult.Success {
		t.Fatalf("rename result = %+v, want success", renameResult)
	}
	if branch := fixture.branch(t); branch != "feature/renamed" {
		t.Fatalf("branch = %q, want feature/renamed", branch)
	}

	request.Boundary = turnchanges.CheckpointEnd
	endRec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", request)
	if endRec.Code != http.StatusOK {
		t.Fatalf("end checkpoint status = %d, body=%s", endRec.Code, endRec.Body.String())
	}
	var endResult turnchanges.CheckpointResult
	if err := json.Unmarshal(endRec.Body.Bytes(), &endResult); err != nil {
		t.Fatalf("decode end checkpoint: %v", err)
	}

	comparisonRec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/compare", turnchanges.CompareRequest{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: startResult.HashAlgorithm, StartCommitOID: startResult.CommitOID, StartTreeOID: startResult.TreeOID,
		EndCommitOID: endResult.CommitOID, EndTreeOID: endResult.TreeOID,
		ComparisonVersion: turnchanges.ComparisonVersion2,
	})
	if comparisonRec.Code != http.StatusOK {
		t.Fatalf("compare status = %d, body=%s", comparisonRec.Code, comparisonRec.Body.String())
	}
	var comparison turnchanges.CheckpointComparison
	if err := json.Unmarshal(comparisonRec.Body.Bytes(), &comparison); err != nil {
		t.Fatalf("decode comparison: %v", err)
	}
	if comparison.ComparisonBasis != turnchanges.ComparisonDirect || !comparison.Complete || comparison.FileCount != 0 {
		t.Fatalf("comparison = %+v, want complete direct zero-file basis", comparison)
	}
	if startResult.Provenance == nil || endResult.Provenance == nil ||
		startResult.Provenance.HeadRef != "refs/heads/feature/work" ||
		endResult.Provenance.HeadRef != "refs/heads/feature/renamed" {
		t.Fatalf("endpoint refs = %+v -> %+v, want actual symbolic refs", startResult.Provenance, endResult.Provenance)
	}
	if !strings.EqualFold(startResult.Provenance.HashAlgorithm, "sha1") &&
		!strings.EqualFold(startResult.Provenance.HashAlgorithm, "sha256") {
		t.Fatalf("hash algorithm = %q", startResult.Provenance.HashAlgorithm)
	}
}
