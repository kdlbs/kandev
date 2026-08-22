package service

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

func TestApplyTaskEnvironmentToWorkspaceInfoProjectsDockerControlHandle(t *testing.T) {
	info := &lifecycle.WorkspaceInfo{}
	applyTaskEnvironmentToWorkspaceInfo(info, &models.TaskEnvironment{
		ID:                                "environment-1",
		ContainerID:                       "container-1",
		ContainerControlAuthTokenSecretID: "container-control-secret",
	})

	if got := info.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret]; got != "container-control-secret" {
		t.Fatalf("container control handle = %q, want environment control handle", got)
	}
}
