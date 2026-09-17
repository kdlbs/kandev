package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestNewResumeLaunchRequestCarriesExactProfileSnapshot(t *testing.T) {
	req, _ := newResumeLaunchRequest(&v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", AgentProfileID: "profile-1",
	}, true, ResumeOptions{
		ExactProfile:         true,
		ExactProfileModel:    "gpt-5.6-codex",
		ExactProfileRevision: 1726500000000000000,
	})

	if !req.ExactProfile || req.ExactProfileModel != "gpt-5.6-codex" || req.ExactProfileRevision != 1726500000000000000 {
		t.Fatalf("exact profile snapshot = %+v, want exact model and revision", req)
	}
}
