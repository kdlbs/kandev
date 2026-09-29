package service

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestApplyRepositoryUpdates_AppliesRemoteURLFromJSON(t *testing.T) {
	repo := &models.Repository{RemoteURL: "https://github.com/owner/old.git"}
	var updates UpdateRepositoryRequest
	if err := json.Unmarshal([]byte(`{"remote_url":"https://github.com/owner/repo.git"}`), &updates); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if err := applyRepositoryUpdates(repo, &updates); err != nil {
		t.Fatalf("applyRepositoryUpdates: %v", err)
	}
	if repo.RemoteURL != "https://github.com/owner/repo.git" {
		t.Errorf("RemoteURL = %q, want updated value", repo.RemoteURL)
	}
}

func TestExactResourceIDsAreNotAcceptedFromJSON(t *testing.T) {
	workflow, err := json.Marshal(CreateWorkflowRequest{ID: "host-assigned-workflow"})
	if err != nil {
		t.Fatalf("marshal workflow request: %v", err)
	}
	repository, err := json.Marshal(CreateRepositoryRequest{ID: "host-assigned-repository"})
	if err != nil {
		t.Fatalf("marshal repository request: %v", err)
	}
	for name, payload := range map[string]string{"workflow": string(workflow), "repository": string(repository)} {
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			t.Fatalf("unmarshal %s request: %v", name, err)
		}
		if _, exists := decoded["id"]; exists {
			t.Errorf("%s request exposes host-assigned ID: %s", name, payload)
		}
	}
}
