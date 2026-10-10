package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointRoutesUseRegisteredRepositoryAndExactInterval(t *testing.T) {
	fixture := newGitAPIFixture(t)
	request := turnchanges.CheckpointRequest{
		ChangeSetID: "8d244e9c-0686-4264-b455-270724658999",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
		Boundary:    turnchanges.CheckpointStart,
	}
	start := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", request)
	if start.Code != http.StatusOK {
		t.Fatalf("start checkpoint status = %d, body=%s", start.Code, start.Body.String())
	}
	var startResult turnchanges.CheckpointResult
	if err := json.Unmarshal(start.Body.Bytes(), &startResult); err != nil {
		t.Fatalf("decode start checkpoint: %v", err)
	}
	if startResult.Boundary != turnchanges.CheckpointStart || startResult.TreeOID == "" || startResult.ReachabilityRef == "" {
		t.Fatalf("start checkpoint = %+v, want immutable start endpoint", startResult)
	}

	writeFileAPI(t, fixture.repo, "between-turns.txt", "captured at end\n")
	request.Boundary = turnchanges.CheckpointEnd
	end := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", request)
	if end.Code != http.StatusOK {
		t.Fatalf("end checkpoint status = %d, body=%s", end.Code, end.Body.String())
	}
	var endResult turnchanges.CheckpointResult
	if err := json.Unmarshal(end.Body.Bytes(), &endResult); err != nil {
		t.Fatalf("decode end checkpoint: %v", err)
	}
	comparison := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/compare", turnchanges.CompareRequest{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: startResult.HashAlgorithm, StartCommitOID: startResult.CommitOID, StartTreeOID: startResult.TreeOID,
		EndCommitOID: endResult.CommitOID, EndTreeOID: endResult.TreeOID,
	})
	if comparison.Code != http.StatusOK {
		t.Fatalf("compare status = %d, body=%s", comparison.Code, comparison.Body.String())
	}
	var result turnchanges.CheckpointComparison
	if err := json.Unmarshal(comparison.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode comparison: %v", err)
	}
	if !result.Complete || result.FileCount != 1 || string(result.Files[0].PathBytes) != "between-turns.txt" {
		t.Fatalf("comparison = %+v, want only the file changed during the interval", result)
	}
}

func TestTurnCheckpointExportRouteReturnsDurableVariants(t *testing.T) {
	fixture := newGitAPIFixture(t)
	request := turnchanges.CheckpointRequest{
		ChangeSetID: "8d244e9c-0686-4264-b455-270724658999",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
		Boundary:    turnchanges.CheckpointStart,
	}
	startRec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", request)
	if startRec.Code != http.StatusOK {
		t.Fatalf("capture start status = %d, body=%s", startRec.Code, startRec.Body.String())
	}
	var startResult turnchanges.CheckpointResult
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResult); err != nil {
		t.Fatalf("decode start checkpoint: %v", err)
	}
	writeFileAPI(t, fixture.repo, "export.txt", "retained after cleanup\n")
	request.Boundary = turnchanges.CheckpointEnd
	endRec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", request)
	if endRec.Code != http.StatusOK {
		t.Fatalf("capture end status = %d, body=%s", endRec.Code, endRec.Body.String())
	}
	var endResult turnchanges.CheckpointResult
	if err := json.Unmarshal(endRec.Body.Bytes(), &endResult); err != nil {
		t.Fatalf("decode end checkpoint: %v", err)
	}
	rec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/export", turnchanges.ExportRequest{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: startResult.HashAlgorithm, StartCommitOID: startResult.CommitOID, StartTreeOID: startResult.TreeOID,
		EndCommitOID: endResult.CommitOID, EndTreeOID: endResult.TreeOID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var result turnchanges.CheckpointExport
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if !result.Complete || len(result.Files) != 1 || string(result.Files[0].File.PathBytes) != "export.txt" ||
		!json.Valid(rec.Body.Bytes()) || len(result.Files[0].NewRendering) == 0 {
		t.Fatalf("export = %+v, want exact file with new rendering bytes", result)
	}
}

func TestTurnCheckpointRouteRejectsUnregisteredRepository(t *testing.T) {
	fixture := newGitAPIFixture(t)
	rec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", turnchanges.CheckpointRequest{
		ChangeSetID: "8d244e9c-0686-4264-b455-270724658999",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
		Repo:        "../outside",
		Boundary:    turnchanges.CheckpointStart,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unregistered repository status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response turnCheckpointErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if response.Error == "" {
		t.Fatal("expected registered-scope error")
	}
}

func TestTurnCheckpointRouteReturnsTypedCaptureReason(t *testing.T) {
	fixture := newGitAPIFixture(t)
	rec := postGitAPI(t, fixture.server, "/api/v1/git/turn-changes/checkpoint", turnchanges.CheckpointRequest{
		ChangeSetID: "invalid",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
		Boundary:    turnchanges.CheckpointStart,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid identity status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response turnCheckpointErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode typed error: %v", err)
	}
	if response.Reason != turnchanges.ReasonUnsafeGitState {
		t.Fatalf("reason = %q, want %q", response.Reason, turnchanges.ReasonUnsafeGitState)
	}
}

func TestTurnCheckpointScopesExposeRegisteredRepositoryManifest(t *testing.T) {
	fixture := newGitAPIFixture(t)
	rec := getGitAPI(t, fixture.server, "/api/v1/git/turn-changes/scopes")
	if rec.Code != http.StatusOK {
		t.Fatalf("scopes status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode scopes: %v", err)
	}
	if len(response.Scopes) != 1 || response.Scopes[0] != "" {
		t.Fatalf("registered scopes = %#v, want the root checkout", response.Scopes)
	}
}
