package dto

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestFromTaskWithSessionInfoIncludesPrimaryAgentProfileID(t *testing.T) {
	profileID := "profile-1"
	got := FromTaskWithSessionInfo(
		&models.Task{ID: "task-1"},
		nil,
		nil,
		models.ReviewStatusNone,
		nil,
		nil,
		nil,
		nil,
		nil,
		&profileID,
		nil,
		nil,
		nil,
	)
	if got.PrimaryAgentProfileID == nil || *got.PrimaryAgentProfileID != profileID {
		t.Fatalf("primary agent profile id = %v, want %q", got.PrimaryAgentProfileID, profileID)
	}
}
