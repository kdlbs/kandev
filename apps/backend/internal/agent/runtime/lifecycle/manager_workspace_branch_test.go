package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type branchSnapshotWriter struct {
	executionID string
	branch      string
}

func (w *branchSnapshotWriter) UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error {
	return nil
}

func (w *branchSnapshotWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	return nil
}

func (w *branchSnapshotWriter) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

func (w *branchSnapshotWriter) UpdateExecutorRunningWorktreeBranch(_ context.Context, _, executionID, branch string) error {
	w.executionID = executionID
	w.branch = branch
	return nil
}

func TestRenameBranchForSessionUsesRepositoryScopedAgentctlOperation(t *testing.T) {
	var got struct {
		NewName string `json:"new_name"`
		Repo    string `json:"repo"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/git/rename-branch" {
			t.Fatalf("path = %q, want rename endpoint", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"operation": "rename_branch",
		})
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
		ID:        "execution-1",
		SessionID: "session-1",
		Metadata:  map[string]interface{}{MetadataKeyWorktreeBranch: "feature/provisional-abc"},
		agentctl:  agentctlclient.NewClient(parsed.Hostname(), port, newTestLogger()),
	}
	store := NewExecutionStore()
	if err := store.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	mgr := &Manager{executionStore: store}

	result, err := mgr.RenameBranchForSession(t.Context(), "session-1", "feature/final-title-abc", "backend")
	if err != nil {
		t.Fatalf("RenameBranchForSession returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("result = %#v, want successful git operation", result)
	}
	if got.NewName != "feature/final-title-abc" || got.Repo != "backend" {
		t.Fatalf("request = %#v, want final branch and backend repo", got)
	}
	if gotBranch, _ := execution.Metadata[MetadataKeyWorktreeBranch].(string); gotBranch != "feature/provisional-abc" {
		t.Fatalf("non-primary metadata branch = %q, want unchanged", gotBranch)
	}
}

func TestRenameBranchForSessionUpdatesPrimaryExecutionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "operation": "rename_branch"})
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
		ID:        "execution-1",
		SessionID: "session-1",
		Metadata:  map[string]interface{}{MetadataKeyWorktreeBranch: "feature/provisional-abc"},
		agentctl:  agentctlclient.NewClient(parsed.Hostname(), port, newTestLogger()),
	}
	store := NewExecutionStore()
	if err := store.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	mgr := &Manager{executionStore: store}
	writer := &branchSnapshotWriter{}
	mgr.SetExecutorRunningWriter(writer)

	_, err = mgr.RenameBranchForSessionWithPrimary(t.Context(), "session-1", "feature/final-title-abc", "backend", true)
	if err != nil {
		t.Fatalf("RenameBranchForSession returned error: %v", err)
	}
	if got, _ := execution.Metadata[MetadataKeyWorktreeBranch].(string); got != "feature/final-title-abc" {
		t.Fatalf("primary metadata branch = %q, want final branch", got)
	}
	if writer.branch != "feature/final-title-abc" {
		t.Fatalf("running snapshot branch = %q, want final branch", writer.branch)
	}
	if writer.executionID != "execution-1" {
		t.Fatalf("running snapshot execution = %q, want execution-1", writer.executionID)
	}
}
