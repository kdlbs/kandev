package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestFromRepositoryIncludesRemoteURL(t *testing.T) {
	payload, err := json.Marshal(FromRepository(&models.Repository{
		ID:        "repo-1",
		RemoteURL: "https://github.com/owner/repo.git",
	}))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded["remote_url"] != "https://github.com/owner/repo.git" {
		t.Errorf("remote_url = %v, want %q; payload = %s", decoded["remote_url"], "https://github.com/owner/repo.git", payload)
	}
}
