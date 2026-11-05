package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func marshalSessionDTO(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal session DTO: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal session DTO: %v", err)
	}
	return decoded
}

func TestTaskSessionAgentWorkspaceProjection(t *testing.T) {
	known := &models.TaskSession{
		ID: "session-known", TaskID: "task-1",
		WorkspacePath:      "/task-root",
		AgentWorkspacePath: "/task-root/master",
	}
	full := FromTaskSession(known)
	summary := FromTaskSessionSummary(known)
	if full.AgentWorkspacePath == nil || *full.AgentWorkspacePath != "/task-root/master" {
		t.Fatalf("full AgentWorkspacePath = %v, want /task-root/master", full.AgentWorkspacePath)
	}
	if summary.AgentWorkspacePath == nil || *summary.AgentWorkspacePath != "/task-root/master" {
		t.Fatalf("summary AgentWorkspacePath = %v, want /task-root/master", summary.AgentWorkspacePath)
	}

	fullJSON := marshalSessionDTO(t, full)
	if fullJSON["agent_workspace_path"] != "/task-root/master" {
		t.Fatalf("full JSON agent_workspace_path = %v, want /task-root/master", fullJSON["agent_workspace_path"])
	}
	summaryJSON := marshalSessionDTO(t, summary)
	if summaryJSON["agent_workspace_path"] != "/task-root/master" {
		t.Fatalf("summary JSON agent_workspace_path = %v, want /task-root/master", summaryJSON["agent_workspace_path"])
	}

	unknown := &models.TaskSession{ID: "session-unknown", TaskID: "task-1", WorkspacePath: "/task-root"}
	if got := FromTaskSession(unknown).AgentWorkspacePath; got != nil {
		t.Fatalf("full AgentWorkspacePath = %v, want nil", got)
	}
	if got := FromTaskSessionSummary(unknown).AgentWorkspacePath; got != nil {
		t.Fatalf("summary AgentWorkspacePath = %v, want nil", got)
	}
	fullUnknownJSON := marshalSessionDTO(t, FromTaskSession(unknown))
	if got, exists := fullUnknownJSON["agent_workspace_path"]; !exists || got != nil {
		t.Fatalf("full JSON agent_workspace_path = %v (exists=%v), want explicit null", got, exists)
	}
	summaryUnknownJSON := marshalSessionDTO(t, FromTaskSessionSummary(unknown))
	if got, exists := summaryUnknownJSON["agent_workspace_path"]; !exists || got != nil {
		t.Fatalf("summary JSON agent_workspace_path = %v (exists=%v), want explicit null", got, exists)
	}
}

func TestTaskSessionModelToAPIIncludesKnownAgentWorkspace(t *testing.T) {
	session := &models.TaskSession{ID: "session-known", TaskID: "task-1", AgentWorkspacePath: "/task-root/master"}
	if got := session.ToAPI()["agent_workspace_path"]; got != "/task-root/master" {
		t.Fatalf("ToAPI agent_workspace_path = %v, want /task-root/master", got)
	}
	unknown := &models.TaskSession{ID: "session-unknown", TaskID: "task-1"}
	if _, exists := unknown.ToAPI()["agent_workspace_path"]; exists {
		t.Fatal("ToAPI should omit an unknown agent workspace")
	}
}
