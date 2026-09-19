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

func TestApplyTaskEnvironmentToWorkspaceInfoProjectsEnvironmentAttachmentHandles(t *testing.T) {
	tests := []struct {
		name string
		env  *models.TaskEnvironment
		key  string
		want string
	}{
		{
			name: "Docker bootstrap nonce",
			env: &models.TaskEnvironment{
				ContainerID:                     "container-1",
				ContainerBootstrapNonceSecretID: "bootstrap-secret",
			},
			key:  lifecycle.MetadataKeyBootstrapNonceSecret,
			want: "bootstrap-secret",
		},
		{
			name: "SSH canonical task directory",
			env: &models.TaskEnvironment{
				ExecutorType:  string(models.ExecutorTypeSSH),
				WorkspacePath: "/remote/tasks/task-1",
			},
			key:  lifecycle.MetadataKeySSHRemoteTaskDir,
			want: "/remote/tasks/task-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &lifecycle.WorkspaceInfo{}
			applyTaskEnvironmentToWorkspaceInfo(info, tt.env)

			if got := info.Metadata[tt.key]; got != tt.want {
				t.Fatalf("metadata[%q] = %q, want environment attachment handle", tt.key, got)
			}
		})
	}
}
