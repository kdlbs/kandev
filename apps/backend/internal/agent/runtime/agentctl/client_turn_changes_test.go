package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointClientOperations(t *testing.T) {
	response := `{"change_set_id":"8d244e9c-0686-4264-b455-270724658999","checkout_id":"36ce0fcc-90fe-45da-a5a4-57640ea28523","boundary":"start","commit_oid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tree_oid":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","hash_algorithm":"sha1","reachability_ref":"refs/kandev/turn-changes/8d244e9c-0686-4264-b455-270724658999/36ce0fcc-90fe-45da-a5a4-57640ea28523/start"}`
	server, captured := captureServer(t, jsonResponder(http.StatusOK, response))
	client := newHTTPOnlyClient(server.URL)
	result, err := client.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: "8d244e9c-0686-4264-b455-270724658999",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
		Boundary:    turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("CaptureTurnCheckpoint: %v", err)
	}
	if captured.Method != http.MethodPost || captured.Path != "/api/v1/git/turn-changes/checkpoint" {
		t.Fatalf("request = %s %s, want checkpoint POST", captured.Method, captured.Path)
	}
	if result.TreeOID != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || result.Boundary != turnchanges.CheckpointStart {
		t.Fatalf("result = %+v, want decoded endpoint", result)
	}

	server, captured = captureServer(t, jsonResponder(http.StatusOK, `{"complete":true,"file_count":0,"files":[]}`))
	client = newHTTPOnlyClient(server.URL)
	comparison, err := client.CompareTurnCheckpoints(context.Background(), turnchanges.CompareRequest{
		ChangeSetID: "8d244e9c-0686-4264-b455-270724658999",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
	})
	if err != nil {
		t.Fatalf("CompareTurnCheckpoints: %v", err)
	}
	if captured.Path != "/api/v1/git/turn-changes/compare" || !comparison.Complete {
		t.Fatalf("request/result = %s, %+v", captured.Path, comparison)
	}

	response = `{"change_set_id":"8d244e9c-0686-4264-b455-270724658999","checkout_id":"36ce0fcc-90fe-45da-a5a4-57640ea28523","files":[{"file":{"path_bytes":"Zm9v","kind":"modified"},"canonical_patch":"ZGlmZg==","content_availability":"ready"}],"complete":true,"export_bytes":4}`
	server, captured = captureServer(t, jsonResponder(http.StatusOK, response))
	client = newHTTPOnlyClient(server.URL)
	export, err := client.ExportTurnCheckpoint(context.Background(), turnchanges.ExportRequest{
		ChangeSetID: "8d244e9c-0686-4264-b455-270724658999",
		CheckoutID:  "36ce0fcc-90fe-45da-a5a4-57640ea28523",
	})
	if err != nil {
		t.Fatalf("ExportTurnCheckpoint: %v", err)
	}
	if captured.Path != "/api/v1/git/turn-changes/export" || !export.Complete || string(export.Files[0].File.PathBytes) != "foo" || string(export.Files[0].CanonicalPatch) != "diff" {
		t.Fatalf("export request/result = %s, %+v", captured.Path, export)
	}
	encoded, err := json.Marshal(export)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("marshal export response: bytes=%d err=%v", len(encoded), err)
	}
}

func TestTurnCheckpointClientPreservesTypedError(t *testing.T) {
	server, _ := captureServer(t, jsonResponder(http.StatusNotImplemented, `{"error":"unsupported Git format","reason":"unsupported_executor"}`))
	client := newHTTPOnlyClient(server.URL)
	_, err := client.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{})
	var checkpointErr *TurnCheckpointClientError
	if !errors.As(err, &checkpointErr) {
		t.Fatalf("error = %v, want TurnCheckpointClientError", err)
	}
	if checkpointErr.Status != http.StatusNotImplemented || checkpointErr.Reason != turnchanges.ReasonUnsupportedExecutor {
		t.Fatalf("typed error = %+v", checkpointErr)
	}
}

func TestTurnCheckpointClientReadsRegisteredScopes(t *testing.T) {
	server, captured := captureServer(t, jsonResponder(http.StatusOK, `{"scopes":["","apps/web"]}`))
	client := newHTTPOnlyClient(server.URL)
	scopes, err := client.TurnCheckpointRepositoryScopes(context.Background())
	if err != nil {
		t.Fatalf("TurnCheckpointRepositoryScopes: %v", err)
	}
	if captured.Method != http.MethodGet || captured.Path != "/api/v1/git/turn-changes/scopes" {
		t.Fatalf("request = %s %s, want registered-scope GET", captured.Method, captured.Path)
	}
	if len(scopes) != 2 || scopes[0] != "" || scopes[1] != "apps/web" {
		t.Fatalf("scopes = %#v, want root and apps/web", scopes)
	}
}
