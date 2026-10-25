package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestFromRepositorySetCarriesOrderedMembership(t *testing.T) {
	created := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	set := &models.RepositorySet{
		ID:          "set-1",
		WorkspaceID: "ws-1",
		Name:        "Full-stack",
		Description: "web + gateway",
		Items: []models.RepositorySetItem{
			{ID: "item-1", RepositorySetID: "set-1", RepositoryID: "repo-web", Position: 0, BaseBranch: "develop"},
			{ID: "item-2", RepositorySetID: "set-1", RepositoryID: "repo-gateway", Position: 1},
		},
		CreatedAt: created,
		UpdatedAt: created,
	}

	got := FromRepositorySet(set)

	if got.ID != "set-1" || got.WorkspaceID != "ws-1" || got.Name != "Full-stack" {
		t.Fatalf("dto = %+v", got)
	}
	if got.Description != "web + gateway" {
		t.Fatalf("description = %q", got.Description)
	}
	if len(got.Repositories) != 2 {
		t.Fatalf("repositories = %+v", got.Repositories)
	}
	if got.Repositories[0].RepositoryID != "repo-web" || got.Repositories[0].Position != 0 {
		t.Fatalf("first member = %+v", got.Repositories[0])
	}
	if got.Repositories[0].BaseBranch != "develop" {
		t.Fatalf("first member base branch = %q, want %q", got.Repositories[0].BaseBranch, "develop")
	}
	if got.Repositories[1].RepositoryID != "repo-gateway" || got.Repositories[1].Position != 1 {
		t.Fatalf("second member = %+v", got.Repositories[1])
	}
	if !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(created) {
		t.Fatalf("timestamps = %v / %v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestFromRepositorySetEmptyMembershipSerializesAsArray(t *testing.T) {
	// A set whose repositories were all deleted must serialize as [] rather than
	// null: the web store indexes the list without a nil check.
	got := FromRepositorySet(&models.RepositorySet{ID: "set-1", WorkspaceID: "ws-1", Name: "Empty"})

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Repositories []RepositorySetItemDTO `json:"repositories"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Repositories == nil {
		t.Fatalf("repositories serialized as null: %s", encoded)
	}
	if len(decoded.Repositories) != 0 {
		t.Fatalf("repositories = %+v", decoded.Repositories)
	}
}
