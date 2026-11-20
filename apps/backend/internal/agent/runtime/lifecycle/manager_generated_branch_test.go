package lifecycle

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func generatedBranchTestManager(t *testing.T, response map[string]interface{}, got *map[string]any, requestCount *int) (*Manager, *AgentExecution) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount != nil {
			*requestCount++
		}
		if r.URL.Path != "/api/v1/git/rename-branch" {
			t.Errorf("path = %q, want rename endpoint", r.URL.Path)
		}
		if got != nil {
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	execution := &AgentExecution{
		ID:        "execution-generated",
		SessionID: "session-generated",
		metadata:  map[string]interface{}{MetadataKeyWorktreeBranch: "feature/provisional"},
		agentctl:  agentctlclient.NewClient(parsed.Hostname(), port, newTestLogger()),
	}
	store := NewExecutionStore()
	if err := store.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	return &Manager{executionStore: store}, execution
}

func TestRenameGeneratedBranchForSessionUsesExpectedBranch(t *testing.T) {
	var got map[string]any
	requestCount := 0
	mgr, execution := generatedBranchTestManager(t, map[string]interface{}{
		"success": true, "operation": "rename_branch",
	}, &got, &requestCount)
	writer := &branchSnapshotWriter{}
	mgr.SetExecutorRunningWriter(writer)

	result, err := mgr.RenameGeneratedBranchForSession(
		t.Context(), "session-generated", "feature/provisional", "feature/final", "backend", true)
	if err != nil {
		t.Fatalf("RenameGeneratedBranchForSession returned error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("rename requests = %d, want exactly one guarded request", requestCount)
	}
	if result == nil || !result.Success {
		t.Fatalf("result = %#v, want successful git operation", result)
	}
	if got["new_name"] != "feature/final" || got["repo"] != "backend" ||
		got["expected_generated_branch"] != "feature/provisional" {
		t.Fatalf("request = %#v, want guarded rename payload", got)
	}
	if branch := execution.metadataString(MetadataKeyWorktreeBranch); branch != "feature/final" {
		t.Fatalf("metadata branch = %q, want final branch", branch)
	}
	if writer.branch != "feature/final" || writer.executionID != "execution-generated" {
		t.Fatalf("snapshot = %q/%q, want final branch for execution-generated", writer.branch, writer.executionID)
	}
}

func TestRenameGeneratedBranchForSessionRequiresExpectedBranch(t *testing.T) {
	mgr, _ := generatedBranchTestManager(t, map[string]interface{}{"success": true}, nil, nil)
	if _, err := mgr.RenameGeneratedBranchForSession(
		t.Context(), "session-generated", "", "feature/final", "", true); err == nil {
		t.Fatal("RenameGeneratedBranchForSession accepted an empty expected branch")
	}
}

func TestRenameGeneratedBranchForSessionKeepsMetadataOnRefusal(t *testing.T) {
	mgr, execution := generatedBranchTestManager(t, map[string]interface{}{
		"success": false, "operation": "rename_branch", "error": "expected generated branch mismatch",
	}, nil, nil)
	writer := &branchSnapshotWriter{}
	mgr.SetExecutorRunningWriter(writer)

	result, err := mgr.RenameGeneratedBranchForSession(
		t.Context(), "session-generated", "feature/other", "feature/final", "", true)
	if err != nil {
		t.Fatalf("RenameGeneratedBranchForSession returned error: %v", err)
	}
	if result == nil || result.Success || result.Error == "" {
		t.Fatalf("result = %#v, want refused operation", result)
	}
	if branch := execution.metadataString(MetadataKeyWorktreeBranch); branch != "feature/provisional" {
		t.Fatalf("metadata branch = %q, want unchanged provisional branch", branch)
	}
	if writer.branch != "" {
		t.Fatalf("snapshot branch = %q, want no write", writer.branch)
	}
}

func TestRenameGeneratedBranchForSessionReportsPrimarySnapshotFailure(t *testing.T) {
	mgr, _ := generatedBranchTestManager(t, map[string]interface{}{
		"success": true, "operation": "rename_branch",
	}, nil, nil)
	mgr.SetExecutorRunningWriter(&branchSnapshotWriter{err: models.ErrExecutionRotated})

	result, err := mgr.RenameGeneratedBranchForSession(
		t.Context(), "session-generated", "feature/provisional", "feature/final", "", true)
	if err == nil || result == nil || !result.Success {
		t.Fatalf("result = %#v, err = %v; want snapshot error with successful Git result", result, err)
	}
	var snapshotErr *BranchSnapshotError
	if !errors.As(err, &snapshotErr) {
		t.Fatalf("error = %v, want BranchSnapshotError", err)
	}
}
