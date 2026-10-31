package dto

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

func TestFromTaskProjectsQuickChatCreationIdentityOnLifecycleDTO(t *testing.T) {
	creationID := "a7c0fd51-6526-4732-95bc-c6e25f745e7f"
	externalID, err := service.NewQuickChatCreationExternalID("user-a", creationID)
	if err != nil {
		t.Fatalf("NewQuickChatCreationExternalID: %v", err)
	}
	dto := FromTask(&models.Task{ID: "task-1", ExternalID: externalID, IsEphemeral: true})
	if dto.QuickChatCreationID != creationID {
		t.Fatalf("QuickChatCreationID = %q, want %q", dto.QuickChatCreationID, creationID)
	}
	ordinary := FromTask(&models.Task{ID: "task-2", ExternalID: "user-requested-external-id"})
	if ordinary.QuickChatCreationID != "" {
		t.Fatalf("ordinary task creation identity = %q, want omitted", ordinary.QuickChatCreationID)
	}
}
