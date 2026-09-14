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
